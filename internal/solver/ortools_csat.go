//go:build linux && ortools

package solver

/*
#cgo linux CFLAGS: -I${SRCDIR}/../../third_party/ortools/include -I${SRCDIR}/../../third_party/ortools_bind
#cgo linux LDFLAGS: ${SRCDIR}/../../third_party/ortools_bind/bind.o -Wl,--start-group -L${SRCDIR}/../../third_party/ortools/lib -lortools -lortools_deps -Wl,--end-group -lstdc++ -lpthread -ldl -Wl,-rpath,$ORIGIN
#include <stdlib.h>
#include "bind.h"
*/
import "C"

import (
	"runtime"
	"time"

	"timetable/internal/domain"
)

func init() {
	preciseSolver = ortoolsSolve
}

// ortoolsSolve delegates to the OR-Tools CP-SAT C++ binding.
// Решение приходит в буферы, выделенные Go-стороной (ortools_solve_out):
// C не выделяет памяти — нет ручного free, нет unsafe-конверсий.
// (Примечание: при изменении bind.cpp форсируйте пересборку этого файла —
// go build не отслеживает bind.o как зависимость пакета.)
func ortoolsSolve(in SolveInput, parallelism int, timeout time.Duration) (Result, bool) {
	in = appendStudentGroupConstraints(in)
	in, cfgDays := prepareDaysMask(in)
	days := cfgDays
	if days <= 0 {
		days = 6
	}
	slots := in.Config.SlotsPerDay
	if slots <= 0 {
		slots = 8
	}

	nl := len(in.Lessons)
	hours := make([]C.int, nl)
	lclass := make([]C.int, nl)
	lteacher := make([]C.int, nl)
	lsubject := make([]C.int, nl)
	occCount := 0
	for i, l := range in.Lessons {
		h := l.HoursPerWeek
		if h < 1 {
			h = 1
		}
		hours[i] = C.int(h)
		occCount += h
		lclass[i] = C.int(l.ClassID)
		lteacher[i] = C.int(l.TeacherID)
		lsubject[i] = C.int(l.SubjectID)
	}

	// room-type ids (0 = any)
	roomTypeID := map[string]int{"any": 0}
	next := 1
	assignType := func(s string) int {
		if s == "" {
			return 0
		}
		if id, ok := roomTypeID[s]; ok {
			return id
		}
		id := next
		next++
		roomTypeID[s] = id
		return id
	}
	lreq := make([]C.int, nl)
	for i, l := range in.Lessons {
		rt := "any"
		if subj, ok := in.Subjects[l.SubjectID]; ok && subj.RequiresRoomType != "" {
			rt = subj.RequiresRoomType
		}
		lreq[i] = C.int(assignType(rt))
	}

	nr := len(in.Rooms)
	// Ничего не планируем без уроков/кабинетов — как и прежний nullptr.
	if nl == 0 || occCount == 0 || nr == 0 {
		return Result{Entries: nil, Placed: 0, Total: occCount, Violations: 0}, true
	}
	roomIDs := make([]C.int, nr)
	roomTypes := make([]C.int, nr)
	for i, r := range in.Rooms {
		roomIDs[i] = C.int(r.ID)
		roomTypes[i] = C.int(assignType(r.RoomType))
	}

	// constraints
	cts := make([]C.CConstraint, len(in.Constraints))
	ctCount := len(in.Constraints)
	for i, c := range in.Constraints {
		cts[i].ctype = C.int(constraintTypeCode(c.Type))
		cts[i].entity_type = C.int(entityTypeCode(c.EntityType))
		cts[i].entity_id = C.int(c.EntityID)
		if c.DayOfWeek != nil {
			cts[i].day = C.int(*c.DayOfWeek)
		} else {
			cts[i].day = -1
		}
		if c.TimeslotStart != nil {
			cts[i].slot_start = C.int(*c.TimeslotStart)
		} else {
			cts[i].slot_start = -1
		}
		if c.TimeslotEnd != nil {
			cts[i].slot_end = C.int(*c.TimeslotEnd)
		} else {
			cts[i].slot_end = -1
		}
		cts[i].value = C.int(c.Weight)
		if c.IsHard {
			cts[i].is_hard = 1
		} else {
			cts[i].is_hard = 0
		}
	}

	workers := parallelism
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// CGO needs a valid pointer even when the count passed to C is 0.
	if ctCount == 0 {
		cts = []C.CConstraint{{}}
	}

	// Выходные буферы: по одному на поле, вместимость — число вхождений.
	outLesson := make([]C.int, occCount)
	outClass := make([]C.int, occCount)
	outTeacher := make([]C.int, occCount)
	outSubject := make([]C.int, occCount)
	outRoom := make([]C.int, occCount)
	outDay := make([]C.int, occCount)
	outSlot := make([]C.int, occCount)
	var outCount C.int

	ok := C.ortools_solve_out(
		C.int(nl), &hours[0], &lclass[0], &lteacher[0], &lsubject[0], &lreq[0],
		C.int(nr), &roomIDs[0], &roomTypes[0],
		C.int(days), C.int(slots),
		C.int(ctCount), &cts[0],
		C.int(int(timeout.Milliseconds())), C.int(workers),
		&outLesson[0], &outClass[0], &outTeacher[0], &outSubject[0],
		&outRoom[0], &outDay[0], &outSlot[0],
		C.int(occCount), &outCount,
	)
	runtime.KeepAlive(outLesson)
	if ok == 0 {
		return Result{}, false
	}

	n := int(outCount)
	if n > occCount {
		n = occCount
	}
	entries := make([]domain.ScheduleEntry, 0, n)
	placed := 0
	for i := 0; i < n; i++ {
		if int(outDay[i]) < 0 {
			continue // урок не удалось разместить (перегруженная школа)
		}
		placed++
		// bind.cpp reports the lesson *index* into in.Lessons, not the DB
		// lesson ID — map it back here. A real ID is required for the
		// schedule_entries foreign key.
		li := int(outLesson[i])
		if li < 0 || li >= nl {
			continue
		}
		// When no rooms are defined the solver outputs room_id=0 which
		// does not exist in the database — skip to avoid FK constraint error.
		if len(in.Rooms) == 0 && int(outRoom[i]) == 0 {
			continue
		}
		entries = append(entries, domain.ScheduleEntry{
			SchoolID:  in.SchoolID,
			LessonID:  in.Lessons[li].ID,
			ClassID:   int(outClass[i]),
			TeacherID: int(outTeacher[i]),
			SubjectID: int(outSubject[i]),
			RoomID:    int(outRoom[i]),
			DayOfWeek: int(outDay[i]),
			Timeslot:  int(outSlot[i]),
		})
	}
	return Result{Entries: entries, Placed: placed, Total: n, Violations: 0}, true
}
