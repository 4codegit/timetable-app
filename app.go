package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"timetable/internal/db"
	"timetable/internal/domain"
	"timetable/internal/io"
	"timetable/internal/pdf"
	"timetable/internal/solver"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound backend.
type App struct {
	ctx   context.Context
	store *db.Store
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Greet is a sanity endpoint.
func (a *App) Greet(name string) string {
	return "Hello " + name + ", welcome to Timetable!"
}

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

// ---- Subjects ----

func (a *App) CreateSubject(s domain.Subject) (*domain.Subject, error) {
	return a.store.CreateSubject(s)
}

func (a *App) ListSubjects(schoolID int) ([]domain.Subject, error) {
	return a.store.ListSubjects(schoolID)
}

// ---- Classes ----

func (a *App) CreateClass(c domain.SchoolClass) (*domain.SchoolClass, error) {
	return a.store.CreateClass(c)
}

func (a *App) ListClasses(schoolID int) ([]domain.SchoolClass, error) {
	return a.store.ListClasses(schoolID)
}

// ---- Rooms ----

func (a *App) CreateRoom(r domain.Room) (*domain.Room, error) {
	return a.store.CreateRoom(r)
}

func (a *App) ListRooms(schoolID int) ([]domain.Room, error) {
	return a.store.ListRooms(schoolID)
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

// UpdateTeacher edits name/short name/max hours of a teacher.
func (a *App) UpdateTeacher(t domain.Teacher) error {
	return a.store.UpdateTeacher(t)
}

// UpdateSubject edits name/short name/room type of a subject.
func (a *App) UpdateSubject(sub domain.Subject) error {
	return a.store.UpdateSubject(sub)
}

// UpdateClass edits name/grade/student count of a class.
func (a *App) UpdateClass(c domain.SchoolClass) error {
	return a.store.UpdateClass(c)
}

// UpdateRoom edits name/capacity/room type of a room.
func (a *App) UpdateRoom(r domain.Room) error {
	return a.store.UpdateRoom(r)
}

func (a *App) DeleteTeacher(id int) error {
	return a.store.DeleteTeacher(id)
}

func (a *App) DeleteSubject(id int) error {
	return a.store.DeleteSubject(id)
}

func (a *App) DeleteClass(id int) error {
	return a.store.DeleteClass(id)
}

func (a *App) DeleteRoom(id int) error {
	return a.store.DeleteRoom(id)
}

func (a *App) DeleteConstraint(id int) error {
	return a.store.DeleteConstraint(id)
}

func (a *App) DeleteScheduleEntry(id int) error {
	return a.store.DeleteScheduleEntry(id)
}

// SaveFile writes a base64-encoded payload (e.g. a generated PDF) to disk at
// the path chosen by the user via the native Save dialog.
func (a *App) SaveFile(path string, b64 string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveFileWithDialog shows a native save dialog and returns the chosen path
// (empty string if cancelled). Uses Wails v2 runtime.SaveFileDialog.
func (a *App) SaveFileWithDialog(defaultName string) (string, error) {
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Сохранить файл",
		DefaultFilename:      defaultName,
		CanCreateDirectories: true,
	})
}

// SaveExport writes a base64 payload (e.g. a generated PDF/CSV) directly into
// the user's Downloads folder under the given filename. This avoids the native
// GTK/xdg-desktop-portal save dialog, which is unreliable inside an AppImage on
// Wayland. Returns the absolute path the file was written to.
func (a *App) SaveExport(filename string, b64 string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	dir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dir = home
	}
	name := filepath.Base(strings.TrimSpace(filename))
	if name == "" || name == "." || name == string(os.PathSeparator) {
		name = "export"
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ---- Constraints ----

func (a *App) CreateConstraint(c domain.Constraint) (*domain.Constraint, error) {
	return a.store.CreateConstraint(c)
}

func (a *App) ListConstraints(schoolID int) ([]domain.Constraint, error) {
	return a.store.ListConstraints(schoolID)
}

// ---- Scheduling ----

// loadSolveInput собирает SolveInput из хранилища (общее для Generate и
// GeneratePrecise — раньше два одинаковых блока загрузки дублировались).
func (a *App) loadSolveInput(schoolID, days, slots, daysMask int) (solver.SolveInput, error) {
	lessons, err := a.store.ListLessons(schoolID)
	if err != nil {
		return solver.SolveInput{}, err
	}
	ts, _ := a.store.ListTeachers(schoolID)
	cs, _ := a.store.ListClasses(schoolID)
	rs, _ := a.store.ListRooms(schoolID)
	subs, _ := a.store.ListSubjects(schoolID)
	cons, _ := a.store.ListConstraints(schoolID)
	return solver.SolveInput{
		SchoolID:    schoolID,
		Lessons:     lessons,
		Teachers:    toTeacherMap(ts),
		Classes:     toClassMap(cs),
		Rooms:       rs,
		Subjects:    toSubjMap(subs),
		Constraints: cons,
		Config:      domain.SchedulingConfig{DaysPerWeek: days, SlotsPerDay: slots, DaysMask: daysMask},
	}, nil
}

// Generate runs the CSP solver and persists the result.
func (a *App) Generate(schoolID, days, slots, daysMask int) (*solver.Result, error) {
	in, err := a.loadSolveInput(schoolID, days, slots, daysMask)
	if err != nil {
		return nil, err
	}
	res := solver.Solve(a.ctx, in, runtime.NumCPU(), 30*time.Second)

	err = a.store.ReplaceSchedule(schoolID, res.Entries)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// ListSchedule returns stored entries.
func (a *App) ListSchedule(schoolID int) ([]domain.ScheduleEntry, error) {
	return a.store.ListSchedule(schoolID)
}

// HasPreciseSolver reports whether the running binary was compiled with the
// OR-Tools CP-SAT solver (-tags ortools). The frontend uses this to display
// an accurate badge / warning instead of falsely claiming "CP-SAT enabled"
// when the pure-Go fallback is what actually runs.
func (a *App) HasPreciseSolver() bool {
	return solver.HasPreciseSolver()
}

// MoveEntry relocates a schedule entry after a manual drag-and-drop edit.
func (a *App) MoveEntry(id, day, slot int) error {
	return a.store.MoveEntry(id, day, slot)
}

// SwapEntries atomically swaps two schedule entries' day/slot in one transaction.
func (a *App) SwapEntries(id1, day1, slot1, id2, day2, slot2 int) error {
	return a.store.SwapEntries(id1, day1, slot1, id2, day2, slot2)
}

// ReplaceSchedule overwrites the whole schedule (used by undo).
func (a *App) ReplaceSchedule(schoolID int, entries []domain.ScheduleEntry) error {
	return a.store.ReplaceSchedule(schoolID, entries)
}

// GeneratePrecise prefers the OR-Tools CP-SAT solver (when compiled with -tags ortools),
// otherwise falls back to the pure-Go backtracking solver.
func (a *App) GeneratePrecise(schoolID, days, slots, daysMask int) (*solver.Result, error) {
	in, err := a.loadSolveInput(schoolID, days, slots, daysMask)
	if err != nil {
		return nil, err
	}
	res := solver.SolvePrecise(a.ctx, in, runtime.NumCPU(), 12*time.Second)
	if err := a.store.ReplaceSchedule(schoolID, res.Entries); err != nil {
		return nil, err
	}
	return &res, nil
}

// ---- School settings (grid size + bell schedule) ----

type period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type schoolSettings struct {
	Days     int      `json:"days"`
	Slots    int      `json:"slots"`
	DaysMask int      `json:"days_mask"` // bit 0 = Пн … bit 6 = Вс; 0 = derive from Days
	Periods  []period `json:"periods"`
}

func defaultPeriods(n int) []period {
	out := make([]period, n)
	for i := 0; i < n; i++ {
		startMin := 8*60 + i*45
		out[i] = period{
			Start: fmt.Sprintf("%02d:%02d", startMin/60, startMin%60),
			End:   fmt.Sprintf("%02d:%02d", (startMin+45)/60, (startMin+45)%60),
		}
	}
	return out
}

func (a *App) loadSettings(schoolID int) schoolSettings {
	raw, _ := a.store.GetSchoolSettings(schoolID)
	st := schoolSettings{Days: 6, Slots: 8}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &st)
	}
	if st.Days <= 0 {
		st.Days = 6
	}
	if st.Slots <= 0 {
		st.Slots = 8
	}
	if st.DaysMask == 0 {
		// Legacy settings (or fresh school): first Days days from Monday.
		for d := 0; d < st.Days && d < 7; d++ {
			st.DaysMask |= 1 << d
		}
	}
	if len(st.Periods) != st.Slots {
		st.Periods = defaultPeriods(st.Slots)
	}
	return st
}

func (a *App) saveSettings(schoolID int, st schoolSettings) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return a.store.UpdateSchoolSettings(schoolID, string(b))
}

// GetSchoolSettings returns the raw settings JSON for a school.
func (a *App) GetSchoolSettings(schoolID int) (string, error) {
	return a.store.GetSchoolSettings(schoolID)
}

// UpdateSchoolSettings persists the raw settings JSON for a school.
func (a *App) UpdateSchoolSettings(schoolID int, settings string) error {
	return a.store.UpdateSchoolSettings(schoolID, settings)
}

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

// ---- PDF export ----
//
// ExportPDF generates the timetable PDF on the Go side (via internal/pdf,
// which uses signintech/gopdf + an embedded DejaVu Sans font for Cyrillic)
// and returns it as a base64 string. The frontend hands this straight to
// the existing SaveExport flow, so the user-facing save dialog is
// unchanged. Moving PDF generation off jsPDF fixes the long-standing
// "datauristring is unreliable in Wails WebKit" issue and produces
// reliable Cyrillic text in every PDF.

// PDFOptions is the JSON shape the frontend sends to ExportPDF. All
// fields are simple scalars so the JS↔Go boundary stays trivial.
type PDFOptions struct {
	Mode         string `json:"mode"`        // "school" | "class" | "teacher" | "room"
	PageSize     string `json:"page_size"`   // "A0".."A4"
	Orientation  string `json:"orientation"` // "landscape" | "portrait"
	ShowTeacher  bool   `json:"show_teacher"`
	ShowRoom     bool   `json:"show_room"`
	WeekdaysOnly bool   `json:"weekdays_only"`
	BW           bool   `json:"bw"`
	Days         int    `json:"days"`      // grid size from the UI; 0 = use saved settings
	Slots        int    `json:"slots"`     // (the UI grid may differ from saved settings)
	DaysMask     int    `json:"days_mask"` // school-day checkboxes; 0 = saved settings
}

// ExportPDF renders the schedule for the given school and returns the
// PDF as base64 (so the frontend's existing SaveExport can write it).
func (a *App) ExportPDF(schoolID int, optionsJSON string) (string, error) {
	opts := PDFOptions{
		Mode:         "school",
		PageSize:     "A2",
		Orientation:  "landscape",
		ShowTeacher:  true,
		ShowRoom:     true,
		WeekdaysOnly: false,
		BW:           false,
	}
	if strings.TrimSpace(optionsJSON) != "" {
		if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
			return "", fmt.Errorf("parse PDF options: %w", err)
		}
	}
	if opts.Mode != "school" && opts.Mode != "class" && opts.Mode != "teacher" && opts.Mode != "room" {
		opts.Mode = "school"
	}
	if opts.PageSize == "" {
		opts.PageSize = "A2"
	}
	if opts.Orientation != "portrait" {
		opts.Orientation = "landscape"
	}

	// Load all the data we need.
	sc, err := a.store.GetSchool(schoolID)
	if err != nil {
		return "", fmt.Errorf("load school: %w", err)
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
	entries, err := a.store.ListSchedule(schoolID)
	if err != nil {
		return "", err
	}
	settings := a.loadSettings(schoolID)
	days := settings.Days
	slots := settings.Slots
	if days <= 0 {
		days = 6
	}
	if slots <= 0 {
		slots = 8
	}
	// The UI grid (what the user sees on screen) wins over the saved
	// settings: the export must match the schedule that was generated
	// with the UI's days/slots, which may differ from the settings JSON
	// until the user saves the settings tab.
	if opts.Days > 0 {
		days = opts.Days
	}
	if opts.Slots > 0 {
		slots = opts.Slots
	}
	daysMask := settings.DaysMask
	if opts.DaysMask > 0 {
		daysMask = opts.DaysMask
	}
	if days <= 0 || days > 7 {
		days = 6
	}
	if slots <= 0 || slots > 14 {
		slots = 8
	}

	// Общие map-резолверы: O(1) вместо линейного скана (PDF зовёт их на
	// каждую ячейку каждой строки — на большой школе сотни тысяч вызовов).
	subjByID := toSubjMap(subs)
	teachByID := toTeacherMap(ts)
	classByID := toClassMap(cs)
	roomByID := toRoomMap(rs)

	// Resolve mode → row list.
	var rows []pdf.Row
	switch opts.Mode {
	case "school", "class":
		// Класс учится в своём кабинете: кабинет задан полем класса
		// (room_id) и пишется один раз в подписи («10А-каб:68»), а из
		// ячеек убран — ученики не ходят по кабинетам.
		hasLessons := map[int]bool{}
		for _, e := range entries {
			hasLessons[e.ClassID] = true
		}
		// Делёные уроки: записи расписания лежат на подгруппах —
		// считаем, что у родителя есть уроки, если записи есть хотя бы
		// у одной его подгруппы (иначе «вся школа» выдавала пустой PDF).
		for _, c := range cs {
			if c.SubgroupOf != nil && hasLessons[c.ID] {
				hasLessons[*c.SubgroupOf] = true
			}
		}
		roomName := func(roomID int) string { return roomByID[roomID].Name }
		for _, c := range cs {
			// Подгруппы рисуются внутри таблицы родителя (разделённые
			// ячейки) — отдельных таблиц для них не создаём.
			if c.SubgroupOf != nil {
				continue
			}
			// «Вся школа» — только классы с уроками: пустые классы
			// превращают плакат в страницу пустых рамок.
			if opts.Mode == "school" && !hasLessons[c.ID] {
				continue
			}
			label := c.Name
			if c.RoomID != 0 {
				if rn := roomName(c.RoomID); rn != "" {
					label += "-каб:" + rn
				}
			}
			rows = append(rows, pdf.Row{ID: c.ID, Label: label})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	case "teacher":
		for _, t := range ts {
			rows = append(rows, pdf.Row{ID: t.ID, Label: t.Name})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	case "room":
		for _, r := range rs {
			rows = append(rows, pdf.Row{ID: r.ID, Label: r.Name})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	}

	// Conflict detection: конфликт = ДВА урока с одним и тем же
	// учителем/классом/кабинетом в одно время. Ключ — значение поля
	// + время: группировка только по времени красила всю школу
	// (в каждом слоте законно стоят уроки разных классов).
	type occKey struct{ v, day, slot int }
	conflicts := map[int]bool{}
	{
		busyT := map[occKey][]int{}
		busyC := map[occKey][]int{}
		busyR := map[occKey][]int{}
		// Кабинет: уроки одной семьи класса (родитель + его подгруппы)
		// могут делить кабинет — половины класса вмещаются. Конфликт —
		// только если время делит ДРУГАЯ семья.
		familyOf := map[int]int{} // classID → родительский класс (или себя)
		for _, c := range cs {
			if c.SubgroupOf != nil {
				familyOf[c.ID] = *c.SubgroupOf
			} else {
				familyOf[c.ID] = c.ID
			}
		}
		entryClass := map[int]int{} // entryID → classID (семейное исключение кабинетов)
		for _, e := range entries {
			entryClass[e.ID] = e.ClassID
			k := occKey{e.TeacherID, e.DayOfWeek, e.Timeslot}
			busyT[k] = append(busyT[k], e.ID)
			k = occKey{e.ClassID, e.DayOfWeek, e.Timeslot}
			busyC[k] = append(busyC[k], e.ID)
			if e.RoomID != 0 {
				k = occKey{e.RoomID, e.DayOfWeek, e.Timeslot}
				busyR[k] = append(busyR[k], e.ID)
			}
		}
		for _, ids := range busyT {
			if len(ids) > 1 {
				for _, id := range ids {
					conflicts[id] = true
				}
			}
		}
		for _, ids := range busyC {
			if len(ids) > 1 {
				for _, id := range ids {
					conflicts[id] = true
				}
			}
		}
		for _, ids := range busyR {
			if len(ids) > 1 {
				// Половины одной семьи (делёный урок) легально делят
				// кабинет; разные семьи в одном кабинете — конфликт.
				sameFamily := true
				for i := 1; i < len(ids); i++ {
					if familyOf[entryClass[ids[i]]] != familyOf[entryClass[ids[0]]] {
						sameFamily = false
						break
					}
				}
				if sameFamily {
					continue
				}
				for _, id := range ids {
					conflicts[id] = true
				}
			}
		}
	}

	// subParentID: id родителя, если класс — подгруппа (иначе 0).
	subParent := map[int]int{} // classID → родитель (0 = не подгруппа)
	for _, c := range cs {
		if c.SubgroupOf != nil {
			subParent[c.ID] = *c.SubgroupOf
		}
	}
	subParentID := func(classID int) int { return subParent[classID] }

	// Build a lookup so CellAt is O(1) per (row, day, slot).
	// В ячейке может быть несколько уроков — параллельные подгруппы
	// одного класса: CellAt отдаёт первый, CellSubs — остальные.
	type cellKey struct{ day, slot int }
	type cellInfo struct {
		subjectID, teacherID, roomID, classID int
		isSub                                 bool
		conflict                              bool
	}
	lookup := map[int]map[cellKey][]cellInfo{} // rowID -> (day,slot) -> info
	for _, e := range entries {
		var rowID int
		isSub := false
		switch opts.Mode {
		case "school", "class":
			rowID = e.ClassID
			// Урок подгруппы прикрепляется к строке родительского класса.
			if p := subParentID(e.ClassID); p != 0 {
				rowID = p
				isSub = true
			}
		case "teacher":
			rowID = e.TeacherID
		case "room":
			rowID = e.RoomID
		}
		if lookup[rowID] == nil {
			lookup[rowID] = map[cellKey][]cellInfo{}
		}
		k := cellKey{e.DayOfWeek, e.Timeslot}
		// Два урока в одной ячейке (конфликт или параллельные подгруппы):
		// целоклассовый урок — первым, подгруппы — следом.
		info := cellInfo{
			subjectID: e.SubjectID,
			teacherID: e.TeacherID,
			roomID:    e.RoomID,
			classID:   e.ClassID,
			isSub:     isSub,
			conflict:  conflicts[e.ID],
		}
		list := lookup[rowID][k]
		if !info.isSub && len(list) > 0 && list[0].isSub {
			lookup[rowID][k] = append([]cellInfo{info}, list...)
		} else {
			lookup[rowID][k] = append(list, info)
		}
	}

	// Resolvers — замыкания поверх map (см. комментарий у map-ов выше).
	subjName := func(id int) string {
		if s, ok := subjByID[id]; ok {
			return s.Name
		}
		return "?"
	}
	teachName := func(id int) string {
		if t, ok := teachByID[id]; ok {
			if t.ShortName != "" {
				return t.ShortName
			}
			return t.Name
		}
		return "?"
	}
	className := func(id int) string {
		if c, ok := classByID[id]; ok {
			return c.Name
		}
		return "?"
	}
	roomName := func(id int) string {
		if r, ok := roomByID[id]; ok {
			return r.Name
		}
		return "?"
	}

	// Subject colour palette — mirrors the frontend's subjectColor().
	// Насыщенные цвета чипов: белый текст читается на любом из них.
	subjectColor := func(id int) string {
		palette := []string{"#3b82f6", "#10b981", "#f59e0b", "#8b5cf6", "#ec4899", "#06b6d4", "#6366f1", "#14b8a6", "#f97316", "#a855f7", "#0ea5e9", "#ca8a04", "#65a30d", "#db2777"}
		idx := id
		if idx < 0 {
			idx = -idx
		}
		return palette[idx%len(palette)]
	}

	// Periods.
	periods := make([]pdf.Period, slots)
	for i, p := range settings.Periods {
		if i >= slots {
			break
		}
		periods[i] = pdf.Period{Start: p.Start, End: p.End}
	}

	// Title for the header line.
	titleMap := map[string]string{
		"school":  "вся школа",
		"class":   "по классам",
		"teacher": "по учителям",
		"room":    "по кабинетам",
	}
	title := titleMap[opts.Mode]
	if title == "" {
		title = "расписание"
	}

	// Build the pdf.Options.
	po := pdf.Options{
		SchoolName: sc.Name,
		Title:      title,
		Days:       days,
		Slots:      slots,
		DaysMask:   daysMask,
		Periods:    periods,
		Mode:       opts.Mode,
		Rows:       rows,
		CellAt: func(rowID, day, slot int) (pdf.Cell, bool) {
			if lookup[rowID] == nil {
				return pdf.Cell{}, false
			}
			info, ok := lookup[rowID][cellKey{day, slot}]
			if !ok || len(info) == 0 {
				return pdf.Cell{}, false
			}
			return pdf.Cell{
				SubjectID: info[0].subjectID,
				TeacherID: info[0].teacherID,
				RoomID:    info[0].roomID,
				ClassID:   info[0].classID,
				Conflict:  info[0].conflict,
			}, true
		},
		CellSubs: func(rowID, day, slot int) []pdf.Cell {
			if lookup[rowID] == nil {
				return nil
			}
			infos := lookup[rowID][cellKey{day, slot}]
			if len(infos) < 2 {
				return nil
			}
			var out []pdf.Cell
			for _, ci := range infos[1:] {
				out = append(out, pdf.Cell{
					SubjectID: ci.subjectID,
					TeacherID: ci.teacherID,
					RoomID:    ci.roomID,
					ClassID:   ci.classID,
					Conflict:  ci.conflict,
				})
			}
			return out
		},
		// Имя учителя в ячейке не нужно в режиме «по учителям» — там
		// учитель и есть владелец страницы (ячейка показывает класс).
		ShowTeacher: opts.ShowTeacher && opts.Mode != "teacher",
		// Кабинет в ячейках не нужен: у класса он в подписи («5А-каб:70»),
		// учитель ходит по ним (см. режим «по учителям»), кабинет — сам
		// владелец страницы в своём режиме.
		ShowRoom:     false,
		WeekdaysOnly: opts.WeekdaysOnly,
		BW:           opts.BW,
		PageSize:     opts.PageSize,
		Orientation:  opts.Orientation,
		SubjectName:  subjName,
		ClassName:    className,
		TeacherName:  teachName,
		RoomName:     roomName,
		SubjectColor: subjectColor,
	}

	pdfBytes, err := pdf.Render(po)
	if err != nil {
		return "", fmt.Errorf("render pdf: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pdfBytes), nil
}
