package main

import (
	"fmt"
	"strconv"

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

	daysEd := widget.NewEntry()
	daysEd.SetPlaceHolder("Дней")
	daysEd.SetText("6")
	slotsEd := widget.NewEntry()
	slotsEd.SetPlaceHolder("Слотов")
	slotsEd.SetText("8")

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

	refresh := func() {
		// days/slots влияют только на сетку, но отчёт считает всё
		_, _ = strconv.Atoi(daysEd.Text)
		_, _ = strconv.Atoi(slotsEd.Text)

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
		violationsLabel.SetText("0")

		unplacedList.Refresh()
		conflictsList.Refresh()
	}

	refreshBtn.OnTapped = refresh
	refresh()

	return container.NewBorder(
		// Шапка: заголовок, параметры, статистика
		container.NewVBox(
			widget.NewLabelWithStyle("Проблемы расписания", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
			container.NewBorder(
				nil, nil,
				container.NewHBox(daysEd, slotsEd), nil,
				refreshBtn,
			),
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
			container.NewBorder(
				widget.NewLabelWithStyle("Накладки", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				nil, nil, nil,
				conflictsList,
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

	type cellKey struct{ v, day, slot int }
	conflictIDs := map[int]bool{}
	mark := func(m map[cellKey][]int) {
		for _, ids := range m {
			if len(ids) > 1 {
				for _, id := range ids {
					conflictIDs[id] = true
				}
			}
		}
	}
	busyT := map[cellKey][]int{}
	busyC := map[cellKey][]int{}
	busyR := map[cellKey][]int{}
	for _, e := range entries {
		k := cellKey{e.TeacherID, e.DayOfWeek, e.Timeslot}
		busyT[k] = append(busyT[k], e.ID)
		k = cellKey{e.ClassID, e.DayOfWeek, e.Timeslot}
		busyC[k] = append(busyC[k], e.ID)
		if e.RoomID != 0 {
			k = cellKey{e.RoomID, e.DayOfWeek, e.Timeslot}
			busyR[k] = append(busyR[k], e.ID)
		}
	}
	mark(busyT)
	mark(busyC)
	mark(busyR)

	entryMap := map[int]domain.ScheduleEntry{}
	for _, e := range entries {
		entryMap[e.ID] = e
	}
	dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

	var out []string
	for id := range conflictIDs {
		e := entryMap[id]
		day := "Д?"
		if e.DayOfWeek >= 0 && e.DayOfWeek < len(dayNames) {
			day = dayNames[e.DayOfWeek]
		}
		out = append(out, fmt.Sprintf("%s  %s  П%d: %s (%s)",
			className(e.ClassID), day, e.Timeslot+1,
			teachName(e.TeacherID), subjName(e.SubjectID)))
	}
	return out
}

func statCard(title string, lbl *widget.Label) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel(title),
		lbl,
	)
}
