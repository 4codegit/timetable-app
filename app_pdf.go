package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"timetable/internal/pdf"
)

// ---- PDF export ----

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
		return "", fmt.Errorf("load teachers: %w", err)
	}
	cs, err := a.store.ListClasses(schoolID)
	if err != nil {
		return "", fmt.Errorf("load classes: %w", err)
	}
	rs, err := a.store.ListRooms(schoolID)
	if err != nil {
		return "", fmt.Errorf("load rooms: %w", err)
	}
	subs, err := a.store.ListSubjects(schoolID)
	if err != nil {
		return "", fmt.Errorf("load subjects: %w", err)
	}
	entries, err := a.store.ListSchedule(schoolID)
	if err != nil {
		return "", fmt.Errorf("load schedule: %w", err)
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
