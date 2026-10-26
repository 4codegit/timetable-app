package main

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

// buildIssuesView — отчёт о проблемах. Статистика сверху, под ней
// HSplit с двумя виртуализованными списками: каждый занимает свою
// половину окна и скроллится сам.
func buildIssuesView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return container.NewVBox(
			widget.NewLabel("Проблемы"),
			widget.NewSeparator(),
			widget.NewLabel("Выберите школу в разделе «Справочники»"),
		)
	}

	refreshBtn := widget.NewButton("Анализировать", func() {})
	refreshBtn.Importance = widget.HighImportance

	placedLabel := widget.NewLabel("0 / 0")
	unplacedLabel := widget.NewLabel("0")
	conflictsLabel := widget.NewLabel("0")
	violationsLabel := widget.NewLabel("0")

	statBox := container.NewGridWithColumns(4,
		statCard("Размещено", placedLabel),
		statCard("Не размещено", unplacedLabel),
		statCard("Накладок", conflictsLabel),
		statCard("Нарушений", violationsLabel),
	)

	unplacedList := widget.NewList(
		func() int {
			items := unplacedLessons(store)
			return len(items)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			items := unplacedLessons(store)
			if i >= len(items) {
				return
			}
			obj.(*widget.Label).SetText(items[i])
		},
	)

	conflictsList := widget.NewList(
		func() int {
			items := conflictRows(store)
			return len(items)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			items := conflictRows(store)
			if i >= len(items) {
				return
			}
			obj.(*widget.Label).SetText(items[i])
		},
	)
	violationsList := widget.NewList(
		func() int { return len(constraintViolationRows(store)) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			items := constraintViolationRows(store)
			if i < len(items) {
				obj.(*widget.Label).SetText(items[i])
			}
		},
	)

	refresh := func() {
		entries, err := store.ListSchedule(activeSchoolID)
		if err != nil {
			showError(w, "Ошибка загрузки расписания: "+err.Error())
			return
		}
		lessons, err := store.ListLessons(activeSchoolID)
		if err != nil {
			showError(w, "Ошибка загрузки уроков: "+err.Error())
			return
		}

		totalPlanned := 0
		for _, l := range lessons {
			totalPlanned += l.HoursPerWeek
		}
		placed := len(entries)
		unplaced := totalPlanned - placed
		if unplaced < 0 {
			unplaced = 0
		}

		placedLabel.SetText(fmt.Sprintf("%d / %d", placed, totalPlanned))
		unplacedLabel.SetText(fmt.Sprintf("%d", unplaced))
		conflictsLabel.SetText(fmt.Sprintf("%d", len(conflictRows(store))))
		violations := constraintViolationRows(store)
		violationsLabel.SetText(fmt.Sprintf("%d", len(violations)))

		unplacedList.Refresh()
		conflictsList.Refresh()
		violationsList.Refresh()
	}

	refreshBtn.OnTapped = refresh
	refresh()

	return container.NewBorder(
		// Шапка: заголовок, параметры, статистика
		container.NewVBox(
			widget.NewLabelWithStyle("Проблемы расписания", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
			container.NewHBox(widget.NewLabel("Проверяется текущее расписание, ограничения и настройки недели."), refreshBtn),
			widget.NewSeparator(),
			statBox,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		// Два списка делят всё окно пополам
		container.NewHSplit(
			container.NewBorder(
				widget.NewLabelWithStyle("Не размещённые уроки", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				nil, nil, nil,
				unplacedList,
			),
			container.NewVSplit(
				container.NewBorder(widget.NewLabelWithStyle("Накладки", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, conflictsList),
				container.NewBorder(widget.NewLabelWithStyle("Нарушения запретов времени", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, violationsList),
			),
		),
	)
}

// unplacedLessons возвращает строки «не размещённых» уроков для списка.
func unplacedLessons(store *db.Store) []string {
	entries, err := store.ListSchedule(activeSchoolID)
	if err != nil {
		return nil
	}
	lessons, err := store.ListLessons(activeSchoolID)
	if err != nil {
		return nil
	}
	classes, _ := store.ListClasses(activeSchoolID)
	subjects, _ := store.ListSubjects(activeSchoolID)

	className := func(id int) string {
		for _, c := range classes {
			if c.ID == id {
				return c.Name
			}
		}
		return "?"
	}
	subjName := func(id int) string {
		for _, s := range subjects {
			if s.ID == id {
				return s.Name
			}
		}
		return "?"
	}

	lessonPlaced := map[int]int{}
	for _, e := range entries {
		lessonPlaced[e.LessonID]++
	}

	var out []string
	for _, l := range lessons {
		if lessonPlaced[l.ID] < l.HoursPerWeek {
			out = append(out, fmt.Sprintf("Урок ID %d: %s (%s) — нужно %d ч, размещено %d",
				l.ID, className(l.ClassID), subjName(l.SubjectID), l.HoursPerWeek, lessonPlaced[l.ID]))
		}
	}
	return out
}

// conflictRows возвращает строки о накладках (учитель/класс/кабинет в
// одном слоте).
func conflictRows(store *db.Store) []string {
	entries, err := store.ListSchedule(activeSchoolID)
	if err != nil {
		return nil
	}
	classes, _ := store.ListClasses(activeSchoolID)
	subjects, _ := store.ListSubjects(activeSchoolID)
	teachers, _ := store.ListTeachers(activeSchoolID)
	rooms, _ := store.ListRooms(activeSchoolID)

	className := func(id int) string {
		for _, c := range classes {
			if c.ID == id {
				return c.Name
			}
		}
		return "?"
	}
	subjName := func(id int) string {
		for _, s := range subjects {
			if s.ID == id {
				return s.Name
			}
		}
		return "?"
	}
	teachName := func(id int) string {
		for _, t := range teachers {
			if t.ID == id {
				if t.ShortName != "" {
					return t.ShortName
				}
				return t.Name
			}
		}
		return "?"
	}

	type key struct{ id, day, slot int }
	group := func(field string) map[key][]domain.ScheduleEntry {
		out := map[key][]domain.ScheduleEntry{}
		for _, e := range entries {
			id := e.TeacherID
			if field == "class" {
				id = e.ClassID
			}
			if field == "room" {
				id = e.RoomID
				if id == 0 {
					continue
				}
			}
			k := key{id, e.DayOfWeek, e.Timeslot}
			out[k] = append(out[k], e)
		}
		return out
	}
	classByID := map[int]domain.SchoolClass{}
	for _, c := range classes {
		classByID[c.ID] = c
	}
	family := func(id int) int {
		if c, ok := classByID[id]; ok && c.SubgroupOf != nil {
			return *c.SubgroupOf
		}
		return id
	}
	roomName := func(id int) string {
		for _, r := range rooms {
			if r.ID == id {
				return r.Name
			}
		}
		return "?"
	}
	dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	var out []string
	appendConflicts := func(groups map[key][]domain.ScheduleEntry, label string) {
		for k, items := range groups {
			if len(items) < 2 {
				continue
			}
			day := "Д?"
			if k.day >= 0 && k.day < len(dayNames) {
				day = dayNames[k.day]
			}
			who := "?"
			switch label {
			case "Учитель":
				who = teachName(k.id)
			case "Класс":
				who = className(k.id)
			case "Кабинет":
				who = roomName(k.id)
			}
			parts := make([]string, 0, len(items))
			for _, e := range items {
				parts = append(parts, className(e.ClassID)+" — "+subjName(e.SubjectID))
			}
			out = append(out, fmt.Sprintf("%s, урок %d · %s %s: %s", day, k.slot+1, label, who, strings.Join(parts, "; ")))
		}
	}
	appendConflicts(group("teacher"), "Учитель")
	appendConflicts(group("class"), "Класс")
	for k, items := range group("room") {
		var conflicting []domain.ScheduleEntry
		for i := range items {
			for j := i + 1; j < len(items); j++ {
				if family(items[i].ClassID) != family(items[j].ClassID) {
					conflicting = append(conflicting, items[i], items[j])
				}
			}
		}
		if len(conflicting) < 2 {
			continue
		}
		day := "Д?"
		if k.day >= 0 && k.day < len(dayNames) {
			day = dayNames[k.day]
		}
		seen := map[int]bool{}
		parts := []string{}
		for _, e := range conflicting {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			parts = append(parts, className(e.ClassID)+" — "+subjName(e.SubjectID))
		}
		out = append(out, fmt.Sprintf("%s, урок %d · Кабинет %s: %s", day, k.slot+1, roomName(k.id), strings.Join(parts, "; ")))
	}
	sort.Strings(out)
	return out
}

func constraintViolationRows(store *db.Store) []string {
	entries, err := store.ListSchedule(activeSchoolID)
	if err != nil {
		return nil
	}
	constraints, err := store.ListConstraints(activeSchoolID)
	if err != nil {
		return nil
	}
	classes, _ := store.ListClasses(activeSchoolID)
	teachers, _ := store.ListTeachers(activeSchoolID)
	rooms, _ := store.ListRooms(activeSchoolID)
	subjects, _ := store.ListSubjects(activeSchoolID)
	className := map[int]string{}
	for _, x := range classes {
		className[x.ID] = x.Name
	}
	teacherName := map[int]string{}
	for _, x := range teachers {
		teacherName[x.ID] = x.Name
	}
	roomName := map[int]string{}
	for _, x := range rooms {
		roomName[x.ID] = x.Name
	}
	subjectName := map[int]string{}
	for _, x := range subjects {
		subjectName[x.ID] = x.Name
	}
	familyOf := map[int]int{}
	for _, c := range classes {
		familyOf[c.ID] = c.ID
		if c.SubgroupOf != nil {
			familyOf[c.ID] = *c.SubgroupOf
		}
	}
	dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	var out []string
	settings := loadFyneSettings(store, activeSchoolID)
	for _, e := range entries {
		if e.DayOfWeek < 0 || e.DayOfWeek > 6 || e.Timeslot < 0 || e.Timeslot >= settings.Slots {
			out = append(out, fmt.Sprintf("Запись урока %d находится за пределами заданной сетки", e.ID))
			continue
		}
		if settings.DaysMask&(1<<e.DayOfWeek) == 0 {
			out = append(out, fmt.Sprintf("%s, урок %d · занятие стоит в день, который отмечен как неучебный", dayNames[e.DayOfWeek], e.Timeslot+1))
		}
	}
	for _, c := range constraints {
		if !c.IsHard || c.Type != "teacher_unavailable" && c.Type != "class_unavailable" && c.Type != "room_unavailable" {
			continue
		}
		for _, e := range entries {
			entity := ""
			switch c.EntityType {
			case "teacher":
				if c.EntityID == e.TeacherID {
					entity = teacherName[e.TeacherID]
				}
			case "class":
				if c.EntityID == e.ClassID || c.EntityID == familyOf[e.ClassID] {
					entity = className[e.ClassID]
				}
			case "room":
				if c.EntityID == e.RoomID && e.RoomID != 0 {
					entity = roomName[e.RoomID]
				}
			}
			if entity == "" || c.DayOfWeek != nil && *c.DayOfWeek != e.DayOfWeek || c.TimeslotStart != nil && e.Timeslot < *c.TimeslotStart || c.TimeslotEnd != nil && e.Timeslot > *c.TimeslotEnd {
				continue
			}
			day := "Д?"
			if e.DayOfWeek >= 0 && e.DayOfWeek < len(dayNames) {
				day = dayNames[e.DayOfWeek]
			}
			out = append(out, fmt.Sprintf("%s, урок %d · %s недоступен: %s — %s", day, e.Timeslot+1, c.EntityType, entity, className[e.ClassID]))
		}
	}
	// Обнаруживаем нарушения правила «не более N одинаковых предметов подряд».
	type runKey struct{ family, day, subject int }
	occupied := map[runKey]map[int]bool{}
	for _, e := range entries {
		k := runKey{family: familyOf[e.ClassID], day: e.DayOfWeek, subject: e.SubjectID}
		if occupied[k] == nil {
			occupied[k] = map[int]bool{}
		}
		occupied[k][e.Timeslot] = true
	}
	for _, c := range constraints {
		if !c.IsHard || c.Type != "max_consecutive_subject" || c.Weight < 1 {
			continue
		}
		for k, slots := range occupied {
			if c.EntityType == "subject" && c.EntityID != k.subject {
				continue
			}
			run := 0
			for slot := 0; slot <= settings.Slots; slot++ {
				if slot < settings.Slots && slots[slot] {
					run++
					continue
				}
				if run > c.Weight {
					classLabel := className[k.family]
					if classLabel == "" {
						classLabel = fmt.Sprintf("класс %d", k.family)
					}
					subjectLabel := subjectName[k.subject]
					if subjectLabel == "" {
						subjectLabel = "предмет"
					}
					day := "Д?"
					if k.day >= 0 && k.day < len(dayNames) {
						day = dayNames[k.day]
					}
					out = append(out, fmt.Sprintf("%s · %s: %s подряд (%d), разрешено %d", day, classLabel, subjectLabel, run, c.Weight))
				}
				run = 0
			}
		}
	}
	sort.Strings(out)
	return out
}

func statCard(title string, lbl *widget.Label) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel(title),
		lbl,
	)
}
