package main

import (
	"timetable/internal/domain"
	"timetable/internal/io"
)

// ---- Import / Export / CSV ----

func (a *App) ExportAll(schoolID int) (*io.Snapshot, error) {
	return io.ExportAll(a.store, schoolID)
}

func (a *App) ImportAll(data string) (*domain.School, error) {
	snap, err := io.ParseSnapshot([]byte(data))
	if err != nil {
		return nil, err
	}
	return io.ImportAll(a.store, snap)
}

// ScheduleCSV builds a CSV export of the schedule for the given school.
func (a *App) ScheduleCSV(schoolID, days, slots int) (string, error) {
	entries, err := a.store.ListSchedule(schoolID)
	if err != nil {
		return "", err
	}
	ts, err := a.store.ListTeachers(schoolID)
	if err != nil {
		return "", err
	}
	cs, err := a.store.ListClasses(schoolID)
	if err != nil {
		return "", err
	}
	rs, err := a.store.ListRooms(schoolID)
	if err != nil {
		return "", err
	}
	subs, err := a.store.ListSubjects(schoolID)
	if err != nil {
		return "", err
	}
	return io.ScheduleCSV(entries, toClassMap(cs), toTeacherMap(ts), toSubjMap(subs), toRoomMap(rs), days, slots)
}
