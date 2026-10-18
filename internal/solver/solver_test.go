package solver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"timetable/internal/domain"
)

func TestSolveNoConflicts(t *testing.T) {
	teachers := map[int]domain.Teacher{
		1: {ID: 1, Name: "Иванов"},
		2: {ID: 2, Name: "Петрова"},
	}
	classes := map[int]domain.SchoolClass{
		1: {ID: 1, Name: "10А"},
		2: {ID: 2, Name: "11Б"},
	}
	subjects := map[int]domain.Subject{
		1: {ID: 1, Name: "Математика"},
		2: {ID: 2, Name: "Физика"},
	}
	rooms := []domain.Room{
		{ID: 1, Name: "301", RoomType: "any"},
		{ID: 2, Name: "302", RoomType: "any"},
	}
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 1, SubjectID: 1, TeacherID: 1, HoursPerWeek: 5},
		{ID: 2, ClassID: 1, SubjectID: 2, TeacherID: 2, HoursPerWeek: 3},
		{ID: 3, ClassID: 2, SubjectID: 1, TeacherID: 1, HoursPerWeek: 4},
		{ID: 4, ClassID: 2, SubjectID: 2, TeacherID: 2, HoursPerWeek: 3},
	}
	cfg := domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 8}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects, Constraints: nil, Config: cfg,
	}
	res := Solve(context.Background(), in, 4, 20*time.Second)
	if res.Placed != res.Total {
		t.Fatalf("not all lessons placed: %d/%d", res.Placed, res.Total)
	}
	// verify no hard conflicts
	teacherBusy := map[int]map[int]bool{}
	classBusy := map[int]map[int]bool{}
	roomBusy := map[int]map[int]bool{}
	for _, e := range res.Entries {
		key := e.DayOfWeek*1000 + e.Timeslot
		if teacherBusy[e.TeacherID] == nil {
			teacherBusy[e.TeacherID] = map[int]bool{}
		}
		if classBusy[e.ClassID] == nil {
			classBusy[e.ClassID] = map[int]bool{}
		}
		if roomBusy[e.RoomID] == nil {
			roomBusy[e.RoomID] = map[int]bool{}
		}
		if teacherBusy[e.TeacherID][key] {
			t.Fatalf("teacher %d double-booked at %d", e.TeacherID, key)
		}
		if classBusy[e.ClassID][key] {
			t.Fatalf("class %d double-booked at %d", e.ClassID, key)
		}
		if roomBusy[e.RoomID][key] {
			t.Fatalf("room %d double-booked at %d", e.RoomID, key)
		}
		teacherBusy[e.TeacherID][key] = true
		classBusy[e.ClassID][key] = true
		roomBusy[e.RoomID][key] = true
	}
	t.Logf("placed %d/%d, soft violations %d", res.Placed, res.Total, res.Violations)
}

// TestSolvePreciseFallback verifies that SolvePrecise returns a valid result even when the
// OR-Tools build tag is not compiled in (preciseSolver == nil -> pure-Go fallback).
func TestSolvePreciseFallback(t *testing.T) {
	teachers := map[int]domain.Teacher{1: {ID: 1, Name: "Иванов"}}
	classes := map[int]domain.SchoolClass{1: {ID: 1, Name: "10А"}}
	subjects := map[int]domain.Subject{1: {ID: 1, Name: "Математика"}}
	rooms := []domain.Room{{ID: 1, Name: "301"}}
	lessons := []domain.Lesson{{ID: 1, ClassID: 1, SubjectID: 1, TeacherID: 1, HoursPerWeek: 5}}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects, Config: domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 8},
	}
	res := SolvePrecise(context.Background(), in, 2, 20*time.Second)
	if res.Placed != res.Total {
		t.Fatalf("SolvePrecise fallback placed %d/%d", res.Placed, res.Total)
	}
}

// TestSolveDaysMask verifies the school-day checkboxes (DaysMask): a mask
// without Wednesday must keep every placed entry off day 2 while still
// placing everything.
func TestSolveDaysMask(t *testing.T) {
	teachers := map[int]domain.Teacher{1: {ID: 1, Name: "Иванов"}, 2: {ID: 2, Name: "Петрова"}}
	classes := map[int]domain.SchoolClass{1: {ID: 1, Name: "10А"}, 2: {ID: 2, Name: "11Б"}}
	subjects := map[int]domain.Subject{1: {ID: 1, Name: "Математика"}, 2: {ID: 2, Name: "Физика"}}
	rooms := []domain.Room{{ID: 1, Name: "301"}, {ID: 2, Name: "302"}}
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 1, SubjectID: 1, TeacherID: 1, HoursPerWeek: 5},
		{ID: 2, ClassID: 1, SubjectID: 2, TeacherID: 2, HoursPerWeek: 3},
		{ID: 3, ClassID: 2, SubjectID: 1, TeacherID: 1, HoursPerWeek: 4},
		{ID: 4, ClassID: 2, SubjectID: 2, TeacherID: 2, HoursPerWeek: 3},
	}
	// 6-day grid, Wednesday (day 2) unchecked: Пн, Вт, Чт, Пт, Сб.
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects,
		Config: domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 8, DaysMask: 0b111011},
	}
	res := Solve(context.Background(), in, 4, 20*time.Second)
	if res.Placed != res.Total {
		t.Fatalf("not all lessons placed: %d/%d", res.Placed, res.Total)
	}
	for _, e := range res.Entries {
		if e.DayOfWeek == 2 {
			t.Fatalf("entry %d placed on masked-off Wednesday", e.ID)
		}
		if e.DayOfWeek > 5 {
			t.Fatalf("entry %d placed outside the 6-day grid: day %d", e.ID, e.DayOfWeek)
		}
	}
}

// TestPrepareDaysMask pins the mask plumbing itself: synthetic hard
// constraints for every inactive day, original slice untouched.
func TestPrepareDaysMask(t *testing.T) {
	lessons := []domain.Lesson{{ID: 1, ClassID: 7, SubjectID: 1, TeacherID: 1, HoursPerWeek: 2}}
	in := SolveInput{
		Lessons: lessons,
		Config:  domain.SchedulingConfig{DaysPerWeek: 5, SlotsPerDay: 6, DaysMask: 0b10001}, // Пн + Пт
	}
	if len(in.Constraints) != 0 {
		t.Fatal("precondition: no constraints")
	}
	out, days := prepareDaysMask(in)
	if days != 5 {
		t.Fatalf("days = %d, want 5 (highest active day index + 1)", days)
	}
	if len(out.Constraints) != 3 { // Вт, Ср, Чт forbidden
		t.Fatalf("constraints = %d, want 3", len(out.Constraints))
	}
	for _, c := range out.Constraints {
		if c.Type != "class_unavailable" || c.EntityID != 7 || !c.IsHard || c.DayOfWeek == nil {
			t.Fatalf("unexpected synthetic constraint: %+v", c)
		}
		if *c.DayOfWeek == 0 || *c.DayOfWeek == 4 {
			t.Fatalf("active day %d must not be forbidden", *c.DayOfWeek)
		}
	}
	// Zero mask = legacy behaviour, nothing injected.
	out0, days0 := prepareDaysMask(SolveInput{Lessons: lessons, Config: domain.SchedulingConfig{DaysPerWeek: 6}})
	if len(out0.Constraints) != 0 || days0 != 6 {
		t.Fatalf("zero mask should be a no-op, got %d constraints, days %d", len(out0.Constraints), days0)
	}
}

// TestSubgroupsParallel — ядро логики подгрупп: две подгруппы одного
// класса МОГУТ идти параллельно с двумя разными учителями, а урок
// родительского класса не может пересекаться ни с одной подгруппой.
func TestSubgroupsParallel(t *testing.T) {
	teachers := map[int]domain.Teacher{
		1: {ID: 1, Name: "У1"}, 2: {ID: 2, Name: "У2"}, 3: {ID: 3, Name: "У3"},
	}
	classes := map[int]domain.SchoolClass{
		1: {ID: 1, Name: "5А"},
		2: {ID: 2, Name: "5А-гр1", SubgroupOf: intPtr(1)},
		3: {ID: 3, Name: "5А-гр2", SubgroupOf: intPtr(1)},
	}
	subjects := map[int]domain.Subject{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}}
	rooms := []domain.Room{{ID: 1, Name: "101"}, {ID: 2, Name: "301"}}
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 2, SubjectID: 1, TeacherID: 1, HoursPerWeek: 1},
		{ID: 2, ClassID: 3, SubjectID: 2, TeacherID: 2, HoursPerWeek: 1},
		{ID: 3, ClassID: 1, SubjectID: 3, TeacherID: 3, HoursPerWeek: 1},
	}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects,
		Config: domain.SchedulingConfig{DaysPerWeek: 2, SlotsPerDay: 4},
		Constraints: []domain.Constraint{
			// У1 и У2 свободны ТОЛЬКО в (0,0) — их подгруппы обязаны встать параллельно
			{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 1, DayOfWeek: intPtr(0), TimeslotStart: intPtr(1), TimeslotEnd: intPtr(3), IsHard: true},
			{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 1, DayOfWeek: intPtr(1), TimeslotStart: intPtr(0), TimeslotEnd: intPtr(3), IsHard: true},
			{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 2, DayOfWeek: intPtr(0), TimeslotStart: intPtr(1), TimeslotEnd: intPtr(3), IsHard: true},
			{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 2, DayOfWeek: intPtr(1), TimeslotStart: intPtr(0), TimeslotEnd: intPtr(3), IsHard: true},
		},
	}
	res := Solve(context.Background(), in, 2, 15*time.Second)
	if res.Placed != 3 {
		t.Fatalf("placed %d/%d", res.Placed, res.Total)
	}
	pos := map[int][2]int{}
	for _, e := range res.Entries {
		pos[e.LessonID] = [2]int{e.DayOfWeek, e.Timeslot}
	}
	l1, l2, l3 := pos[1], pos[2], pos[3]
	if l1 != l2 {
		t.Fatalf("подгруппы не параллельны: гр1=%v гр2=%v", l1, l2)
	}
	if l3 == l1 {
		t.Fatalf("урок родителя пересёкся с подгруппой: %v", l3)
	}
}

func intPtr(v int) *int { return &v }

// synthSchool строит синтетическую школу (только в памяти, без БД):
// nClasses классов, 8 предметов, ~nClasses учителей, 12 кабинетов.
// У каждого класса 7 предметов по 3-5 часов — ~30 часов в неделю.
func synthSchool(nClasses int) SolveInput {
	teachers := map[int]domain.Teacher{}
	for i := 1; i <= nClasses; i++ {
		teachers[i] = domain.Teacher{ID: i, Name: fmt.Sprintf("Учитель %d", i), MaxHoursPerWeek: 34}
	}
	subjects := map[int]domain.Subject{}
	subjNames := []string{"Математика", "Физика", "Химия", "Литература", "История", "Английский", "Информатика", "Физкультура"}
	for i, n := range subjNames {
		subjects[i+1] = domain.Subject{ID: i + 1, Name: n}
	}
	classes := map[int]domain.SchoolClass{}
	for i := 1; i <= nClasses; i++ {
		classes[i] = domain.SchoolClass{ID: i, Name: fmt.Sprintf("%dА", i)}
	}
	rooms := []domain.Room{}
	for i := 1; i <= nClasses*4/5; i++ {
		rooms = append(rooms, domain.Room{ID: i, Name: fmt.Sprintf("%d", 100+i)})
	}
	lessons := make([]domain.Lesson, 0, nClasses*7)
	id := 1
	for c := 1; c <= nClasses; c++ {
		for s := 1; s <= 7; s++ {
			h := 3 + (s+c)%3 // 3..5 часов
			lessons = append(lessons, domain.Lesson{
				ID: id, ClassID: c, SubjectID: s, TeacherID: c, HoursPerWeek: h,
			})
			id++
		}
	}
	return SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects,
		Config: domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 7},
	}
}

// TestSolveLargeSchool — крупная школа должна размещаться полностью за
// разумное время; это и регрессия на полноту, и ориентир для оптимизаций
// перебора (горячий цикл которого переписан на предвычисленные кандидаты).
func TestSolveLargeSchool(t *testing.T) {
	if testing.Short() {
		t.Skip("long-running test")
	}
	in := synthSchool(20)
	res := Solve(context.Background(), in, 4, 10*time.Second)
	if res.Total == 0 {
		t.Fatal("no occurrences built")
	}
	if res.Placed != res.Total {
		t.Fatalf("large school not fully placed: %d/%d", res.Placed, res.Total)
	}
}

func BenchmarkSolve20Classes(b *testing.B) {
	in := synthSchool(20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(context.Background(), in, 4, 5*time.Second)
	}
}

func BenchmarkSolve10Classes(b *testing.B) {
	in := synthSchool(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(context.Background(), in, 4, 2*time.Second)
	}
}

// TestSolveImpossibleLessonSkippedNotZeroed: учитель, закрытый жёстким
// «недоступен» на всю неделю, раньше обнулял ВСЮ генерацию (0 из N),
// хотя остальные уроки расставляются без проблем. Теперь невозможные
// вхождения пропускаются, остальная школа строится.
func TestSolveImpossibleLessonSkippedNotZeroed(t *testing.T) {
	teachers := map[int]domain.Teacher{
		1: {ID: 1, Name: "Иванов"},
		2: {ID: 2, Name: "Петрова"},
	}
	classes := map[int]domain.SchoolClass{
		1: {ID: 1, Name: "5Г"},
		2: {ID: 2, Name: "5Д"},
		3: {ID: 3, Name: "10А"},
	}
	subjects := map[int]domain.Subject{
		1: {ID: 1, Name: "Государ.язык"},
		2: {ID: 2, Name: "Математика"},
	}
	rooms := []domain.Room{{ID: 1, Name: "201", RoomType: "any"}}
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 1, SubjectID: 1, TeacherID: 1, HoursPerWeek: 2},
		{ID: 2, ClassID: 2, SubjectID: 1, TeacherID: 1, HoursPerWeek: 2},
		{ID: 3, ClassID: 3, SubjectID: 1, TeacherID: 1, HoursPerWeek: 2},
		{ID: 4, ClassID: 1, SubjectID: 2, TeacherID: 2, HoursPerWeek: 2},
		{ID: 5, ClassID: 2, SubjectID: 2, TeacherID: 2, HoursPerWeek: 2},
		{ID: 6, ClassID: 3, SubjectID: 2, TeacherID: 2, HoursPerWeek: 2},
	}
	bannedAllWeek := domain.Constraint{
		ID: 1, SchoolID: 1, Type: "teacher_unavailable",
		EntityType: "teacher", EntityID: 1, IsHard: true,
	}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects,
		Constraints: []domain.Constraint{bannedAllWeek},
		Config:      domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 8},
	}
	res := Solve(context.Background(), in, 2, 10*time.Second)
	if res.Total != 12 {
		t.Fatalf("total = %d, want 12", res.Total)
	}
	if res.Placed != 6 {
		t.Fatalf("placed = %d, want 6: математика обязана расставиться, а не обнулять всю школу", res.Placed)
	}
	for _, e := range res.Entries {
		if e.TeacherID == 1 {
			t.Fatalf("запрещённый всю неделю учитель размещён: %+v", e)
		}
	}
}

// TestSolveDividedFamilySkippedWhenImpossible: если статически невозможна
// любая часть делёной пары (база или половинка), пропускается вся семья —
// одиночная половинка давала бы конфликт «половинка без пары».
func TestSolveDividedFamilySkippedWhenImpossible(t *testing.T) {
	parent := 10
	pPtr := &parent
	teachers := map[int]domain.Teacher{
		3: {ID: 3, Name: "Сидоров"}, // закрыт всю неделю — база семьи
		4: {ID: 4, Name: "Кузнецова"},
		2: {ID: 2, Name: "Петрова"},
	}
	classes := map[int]domain.SchoolClass{
		10: {ID: 10, Name: "9А"},
		11: {ID: 11, Name: "9А-1", SubgroupOf: pPtr},
		12: {ID: 12, Name: "9А-2", SubgroupOf: pPtr},
	}
	subjects := map[int]domain.Subject{
		1: {ID: 1, Name: "Ин.яз"},
		2: {ID: 2, Name: "Математика"},
	}
	rooms := []domain.Room{{ID: 1, Name: "201", RoomType: "any"}}
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 11, SubjectID: 1, TeacherID: 3, HoursPerWeek: 1}, // база (первая с ключом 10:1)
		{ID: 2, ClassID: 12, SubjectID: 1, TeacherID: 4, HoursPerWeek: 1}, // половинка
		{ID: 3, ClassID: 11, SubjectID: 2, TeacherID: 2, HoursPerWeek: 1}, // обычный урок — должен встать
	}
	bannedAllWeek := domain.Constraint{
		ID: 1, SchoolID: 1, Type: "teacher_unavailable",
		EntityType: "teacher", EntityID: 3, IsHard: true,
	}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects,
		Constraints: []domain.Constraint{bannedAllWeek},
		Config:      domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 8},
	}
	res := Solve(context.Background(), in, 2, 10*time.Second)
	if res.Total != 3 {
		t.Fatalf("total = %d, want 3", res.Total)
	}
	if res.Placed != 1 {
		t.Fatalf("placed = %d, want 1: только математика; семья делёного урока обязана пропуститься целиком", res.Placed)
	}
	if len(res.Entries) != 1 || res.Entries[0].LessonID != 3 {
		t.Fatalf("ожидалась только математика (lesson 3), получено: %+v", res.Entries)
	}
}

// TestSolveNoRoomsPlacesNothing: школа без кабинетов — ставить некуда,
// но солвер не падает и не зацикливается: 0 размещено, total честный.
func TestSolveNoRoomsPlacesNothing(t *testing.T) {
	teachers := map[int]domain.Teacher{1: {ID: 1, Name: "Иванов"}}
	classes := map[int]domain.SchoolClass{1: {ID: 1, Name: "5Г"}}
	subjects := map[int]domain.Subject{1: {ID: 1, Name: "Математика"}}
	lessons := []domain.Lesson{{ID: 1, ClassID: 1, SubjectID: 1, TeacherID: 1, HoursPerWeek: 2}}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: nil, Subjects: subjects,
		Config: domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 8},
	}
	res := Solve(context.Background(), in, 2, 5*time.Second)
	if res.Total != 2 {
		t.Fatalf("total = %d, want 2", res.Total)
	}
	if res.Placed != 0 {
		t.Fatalf("placed = %d, want 0 (кабинетов нет)", res.Placed)
	}
}
