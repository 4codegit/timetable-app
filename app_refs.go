package main

import (
	"timetable/internal/domain"
)

// ---- Schools ----

func (a *App) CreateSchool(name string) (*domain.School, error) {
	return a.store.CreateSchool(name)
}

func (a *App) ListSchools() ([]domain.School, error) {
	return a.store.ListSchools()
}

func (a *App) DeleteSchool(id int) error {
	return a.store.DeleteSchool(id)
}

func (a *App) SchoolHasLessons(schoolID int) (bool, error) {
	return a.store.SchoolHasLessons(schoolID)
}

func (a *App) SchoolHasSchedule(schoolID int) (bool, error) {
	return a.store.SchoolHasSchedule(schoolID)
}

// ---- Teachers ----

func (a *App) CreateTeacher(t domain.Teacher) (*domain.Teacher, error) {
	return a.store.CreateTeacher(t)
}

func (a *App) ListTeachers(schoolID int) ([]domain.Teacher, error) {
	return a.store.ListTeachers(schoolID)
}

func (a *App) UpdateTeacher(t domain.Teacher) error {
	return a.store.UpdateTeacher(t)
}

func (a *App) DeleteTeacher(id int) error {
	return a.store.DeleteTeacher(id)
}

// ---- Subjects ----

func (a *App) CreateSubject(s domain.Subject) (*domain.Subject, error) {
	return a.store.CreateSubject(s)
}

func (a *App) ListSubjects(schoolID int) ([]domain.Subject, error) {
	return a.store.ListSubjects(schoolID)
}

func (a *App) UpdateSubject(sub domain.Subject) error {
	return a.store.UpdateSubject(sub)
}

func (a *App) DeleteSubject(id int) error {
	return a.store.DeleteSubject(id)
}

// ---- Classes ----

func (a *App) CreateClass(c domain.SchoolClass) (*domain.SchoolClass, error) {
	return a.store.CreateClass(c)
}

func (a *App) ListClasses(schoolID int) ([]domain.SchoolClass, error) {
	return a.store.ListClasses(schoolID)
}

func (a *App) UpdateClass(c domain.SchoolClass) error {
	return a.store.UpdateClass(c)
}

func (a *App) DeleteClass(id int) error {
	return a.store.DeleteClass(id)
}

// ---- Rooms ----

func (a *App) CreateRoom(r domain.Room) (*domain.Room, error) {
	return a.store.CreateRoom(r)
}

func (a *App) ListRooms(schoolID int) ([]domain.Room, error) {
	return a.store.ListRooms(schoolID)
}

func (a *App) UpdateRoom(r domain.Room) error {
	return a.store.UpdateRoom(r)
}

func (a *App) DeleteRoom(id int) error {
	return a.store.DeleteRoom(id)
}

// ---- Lessons ----

func (a *App) CreateLesson(l domain.Lesson) (*domain.Lesson, error) {
	return a.store.CreateLesson(l)
}

func (a *App) ListLessons(schoolID int) ([]domain.Lesson, error) {
	return a.store.ListLessons(schoolID)
}

func (a *App) DeleteLesson(id int) error {
	return a.store.DeleteLesson(id)
}

func (a *App) UpdateLesson(l domain.Lesson) (*domain.Lesson, error) {
	if err := a.store.UpdateLesson(l); err != nil {
		return nil, err
	}
	return &l, nil
}

// ---- Constraints ----

func (a *App) CreateConstraint(c domain.Constraint) (*domain.Constraint, error) {
	return a.store.CreateConstraint(c)
}

func (a *App) ListConstraints(schoolID int) ([]domain.Constraint, error) {
	return a.store.ListConstraints(schoolID)
}

func (a *App) DeleteConstraint(id int) error {
	return a.store.DeleteConstraint(id)
}
