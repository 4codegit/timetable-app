package solver

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"timetable/internal/domain"
)

// Occurrence is one required placement of a lesson.
type Occurrence struct {
	Lesson      domain.Lesson
	Index       int // which occurrence (0..hours-1)
	RoomChoices []int
}

// SolveInput bundles everything the solver needs.
type SolveInput struct {
	SchoolID    int
	Lessons     []domain.Lesson
	Teachers    map[int]domain.Teacher
	Classes     map[int]domain.SchoolClass
	Rooms       []domain.Room
	Subjects    map[int]domain.Subject
	Constraints []domain.Constraint
	Config      domain.SchedulingConfig
}

// Result holds the best schedule found and its metric.
type Result struct {
	Entries    []domain.ScheduleEntry `json:"entries"`
	Placed     int                    `json:"placed"`
	Total      int                    `json:"total"`
	Violations int                    `json:"violations"` // soft constraint violations
	ElapsedMs  int64                  `json:"elapsed_ms"` // wall-clock time spent solving
}

// prepareDaysMask applies Config.DaysMask (school-day checkboxes): the
// grid shrinks/grows to cover the highest active weekday and every
// inactive day becomes a hard class_unavailable constraint, which all
// three engines (heuristic backtracking, OR-Tools CP-SAT, its Windows
// twin) already understand. Returns the adjusted input and the effective
// day count.
func prepareDaysMask(in SolveInput) (SolveInput, int) {
	mask := in.Config.DaysMask
	if mask == 0 {
		return in, in.Config.DaysPerWeek
	}
	days := 0
	for d := 0; d < 7; d++ {
		if mask&(1<<d) != 0 {
			days = d + 1
		}
	}
	if in.Config.DaysPerWeek > days {
		days = in.Config.DaysPerWeek
	}
	out := in
	out.Constraints = append([]domain.Constraint(nil), in.Constraints...)
	classIDs := map[int]bool{}
	for _, l := range in.Lessons {
		classIDs[l.ClassID] = true
	}
	for d := 0; d < days; d++ {
		if mask&(1<<d) != 0 {
			continue
		}
		dow := d
		for cid := range classIDs {
			out.Constraints = append(out.Constraints, domain.Constraint{
				Type:       "class_unavailable",
				EntityType: "class",
				EntityID:   cid,
				DayOfWeek:  &dow,
				IsHard:     true,
			})
		}
	}
	return out, days
}

// Solve runs parallel randomised-restart CSP search.
func Solve(ctx context.Context, in SolveInput, parallelism int, timeout time.Duration) Result {
	if parallelism < 1 {
		parallelism = 1
	}
	in, cfgDays := prepareDaysMask(in)
	days := cfgDays
	if days <= 0 {
		days = 6
	}
	slots := in.Config.SlotsPerDay
	if slots <= 0 {
		slots = 8
	}

	// Всё, что не зависит от состояния поиска (кандидаты по HardSet,
	// порядок обхода, правила), считаем ОДИН раз на вызов Solve —
	// раньше это пересчитывалось в каждом узле перебора и в каждой
	// попытке рестарта.
	env := buildSearchEnv(in, days, slots)

	var best Result
	best.Total = len(env.occ)
	var mu sync.Mutex
	update := func(r Result) {
		mu.Lock()
		defer mu.Unlock()
		if r.Placed > best.Placed || (r.Placed == best.Placed && r.Violations < best.Violations) {
			best = r
		}
	}

	start := time.Now()

	workerCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var wg sync.WaitGroup
	for w := 0; w < parallelism; w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			// Статика env общая (read-only), scratch-буферы — свои
			// на воркера: они мутируются в глубине перебора.
			local := env.forWorker()
			r := runRestarts(workerCtx, local, rng)
			if r.Placed > 0 {
				update(r)
			}
		}(time.Now().UnixNano() + int64(w*7919))
	}
	wg.Wait()
	best.ElapsedMs = time.Since(start).Milliseconds()
	return best
}

// runRestarts performs multiple randomised backtracking attempts and keeps the best.
func runRestarts(ctx context.Context, env searchEnv, rng *rand.Rand) Result {
	best := Result{Total: len(env.occ)}
	deadline, _ := ctx.Deadline()
	for {
		if time.Now().After(deadline) {
			break
		}
		// Короткие попытки: на большой школе один перебор может съесть
		// весь дедлайн — ограничиваем 5 секундами, чтобы рестарты
		// продолжались и лучшее частичное решение накапливалось.
		attemptDl := time.Now().Add(5 * time.Second)
		if attemptDl.After(deadline) {
			attemptDl = deadline
		}
		r := backtrackDl(env, rng, attemptDl)
		if r.Placed > best.Placed || (r.Placed == best.Placed && r.Violations < best.Violations) {
			best = r
		}
		if r.Placed == env.placeable && r.Violations == 0 {
			break // perfect
		}
		if r.Placed == env.placeable {
			// good enough, keep searching a bit for fewer violations
			if time.Now().After(deadline.Add(-200 * time.Millisecond)) {
				break
			}
		}
	}
	return best
}

// searchEnv — предвычисленная, неизменяемая между попытками поиска часть
// модели: occurrences, жёсткие запреты, статические слоты по HardSet,
// порядок обхода, правила и ПЛОТНЫЕ индексы сущностей (учителя/классы/
// кабинеты → 0..n-1), благодаря которым горячий цикл перебора работает
// с плоскими массивами без единого map-lookup-а.
// Мутируемые scratch-буферы (candBuf, famScale) выделяются на воркера
// через forWorker: сама переборная рекурсия однопоточна внутри воркера.
type searchEnv struct {
	in     SolveInput
	occ    []Occurrence
	hard   HardSet
	days   int
	slots  int
	stride int // days*slots — размер «сетки» одной сущности

	nT, nC, nR int // плотные количества учителей/классов/кабинетов

	order     []int       // порядок обхода: базы, за каждой — её половинки
	baseOfOcc map[int]int // occ idx → occ idx базового урока (только половинки)
	placeable int         // вхождения, которые вообще можно поставить (= len(order)); остальные статически невозможны
	staticDS  [][]cell    // occurrence → (d,s)-слоты, разрешённые HardSet (room=0)
	candBuf   [][]cell    // occurrence → scratch для динамической фильтрации
	famScale  []int       // scratch для familyRunFlat

	// Плотные предвычисления на occurrence: индексы и правила, которые
	// раньше вычислялись map-lookup-ами в каждом узле перебора.
	occTIdx     []int          // → плотный индекс учителя
	occCIdx     []int          // → плотный индекс класса
	occCS       []classSetsIdx // → check/mark наборы (базы уже умножены на stride)
	occRoomIdx  [][]int        // → плотные индексы кабинетов (параллельно RoomChoices)
	occTMaxDay  []int          // макс. уроков в день учителя (0 = нет правила)
	occCMaxDay  []int
	occTMaxCons []int // макс. подряд уроков учителя (0 = нет правила)
	occCMaxCons []int
	occFamRule  []int   // макс. подряд одного предмета в семье класса (0 = нет)
	occFamIdx   [][]int // семейная шкала (плотные базы) для occFamRule

	bodies    map[int][]int     // classID → тела учеников (валидация обеда)
	classSets map[int]classSets // classID → check/mark наборы (DB id)
	family    map[int][]int     // classID → семейная шкала (DB id)
	runRules  []runRule         // «не более N одного предмета подряд»
	rulesOn   bool

	teacherMaxCons map[int]int
	classMaxCons   map[int]int
	teacherMaxDay  map[int]int
	classMaxDay    map[int]int
}

// runRule — «не более N уроков одного предмета подряд в день» (модель aSc):
// правило для предмета (subj > 0) или для всех предметов (subj == 0).
type runRule struct{ subj, maxRun int }

// classSetsIdx — плотная версия classSets: check/mark хранят базы
// ячеек классов (плотный индекс × stride), к которым в узле прибавляется
// смещение (day*slots + slot).
type classSetsIdx struct {
	check []int
	mark  []int
}

// forWorker копирует окружение с собственными scratch-буферами: статика
// (occ, staticDS, order, правила, плотные индексы) общая и в поиске
// только читается.
func (e searchEnv) forWorker() searchEnv {
	out := e
	out.candBuf = make([][]cell, len(e.occ))
	for i := range e.occ {
		// худший размер после динамической фильтрации: слоты × комнаты
		out.candBuf[i] = make([]cell, 0, len(e.staticDS[i])*len(e.occ[i].RoomChoices))
	}
	out.famScale = make([]int, e.slots)
	return out
}

// buildSearchEnv предвычисляет всё статическое: слоты по HardSet,
// пары делёных уроков за O(n) вместо O(n²), семейные шкалы, карты правил
// и плотные индексы всех сущностей.
func buildSearchEnv(in SolveInput, days, slots int) searchEnv {
	occ := buildOccurrences(in, days, slots)
	hard := buildHard(in, days, slots)
	stride := days * slots
	env := searchEnv{
		in: in, occ: occ, hard: hard, days: days, slots: slots, stride: stride,
		baseOfOcc:      map[int]int{},
		bodies:         buildStudentBodies(in.Classes),
		classSets:      buildClassSets(in.Classes),
		family:         map[int][]int{},
		teacherMaxCons: map[int]int{},
		classMaxCons:   map[int]int{},
		teacherMaxDay:  map[int]int{},
		classMaxDay:    map[int]int{},
	}

	// Плотные индексы: все сущности из справочников плюс сущности,
	// на которые ссылаются уроки (защита от битых ссылок: старый код
	// в этом месте падал бы на nil-карте занятости).
	tPos := map[int]int{}
	for t := range in.Teachers {
		tPos[t] = len(tPos)
	}
	cPos := map[int]int{}
	for c := range in.Classes {
		cPos[c] = len(cPos)
	}
	rPos := map[int]int{}
	for _, r := range in.Rooms {
		rPos[r.ID] = len(rPos)
	}
	for _, l := range in.Lessons {
		if _, ok := tPos[l.TeacherID]; !ok {
			tPos[l.TeacherID] = len(tPos)
		}
		if _, ok := cPos[l.ClassID]; !ok {
			cPos[l.ClassID] = len(cPos)
		}
	}
	env.nT, env.nC, env.nR = len(tPos), len(cPos), len(rPos)

	// Статические (d,s)-слоты: только HardSet-фильтр, без занятости.
	// Комнаты не в статике — HardSet.Forbidden не знает комнат (roomID=0),
	// и перечисляются они в узле, где видна занятость. Слот занят —
	// все его комнаты отбрасываются одной проверкой (а не 16×).
	env.staticDS = make([][]cell, len(occ))
	for i, o := range occ {
		list := make([]cell, 0, days*slots)
		for d := 0; d < days; d++ {
			for s := 0; s < slots; s++ {
				if hard.Forbidden(o.Lesson.TeacherID, o.Lesson.ClassID, 0, d, s) {
					continue
				}
				list = append(list, cell{day: d, slot: s})
			}
		}
		env.staticDS[i] = list
	}

	// Делёные пары (модель aSc): урок подгруппы с тем же предметом, что и
	// урок родительского класса. Половинки ставятся СРАЗУ после базы.
	halfOfLesson := map[int]int{} // id урока-половинки → id базового урока
	byKey := map[string]int{}
	for _, l := range in.Lessons {
		if p := subParentOfClass(in.Classes, l.ClassID); p != 0 {
			key := fmt.Sprintf("%d:%d", p, l.SubjectID)
			if base, ok := byKey[key]; ok {
				halfOfLesson[l.ID] = base
			} else {
				byKey[key] = l.ID
			}
		}
	}
	// Пары делёных уроков: k-е вхождение половинки ↔ k-е вхождение базы
	// (вхождения одного урока идут подряд — buildOccurrences). Раньше все
	// вхождения половинки приковывались к ПОСЛЕДНЕМУ вхождению базы: при
	// часах ≥ 2 второе вхождение половинки не могло встать параллельно
	// первому (тот же класс дважды в слоте) — делёный урок с 2+ часами
	// не размещался вовсе. Для часов = 1 (обычный случай) пары те же,
	// поведение не меняется.
	firstOcc := map[int]int{} // lessonID → индекс первого вхождения
	occCount := map[int]int{} // lessonID → число вхождений
	for i := range occ {
		lid := occ[i].Lesson.ID
		if _, ok := firstOcc[lid]; !ok {
			firstOcc[lid] = i
		}
		occCount[lid]++
	}
	for i := range occ {
		lid := occ[i].Lesson.ID
		base, isHalf := halfOfLesson[lid]
		if !isHalf {
			continue
		}
		bi, ok := firstOcc[base]
		if !ok {
			continue
		}
		if k := i - firstOcc[lid]; k >= 0 && k < occCount[base] {
			env.baseOfOcc[i] = bi + k
		}
	}

	// Статически невозможные вхождения: ставить некуда В ПРИНЦИПЕ — все
	// ячейки недели закрыты жёсткими запретами «недоступен» (учитель или
	// класс), либо предмету не подходит ни один кабинет. Раньше одно
	// такое вхождение обнуляло всю попытку: rec() не находил кандидатов,
	// откатывался до корня, и генерация возвращала «0 из N» для ВСЕЙ
	// школы — хотя остальные уроки расставлялись бы без проблем. Теперь
	// невозможные уроки исключаются из обхода: остальная школа строится,
	// а пропущенные часы видны в отчёте с конкретной причиной.
	skip := make([]bool, len(occ))
	for i := range occ {
		if len(env.staticDS[i]) == 0 || len(occ[i].RoomChoices) == 0 {
			skip[i] = true
		}
	}
	// Семья делёного урока (база + её половинки) ставится только целиком:
	// невозможна любая часть → пропускаем всю семью, иначе одиночная
	// половинка или база без пары дала бы конфликт «половинка без пары».
	familyOf := map[int][]int{} // occ idx базы → occ idx её половинок
	for h, b := range env.baseOfOcc {
		familyOf[b] = append(familyOf[b], h)
	}
	for b, hs := range familyOf {
		bad := skip[b]
		for _, h := range hs {
			if skip[h] {
				bad = true
				break
			}
		}
		if bad {
			skip[b] = true
			for _, h := range hs {
				skip[h] = true
			}
		}
	}

	// Порядок обхода: наиболее ограниченные вперёд, половинки — сразу
	// за своей базой (параллельно с ней, в один слот).
	all := make([]int, len(occ))
	for i := range all {
		all[i] = i
	}
	sort.SliceStable(all, func(a, b int) bool {
		return len(occ[all[a]].RoomChoices) < len(occ[all[b]].RoomChoices)
	})
	placed := make([]bool, len(occ))
	order := make([]int, 0, len(occ))
	for _, i := range all {
		if placed[i] || skip[i] {
			continue
		}
		if _, isHalf := env.baseOfOcc[i]; isHalf {
			continue // половинки — во втором проходе, после баз
		}
		order = append(order, i)
		placed[i] = true
		// сразу за базой — её половинки
		for _, j := range all {
			if !placed[j] && !skip[j] && env.baseOfOcc[j] == i {
				order = append(order, j)
				placed[j] = true
			}
		}
	}
	for _, i := range all {
		if !placed[i] && !skip[i] {
			order = append(order, i)
			placed[i] = true
		}
	}
	env.order = order
	env.placeable = len(order)

	// Правила из ограничений: подряд/в день — карты по сущности.
	for _, c := range in.Constraints {
		if !c.IsHard || c.Type != "max_consecutive_subject" || c.Weight < 1 {
			continue
		}
		if c.EntityType == "subject" {
			env.runRules = append(env.runRules, runRule{c.EntityID, c.Weight})
		} else if c.EntityType == "school" {
			env.runRules = append(env.runRules, runRule{0, c.Weight})
		}
	}
	env.rulesOn = len(env.runRules) > 0
	for _, c := range in.Constraints {
		if !c.IsHard {
			continue
		}
		switch c.Type {
		case "max_consecutive":
			if c.EntityType == "teacher" {
				env.teacherMaxCons[c.EntityID] = c.Weight
			}
			if c.EntityType == "class" {
				env.classMaxCons[c.EntityID] = c.Weight
			}
		case "max_lessons_per_day":
			if c.EntityType == "teacher" {
				env.teacherMaxDay[c.EntityID] = c.Weight
			}
			if c.EntityType == "class" {
				env.classMaxDay[c.EntityID] = c.Weight
			}
		}
	}
	// Семейная шкала для runRules: у подгруппы — тело родителя,
	// иначе своё. Проверка идёт по сквозной шкале семьи класса
	// (родитель + подгруппы): половинки в одном слоте — один период.
	if env.rulesOn {
		for i := range occ {
			cid := occ[i].Lesson.ClassID
			fam := cid
			if p := subParentOfClass(in.Classes, cid); p != 0 {
				fam = p
			}
			if _, ok := env.family[cid]; !ok {
				env.family[cid] = env.bodies[fam]
			}
		}
	}

	// Плотные предвычисления на occurrence.
	n := len(occ)
	env.occTIdx = make([]int, n)
	env.occCIdx = make([]int, n)
	env.occCS = make([]classSetsIdx, n)
	env.occRoomIdx = make([][]int, n)
	env.occTMaxDay = make([]int, n)
	env.occCMaxDay = make([]int, n)
	env.occTMaxCons = make([]int, n)
	env.occCMaxCons = make([]int, n)
	env.occFamRule = make([]int, n)
	env.occFamIdx = make([][]int, n)
	for i, o := range occ {
		tID, cID := o.Lesson.TeacherID, o.Lesson.ClassID
		tIdx, cIdx := tPos[tID], cPos[cID]
		env.occTIdx[i] = tIdx
		env.occCIdx[i] = cIdx
		ri := make([]int, len(o.RoomChoices))
		for k, r := range o.RoomChoices {
			ri[k] = rPos[r]
		}
		env.occRoomIdx[i] = ri
		if cs, ok := denseClassSets(env.classSets[cID], cPos, stride); ok {
			env.occCS[i] = cs
		}
		// класс вне справочника (битая ссылка) — без проверок и меток,
		// как в исходном переборе (нулевой classSets)
		env.occTMaxDay[i] = env.teacherMaxDay[tID]
		env.occCMaxDay[i] = env.classMaxDay[cID]
		env.occTMaxCons[i] = env.teacherMaxCons[tID]
		env.occCMaxCons[i] = env.classMaxCons[cID]
		if env.rulesOn {
			maxRun := 0
			for _, rr := range env.runRules {
				if rr.subj == 0 || rr.subj == o.Lesson.SubjectID {
					if maxRun == 0 || rr.maxRun < maxRun {
						maxRun = rr.maxRun
					}
				}
			}
			env.occFamRule[i] = maxRun
			if maxRun > 0 {
				famDB := env.family[cID]
				fam := make([]int, len(famDB))
				for k, cid := range famDB {
					fam[k] = cPos[cid] * stride
				}
				env.occFamIdx[i] = fam
			}
		}
	}
	return env
}

// denseClassSets переводит check/mark наборы из DB id в плотные базы ячеек
// (плотный индекс × stride). Ссылки всегда из справочника классов, поэтому
// miss невозможен; нулевой classSets (класс вне справочника) остаётся
// пустым — как в исходном переборе.
func denseClassSets(cs classSets, cPos map[int]int, stride int) (classSetsIdx, bool) {
	toBase := func(ids []int) []int {
		out := make([]int, len(ids))
		for i, id := range ids {
			idx, ok := cPos[id]
			if !ok {
				return nil
			}
			out[i] = idx * stride
		}
		return out
	}
	check := toBase(cs.check)
	mark := toBase(cs.mark)
	if check == nil || mark == nil {
		return classSetsIdx{}, false
	}
	return classSetsIdx{check: check, mark: mark}, true
}

// backtrackDl — перебор с жёстким дедлайном ВНУТРИ поиска: на больших
// школах (сотни уроков) один вызов перебора без проверки времени уходил
// в экспоненциальный откат на часы, игнорируя дедлайн рестартов.
// Проверка каждые 4096 узлов прерывает поиск — наверх идёт лучшее
// найденное к этому моменту.
//
// Занятость — ПЛОСКИЕ массивы (сущность×days×slots): индекс ячейки =
// сущность*stride + day*slots + slot. Профиль старого перебора показывал
// до 52% времени в map-lookup-ах занятости; здесь их нет вовсе.
func backtrackDl(env searchEnv, rng *rand.Rand, deadline time.Time) Result {
	in, occ := env.in, env.occ
	days, slots, stride := env.days, env.slots, env.stride

	tBusy := make([]bool, env.nT*stride) // учитель занят
	cBusy := make([]int, env.nC*stride)  // класс занят (ID предмета; 0 = свободен)
	rBusy := make([]bool, env.nR*stride) // кабинет занят
	tDay := make([]int, env.nT*days)     // уроков в день у учителя
	cDay := make([]int, env.nC*days)     // уроков в день у класса

	assign := make([]cell, len(occ))
	var solution []cell
	ok := false
	nodes := 0
	aborted := false

	var rec func(k int) bool
	rec = func(k int) bool {
		if aborted {
			return false
		}
		if nodes++; nodes&4095 == 0 && !deadline.IsZero() && time.Now().After(deadline) {
			aborted = true
			return false
		}
		if k == len(env.order) {
			entries := buildEntries(in, assign, occ)
			if !validateAggregates(in, env.bodies, entries, days, slots) {
				return false
			}
			sol := make([]cell, len(assign))
			copy(sol, assign)
			solution = sol
			ok = true
			return true
		}
		oi := env.order[k]
		o := occ[oi]
		cs := env.occCS[oi]
		roomIdx := env.occRoomIdx[oi]
		subj := o.Lesson.SubjectID
		tMaxDay := env.occTMaxDay[oi]
		cMaxDay := env.occCMaxDay[oi]
		tMaxCons := env.occTMaxCons[oi]
		cMaxCons := env.occCMaxCons[oi]
		famRule := env.occFamRule[oi]
		famIdx := env.occFamIdx[oi]
		tBase := env.occTIdx[oi] * stride
		cBase := env.occCIdx[oi] * stride
		tDayBase := env.occTIdx[oi] * days
		cDayBase := env.occCIdx[oi] * days

		// Динамическая фильтрация кандидатов в scratch-буфер: занятость
		// учителя/класса/кабинета не меняется между итерациями уровня
		// (метки ставятся и СНИМАются внутри итерации), поэтому фильтр
		// достаточно вычислить один раз на узел. Проверки учителя и
		// класса — один раз на (d,s)-слот; комнаты перебираются внутри
		// свободного слота.
		cands := env.candBuf[oi][:0]
		for _, dsc := range env.staticDS[oi] {
			ds := dsc.day*slots + dsc.slot
			if tBusy[tBase+ds] {
				continue
			}
			if !classFreeFlat(cs.check, cBusy, ds, subj) {
				continue
			}
			for k2, r := range o.RoomChoices {
				if rBusy[roomIdx[k2]*stride+ds] {
					continue
				}
				cands = append(cands, cell{day: dsc.day, slot: dsc.slot, room: r, rIdx: roomIdx[k2]})
			}
		}
		// Делёная половинка: ставится ПАРАЛЛЕЛЬНО своей базе — кандидаты
		// только в слотах, где уже стоит базовый урок.
		if baseOcc, isHalf := env.baseOfOcc[oi]; isHalf {
			baseCell := assign[baseOcc]
			if baseCell.room != 0 {
				filtered := cands[:0]
				for _, cc := range cands {
					if cc.day == baseCell.day && cc.slot == baseCell.slot {
						filtered = append(filtered, cc)
					}
				}
				cands = filtered
			}
		}
		shuffle(cands, rng)
		for _, cc := range cands {
			ds := cc.day*slots + cc.slot
			if tMaxDay > 0 && tDay[tDayBase+cc.day]+1 > tMaxDay {
				continue
			}
			if cMaxDay > 0 && cDay[cDayBase+cc.day]+1 > cMaxDay {
				continue
			}
			if tMaxCons > 0 && runAtFlatBool(tBusy, tBase+cc.day*slots, cc.slot, slots) > tMaxCons {
				continue
			}
			if cMaxCons > 0 && runAtFlatInt(cBusy, cBase+cc.day*slots, cc.slot, slots) > cMaxCons {
				continue
			}
			if famRule > 0 && familyRunFlat(cBusy, famIdx, stride, cc.day, cc.slot, subj, slots, env.famScale) > famRule {
				continue
			}
			tBusy[tBase+ds] = true
			for _, cb := range cs.mark {
				cBusy[cb+ds] = subj
			}
			rBusy[cc.rIdx*stride+ds] = true
			tDay[tDayBase+cc.day]++
			cDay[cDayBase+cc.day]++
			assign[oi] = cc
			if rec(k + 1) {
				return true
			}
			if aborted {
				// Прерваны по дедлайну: оставляем текущую цепочку
				// размещений в assign как частичное решение (карты
				// занятости локальны для попытки и будут выброшены).
				return false
			}
			tBusy[tBase+ds] = false
			for _, cb := range cs.mark {
				cBusy[cb+ds] = 0
			}
			rBusy[cc.rIdx*stride+ds] = false
			tDay[tDayBase+cc.day]--
			cDay[cDayBase+cc.day]--
			// ВАЖНО: очищаем и саму ячейку — иначе после отката в assign
			// остаётся «призрачный» урок, и частичный результат получает
			// двойные брони (весь интерфейс краснел от конфликтов).
			assign[oi] = cell{}
		}
		return false
	}
	rec(0)

	if ok {
		entries := buildEntries(in, solution, occ)
		// Placed — только реально стоящие вхождения: статически
		// невозможные уроки в order не входят и не должны считаться.
		return Result{Entries: entries, Placed: len(entries), Total: len(occ), Violations: softViolations(in, entries, days)}
	}
	// Прерваны по дедлайну: возвращаем частичное размещение — assign
	// содержит все успешно поставленные на данный момент уроки.
	partialCount := 0
	for _, c := range assign {
		if c.room != 0 {
			partialCount++
		}
	}
	if partialCount > 0 {
		entries := buildEntries(in, assign, occ)
		return Result{Entries: entries, Placed: len(entries), Total: len(occ), Violations: softViolations(in, entries, days)}
	}
	return Result{Entries: nil, Placed: 0, Total: len(occ)}
}

type cell struct {
	day, slot, room int
	rIdx            int // плотный индекс кабинета (для плоской карты занятости)
}

// classFreeFlat: слот свободен для урока с предметом subj. Занятость
// хранит ID предмета: занято ТЕМ ЖЕ предметом (делёный урок, обе
// половинки параллельно) — не блокирует; другим предметом — блокирует.
// cb — плотные базы классов (idx*stride), ds — смещение слота.
func classFreeFlat(check []int, cBusy []int, ds int, subj int) bool {
	for _, cb := range check {
		busySubj := cBusy[cb+ds]
		if busySubj == 0 {
			continue
		}
		if busySubj == subj {
			continue // делёный предмет: обе половинки параллельно
		}
		return false // слот занят другим предметом этого класса
	}
	return true
}

// runAtFlatBool returns the length of the consecutive occupied run through
// slot if it were occupied. base — начало дня в плоской карте.
func runAtFlatBool(g []bool, base, slot, slots int) int {
	cnt := 1
	for s := slot - 1; s >= 0 && g[base+s]; s-- {
		cnt++
	}
	for s := slot + 1; s < slots && g[base+s]; s++ {
		cnt++
	}
	return cnt
}

func runAtFlatInt(g []int, base, slot, slots int) int {
	cnt := 1
	for s := slot - 1; s >= 0 && g[base+s] != 0; s-- {
		cnt++
	}
	for s := slot + 1; s < slots && g[base+s] != 0; s++ {
		cnt++
	}
	return cnt
}

// familyRunFlat: длина серии одинаковых предметов в сквозной шкале
// семьи класса (родитель + подгруппы) на день d, если поставить subj
// в слот s. Занятость семьи в слоте — первый ненулевой предмет среди
// членов семьи; половинки в одном слоте считаются одним периодом.
// fam — плотные базы ячеек членов семьи; day — scratch-буфер воркера:
// вызов синхронный, рекурсия внутрь не уходит, переиспользование
// безопасно и убирает аллокацию из самого горячего цикла перебора.
func familyRunFlat(cBusy []int, fam []int, stride, d, s, subj, slots int, day []int) int {
	dayBase := d * slots
	for t := 0; t < slots; t++ {
		day[t] = 0
		for _, fb := range fam {
			if v := cBusy[fb+dayBase+t]; v != 0 {
				day[t] = v
				break
			}
		}
	}
	day[s] = subj
	run := 1
	for t := s - 1; t >= 0 && day[t] == subj; t-- {
		run++
	}
	for t := s + 1; t < slots && day[t] == subj; t++ {
		run++
	}
	return run
}

// HardSet holds precomputed forbidden (teacher/class/room, day, slot) cells.
type HardSet struct {
	teacher map[int]map[int]bool // teacherID -> day*slots+slot
	class   map[int]map[int]bool
	room    map[int]map[int]bool
}

func (h HardSet) Forbidden(teacherID, classID, roomID, day, slot int) bool {
	key := day*1000 + slot
	if h.teacher[teacherID][key] {
		return true
	}
	if h.class[classID][key] {
		return true
	}
	if roomID != 0 && h.room[roomID][key] {
		return true
	}
	return false
}

func buildHard(in SolveInput, days, slots int) HardSet {
	h := HardSet{
		teacher: map[int]map[int]bool{},
		class:   map[int]map[int]bool{},
		room:    map[int]map[int]bool{},
	}
	for _, c := range in.Constraints {
		if !c.IsHard {
			continue
		}
		if c.Type != "teacher_unavailable" && c.Type != "class_unavailable" && c.Type != "room_unavailable" {
			continue
		}
		dow := -1
		if c.DayOfWeek != nil {
			dow = *c.DayOfWeek
		}
		ts := -1
		if c.TimeslotStart != nil {
			ts = *c.TimeslotStart
		}
		target := map[int]map[int]bool{}
		switch c.EntityType {
		case "teacher":
			target = h.teacher
		case "class":
			target = h.class
		case "room":
			target = h.room
		}
		if target[c.EntityID] == nil {
			target[c.EntityID] = map[int]bool{}
		}
		te := ts
		if c.TimeslotEnd != nil {
			te = *c.TimeslotEnd
		}
		if dow >= 0 && ts >= 0 {
			for sl := ts; sl <= te && sl < slots; sl++ {
				target[c.EntityID][dow*1000+sl] = true
			}
		} else if dow >= 0 {
			for s := 0; s < slots; s++ {
				target[c.EntityID][dow*1000+s] = true
			}
		} else if ts >= 0 {
			for d := 0; d < days; d++ {
				target[c.EntityID][d*1000+ts] = true
			}
		} else {
			for d := 0; d < days; d++ {
				for s := 0; s < slots; s++ {
					target[c.EntityID][d*1000+s] = true
				}
			}
		}
	}
	return h
}

func buildOccurrences(in SolveInput, days, slots int) []Occurrence {
	var occ []Occurrence
	for _, l := range in.Lessons {
		h := l.HoursPerWeek
		if h < 1 {
			h = 1
		}
		subj := in.Subjects[l.SubjectID]
		rooms := allowedRooms(subj, in.Rooms)
		for i := 0; i < h; i++ {
			occ = append(occ, Occurrence{Lesson: l, Index: i, RoomChoices: rooms})
		}
	}
	return occ
}

// allowedRooms filters rooms by the subject's required room type.
func allowedRooms(subj domain.Subject, rooms []domain.Room) []int {
	rt := subj.RequiresRoomType
	if rt == "" {
		rt = "any"
	}
	var out []int
	for _, r := range rooms {
		if rt == "any" || r.RoomType == "any" || r.RoomType == rt {
			out = append(out, r.ID)
		}
	}
	if len(out) == 0 && len(rooms) > 0 {
		for _, r := range rooms {
			out = append(out, r.ID)
		}
	}
	return out
}

// softViolations counts soft-constraint penalties (gaps between same-class lessons, preferences).
func softViolations(in SolveInput, entries []domain.ScheduleEntry, days int) int {
	v := 0
	// soft constraints: prefer_morning / max_gaps style => small penalties for afternoon clustering
	for _, c := range in.Constraints {
		if c.IsHard {
			continue
		}
		switch c.Type {
		case "prefer_morning":
			for _, e := range entries {
				if matchesEntity(c, e) && e.Timeslot >= 4 {
					v += 1
				}
			}
		case "max_gaps":
			// count gaps per day per class
			gaps := gapCount(entries, c.EntityID)
			if gaps > c.Weight {
				v += gaps - c.Weight
			}
		case "min_lessons_per_day":
			byDay := map[int]int{}
			for _, e := range entries {
				if matchesEntity(c, e) {
					byDay[e.DayOfWeek]++
				}
			}
			for d := 0; d < days; d++ {
				if byDay[d] < c.Weight {
					v += c.Weight - byDay[d]
				}
			}
		}
	}
	return v
}

func matchesEntity(c domain.Constraint, e domain.ScheduleEntry) bool {
	switch c.EntityType {
	case "teacher":
		return e.TeacherID == c.EntityID
	case "class":
		return e.ClassID == c.EntityID
	case "room":
		return e.RoomID == c.EntityID
	case "lesson":
		return e.LessonID == c.EntityID
	case "school":
		return true
	}
	return false
}

func gapCount(entries []domain.ScheduleEntry, classID int) int {
	byDay := map[int][]int{}
	for _, e := range entries {
		if e.ClassID != classID {
			continue
		}
		byDay[e.DayOfWeek] = append(byDay[e.DayOfWeek], e.Timeslot)
	}
	total := 0
	for _, slots := range byDay {
		if len(slots) < 2 {
			continue
		}
		sort.Ints(slots)
		for i := 1; i < len(slots); i++ {
			if slots[i]-slots[i-1] > 1 {
				total++
			}
		}
	}
	return total
}

func shuffle(c []cell, rng *rand.Rand) {
	rng.Shuffle(len(c), func(i, j int) { c[i], c[j] = c[j], c[i] })
}

// buildStudentBodies returns, for each class, the set of class ids whose
// student bodies overlap: у родителя тело включает подгруппы, у подгруппы —
// только она сама (две подгруппы одного родителя disjoint).
func buildStudentBodies(classes map[int]domain.SchoolClass) map[int][]int { // lint: keep
	children := map[int][]int{}
	for id, c := range classes {
		if c.SubgroupOf != nil {
			children[*c.SubgroupOf] = append(children[*c.SubgroupOf], id)
		}
	}
	res := map[int][]int{}
	for id := range classes {
		set := []int{id}
		if subs, ok := children[id]; ok {
			set = append(set, subs...)
		}
		res[id] = set
	}
	return res
}

// subParentOfClass: id родительского класса, если класс — подгруппа (иначе 0).
func subParentOfClass(classes map[int]domain.SchoolClass, classID int) int {
	if c, ok := classes[classID]; ok && c.SubgroupOf != nil {
		return *c.SubgroupOf
	}
	return 0
}

// classSets — наборы для размещения урока класса:
// check — чью занятость проверяем, mark — чью помечаем после установки.
// Родитель: проверяет и занимает себя + подгруппы (пересечение запрещено).
// Подгруппа: проверяет себя И родителя, занимает ТОЛЬКО себя — поэтому
// две подгруппы одного класса могут идти параллельно с двумя учителями.
type classSets struct {
	check []int
	mark  []int
}

func buildClassSets(classes map[int]domain.SchoolClass) map[int]classSets {
	children := map[int][]int{}
	parentOf := map[int]int{}
	for id, c := range classes {
		if c.SubgroupOf != nil {
			if _, ok := classes[*c.SubgroupOf]; ok {
				children[*c.SubgroupOf] = append(children[*c.SubgroupOf], id)
				parentOf[id] = *c.SubgroupOf
			}
		}
	}
	res := map[int]classSets{}
	for id := range classes {
		cs := classSets{check: []int{id}, mark: []int{id}}
		if subs, ok := children[id]; ok {
			cs.check = append(cs.check, subs...)
			cs.mark = append(cs.mark, subs...)
		}
		if p, ok := parentOf[id]; ok {
			cs.check = append(cs.check, p)
		}
		res[id] = cs
	}
	return res
}

func buildEntries(in SolveInput, assign []cell, occ []Occurrence) []domain.ScheduleEntry {
	entries := make([]domain.ScheduleEntry, 0, len(occ))
	for i := range occ {
		c := assign[i]
		if c.room == 0 {
			continue
		}
		o := occ[i]
		entries = append(entries, domain.ScheduleEntry{
			SchoolID:  in.SchoolID,
			LessonID:  o.Lesson.ID,
			ClassID:   o.Lesson.ClassID,
			TeacherID: o.Lesson.TeacherID,
			SubjectID: o.Lesson.SubjectID,
			RoomID:    c.room,
			DayOfWeek: c.day,
			Timeslot:  c.slot,
		})
	}
	return entries
}

// validateAggregates checks day-level hard constraints that cannot be enforced incrementally.
func validateAggregates(in SolveInput, bodies map[int][]int, entries []domain.ScheduleEntry, days, slots int) bool {
	for _, c := range in.Constraints {
		if !c.IsHard {
			continue
		}
		if c.Type == "lunch_break" {
			if !lunchOK(in, bodies, c, entries, days, slots) {
				return false
			}
		}
	}
	return true
}

func lunchOK(in SolveInput, bodies map[int][]int, c domain.Constraint, entries []domain.ScheduleEntry, days, slots int) bool {
	occ := make([][]bool, days)
	for d := range occ {
		occ[d] = make([]bool, slots)
	}
	for _, e := range entries {
		var inEnt bool
		switch c.EntityType {
		case "school":
			inEnt = true
		case "teacher":
			inEnt = e.TeacherID == c.EntityID
		case "class":
			inEnt = sharesStudent(bodies, c.EntityID, e.ClassID)
		}
		if inEnt && e.DayOfWeek < days && e.Timeslot < slots {
			occ[e.DayOfWeek][e.Timeslot] = true
		}
	}
	start, end := 0, slots-1
	if c.TimeslotStart != nil {
		start = *c.TimeslotStart
	}
	if c.TimeslotEnd != nil {
		end = *c.TimeslotEnd
	}
	if end >= slots {
		end = slots - 1
	}
	for d := 0; d < days; d++ {
		free := false
		for s := start; s <= end; s++ {
			if !occ[d][s] {
				free = true
				break
			}
		}
		if !free {
			return false
		}
	}
	return true
}

func sharesStudent(bodies map[int][]int, a, b int) bool {
	sa := bodies[a]
	if sa == nil {
		sa = []int{a}
	}
	sb := bodies[b]
	if sb == nil {
		sb = []int{b}
	}
	set := map[int]bool{}
	for _, x := range sa {
		set[x] = true
	}
	for _, x := range sb {
		if set[x] {
			return true
		}
	}
	return false
}

// preciseSolver is assigned by the OR-Tools build-tagged file (internal/solver/ortools_csat.go)
// when the binary is compiled with `-tags ortools`. It is nil otherwise.
var preciseSolver func(in SolveInput, parallelism int, timeout time.Duration) (Result, bool)

// HasPreciseSolver reports whether the running binary was compiled with the
// OR-Tools CP-SAT solver (-tags ortools). When false, SolvePrecise silently
// falls back to the pure-Go backtracking solver. The frontend uses this to
// tell the user the truth instead of flashing "CP-SAT enabled" when the
// heavy C++ dependency is not actually linked in.
func HasPreciseSolver() bool {
	return preciseSolver != nil
}

// SolvePrecise prefers the OR-Tools CP-SAT solver when compiled in; otherwise falls back to the
// pure-Go backtracking solver so the default build always works.
func SolvePrecise(ctx context.Context, in SolveInput, parallelism int, timeout time.Duration) Result {
	start := time.Now()
	if preciseSolver != nil {
		r, ok := preciseSolver(in, parallelism, timeout)
		if ok && r.Placed >= r.Total {
			r.ElapsedMs = time.Since(start).Milliseconds()
			return r // всё размещено — эвристика не нужна
		}
		// Школа перегружена или модель тяжёлая: эвристика может
		// разместить больше — считаем её и берём лучший результат.
		h := Solve(ctx, in, parallelism, timeout)
		totalMs := time.Since(start).Milliseconds()
		if h.Placed > r.Placed || !ok {
			h.ElapsedMs = totalMs
			return h
		}
		r.ElapsedMs = totalMs
		return r
	}
	return Solve(ctx, in, parallelism, timeout)
}
