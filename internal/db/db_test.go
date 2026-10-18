package db

import (
	"path/filepath"
	"testing"

	"timetable/internal/domain"
)

func TestStoreCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := s.CreateSchool("Тест")
	if err != nil {
		t.Fatal(err)
	}
	sid := sc.ID

	te, _ := s.CreateTeacher(domain.Teacher{SchoolID: sid, Name: "Иванов", MaxHoursPerWeek: 30})
	if _, err := s.CreateSubject(domain.Subject{SchoolID: sid, Name: "Математика"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateClass(domain.SchoolClass{SchoolID: sid, Name: "10А"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRoom(domain.Room{SchoolID: sid, Name: "301"}); err != nil {
		t.Fatal(err)
	}
	l, err := s.CreateLesson(domain.Lesson{SchoolID: sid, ClassID: 1, SubjectID: 1, TeacherID: te.ID, HoursPerWeek: 5})
	if err != nil {
		t.Fatal(err)
	}
	ts, _ := s.ListTeachers(sid)
	if len(ts) != 1 {
		t.Fatalf("expected 1 teacher, got %d", len(ts))
	}
	ls, lerr := s.ListLessons(sid)
	if lerr != nil {
		t.Fatalf("ListLessons error: %v", lerr)
	}
	if len(ls) != 1 || ls[0].HoursPerWeek != 5 {
		t.Fatalf("lesson not persisted correctly: %+v (err=%v)", ls, lerr)
	}
	if err := s.SaveSchedule([]domain.ScheduleEntry{
		{SchoolID: sid, LessonID: l.ID, ClassID: 1, TeacherID: te.ID, SubjectID: 1, RoomID: 1, DayOfWeek: 0, Timeslot: 0},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.ListSchedule(sid)
	if len(got) != 1 {
		t.Fatalf("expected 1 schedule entry, got %d", len(got))
	}

	// MoveEntry relocates a single entry after a manual edit.
	if err := s.MoveEntry(got[0].ID, 2, 3); err != nil {
		t.Fatal(err)
	}
	moved, _ := s.ListSchedule(sid)
	if len(moved) != 1 || moved[0].DayOfWeek != 2 || moved[0].Timeslot != 3 {
		t.Fatalf("MoveEntry failed: %+v", moved)
	}

	// ReplaceSchedule clears then writes the new set atomically.
	if err := s.ReplaceSchedule(sid, []domain.ScheduleEntry{
		{SchoolID: sid, LessonID: l.ID, ClassID: 1, TeacherID: te.ID, SubjectID: 1, RoomID: 1, DayOfWeek: 1, Timeslot: 1},
		{SchoolID: sid, LessonID: l.ID, ClassID: 1, TeacherID: te.ID, SubjectID: 1, RoomID: 1, DayOfWeek: 4, Timeslot: 6},
	}); err != nil {
		t.Fatal(err)
	}
	repl, _ := s.ListSchedule(sid)
	if len(repl) != 2 {
		t.Fatalf("ReplaceSchedule expected 2 entries, got %d", len(repl))
	}
}

// TestSwapEntries verifies that two schedule entries actually exchange their
// (day, slot) coordinates when SwapEntries is called with the "new target
// position" semantics used by the frontend (id1 -> (day1,slot1), id2 ->
// (day2,slot2)). This is a regression test for a bug where the SQL UPDATE
// statements applied the new positions to the wrong rows, making the swap a
// silent no-op.
func TestSwapEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := s.CreateSchool("Тест Swap")
	if err != nil {
		t.Fatal(err)
	}
	sid := sc.ID

	te, _ := s.CreateTeacher(domain.Teacher{SchoolID: sid, Name: "Иванов", MaxHoursPerWeek: 30})
	s.CreateSubject(domain.Subject{SchoolID: sid, Name: "Математика"})
	s.CreateClass(domain.SchoolClass{SchoolID: sid, Name: "10А"})
	s.CreateRoom(domain.Room{SchoolID: sid, Name: "301"})
	lesson, err := s.CreateLesson(domain.Lesson{SchoolID: sid, ClassID: 1, SubjectID: 1, TeacherID: te.ID, HoursPerWeek: 5})
	if err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}

	// Two entries at distinct (day, slot) coordinates.
	//   A currently at (1, 2)  -- Monday period 3
	//   B currently at (3, 4)  -- Wednesday period 5
	if err := s.SaveSchedule([]domain.ScheduleEntry{
		{SchoolID: sid, LessonID: lesson.ID, ClassID: 1, TeacherID: te.ID, SubjectID: 1, RoomID: 1, DayOfWeek: 1, Timeslot: 2},
		{SchoolID: sid, LessonID: lesson.ID, ClassID: 1, TeacherID: te.ID, SubjectID: 1, RoomID: 1, DayOfWeek: 3, Timeslot: 4},
	}); err != nil {
		t.Fatalf("SaveSchedule: %v", err)
	}
	got, _ := s.ListSchedule(sid)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	idA, idB := got[0].ID, got[1].ID
	// Make sure we know which is which by their starting position.
	if got[0].DayOfWeek == 3 && got[0].Timeslot == 4 {
		idA, idB = got[1].ID, got[0].ID
	}

	// Frontend semantics: id1 (=A) should move to (3,4); id2 (=B) should
	// move to (1,2). The arguments are the *target* positions.
	if err := s.SwapEntries(idA, 3, 4, idB, 1, 2); err != nil {
		t.Fatalf("SwapEntries: %v", err)
	}
	after, _ := s.ListSchedule(sid)
	if len(after) != 2 {
		t.Fatalf("expected 2 entries after swap, got %d", len(after))
	}
	for _, e := range after {
		if e.ID == idA {
			if e.DayOfWeek != 3 || e.Timeslot != 4 {
				t.Errorf("entry A should be at (3,4) after swap, got (%d,%d)", e.DayOfWeek, e.Timeslot)
			}
		}
		if e.ID == idB {
			if e.DayOfWeek != 1 || e.Timeslot != 2 {
				t.Errorf("entry B should be at (1,2) after swap, got (%d,%d)", e.DayOfWeek, e.Timeslot)
			}
		}
	}
}
