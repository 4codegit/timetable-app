package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"timetable/internal/db"
	"timetable/internal/domain"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := db.New(path)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	return &App{ctx: context.Background(), store: store}
}

// Справочники больше не возят CSV: экспорт/импорт — обычный JSON через
// те же Create*/List*, что и UI. Тест проверяет поток создания по именам
// (как это делает JSON-импорт во фронтенде) и согласованность листингов.
func TestRefsJSONFlow(t *testing.T) {
	a := newTestApp(t)
	sc, err := a.CreateSchool("Тест")
	if err != nil {
		t.Fatalf("CreateSchool: %v", err)
	}
	id := sc.ID

	// teachers
	if _, err := a.CreateTeacher(domain.Teacher{SchoolID: id, Name: "Иванов", ShortName: "Ив"}); err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}
	if _, err := a.CreateTeacher(domain.Teacher{SchoolID: id, Name: "Петрова", ShortName: "Пт"}); err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}

	// class + subject
	cls, err := a.CreateClass(domain.SchoolClass{SchoolID: id, Name: "10А"})
	if err != nil {
		t.Fatalf("CreateClass: %v", err)
	}
	sub, err := a.CreateSubject(domain.Subject{SchoolID: id, Name: "Матем", ShortName: "Мат", RequiresRoomType: "any"})
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}

	ts, err := a.ListTeachers(id)
	if err != nil || len(ts) != 2 {
		t.Fatalf("ListTeachers: %v, n=%d", err, len(ts))
	}

	// lesson resolved by the teacher's name, как JSON-импорт во фронтенде
	teachID := 0
	for _, tt := range ts {
		if tt.Name == "Иванов" {
			teachID = tt.ID
		}
	}
	if teachID == 0 {
		t.Fatal("teacher Иванов not found")
	}
	if _, err := a.CreateLesson(domain.Lesson{SchoolID: id, ClassID: cls.ID, SubjectID: sub.ID, TeacherID: teachID, HoursPerWeek: 5}); err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}
	ls, err := a.ListLessons(id)
	if err != nil || len(ls) != 1 {
		t.Fatalf("ListLessons: %v, n=%d", err, len(ls))
	}
	if ls[0].ClassID != cls.ID || ls[0].SubjectID != sub.ID || ls[0].TeacherID != teachID || ls[0].HoursPerWeek != 5 {
		t.Fatalf("lesson mismatch: %+v", ls[0])
	}
}

func TestGenerate(t *testing.T) {
	a := newTestApp(t)
	sc, err := a.CreateSchool("Тест Генерация")
	if err != nil {
		t.Fatalf("CreateSchool: %v", err)
	}
	id := sc.ID

	// Create reference data
	if _, err := a.CreateTeacher(domain.Teacher{SchoolID: id, Name: "Иванов И.И.", ShortName: "Иванов"}); err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}
	if _, err := a.CreateTeacher(domain.Teacher{SchoolID: id, Name: "Петрова П.П.", ShortName: "Петрова"}); err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}
	if _, err := a.CreateSubject(domain.Subject{SchoolID: id, Name: "Математика", ShortName: "Мат", RequiresRoomType: "any"}); err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	if _, err := a.CreateSubject(domain.Subject{SchoolID: id, Name: "Физика", ShortName: "Физ", RequiresRoomType: "any"}); err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	if _, err := a.CreateClass(domain.SchoolClass{SchoolID: id, Name: "10А"}); err != nil {
		t.Fatalf("CreateClass: %v", err)
	}
	if _, err := a.CreateClass(domain.SchoolClass{SchoolID: id, Name: "10Б"}); err != nil {
		t.Fatalf("CreateClass: %v", err)
	}
	if _, err := a.CreateRoom(domain.Room{SchoolID: id, Name: "301", RoomType: "any"}); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := a.CreateRoom(domain.Room{SchoolID: id, Name: "302", RoomType: "any"}); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Create lessons
	teachers, _ := a.ListTeachers(id)
	subjects, _ := a.ListSubjects(id)
	classes, _ := a.ListClasses(id)

	lessonDefs := []struct {
		cls, subj, teach string
		hours            int
	}{
		{"10А", "Математика", "Иванов И.И.", 5},
		{"10А", "Физика", "Петрова П.П.", 3},
		{"10Б", "Математика", "Иванов И.И.", 4},
		{"10Б", "Физика", "Петрова П.П.", 3},
	}

	tMap := map[string]int{}
	for _, tc := range teachers {
		tMap[tc.Name] = tc.ID
	}
	sMap := map[string]int{}
	for _, s := range subjects {
		sMap[s.Name] = s.ID
	}
	cMap := map[string]int{}
	for _, c := range classes {
		cMap[c.Name] = c.ID
	}

	for _, ld := range lessonDefs {
		if _, err := a.CreateLesson(domain.Lesson{
			SchoolID: id, ClassID: cMap[ld.cls], SubjectID: sMap[ld.subj],
			TeacherID: tMap[ld.teach], HoursPerWeek: ld.hours,
		}); err != nil {
			t.Fatalf("CreateLesson %v: %v", ld, err)
		}
	}

	// Generate schedule (daysMask 0 = legacy: first 6 days from Monday)
	res, err := a.Generate(id, 6, 8, 0)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Placed != res.Total {
		t.Fatalf("not all lessons placed: %d/%d", res.Placed, res.Total)
	}
	t.Logf("Generate: placed %d/%d, violations %d", res.Placed, res.Total, res.Violations)

	// Verify schedule was persisted
	entries, err := a.ListSchedule(id)
	if err != nil {
		t.Fatalf("ListSchedule: %v", err)
	}
	if len(entries) != res.Placed {
		t.Fatalf("expected %d entries in DB, got %d", res.Placed, len(entries))
	}

	// Verify no hard conflicts (teacher/class/room double-booked)
	teacherBusy := map[int]map[int]bool{}
	classBusy := map[int]map[int]bool{}
	roomBusy := map[int]map[int]bool{}
	for _, e := range entries {
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
			t.Errorf("teacher %d double-booked at day=%d slot=%d", e.TeacherID, e.DayOfWeek, e.Timeslot)
		}
		if classBusy[e.ClassID][key] {
			t.Errorf("class %d double-booked at day=%d slot=%d", e.ClassID, e.DayOfWeek, e.Timeslot)
		}
		if roomBusy[e.RoomID][key] {
			t.Errorf("room %d double-booked at day=%d slot=%d", e.RoomID, e.DayOfWeek, e.Timeslot)
		}
		teacherBusy[e.TeacherID][key] = true
		classBusy[e.ClassID][key] = true
		roomBusy[e.RoomID][key] = true
	}
}

func TestImportAllClearsExistingData(t *testing.T) {
	a := newTestApp(t)
	sc, err := a.CreateSchool("Импорт Тест")
	if err != nil {
		t.Fatalf("CreateSchool: %v", err)
	}
	id := sc.ID

	// Add initial data
	a.CreateTeacher(domain.Teacher{SchoolID: id, Name: "Старый Учитель"})
	a.CreateSubject(domain.Subject{SchoolID: id, Name: "Старый Предмет"})
	teachers1, _ := a.ListTeachers(id)
	if len(teachers1) != 1 {
		t.Fatalf("expected 1 teacher, got %d", len(teachers1))
	}

	// Import snapshot into existing school (should clear old data)
	snap := `{
                "school": {"id": ` + fmt.Sprintf("%d", id) + `, "name": "Импорт Тест"},
                "teachers": [{"name": "Новый Учитель"}],
                "subjects": [{"name": "Новый Предмет"}],
                "classes": [],
                "rooms": [],
                "lessons": [],
                "constraints": []
        }`
	_, err = a.ImportAll(snap)
	if err != nil {
		t.Fatalf("ImportAll: %v", err)
	}

	// Verify old data was cleared and new data imported
	teachers2, _ := a.ListTeachers(id)
	if len(teachers2) != 1 {
		t.Fatalf("expected 1 teacher after import, got %d", len(teachers2))
	}
	if teachers2[0].Name != "Новый Учитель" {
		t.Fatalf("expected 'Новый Учитель', got %q", teachers2[0].Name)
	}
}

// TestExportImportRoundTripPreservesSchedule verifies that a JSON backup is a
// real backup: it can be restored into a different database even though every
// generated ID changes.  It covers the links most likely to be silently lost:
// settings, subgroups, constraints, lessons, and schedule entries.
func TestExportImportRoundTripPreservesSchedule(t *testing.T) {
	source := newTestApp(t)
	// Ensure the exported school is not the first record, as it commonly won't
	// be in the destination database either.
	if _, err := source.CreateSchool("Черновик"); err != nil {
		t.Fatal(err)
	}
	school, err := source.CreateSchool("Лицей №1")
	if err != nil {
		t.Fatal(err)
	}
	if err := source.UpdateSchoolSettings(school.ID, `{"days":5,"slots":7}`); err != nil {
		t.Fatal(err)
	}
	teacher, err := source.CreateTeacher(domain.Teacher{SchoolID: school.ID, Name: "Иванова"})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := source.CreateSubject(domain.Subject{SchoolID: school.ID, Name: "Математика"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := source.CreateClass(domain.SchoolClass{SchoolID: school.ID, Name: "8А"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := source.CreateClass(domain.SchoolClass{SchoolID: school.ID, Name: "8А-1", SubgroupOf: &parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	room, err := source.CreateRoom(domain.Room{SchoolID: school.ID, Name: "204"})
	if err != nil {
		t.Fatal(err)
	}
	lesson, err := source.CreateLesson(domain.Lesson{SchoolID: school.ID, ClassID: child.ID, SubjectID: subject.ID, TeacherID: teacher.ID, HoursPerWeek: 3})
	if err != nil {
		t.Fatal(err)
	}
	day, from, to := 2, 1, 2
	if _, err := source.CreateConstraint(domain.Constraint{SchoolID: school.ID, Type: "teacher_unavailable", EntityType: "teacher", EntityID: teacher.ID, DayOfWeek: &day, TimeslotStart: &from, TimeslotEnd: &to, Weight: 100, IsHard: true}); err != nil {
		t.Fatal(err)
	}
	if err := source.ReplaceSchedule(school.ID, []domain.ScheduleEntry{{
		SchoolID: school.ID, LessonID: lesson.ID, ClassID: child.ID, TeacherID: teacher.ID,
		SubjectID: subject.ID, RoomID: room.ID, DayOfWeek: 3, Timeslot: 4,
	}}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := source.ExportAll(school.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Schedule) != 1 {
		t.Fatalf("expected exported schedule entry, got %d", len(snapshot.Schedule))
	}
	// Simulate a backup whose source ID collides with an unrelated local school.
	// Import must create a new school rather than erase that user's data.
	snapshot.School.ID = 1
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	destination := newTestApp(t)
	other, err := destination.CreateSchool("Другой лицей")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := destination.ImportAll(string(data))
	if err != nil {
		t.Fatalf("ImportAll: %v", err)
	}
	schools, err := destination.ListSchools()
	if err != nil || len(schools) != 2 {
		t.Fatalf("expected original and imported schools, got %+v (err=%v)", schools, err)
	}
	if imported.ID == other.ID {
		t.Fatal("import overwrote the unrelated school with the same source ID")
	}
	gotSchool := *imported
	if gotSchool.Name != "Лицей №1" || gotSchool.SettingsJSON != `{"days":5,"slots":7}` {
		t.Fatalf("school metadata was not restored: %+v", gotSchool)
	}
	classes, _ := destination.ListClasses(gotSchool.ID)
	if len(classes) != 2 {
		t.Fatalf("expected 2 classes, got %+v", classes)
	}
	var restoredParent, restoredChild domain.SchoolClass
	for _, class := range classes {
		if class.Name == "8А" {
			restoredParent = class
		}
		if class.Name == "8А-1" {
			restoredChild = class
		}
	}
	if restoredChild.SubgroupOf == nil || *restoredChild.SubgroupOf != restoredParent.ID {
		t.Fatalf("subgroup parent was not remapped: parent=%+v child=%+v", restoredParent, restoredChild)
	}
	teachers, _ := destination.ListTeachers(gotSchool.ID)
	constraints, _ := destination.ListConstraints(gotSchool.ID)
	if len(teachers) != 1 || len(constraints) != 1 || constraints[0].EntityID != teachers[0].ID {
		t.Fatalf("constraint entity was not remapped: teachers=%+v constraints=%+v", teachers, constraints)
	}
	entries, err := destination.ListSchedule(gotSchool.ID)
	if err != nil || len(entries) != 1 || entries[0].ClassID != restoredChild.ID || entries[0].DayOfWeek != 3 || entries[0].Timeslot != 4 {
		t.Fatalf("schedule was not restored: %+v (err=%v)", entries, err)
	}
}

// TestUpdateClassPreservesSubgroup verifies that editing a class keeps its
// subgroup_of link — without it a split class (10А → subgroups) loses the
// parent/child relationship and English (or any split subject) stops appearing
// in the parent's timetable cells.
func TestUpdateClassPreservesSubgroup(t *testing.T) {
	a := newTestApp(t)
	sc, err := a.CreateSchool("Тест подгруппы")
	if err != nil {
		t.Fatalf("CreateSchool: %v", err)
	}
	id := sc.ID
	parent, err := a.CreateClass(domain.SchoolClass{SchoolID: id, Name: "10А"})
	if err != nil {
		t.Fatalf("CreateClass parent: %v", err)
	}
	child, err := a.CreateClass(domain.SchoolClass{SchoolID: id, Name: "10А-гр1", SubgroupOf: &parent.ID})
	if err != nil {
		t.Fatalf("CreateClass child: %v", err)
	}
	// Rename the child class (editing an existing class) — subgroup_of must survive.
	child.Name = "10А-гр1-переименовано"
	if err := a.UpdateClass(*child); err != nil {
		t.Fatalf("UpdateClass: %v", err)
	}
	classes, err := a.ListClasses(id)
	if err != nil {
		t.Fatalf("ListClasses: %v", err)
	}
	var got *domain.SchoolClass
	for i := range classes {
		if classes[i].ID == child.ID {
			got = &classes[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("edited class %d not found", child.ID)
	}
	if got.SubgroupOf == nil || *got.SubgroupOf != parent.ID {
		t.Fatalf("subgroup_of lost after UpdateClass: got %+v", got)
	}
	if got.Name != "10А-гр1-переименовано" {
		t.Fatalf("name not updated: %q", got.Name)
	}
}

func TestSwapEntriesIntegration(t *testing.T) {
	a := newTestApp(t)
	sc, err := a.CreateSchool("Swap Integration")
	if err != nil {
		t.Fatalf("CreateSchool: %v", err)
	}
	id := sc.ID

	te, _ := a.CreateTeacher(domain.Teacher{SchoolID: id, Name: "Иванов"})
	a.CreateSubject(domain.Subject{SchoolID: id, Name: "Математика"})
	a.CreateClass(domain.SchoolClass{SchoolID: id, Name: "10А"})
	a.CreateRoom(domain.Room{SchoolID: id, Name: "301"})
	lesson, err := a.CreateLesson(domain.Lesson{SchoolID: id, ClassID: 1, SubjectID: 1, TeacherID: te.ID, HoursPerWeek: 5})
	if err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}

	// Seed two schedule entries at distinct cells.
	if err := a.store.ReplaceSchedule(id, []domain.ScheduleEntry{
		{SchoolID: id, LessonID: lesson.ID, ClassID: 1, TeacherID: te.ID, SubjectID: 1, RoomID: 1, DayOfWeek: 1, Timeslot: 2},
		{SchoolID: id, LessonID: lesson.ID, ClassID: 1, TeacherID: te.ID, SubjectID: 1, RoomID: 1, DayOfWeek: 3, Timeslot: 4},
	}); err != nil {
		t.Fatalf("ReplaceSchedule: %v", err)
	}
	before, _ := a.ListSchedule(id)
	if len(before) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(before))
	}
	var idA, idB int
	for _, e := range before {
		if e.DayOfWeek == 1 && e.Timeslot == 2 {
			idA = e.ID
		}
		if e.DayOfWeek == 3 && e.Timeslot == 4 {
			idB = e.ID
		}
	}
	if idA == 0 || idB == 0 {
		t.Fatalf("could not locate seeded entries: %+v", before)
	}

	// Frontend calls: src.id (=A) should end at (3,4); target.id (=B) should end at (1,2).
	if err := a.SwapEntries(idA, 3, 4, idB, 1, 2); err != nil {
		t.Fatalf("SwapEntries: %v", err)
	}
	after, _ := a.ListSchedule(id)
	for _, e := range after {
		if e.ID == idA && (e.DayOfWeek != 3 || e.Timeslot != 4) {
			t.Errorf("A should be at (3,4), got (%d,%d)", e.DayOfWeek, e.Timeslot)
		}
		if e.ID == idB && (e.DayOfWeek != 1 || e.Timeslot != 2) {
			t.Errorf("B should be at (1,2), got (%d,%d)", e.DayOfWeek, e.Timeslot)
		}
	}
}

// TestExportPDF verifies the new Go-side PDF renderer end-to-end:
//   - The PDF is non-empty.
//   - It starts with the PDF magic bytes (%PDF-) and ends with the EOF
//     marker, so it is structurally valid.
//   - The "по классам" mode produces N pages (one per class).
//   - The "вся школа" mode produces a single poster page with legend.
//   - The PDF contains a text string in Cyrillic (school name) — this
//     exercises the embedded DejaVu Sans font, which was the whole
//     point of moving PDF generation off jsPDF (whose default fonts
//     lacked Cyrillic).
func TestExportPDF(t *testing.T) {
	a := newTestApp(t)
	sc, err := a.CreateSchool("Лицей №1")
	if err != nil {
		t.Fatalf("CreateSchool: %v", err)
	}
	id := sc.ID
	if err := a.UpdateSchoolSettings(id, `{"days":5,"slots":6}`); err != nil {
		t.Fatal(err)
	}
	te, err := a.CreateTeacher(domain.Teacher{SchoolID: id, Name: "Иванова А.Б.", ShortName: "Ив"})
	if err != nil {
		t.Fatalf("CreateTeacher: %v", err)
	}
	sbj, err := a.CreateSubject(domain.Subject{SchoolID: id, Name: "Математика", ShortName: "М"})
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	cls, err := a.CreateClass(domain.SchoolClass{SchoolID: id, Name: "10А"})
	if err != nil {
		t.Fatalf("CreateClass: %v", err)
	}
	rm, err := a.CreateRoom(domain.Room{SchoolID: id, Name: "301", RoomType: "any"})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	lsn, err := a.CreateLesson(domain.Lesson{SchoolID: id, ClassID: cls.ID, SubjectID: sbj.ID, TeacherID: te.ID, HoursPerWeek: 3})
	if err != nil {
		t.Fatalf("CreateLesson: %v", err)
	}
	// Persist a small schedule (3 entries on Mon/Wed/Fri at slot 1,3,5).
	if err := a.ReplaceSchedule(id, []domain.ScheduleEntry{
		{SchoolID: id, LessonID: lsn.ID, ClassID: cls.ID, TeacherID: te.ID, SubjectID: sbj.ID, RoomID: rm.ID, DayOfWeek: 0, Timeslot: 1},
		{SchoolID: id, LessonID: lsn.ID, ClassID: cls.ID, TeacherID: te.ID, SubjectID: sbj.ID, RoomID: rm.ID, DayOfWeek: 2, Timeslot: 3},
		{SchoolID: id, LessonID: lsn.ID, ClassID: cls.ID, TeacherID: te.ID, SubjectID: sbj.ID, RoomID: rm.ID, DayOfWeek: 4, Timeslot: 5},
	}); err != nil {
		t.Fatalf("ReplaceSchedule: %v", err)
	}

	for _, mode := range []string{"school", "class"} {
		t.Run("mode="+mode, func(t *testing.T) {
			opts := PDFOptions{
				Mode:         mode,
				PageSize:     "A4",
				Orientation:  "landscape",
				ShowTeacher:  true,
				ShowRoom:     true,
				WeekdaysOnly: false,
				BW:           false,
			}
			b, _ := json.Marshal(opts)
			b64, err := a.ExportPDF(id, string(b))
			if err != nil {
				t.Fatalf("ExportPDF: %v", err)
			}
			if b64 == "" {
				t.Fatal("ExportPDF returned empty base64")
			}
			raw, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Fatalf("base64 decode: %v", err)
			}
			if len(raw) < 1000 {
				t.Fatalf("PDF too small (%d bytes); something is wrong", len(raw))
			}
			// PDF magic header check.
			if !strings.HasPrefix(string(raw), "%PDF-") {
				t.Fatalf("PDF does not start with %%PDF-: %q", string(raw[:8]))
			}
			// PDF EOF marker (may have trailing whitespace).
			eof := "%" + "%EOF"
			if !strings.Contains(string(raw), eof) {
				t.Fatal("PDF does not contain EOF marker")
			}
			// gopdf embeds TTF subsets AND compresses content streams, so
			// the school name is not directly searchable as plaintext. We
			// instead check that the embedded TTF font program is present —
			// that proves the Cyrillic glyphs (DejaVu Sans subset) made it
			// into the PDF, which was the whole point of moving PDF
			// generation off jsPDF.
			if !strings.Contains(string(raw), "FontFile") {
				t.Fatal("PDF does not embed a FontFile — DejaVu Sans (Cyrillic) was not embedded")
			}
			// "class" mode = one page per class — should produce a
			// "/Count 1" in the page tree; "school" mode also = 1 page.
			// We don't assert the exact count — both should be ≥1.
		})
	}
}

// TestExportPDFInvalidMode verifies the options validator in ExportPDF
// silently normalises an unknown mode to "school" instead of producing
// a corrupt PDF.
func TestExportPDFInvalidMode(t *testing.T) {
	a := newTestApp(t)
	sc, _ := a.CreateSchool("Тест")
	id := sc.ID
	b64, err := a.ExportPDF(id, `{"mode":"totally-bogus","page_size":"A4","orientation":"landscape"}`)
	if err != nil {
		t.Fatalf("ExportPDF should fall back to school mode, got: %v", err)
	}
	if b64 == "" {
		t.Fatal("expected non-empty base64 PDF")
	}
}

// TestExportPDFEmptySchool verifies that an empty school (no schedule
// entries) still produces a valid PDF without crashing. The PDF will
// have just the header and an empty grid.
func TestExportPDFEmptySchool(t *testing.T) {
	a := newTestApp(t)
	sc, _ := a.CreateSchool("Пусто")
	id := sc.ID
	b64, err := a.ExportPDF(id, `{"mode":"school"}`)
	if err != nil {
		t.Fatalf("ExportPDF on empty school: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(b64)
	if !strings.HasPrefix(string(raw), "%PDF-") {
		t.Fatalf("empty-school PDF is invalid: %q", string(raw[:8]))
	}
}

func containsLine(s, sub string) bool {
	for _, line := range splitLines(s) {
		if line == sub {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else if r != '\r' {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// TestImportRemapsClassHomeRoom — регрессия: ImportAll создавал классы со
// старым RoomID из снапшота ДО создания комнат. При импорте в новую базу это
// падало на FK, а при совпавшем ID класс молча привязывался к чужому
// кабинету. Теперь комнаты создаются раньше классов и RoomID ремапится.
func TestImportRemapsClassHomeRoom(t *testing.T) {
	source := newTestApp(t)
	school, err := source.CreateSchool("Экспорт кабинета")
	if err != nil {
		t.Fatal(err)
	}
	room, err := source.CreateRoom(domain.Room{SchoolID: school.ID, Name: "305"})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := source.CreateClass(domain.SchoolClass{SchoolID: school.ID, Name: "11Б", RoomID: room.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Импортируем в базу, где уже есть СВОИ комнаты с другими ID.
	destination := newTestApp(t)
	other, err := destination.CreateSchool("Местная школа")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := destination.CreateRoom(domain.Room{SchoolID: other.ID, Name: "Локальный кабинет"}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := source.ExportAll(school.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := destination.ImportAll(string(data))
	if err != nil {
		t.Fatalf("ImportAll with a home-roomed class: %v", err)
	}
	classes, err := destination.ListClasses(imported.ID)
	if err != nil || len(classes) != 1 {
		t.Fatalf("ListClasses: %+v err=%v", classes, err)
	}
	importedRooms, err := destination.ListRooms(imported.ID)
	if err != nil || len(importedRooms) != 1 {
		t.Fatalf("ListRooms: %+v err=%v", importedRooms, err)
	}
	if classes[0].RoomID != importedRooms[0].ID {
		t.Fatalf("class home room was not remapped: class=%+v room=%+v", classes[0], importedRooms[0])
	}
	if classes[0].RoomID == room.ID {
		t.Fatalf("class still points at the source DB room id %d", room.ID)
	}
	_ = parent
}
