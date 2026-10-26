package main

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
	timetableio "timetable/internal/io"
	"timetable/internal/pdf"
)

func buildExportView(store *db.Store, w fyne.Window, onImport func(int)) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return container.NewVBox(
			widget.NewLabel("Экспорт"),
			widget.NewSeparator(),
			widget.NewLabel("Выберите школу в разделе «Справочники»"),
		)
	}

	modeSelect := widget.NewSelect([]string{"Вся школа", "По классам", "По учителям", "По кабинетам"}, func(string) {})
	modeSelect.SetSelected("Вся школа")

	pageSizeSelect := widget.NewSelect([]string{"A0", "A1", "A2", "A3", "A4"}, func(string) {})
	pageSizeSelect.SetSelected("A2")

	orientationSelect := widget.NewSelect([]string{"landscape", "portrait"}, func(string) {})
	orientationSelect.SetSelected("landscape")

	showTeacher := widget.NewCheck("Показывать учителей", func(bool) {})
	showTeacher.SetChecked(true)
	showRoom := widget.NewCheck("Показывать кабинеты", func(bool) {})
	showRoom.SetChecked(true)
	weekdaysOnly := widget.NewCheck("Только будни", func(bool) {})
	bw := widget.NewCheck("Ч/б", func(bool) {})

	exportPDFBtn := widget.NewButton("Экспорт PDF", func() {
		opts := pdf.Options{
			Mode:         map[string]string{"Вся школа": "school", "По классам": "class", "По учителям": "teacher", "По кабинетам": "room"}[modeSelect.Selected],
			PageSize:     pageSizeSelect.Selected,
			Orientation:  orientationSelect.Selected,
			ShowTeacher:  showTeacher.Checked,
			ShowRoom:     showRoom.Checked,
			WeekdaysOnly: weekdaysOnly.Checked,
			BW:           bw.Checked,
			Days:         6,
			Slots:        8,
		}

		pdfBytes, err := renderPDF(store, activeSchoolID, opts)
		if err != nil {
			showError(w, "Ошибка PDF: "+err.Error())
			return
		}

		saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			if _, err := writer.Write(pdfBytes); err != nil {
				showError(w, "Ошибка записи: "+err.Error())
			}
			if err := writer.Close(); err != nil {
				showError(w, "Не удалось завершить запись PDF: "+err.Error())
			}
		}, w)
		saveDialog.SetFileName("timetable.pdf")
		saveDialog.Show()
	})
	exportPDFBtn.Importance = widget.HighImportance

	exportCSVBtn := widget.NewButton("Экспорт CSV", func() {
		entries, err := store.ListSchedule(activeSchoolID)
		if err != nil {
			showError(w, "Ошибка загрузки расписания: "+err.Error())
			return
		}
		classes, err := store.ListClasses(activeSchoolID)
		if err != nil {
			showError(w, "Ошибка загрузки классов: "+err.Error())
			return
		}
		teachers, err := store.ListTeachers(activeSchoolID)
		if err != nil {
			showError(w, "Ошибка загрузки учителей: "+err.Error())
			return
		}
		subjects, err := store.ListSubjects(activeSchoolID)
		if err != nil {
			showError(w, "Ошибка загрузки предметов: "+err.Error())
			return
		}
		rooms, err := store.ListRooms(activeSchoolID)
		if err != nil {
			showError(w, "Ошибка загрузки кабинетов: "+err.Error())
			return
		}
		roomByID := make(map[int]domain.Room, len(rooms))
		for _, room := range rooms {
			roomByID[room.ID] = room
		}
		settings := loadFyneSettings(store, activeSchoolID)
		days := make([]int, 0, 7)
		for d := 0; d < 7; d++ {
			if settings.DaysMask&(1<<d) != 0 {
				days = append(days, d)
			}
		}
		data, err := timetableio.ScheduleCSVForDays(entries, toClassMap(classes), toTeacherMap(teachers), toSubjMap(subjects), roomByID, settings.Slots, days)
		if err != nil {
			showError(w, "Ошибка формирования CSV: "+err.Error())
			return
		}
		saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			if _, err := io.WriteString(writer, data); err != nil {
				showError(w, "Ошибка записи CSV: "+err.Error())
			}
			if err := writer.Close(); err != nil {
				showError(w, "Не удалось завершить запись CSV: "+err.Error())
			}
		}, w)
		saveDialog.SetFileName("timetable.csv")
		saveDialog.Show()
	})

	exportJSONBtn := widget.NewButton("Экспорт JSON (резервная копия)", func() {
		snap, err := timetableio.ExportAll(store, activeSchoolID)
		if err != nil {
			showError(w, "Ошибка экспорта: "+err.Error())
			return
		}
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			showError(w, "Ошибка сериализации: "+err.Error())
			return
		}

		saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			if _, err := writer.Write(data); err != nil {
				showError(w, "Ошибка записи: "+err.Error())
			}
			if err := writer.Close(); err != nil {
				showError(w, "Не удалось завершить запись JSON: "+err.Error())
			}
		}, w)
		saveDialog.SetFileName("timetable_backup.json")
		saveDialog.Show()
	})
	exportJSONBtn.Importance = widget.HighImportance

	importJSONBtn := widget.NewButton("Импорт JSON", func() {
		fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			data, err := io.ReadAll(reader)
			if err != nil {
				showError(w, "Ошибка чтения: "+err.Error())
				return
			}
			snap, err := timetableio.ParseSnapshot(data)
			if err != nil {
				showError(w, "Ошибка разбора JSON: "+err.Error())
				return
			}
			dialog.NewConfirm("Импортировать резервную копию?", "Если в этой базе уже есть школа с таким же ID и названием, её данные будут заменены. В остальных случаях импорт создаст отдельную школу.", func(ok bool) {
				if !ok {
					return
				}
				sc, err := timetableio.ImportAll(store, snap)
				if err != nil {
					showError(w, "Ошибка импорта: "+err.Error())
					return
				}
				if onImport != nil {
					onImport(sc.ID)
				}
				showInfo(w, "Импорт завершён успешно: "+sc.Name)
			}, w).Show()
		}, w)
		fileDialog.Show()
	})

	return container.NewVBox(
		widget.NewLabelWithStyle("Экспорт / Импорт", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewBorder(
			nil, nil,
			container.NewHBox(
				container.NewVBox(
					widget.NewLabel("Режим:"),
					modeSelect,
				),
				container.NewVBox(
					widget.NewLabel("Формат:"),
					pageSizeSelect,
				),
				container.NewVBox(
					widget.NewLabel("Ориентация:"),
					orientationSelect,
				),
				layout.NewSpacer(),
			),
			nil,
		),
		widget.NewSeparator(),
		container.NewHBox(
			showTeacher,
			showRoom,
			weekdaysOnly,
			bw,
		),
		widget.NewSeparator(),
		container.NewBorder(
			nil, nil,
			container.NewHBox(
				exportPDFBtn,
				exportCSVBtn,
				exportJSONBtn,
				importJSONBtn,
				layout.NewSpacer(),
			),
			nil,
		),
		widget.NewSeparator(),
		widget.NewLabel("Резервная копия сохраняет всю школу: настройки, подгруппы, ограничения и готовое расписание."),
	)
}

func renderPDF(store *db.Store, schoolID int, opts pdf.Options) ([]byte, error) {
	sc, err := store.GetSchool(schoolID)
	if err != nil {
		return nil, err
	}

	ts, err := store.ListTeachers(schoolID)
	if err != nil {
		return nil, err
	}
	cs, err := store.ListClasses(schoolID)
	if err != nil {
		return nil, err
	}
	rs, err := store.ListRooms(schoolID)
	if err != nil {
		return nil, err
	}
	subs, err := store.ListSubjects(schoolID)
	if err != nil {
		return nil, err
	}
	entries, err := store.ListSchedule(schoolID)
	if err != nil {
		return nil, err
	}

	settings := loadFyneSettings(store, schoolID)
	opts.SchoolName = sc.Name
	if opts.Title == "" {
		opts.Title = "Расписание"
	}
	if opts.Mode != "class" && opts.Mode != "teacher" && opts.Mode != "room" {
		opts.Mode = "school"
	}
	opts.Days, opts.Slots, opts.DaysMask = settings.Days, settings.Slots, settings.DaysMask
	opts.Periods = make([]pdf.Period, len(settings.Periods))
	for i, p := range settings.Periods {
		opts.Periods[i] = pdf.Period{Start: p.Start, End: p.End}
	}
	classByID, teacherByID, roomByID, subjectByID := map[int]domain.SchoolClass{}, map[int]domain.Teacher{}, map[int]domain.Room{}, map[int]domain.Subject{}
	for _, x := range cs {
		classByID[x.ID] = x
	}
	for _, x := range ts {
		teacherByID[x.ID] = x
	}
	for _, x := range rs {
		roomByID[x.ID] = x
	}
	for _, x := range subs {
		subjectByID[x.ID] = x
	}
	parentOf := map[int]int{}
	for _, c := range cs {
		if c.SubgroupOf != nil {
			parentOf[c.ID] = *c.SubgroupOf
		}
	}
	rows := make([]pdf.Row, 0)
	switch opts.Mode {
	case "school", "class":
		hasLessons := map[int]bool{}
		for _, e := range entries {
			id := e.ClassID
			if p := parentOf[id]; p != 0 {
				id = p
			}
			hasLessons[id] = true
		}
		for _, c := range cs {
			if c.SubgroupOf != nil || opts.Mode == "school" && !hasLessons[c.ID] {
				continue
			}
			label := c.Name
			if c.RoomID != 0 && roomByID[c.RoomID].Name != "" {
				label += " · каб. " + roomByID[c.RoomID].Name
			}
			rows = append(rows, pdf.Row{ID: c.ID, Label: label})
		}
	case "teacher":
		for _, t := range ts {
			rows = append(rows, pdf.Row{ID: t.ID, Label: t.Name})
		}
	case "room":
		for _, r := range rs {
			rows = append(rows, pdf.Row{ID: r.ID, Label: r.Name})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return strings.ToLower(rows[i].Label) < strings.ToLower(rows[j].Label) })
	type cellKey struct{ row, day, slot int }
	lookup := map[cellKey][]pdf.Cell{}
	conflicts := map[int]bool{}
	type occKey struct{ entity, day, slot int }
	byTeacher, byClass, byRoom := map[occKey][]domain.ScheduleEntry{}, map[occKey][]domain.ScheduleEntry{}, map[occKey][]domain.ScheduleEntry{}
	family := map[int]int{}
	for _, c := range cs {
		family[c.ID] = c.ID
		if c.SubgroupOf != nil {
			family[c.ID] = *c.SubgroupOf
		}
	}
	for _, e := range entries {
		byTeacher[occKey{e.TeacherID, e.DayOfWeek, e.Timeslot}] = append(byTeacher[occKey{e.TeacherID, e.DayOfWeek, e.Timeslot}], e)
		byClass[occKey{e.ClassID, e.DayOfWeek, e.Timeslot}] = append(byClass[occKey{e.ClassID, e.DayOfWeek, e.Timeslot}], e)
		if e.RoomID != 0 {
			byRoom[occKey{e.RoomID, e.DayOfWeek, e.Timeslot}] = append(byRoom[occKey{e.RoomID, e.DayOfWeek, e.Timeslot}], e)
		}
	}
	markDuplicates := func(groups map[occKey][]domain.ScheduleEntry) {
		for _, group := range groups {
			if len(group) > 1 {
				for _, e := range group {
					conflicts[e.ID] = true
				}
			}
		}
	}
	markDuplicates(byTeacher)
	markDuplicates(byClass)
	for _, group := range byRoom {
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				if family[group[i].ClassID] != family[group[j].ClassID] {
					conflicts[group[i].ID] = true
					conflicts[group[j].ID] = true
				}
			}
		}
	}
	for _, e := range entries {
		rowID := 0
		switch opts.Mode {
		case "school", "class":
			rowID = e.ClassID
			if parentOf[rowID] != 0 {
				rowID = parentOf[rowID]
			}
		case "teacher":
			rowID = e.TeacherID
		case "room":
			rowID = e.RoomID
		}
		cell := pdf.Cell{SubjectID: e.SubjectID, TeacherID: e.TeacherID, RoomID: e.RoomID, ClassID: e.ClassID, Conflict: conflicts[e.ID]}
		key := cellKey{rowID, e.DayOfWeek, e.Timeslot}
		lookup[key] = append(lookup[key], cell)
	}
	opts.Rows = rows
	opts.CellAt = func(rowID, day, slot int) (pdf.Cell, bool) {
		cells := lookup[cellKey{rowID, day, slot}]
		if len(cells) == 0 {
			return pdf.Cell{}, false
		}
		return cells[0], true
	}
	opts.CellSubs = func(rowID, day, slot int) []pdf.Cell {
		cells := lookup[cellKey{rowID, day, slot}]
		if len(cells) < 2 {
			return nil
		}
		return cells[1:]
	}
	opts.SubjectName = func(id int) string {
		if v, ok := subjectByID[id]; ok {
			return v.Name
		}
		return "?"
	}
	opts.TeacherName = func(id int) string {
		if v, ok := teacherByID[id]; ok {
			if v.ShortName != "" {
				return v.ShortName
			}
			return v.Name
		}
		return "?"
	}
	opts.RoomName = func(id int) string {
		if v, ok := roomByID[id]; ok {
			return v.Name
		}
		return ""
	}
	opts.ClassName = func(id int) string {
		if v, ok := classByID[id]; ok {
			return v.Name
		}
		return "?"
	}
	palette := []string{"#3b82f6", "#10b981", "#f59e0b", "#8b5cf6", "#ec4899", "#06b6d4", "#6366f1", "#14b8a6", "#f97316", "#a855f7", "#0ea5e9", "#ca8a04", "#65a30d", "#db2777"}
	opts.SubjectColor = func(id int) string {
		if id < 0 {
			id = -id
		}
		return palette[id%len(palette)]
	}
	if opts.Mode == "teacher" {
		opts.ShowTeacher = false
	}
	return pdf.Render(opts)
}
