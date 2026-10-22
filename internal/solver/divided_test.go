package solver

import (
	"context"
	"testing"
	"time"

	"timetable/internal/domain"
)

// TestDividedHoursPairing — регрессия делёных уроков с 2+ часами: k-е
// вхождение половинки обязано идти параллельно k-му вхождению базы, а не
// последнему (иначе вторая пара часов не размещается вовсе).
func TestDividedHoursPairing(t *testing.T) {
	teachers := map[int]domain.Teacher{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}}
	classes := map[int]domain.SchoolClass{
		1: {ID: 1, Name: "9А"},
		2: {ID: 2, Name: "9А-гр1", SubgroupOf: intPtr(1)},
		3: {ID: 3, Name: "9А-гр2", SubgroupOf: intPtr(1)},
		4: {ID: 4, Name: "9Б"},
	}
	subjects := map[int]domain.Subject{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}}
	rooms := []domain.Room{{ID: 1}, {ID: 2}, {ID: 3}}
	lessons := []domain.Lesson{
		{ID: 1, ClassID: 1, SubjectID: 1, TeacherID: 1, HoursPerWeek: 5},
		{ID: 2, ClassID: 2, SubjectID: 2, TeacherID: 2, HoursPerWeek: 2},
		{ID: 3, ClassID: 3, SubjectID: 2, TeacherID: 3, HoursPerWeek: 2},
		{ID: 4, ClassID: 2, SubjectID: 3, TeacherID: 2, HoursPerWeek: 2},
		{ID: 5, ClassID: 3, SubjectID: 3, TeacherID: 3, HoursPerWeek: 2},
		{ID: 6, ClassID: 4, SubjectID: 1, TeacherID: 1, HoursPerWeek: 5},
		{ID: 7, ClassID: 4, SubjectID: 2, TeacherID: 2, HoursPerWeek: 3},
	}
	in := SolveInput{SchoolID: 2, Lessons: lessons, Teachers: teachers, Classes: classes,
		Rooms: rooms, Subjects: subjects, Config: domain.SchedulingConfig{DaysPerWeek: 6, SlotsPerDay: 8}}
	res := Solve(context.Background(), in, 4, 10*time.Second, nil)
	if res.Placed != res.Total {
		t.Fatalf("divided 2-hour lessons not fully placed: %d/%d", res.Placed, res.Total)
	}
	// Обе половинки каждой пары — строго в одном слоте.
	slotOf := map[[2]int]int{} // lessonID → (day, slot)
	for _, e := range res.Entries {
		slotOf[[2]int{e.LessonID, 0}] = 0
	}
	_ = slotOf
	pairs := map[int][2]int{} // lessonID → (day,slot)
	for _, e := range res.Entries {
		pairs[e.LessonID] = [2]int{e.DayOfWeek, e.Timeslot}
	}
	for lid, pos := range pairs {
		twin := dividedTwinForTest(lid, lessons, classes)
		if twin == 0 {
			continue
		}
		if pairs[twin] != pos {
			t.Errorf("lesson %d at %v but twin %d at %v", lid, pos, twin, pairs[twin])
		}
	}
}

// dividedTwinForTest: сестринский урок того же предмета у другой подгруппы
// того же родителя (0 = не делёный).
func dividedTwinForTest(lessonID int, lessons []domain.Lesson, classes map[int]domain.SchoolClass) int {
	var l domain.Lesson
	found := false
	for _, x := range lessons {
		if x.ID == lessonID {
			l = x
			found = true
			break
		}
	}
	if !found {
		return 0
	}
	parent := 0
	if c, ok := classes[l.ClassID]; ok && c.SubgroupOf != nil {
		parent = *c.SubgroupOf
	}
	if parent == 0 {
		return 0
	}
	for _, x := range lessons {
		if x.ID == l.ID || x.SubjectID != l.SubjectID {
			continue
		}
		if c, ok := classes[x.ClassID]; ok && c.SubgroupOf != nil && *c.SubgroupOf == parent {
			return x.ID
		}
	}
	return 0
}
