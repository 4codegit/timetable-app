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

	te, _ := s.CreateTeacher(domain.Teacher{SchoolID: sid, Name: "Иванов"})
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

	te, _ := s.CreateTeacher(domain.Teacher{SchoolID: sid, Name: "Иванов"})
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

// TestRoomAndSubjectTypesPersisted — регрессия: orDefault проверял значения
// как JSON, и обычная строка "lab" молча заменялась на "any". Из-за этого
// кабинеты никогда не имели свой тип, а фильтр RequiresRoomType у предметов
// всегда вырождался в «любой кабинет».
func TestRoomAndSubjectTypesPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := s.CreateSchool("Типы")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRoom(domain.Room{SchoolID: sc.ID, Name: "Лаборатория", RoomType: "lab"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSubject(domain.Subject{SchoolID: sc.ID, Name: "Информатика", RequiresRoomType: "lab"}); err != nil {
		t.Fatal(err)
	}
	rooms, err := s.ListRooms(sc.ID)
	if err != nil || len(rooms) != 1 {
		t.Fatalf("ListRooms: %+v err=%v", rooms, err)
	}
	if rooms[0].RoomType != "lab" {
		t.Fatalf("room type not persisted: %+v", rooms[0])
	}
	subs, err := s.ListSubjects(sc.ID)
	if err != nil || len(subs) != 1 {
		t.Fatalf("ListSubjects: %+v err=%v", subs, err)
	}
	if subs[0].RequiresRoomType != "lab" {
		t.Fatalf("subject room type not persisted: %+v", subs[0])
	}
	if err := s.UpdateRoom(domain.Room{ID: rooms[0].ID, SchoolID: sc.ID, Name: rooms[0].Name, RoomType: "gym"}); err != nil {
		t.Fatal(err)
	}
	rooms, _ = s.ListRooms(sc.ID)
	if rooms[0].RoomType != "gym" {
		t.Fatalf("UpdateRoom lost the type: %+v", rooms[0])
	}
}

// TestClearSchoolDataWithHomeRoom — регрессия: список таблиц удалял rooms
// раньше classes, а classes.room_id ссылается на rooms(id) без CASCADE —
// очистка данных школы падала на FOREIGN KEY constraint failed, ломая
// повторный импорт в существующую школу.
func TestClearSchoolDataWithHomeRoom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := s.CreateSchool("С кабинетом")
	if err != nil {
		t.Fatal(err)
	}
	room, err := s.CreateRoom(domain.Room{SchoolID: sc.ID, Name: "204"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateClass(domain.SchoolClass{SchoolID: sc.ID, Name: "9А", RoomID: room.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearSchoolData(sc.ID); err != nil {
		t.Fatalf("ClearSchoolData with a home-roomed class: %v", err)
	}
	classes, _ := s.ListClasses(sc.ID)
	rooms, _ := s.ListRooms(sc.ID)
	if len(classes) != 0 || len(rooms) != 0 {
		t.Fatalf("school data not cleared: classes=%d rooms=%d", len(classes), len(rooms))
	}
}
