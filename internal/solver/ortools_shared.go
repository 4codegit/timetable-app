//go:build ortools

package solver

import (
	"fmt"

	"timetable/internal/domain"
)

// subParentOfClass2: id родительского класса, если класс — подгруппа (иначе 0).
func subParentOfClass2(classes map[int]domain.SchoolClass, classID int) int {
	if c, ok := classes[classID]; ok && c.SubgroupOf != nil {
		return *c.SubgroupOf
	}
	return 0
}

func constraintTypeCode(t string) int {
	switch t {
	case "teacher_unavailable":
		return 0
	case "class_unavailable":
		return 1
	case "room_unavailable":
		return 2
	case "max_consecutive":
		return 3
	case "lunch_break":
		return 4
	case "max_lessons_per_day":
		return 5
	case "min_lessons_per_day":
		return 6
	case "prefer_morning":
		return 7
	case "max_gaps":
		return 8
	case "student_group":
		return 9
	case "simultaneous_groups":
		return 10
	case "max_consecutive_subject":
		return 11
	}
	return -1
}

// appendStudentGroupConstraints добавляет синтетические жёсткие
// ограничения student_group: подгруппа (entity_id) + родитель (value).
// TimeslotStart несёт предмет подгруппы — биндинг запрещает только пары
// РАЗНЫХ предметов (делёный предмет: обе половинки параллельно).
func appendStudentGroupConstraints(in SolveInput) SolveInput {
	groups := map[int][]int{}
	for _, c := range in.Classes {
		if c.SubgroupOf != nil {
			if _, ok := in.Classes[*c.SubgroupOf]; ok {
				groups[*c.SubgroupOf] = append(groups[*c.SubgroupOf], c.ID)
			}
		}
	}
	if len(groups) == 0 {
		return in
	}
	out := in
	out.Constraints = append([]domain.Constraint(nil), in.Constraints...)
	for parent, kids := range groups {
		for _, kid := range kids {
			subj := subSubjectOf(kid, in.Lessons)
			// РАЗНЫЕ предметы у родителя и подгруппы: запрет пересечения (9).
			// Одинаковые: одновременность (10) — половинки параллельно.
			out.Constraints = append(out.Constraints, domain.Constraint{
				Type:          "student_group",
				EntityType:    "class",
				EntityID:      kid,
				Weight:        parent,
				TimeslotStart: &subj,
				IsHard:        true,
			})
		}
	}
	// Одновременность: для каждой пары (база, половинка) с одним предметом.
	halfOf := map[int]int{}
	byKey := map[string]int{}
	for _, l := range in.Lessons {
		if p := subParentOfClass2(in.Classes, l.ClassID); p != 0 {
			key := fmt.Sprintf("%d:%d", p, l.SubjectID)
			if base, ok := byKey[key]; ok {
				halfOf[l.ID] = base
			} else {
				byKey[key] = l.ID
			}
		}
	}
	for _, l := range in.Lessons {
		if base, ok := halfOf[l.ID]; ok {
			baseIdx := lessonIndexOf(in.Lessons, base)
			childIdx := lessonIndexOf(in.Lessons, l.ID)
			if baseIdx >= 0 && childIdx >= 0 {
				out.Constraints = append(out.Constraints, domain.Constraint{
					Type:       "simultaneous_groups",
					EntityType: "lesson_pair",
					EntityID:   childIdx,
					Weight:     baseIdx,
					IsHard:     true,
				})
			}
		}
	}
	return out
}

// lessonIndexOf: индекс урока в срезе (-1 если нет).
func lessonIndexOf(lessons []domain.Lesson, id int) int {
	for i, l := range lessons {
		if l.ID == id {
			return i
		}
	}
	return -1
}

// subSubjectOf: предмет первого урока подгруппы (0 если нет).
func subSubjectOf(childClass int, lessons []domain.Lesson) int {
	for _, l := range lessons {
		if l.ClassID == childClass {
			return l.SubjectID
		}
	}
	return 0
}

func entityTypeCode(t string) int {
	switch t {
	case "teacher":
		return 0
	case "class":
		return 1
	case "room":
		return 2
	case "school":
		return 3
	case "subject":
		return 4
	}
	return -1
}
