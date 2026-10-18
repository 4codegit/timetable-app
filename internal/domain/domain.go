package domain

// School is the root tenant of all scheduling data.
type School struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	SettingsJSON string `json:"settings_json"`
}

// Teacher teaches lessons.
type Teacher struct {
	ID              int    `json:"id"`
	SchoolID        int    `json:"school_id"`
	Name            string `json:"name"`
	ShortName       string `json:"short_name"`
	MaxHoursPerWeek int    `json:"max_hours_per_week"`
}

// Subject is a course taught (Math, Physics, ...).
type Subject struct {
	ID               int    `json:"id"`
	SchoolID         int    `json:"school_id"`
	Name             string `json:"name"`
	ShortName        string `json:"short_name"`
	RequiresRoomType string `json:"requires_room_type"`
}

// SchoolClass is a student group (10A, 11B, or a subgroup).
type SchoolClass struct {
	ID         int    `json:"id"`
	SchoolID   int    `json:"school_id"`
	Name       string `json:"name"`
	RoomID     int    `json:"room_id"` // домашний кабинет класса
	SubgroupOf *int   `json:"subgroup_of,omitempty"`
}

// Room is a physical location lessons happen in.
type Room struct {
	ID       int    `json:"id"`
	SchoolID int    `json:"school_id"`
	Name     string `json:"name"`
	RoomType string `json:"room_type"`
}

// Lesson is one scheduled course instance in the study plan.
type Lesson struct {
	ID           int `json:"id"`
	SchoolID     int `json:"school_id"`
	ClassID      int `json:"class_id"`
	SubjectID    int `json:"subject_id"`
	TeacherID    int `json:"teacher_id"`
	HoursPerWeek int `json:"hours_per_week"`
}

// Constraint is a hard or soft rule applied to an entity/time range.
type Constraint struct {
	ID            int    `json:"id"`
	SchoolID      int    `json:"school_id"`
	Type          string `json:"type"`        // teacher_unavailable, class_unavailable, room_unavailable, lesson_fixed, lesson_forbidden, max_gaps, max_days, prefer_morning, prefer_together, prefer_apart
	EntityType    string `json:"entity_type"` // teacher, class, room, lesson
	EntityID      int    `json:"entity_id"`
	DayOfWeek     *int   `json:"day_of_week,omitempty"`
	TimeslotStart *int   `json:"timeslot_start,omitempty"`
	TimeslotEnd   *int   `json:"timeslot_end,omitempty"`
	Weight        int    `json:"weight"`
	IsHard        bool   `json:"is_hard"`
}

// ScheduleEntry is one concrete placement of a lesson occurrence.
type ScheduleEntry struct {
	ID        int `json:"id"`
	SchoolID  int `json:"school_id"`
	LessonID  int `json:"lesson_id"`
	ClassID   int `json:"class_id"`
	TeacherID int `json:"teacher_id"`
	SubjectID int `json:"subject_id"`
	RoomID    int `json:"room_id"`
	DayOfWeek int `json:"day_of_week"`
	Timeslot  int `json:"timeslot"`
}

// SchedulingConfig controls the solver grid size.
type SchedulingConfig struct {
	DaysPerWeek int `json:"days_per_week"`
	SlotsPerDay int `json:"slots_per_day"`
	// DaysMask marks which weekdays are school days: bit 0 = Monday …
	// bit 6 = Sunday. 0 means "DaysPerWeek days from Monday" (legacy
	// behaviour). Days outside the mask are forbidden to the solvers,
	// hidden in the UI grid and skipped in PDF exports.
	DaysMask int `json:"days_mask"`
}
