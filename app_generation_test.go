package main

import (
	"testing"
	"time"

	"timetable/internal/domain"
)

// TestGenStateSetCreatesEntry — регрессия: set() раньше был update-only, и
// записи, созданной newID(), не существовало — любой прогресс терялся, а
// GetGenerationProgress на живую генерацию отвечал «generation not found».
func TestGenStateSetCreatesEntry(t *testing.T) {
	g := &generationState{states: make(map[int64]*GenProgress)}
	id, isNew := g.begin(1)
	if !isNew {
		t.Fatal("begin on idle school must create a new run")
	}
	g.set(id, 0, 7, "running", "")
	p, ok := g.get(id)
	if !ok {
		t.Fatal("set on a fresh id must create the entry (was update-only)")
	}
	if p.Status != "running" || p.Total != 7 {
		t.Fatalf("unexpected progress: %+v", p)
	}
	g.set(id, 7, 7, "done", "")
	p, _ = g.get(id)
	if p.Status != "done" || p.Placed != 7 {
		t.Fatalf("terminal state not stored: %+v", p)
	}
}

// TestGenStateBeginGuardsConcurrentRun: пока генерация школы идёт, второй
// begin получает ТОТ ЖЕ id вместо запуска параллельного солвера.
func TestGenStateBeginGuardsConcurrentRun(t *testing.T) {
	g := &generationState{states: make(map[int64]*GenProgress)}
	id1, isNew := g.begin(5)
	if !isNew {
		t.Fatal("first begin must be new")
	}
	id2, isNew2 := g.begin(5)
	if isNew2 || id2 != id1 {
		t.Fatalf("second begin for the same school must attach to the running one: id1=%d id2=%d new=%v", id1, id2, isNew2)
	}
	// Другая школа может идти параллельно.
	if _, isNew3 := g.begin(6); !isNew3 {
		t.Fatal("begin for another school must start a new run")
	}
	// Завершили первую — школа снова свободна.
	g.set(id1, 1, 1, "done", "")
	if _, isNew4 := g.begin(5); !isNew4 {
		t.Fatal("begin after done must start a new run")
	}
}

// TestStartPreciseGenerationProgressVisible — сквозная регрессия бага, при
// котором фронтенд на первом же поллинге получал «generation not found»,
// показывал ошибку и разблокировал кнопку, пока солвер молча работал.
func TestStartPreciseGenerationProgressVisible(t *testing.T) {
	a := newTestApp(t)
	school, err := a.CreateSchool("Прогресс генерации")
	if err != nil {
		t.Fatal(err)
	}
	teacher, err := a.CreateTeacher(domain.Teacher{SchoolID: school.ID, Name: "Иванов"})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := a.CreateSubject(domain.Subject{SchoolID: school.ID, Name: "Математика"})
	if err != nil {
		t.Fatal(err)
	}
	class, err := a.CreateClass(domain.SchoolClass{SchoolID: school.ID, Name: "7А"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateRoom(domain.Room{SchoolID: school.ID, Name: "101"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateLesson(domain.Lesson{SchoolID: school.ID, ClassID: class.ID, SubjectID: subject.ID, TeacherID: teacher.ID, HoursPerWeek: 2}); err != nil {
		t.Fatal(err)
	}

	id := a.StartPreciseGeneration(school.ID, 5, 7, 0x1F, 10)
	deadline := time.Now().Add(15 * time.Second)
	for {
		p := a.GetGenerationProgress(id)
		if p.Status == "error" {
			t.Fatalf("generation reported error: %+v", p)
		}
		if p.Status == "done" {
			if p.Placed != p.Total || p.Total == 0 {
				t.Fatalf("unexpected final progress: %+v", p)
			}
			break
		}
		if p.Status != "running" {
			t.Fatalf("expected running/done, got %+v", p)
		}
		if time.Now().After(deadline) {
			t.Fatalf("generation did not finish in time, last progress: %+v", p)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
