package io

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"timetable/internal/db"
	"timetable/internal/domain"
)

// Snapshot is the full exportable state of a school.
type Snapshot struct {
	School      domain.School          `json:"school"`
	Teachers    []domain.Teacher       `json:"teachers"`
	Subjects    []domain.Subject       `json:"subjects"`
	Classes     []domain.SchoolClass   `json:"classes"`
	Rooms       []domain.Room          `json:"rooms"`
	Lessons     []domain.Lesson        `json:"lessons"`
	Constraints []domain.Constraint    `json:"constraints"`
	Schedule    []domain.ScheduleEntry `json:"schedule"`
}

// ExportAll gathers a school's data into a Snapshot.
func ExportAll(s *db.Store, schoolID int) (*Snapshot, error) {
	schools, err := s.ListSchools()
	if err != nil {
		return nil, err
	}
	var school domain.School
	found := false
	for _, sc := range schools {
		if sc.ID == schoolID {
			school = sc
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("school %d not found", schoolID)
	}
	t, err := s.ListTeachers(schoolID)
	if err != nil {
		return nil, err
	}
	sub, err := s.ListSubjects(schoolID)
	if err != nil {
		return nil, err
	}
	cl, err := s.ListClasses(schoolID)
	if err != nil {
		return nil, err
	}
	r, err := s.ListRooms(schoolID)
	if err != nil {
		return nil, err
	}
	l, err := s.ListLessons(schoolID)
	if err != nil {
		return nil, err
	}
	c, err := s.ListConstraints(schoolID)
	if err != nil {
		return nil, err
	}
	schedule, err := s.ListSchedule(schoolID)
	if err != nil {
		return nil, err
	}
	return &Snapshot{
		School: school, Teachers: t, Subjects: sub, Classes: cl,
		Rooms: r, Lessons: l, Constraints: c, Schedule: schedule,
	}, nil
}

// ImportAll inserts a snapshot into the DB inside a single transaction. If a
// school with both the same ID and name already exists, its data is replaced;
// otherwise a new school is created so an unrelated local school is never
// overwritten by an ID collision. Old IDs are remapped so every relationship
// (subgroups, constraints, lessons, and schedule) remains valid.
func ImportAll(s *db.Store, snap *Snapshot) (*domain.School, error) {
	if snap == nil {
		return nil, errors.New("import snapshot is empty")
	}
	if strings.TrimSpace(snap.School.Name) == "" {
		return nil, errors.New("school name is required")
	}
	var restored domain.School
	err := s.WithTx(func(tx *db.Store) error {
		var schoolID int
		if snap.School.ID == 0 {
			sc, err := tx.CreateSchool(snap.School.Name)
			if err != nil {
				return fmt.Errorf("create school: %w", err)
			}
			schoolID = sc.ID
			// Restore settings from snapshot
			if snap.School.SettingsJSON != "" {
				if err := tx.UpdateSchoolSettings(schoolID, snap.School.SettingsJSON); err != nil {
					return fmt.Errorf("restore school settings: %w", err)
				}
			}
		} else {
			// A snapshot normally carries the source database's ID.  On a new
			// installation that ID does not exist yet, so create a new school
			// instead of trying to insert children with a dangling foreign key.
			existing, err := tx.GetSchool(snap.School.ID)
			switch {
			case err == nil && existing.Name == snap.School.Name:
				schoolID = snap.School.ID
				if err := tx.ClearSchoolData(schoolID); err != nil {
					return fmt.Errorf("clear school data: %w", err)
				}
				updated := snap.School
				updated.ID = schoolID
				if err := tx.UpdateSchool(updated); err != nil {
					return fmt.Errorf("update school: %w", err)
				}
			case err == nil || errors.Is(err, sql.ErrNoRows):
				// Never overwrite an unrelated local school just because SQLite
				// happened to assign it the same ID as the source installation.
				sc, err := tx.CreateSchool(snap.School.Name)
				if err != nil {
					return fmt.Errorf("create school: %w", err)
				}
				schoolID = sc.ID
				if snap.School.SettingsJSON != "" {
					if err := tx.UpdateSchoolSettings(schoolID, snap.School.SettingsJSON); err != nil {
						return fmt.Errorf("restore school settings: %w", err)
					}
				}
			default:
				return fmt.Errorf("find import school: %w", err)
			}
		}
		restored = domain.School{ID: schoolID, Name: snap.School.Name, SettingsJSON: orEmptyJSON(snap.School.SettingsJSON)}

		// ID remapping: old snapshot ID -> new DB ID.
		teacherIDMap := map[int]int{}
		subjectIDMap := map[int]int{}
		classIDMap := map[int]int{}
		roomIDMap := map[int]int{}
		lessonIDMap := map[int]int{}

		for _, t := range snap.Teachers {
			oldID := t.ID
			t.SchoolID = schoolID
			t.ID = 0
			created, err := tx.CreateTeacher(t)
			if err != nil {
				return fmt.Errorf("create teacher %q: %w", t.Name, err)
			}
			teacherIDMap[oldID] = created.ID
		}

		for _, sub := range snap.Subjects {
			oldID := sub.ID
			sub.SchoolID = schoolID
			sub.ID = 0
			sub.RequiresRoomType = orDefaultStr(sub.RequiresRoomType, "any")
			created, err := tx.CreateSubject(sub)
			if err != nil {
				return fmt.Errorf("create subject %q: %w", sub.Name, err)
			}
			subjectIDMap[oldID] = created.ID
		}

		type subgroupRestore struct{ classID, oldParentID int }
		var subgroups []subgroupRestore
		for _, c := range snap.Classes {
			oldID := c.ID
			c.SchoolID = schoolID
			c.ID = 0
			if c.SubgroupOf != nil {
				subgroups = append(subgroups, subgroupRestore{classID: oldID, oldParentID: *c.SubgroupOf})
				c.SubgroupOf = nil
			}
			created, err := tx.CreateClass(c)
			if err != nil {
				return fmt.Errorf("create class %q: %w", c.Name, err)
			}
			classIDMap[oldID] = created.ID
		}
		for _, subgroup := range subgroups {
			classID, classOK := classIDMap[subgroup.classID]
			parentID, parentOK := classIDMap[subgroup.oldParentID]
			if !classOK || !parentOK {
				return fmt.Errorf("restore subgroup: class reference is missing")
			}
			if err := tx.UpdateClassSubgroup(classID, &parentID); err != nil {
				return fmt.Errorf("restore subgroup: %w", err)
			}
		}

		for _, r := range snap.Rooms {
			oldID := r.ID
			r.SchoolID = schoolID
			r.ID = 0
			r.RoomType = orDefaultStr(r.RoomType, "any")
			created, err := tx.CreateRoom(r)
			if err != nil {
				return fmt.Errorf("create room %q: %w", r.Name, err)
			}
			roomIDMap[oldID] = created.ID
		}

		for _, l := range snap.Lessons {
			oldID := l.ID
			l.SchoolID = schoolID
			l.ID = 0
			var ok bool
			if l.ClassID, ok = classIDMap[l.ClassID]; !ok {
				return fmt.Errorf("lesson references a missing class")
			}
			if l.SubjectID, ok = subjectIDMap[l.SubjectID]; !ok {
				return fmt.Errorf("lesson references a missing subject")
			}
			if l.TeacherID, ok = teacherIDMap[l.TeacherID]; !ok {
				return fmt.Errorf("lesson references a missing teacher")
			}
			created, err := tx.CreateLesson(l)
			if err != nil {
				return fmt.Errorf("create lesson: %w", err)
			}
			lessonIDMap[oldID] = created.ID
		}

		for _, c := range snap.Constraints {
			c.SchoolID = schoolID
			c.ID = 0
			var ids map[int]int
			switch c.EntityType {
			case "school":
				c.EntityID = schoolID
			case "teacher":
				ids = teacherIDMap
			case "class":
				ids = classIDMap
			case "room":
				ids = roomIDMap
			case "lesson":
				ids = lessonIDMap
			default:
				return fmt.Errorf("constraint has unsupported entity type %q", c.EntityType)
			}
			if ids != nil {
				var ok bool
				if c.EntityID, ok = ids[c.EntityID]; !ok {
					return fmt.Errorf("constraint references a missing %s", c.EntityType)
				}
			}
			if _, err := tx.CreateConstraint(c); err != nil {
				return fmt.Errorf("create constraint: %w", err)
			}
		}

		entries := make([]domain.ScheduleEntry, 0, len(snap.Schedule))
		for _, entry := range snap.Schedule {
			entry.ID = 0
			entry.SchoolID = schoolID
			var ok bool
			if entry.LessonID, ok = lessonIDMap[entry.LessonID]; !ok {
				return fmt.Errorf("schedule entry references a missing lesson")
			}
			if entry.ClassID, ok = classIDMap[entry.ClassID]; !ok {
				return fmt.Errorf("schedule entry references a missing class")
			}
			if entry.TeacherID, ok = teacherIDMap[entry.TeacherID]; !ok {
				return fmt.Errorf("schedule entry references a missing teacher")
			}
			if entry.SubjectID, ok = subjectIDMap[entry.SubjectID]; !ok {
				return fmt.Errorf("schedule entry references a missing subject")
			}
			if entry.RoomID, ok = roomIDMap[entry.RoomID]; !ok {
				return fmt.Errorf("schedule entry references a missing room")
			}
			entries = append(entries, entry)
		}
		if err := tx.ReplaceSchedule(schoolID, entries); err != nil {
			return fmt.Errorf("restore schedule: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &restored, nil
}

// orEmptyJSON returns the value if non-empty, otherwise "{}".
func orEmptyJSON(v string) string {
	if v == "" {
		return "{}"
	}
	return v
}

// orDefaultStr returns v if non-empty, otherwise def.
func orDefaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ScheduleCSV renders entries as a week×timeslot CSV keyed by class.
func ScheduleCSV(entries []domain.ScheduleEntry, classes map[int]domain.SchoolClass, teachers map[int]domain.Teacher, subjects map[int]domain.Subject, rooms map[int]domain.Room, days, slots int) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)

	header := []string{"День/Слот"}
	for s := 0; s < slots; s++ {
		header = append(header, fmt.Sprintf("Пара %d", s+1))
	}
	if err := w.Write(header); err != nil {
		return "", err
	}

	dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	// grid[classID][day][slot] = label
	grid := map[int]map[int]map[int]string{}
	for _, e := range entries {
		if grid[e.ClassID] == nil {
			grid[e.ClassID] = map[int]map[int]string{}
		}
		if grid[e.ClassID][e.DayOfWeek] == nil {
			grid[e.ClassID][e.DayOfWeek] = map[int]string{}
		}
		subj := "?"
		if s, ok := subjects[e.SubjectID]; ok {
			subj = s.Name
		}
		teach := "?"
		if t, ok := teachers[e.TeacherID]; ok {
			if t.ShortName != "" {
				teach = t.ShortName
			} else {
				teach = t.Name
			}
		}
		room := "?"
		if r, ok := rooms[e.RoomID]; ok {
			room = r.Name
		}
		grid[e.ClassID][e.DayOfWeek][e.Timeslot] = fmt.Sprintf("%s (%s) %s", subj, teach, room)
	}

	for cid, c := range classes {
		for d := 0; d < days; d++ {
			row := []string{c.Name + " " + dayName(dayNames, d)}
			for s := 0; s < slots; s++ {
				row = append(row, grid[cid][d][s])
			}
			if err := w.Write(row); err != nil {
				return "", err
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}

func dayName(names []string, d int) string {
	if d >= 0 && d < len(names) {
		return names[d]
	}
	return fmt.Sprintf("Д%d", d+1)
}

// ParseSnapshot decodes a JSON snapshot.
func ParseSnapshot(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
