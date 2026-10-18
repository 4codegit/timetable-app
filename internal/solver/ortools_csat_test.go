//go:build ortools && !windows

package solver

import (
	"testing"
	"time"

	"timetable/internal/domain"
)

// TestOrtoolsSubgroupsShareRoom — ядро семейной логики уникальности
// кабинета (bind.cpp): две подгруппы одного родителя МОГУТ идти в одном
// кабинете параллельно (половины класса), урок родительского класса —
// не может делить кабинет ни с подгруппой, ни с посторонним классом.
// Учителя половинок свободны ТОЛЬКО в (0,0) — при единственном кабинете
// расписание существует, только если половинки делят его законно.
// Регрессия на семейные бакеты вместо квадратичного перебора пар:
// группировка обязана сохранить разрешённое деление внутри семьи.
func TestOrtoolsSubgroupsShareRoom(t *testing.T) {
	teachers := map[int]domain.Teacher{
		1: {ID: 1, Name: "У1"}, 2: {ID: 2, Name: "У2"}, 3: {ID: 3, Name: "У3"},
	}
	classes := map[int]domain.SchoolClass{
		1: {ID: 1, Name: "5А"},
		2: {ID: 2, Name: "5А-гр1", SubgroupOf: intPtr(1)},
		3: {ID: 3, Name: "5А-гр2", SubgroupOf: intPtr(1)},
	}
	subjects := map[int]domain.Subject{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}}
	rooms := []domain.Room{{ID: 1, Name: "101"}} // один кабинет на всех
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 2, SubjectID: 1, TeacherID: 1, HoursPerWeek: 1},
		{ID: 2, ClassID: 3, SubjectID: 2, TeacherID: 2, HoursPerWeek: 1},
		{ID: 3, ClassID: 1, SubjectID: 3, TeacherID: 3, HoursPerWeek: 1},
	}
	// У1 и У2 свободны только в (0,0): их уроки обязаны встать параллельно
	// — а значит, в ОДНОМ кабинете (другого нет).
	unavail := []domain.Constraint{
		{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 1, DayOfWeek: intPtr(0), TimeslotStart: intPtr(1), TimeslotEnd: intPtr(3), IsHard: true},
		{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 1, DayOfWeek: intPtr(1), TimeslotStart: intPtr(0), TimeslotEnd: intPtr(3), IsHard: true},
		{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 2, DayOfWeek: intPtr(0), TimeslotStart: intPtr(1), TimeslotEnd: intPtr(3), IsHard: true},
		{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 2, DayOfWeek: intPtr(1), TimeslotStart: intPtr(0), TimeslotEnd: intPtr(3), IsHard: true},
	}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects, Constraints: unavail,
		Config: domain.SchedulingConfig{DaysPerWeek: 2, SlotsPerDay: 4},
	}
	res, ok := ortoolsSolve(in, 4, 30*time.Second)
	if !ok {
		t.Fatal("ortoolsSolve failed")
	}
	if res.Placed != res.Total || res.Total != 3 {
		t.Fatalf("placed %d/%d — половинки не поделили кабинет (семейная логика сломана)", res.Placed, res.Total)
	}
	pos := map[int][2]int{}
	for _, e := range res.Entries {
		pos[e.LessonID] = [2]int{e.DayOfWeek, e.Timeslot}
	}
	if pos[1] != pos[2] {
		t.Fatalf("подгруппы не параллельны: гр1=%v гр2=%v", pos[1], pos[2])
	}
	if pos[3] == pos[1] {
		t.Fatal("урок родителя делит кабинет с подгруппой — семьи слиплись")
	}
}

// TestOrtoolsForeignClassesNoShare — посторонние классы (разные семьи)
// не делят кабинет.
func TestOrtoolsForeignClassesNoShare(t *testing.T) {
	teachers := map[int]domain.Teacher{1: {ID: 1}, 2: {ID: 2}}
	classes := map[int]domain.SchoolClass{1: {ID: 1, Name: "5А"}, 2: {ID: 2, Name: "6Б"}}
	subjects := map[int]domain.Subject{1: {ID: 1}}
	rooms := []domain.Room{{ID: 1, Name: "101"}}
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 1, SubjectID: 1, TeacherID: 1, HoursPerWeek: 2},
		{ID: 2, ClassID: 2, SubjectID: 1, TeacherID: 2, HoursPerWeek: 2},
	}
	// Оба учителя свободны только в двух слотах дня 0 — 4 урока в 2 слотах
	// с одним кабинетом без деления: разместить нельзя (по 2 урока каждого
	// учителя на 2 слота — в каждом слоте ОБА урока, а кабинет один).
	unavail := []domain.Constraint{
		{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 1, DayOfWeek: intPtr(1), TimeslotStart: intPtr(0), TimeslotEnd: intPtr(1), IsHard: true},
		{Type: "teacher_unavailable", EntityType: "teacher", EntityID: 2, DayOfWeek: intPtr(1), TimeslotStart: intPtr(0), TimeslotEnd: intPtr(1), IsHard: true},
	}
	in := SolveInput{
		SchoolID: 1, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects, Constraints: unavail,
		Config: domain.SchedulingConfig{DaysPerWeek: 2, SlotsPerDay: 2},
	}
	res, ok := ortoolsSolve(in, 4, 30*time.Second)
	if !ok {
		t.Fatal("ortoolsSolve failed")
	}
	if res.Placed >= res.Total {
		t.Fatalf("foreign classes shared the room: placed %d/%d — должно быть < total", res.Placed, res.Total)
	}
}

// BenchmarkOrtools20Classes — модель большой школы (560 вхождений):
// старый попарный цикл уникальности кабинетов давал O(D·S·R·O²)
// констрейнтов, семейные бакеты — линейно.
func BenchmarkOrtools20Classes(b *testing.B) {
	in := synthSchool(20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ortoolsSolve(in, 4, 20*time.Second)
	}
}

func BenchmarkOrtools10Classes(b *testing.B) {
	in := synthSchool(10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ortoolsSolve(in, 4, 20*time.Second)
	}
}
