package main

import (
	"timetable/internal/domain"
)

func toTeacherMap(ts []domain.Teacher) map[int]domain.Teacher {
	m := map[int]domain.Teacher{}
	for _, t := range ts {
		m[t.ID] = t
	}
	return m
}

func toClassMap(cs []domain.SchoolClass) map[int]domain.SchoolClass {
	m := map[int]domain.SchoolClass{}
	for _, c := range cs {
		m[c.ID] = c
	}
	return m
}

func toSubjMap(subs []domain.Subject) map[int]domain.Subject {
	m := map[int]domain.Subject{}
	for _, s := range subs {
		m[s.ID] = s
	}
	return m
}

func toRoomMap(rs []domain.Room) map[int]domain.Room {
	m := map[int]domain.Room{}
	for _, r := range rs {
		m[r.ID] = r
	}
	return m
}
