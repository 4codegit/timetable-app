//go:build windows && ortools

package solver

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"timetable/internal/domain"
)

// Mirror of C structs in bind.h (field order + alignment must match).
type cConstraint struct {
	ctype      int32
	entityType int32
	entityID   int32
	day        int32
	slotStart  int32
	slotEnd    int32
	value      int32
	isHard     int32
}

type ortoolsDLL struct {
	mod   *syscall.LazyDLL
	solve *syscall.LazyProc
}

var loadedDLL *ortoolsDLL

func loadDLL() (*ortoolsDLL, error) {
	if loadedDLL != nil {
		return loadedDLL, nil
	}
	mod := syscall.NewLazyDLL("ortools_csat.dll")
	d := &ortoolsDLL{
		mod:   mod,
		solve: mod.NewProc("ortools_solve_out"),
	}
	if err := mod.Load(); err != nil {
		return nil, err
	}
	// Проверка функции заранее: у LazyProc.Call отсутствующая процедура
	// вызывает панику. Старый ortools_csat.dll (без ortools_solve_out)
	// рядом с новым exe должен тихо увести на эвристический решатель,
	// а не уронить приложение.
	if err := d.solve.Find(); err != nil {
		return nil, err
	}
	loadedDLL = d
	return d, nil
}

func init() {
	preciseSolver = ortoolsSolve
}

func ortoolsSolve(in SolveInput, parallelism int, timeout time.Duration) (Result, bool) {
	d, err := loadDLL()
	if err != nil {
		return Result{}, false
	}

	in = appendStudentGroupConstraints(in)
	in, cfgDays := prepareDaysMask(in)
	days := int32(cfgDays)
	if days <= 0 {
		days = 6
	}
	slots := int32(in.Config.SlotsPerDay)
	if slots <= 0 {
		slots = 8
	}

	nl := len(in.Lessons)
	hours := make([]int32, maxInt(nl, 1))
	lclass := make([]int32, maxInt(nl, 1))
	lteacher := make([]int32, maxInt(nl, 1))
	lsubject := make([]int32, maxInt(nl, 1))
	occCount := 0
	for i, l := range in.Lessons {
		h := l.HoursPerWeek
		if h < 1 {
			h = 1
		}
		hours[i] = int32(h)
		occCount += h
		lclass[i] = int32(l.ClassID)
		lteacher[i] = int32(l.TeacherID)
		lsubject[i] = int32(l.SubjectID)
	}

	roomTypeID := map[string]int32{"any": 0}
	next := int32(1)
	assignType := func(s string) int32 {
		if s == "" {
			return 0
		}
		if id, ok := roomTypeID[s]; ok {
			return id
		}
		id := next
		next++
		roomTypeID[s] = id
		return id
	}
	lreq := make([]int32, maxInt(nl, 1))
	for i, l := range in.Lessons {
		rt := "any"
		if subj, ok := in.Subjects[l.SubjectID]; ok && subj.RequiresRoomType != "" {
			rt = subj.RequiresRoomType
		}
		lreq[i] = assignType(rt)
	}

	nr := len(in.Rooms)
	// Ничего не планируем без уроков/кабинетов (C вернул бы 0).
	if nl == 0 || occCount == 0 || nr == 0 {
		return Result{Entries: nil, Placed: 0, Total: occCount, Violations: 0}, true
	}
	roomIDs := make([]int32, nr)
	roomTypes := make([]int32, nr)
	for i, r := range in.Rooms {
		roomIDs[i] = int32(r.ID)
		roomTypes[i] = assignType(r.RoomType)
	}

	cts := make([]cConstraint, maxInt(len(in.Constraints), 1))
	for i, c := range in.Constraints {
		cts[i].ctype = int32(constraintTypeCode(c.Type))
		cts[i].entityType = int32(entityTypeCode(c.EntityType))
		cts[i].entityID = int32(c.EntityID)
		if c.DayOfWeek != nil {
			cts[i].day = int32(*c.DayOfWeek)
		} else {
			cts[i].day = -1
		}
		if c.TimeslotStart != nil {
			cts[i].slotStart = int32(*c.TimeslotStart)
		} else {
			cts[i].slotStart = -1
		}
		if c.TimeslotEnd != nil {
			cts[i].slotEnd = int32(*c.TimeslotEnd)
		} else {
			cts[i].slotEnd = -1
		}
		cts[i].value = int32(c.Weight)
		if c.IsHard {
			cts[i].isHard = 1
		} else {
			cts[i].isHard = 0
		}
	}

	workers := int32(parallelism)
	if workers <= 0 {
		workers = int32(runtime.NumCPU())
	}
	tlim := int32(timeout.Milliseconds())

	// Выходные буферы выделяет Go, C в них только пишет: нет ручного free
	// и нет конверсии uintptr → unsafe.Pointer (результат из DLL).
	outLesson := make([]int32, occCount)
	outClass := make([]int32, occCount)
	outTeacher := make([]int32, occCount)
	outSubject := make([]int32, occCount)
	outRoom := make([]int32, occCount)
	outDay := make([]int32, occCount)
	outSlot := make([]int32, occCount)
	outCount := int32(0)

	r1, _, _ := d.solve.Call(
		uintptr(nl),
		uintptr(unsafe.Pointer(&hours[0])),
		uintptr(unsafe.Pointer(&lclass[0])),
		uintptr(unsafe.Pointer(&lteacher[0])),
		uintptr(unsafe.Pointer(&lsubject[0])),
		uintptr(unsafe.Pointer(&lreq[0])),
		uintptr(nr),
		uintptr(unsafe.Pointer(&roomIDs[0])),
		uintptr(unsafe.Pointer(&roomTypes[0])),
		uintptr(days),
		uintptr(slots),
		uintptr(len(in.Constraints)),
		uintptr(unsafe.Pointer(&cts[0])),
		uintptr(tlim),
		uintptr(workers),
		uintptr(unsafe.Pointer(&outLesson[0])),
		uintptr(unsafe.Pointer(&outClass[0])),
		uintptr(unsafe.Pointer(&outTeacher[0])),
		uintptr(unsafe.Pointer(&outSubject[0])),
		uintptr(unsafe.Pointer(&outRoom[0])),
		uintptr(unsafe.Pointer(&outDay[0])),
		uintptr(unsafe.Pointer(&outSlot[0])),
		uintptr(occCount),
		uintptr(unsafe.Pointer(&outCount)),
	)
	runtime.KeepAlive(outLesson)
	if r1 == 0 {
		return Result{}, false
	}

	n := int(outCount)
	if n > occCount {
		n = occCount
	}
	entries := make([]domain.ScheduleEntry, 0, n)
	placed := 0
	for i := 0; i < n; i++ {
		if outDay[i] < 0 {
			continue // урок не удалось разместить (перегруженная школа)
		}
		placed++
		// bind.cpp отдаёт ИНДЕКС урока в in.Lessons. Проверка границ
		// обязательна: без неё неожиданное значение из DLL валит exe
		// паникой (index out of range) вместо пропуска записи.
		li := int(outLesson[i])
		if li < 0 || li >= nl {
			continue
		}
		// When no rooms are defined the solver outputs room_id=0 which
		// does not exist in the database — skip to avoid FK constraint error.
		if len(in.Rooms) == 0 && outRoom[i] == 0 {
			continue
		}
		entries = append(entries, domain.ScheduleEntry{
			SchoolID:  in.SchoolID,
			LessonID:  in.Lessons[li].ID,
			ClassID:   int(outClass[i]),
			TeacherID: int(outTeacher[i]),
			SubjectID: int(outSubject[i]),
			RoomID:    int(outRoom[i]),
			DayOfWeek: int(outDay[i]),
			Timeslot:  int(outSlot[i]),
		})
	}
	return Result{Entries: entries, Placed: placed, Total: n, Violations: 0}, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
