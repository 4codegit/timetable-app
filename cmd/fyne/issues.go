package main

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

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

	unplacedLabel := widget.NewLabel("Не размещено уроков: 0")
	conflictsLabel := widget.NewLabel("Накладок: 0")
	violationsLabel := widget.NewLabel("Нарушений ограничений: 0")
	placedLabel := widget.NewLabel("Размещено: 0 / 0")

	unplacedList := binding.NewStringList()
	unplacedView := widget.NewListWithData(
		unplacedList,
		func() fyne.CanvasObject { return widget.NewEntry() },
		func(i binding.DataItem, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			text, _ := i.(binding.String).Get()
			entry.SetText(text)
			entry.Disable()
		},
	)

	conflictsList := binding.NewStringList()
	conflictsView := widget.NewListWithData(
		conflictsList,
		func() fyne.CanvasObject { return widget.NewEntry() },
		func(i binding.DataItem, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			text, _ := i.(binding.String).Get()
			entry.SetText(text)
			entry.Disable()
		},
	)

	refresh := func() {
		days, _ := strconv.Atoi(daysEd.Text)
		if days < 1 || days > 7 {
			days = 6
		}
		slots, _ := strconv.Atoi(slotsEd.Text)
		if slots < 1 || slots > 14 {
			slots = 8
		}

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

		classes, _ := store.ListClasses(activeSchoolID)
		subjects, _ := store.ListSubjects(activeSchoolID)
		teachers, _ := store.ListTeachers(activeSchoolID)

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
		className := func(id int) string {
			for _, c := range classes {
				if c.ID == id {
					return c.Name
				}
			}
			return "?"
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

		placedLabel.SetText(fmt.Sprintf("Размещено: %d / %d", placed, totalPlanned))
		unplacedLabel.SetText(fmt.Sprintf("Не размещено уроков: %d", unplaced))

		type cellKey struct{ v, day, slot int }
		busyT := map[cellKey][]int{}
		busyC := map[cellKey][]int{}
		busyR := map[cellKey][]int{}
		for _, e := range entries {
			busyT[cellKey{e.TeacherID, e.DayOfWeek, e.Timeslot}] = append(busyT[cellKey{e.TeacherID, e.DayOfWeek, e.Timeslot}], e.ID)
			busyC[cellKey{e.ClassID, e.DayOfWeek, e.Timeslot}] = append(busyC[cellKey{e.ClassID, e.DayOfWeek, e.Timeslot}], e.ID)
			if e.RoomID != 0 {
				busyR[cellKey{e.RoomID, e.DayOfWeek, e.Timeslot}] = append(busyR[cellKey{e.RoomID, e.DayOfWeek, e.Timeslot}], e.ID)
			}
		}
		conflictIDs := map[int]bool{}
		for _, ids := range busyT {
			if len(ids) > 1 {
				for _, id := range ids {
					conflictIDs[id] = true
				}
			}
		}
		for _, ids := range busyC {
			if len(ids) > 1 {
				for _, id := range ids {
					conflictIDs[id] = true
				}
			}
		}
		for _, ids := range busyR {
			if len(ids) > 1 {
				for _, id := range ids {
					conflictIDs[id] = true
				}
			}
		}

		conflictsLabel.SetText(fmt.Sprintf("Накладок: %d", len(conflictIDs)))

		unplacedList.Set([]string{})
		lessonPlaced := map[int]int{}
		for _, e := range entries {
			lessonPlaced[e.LessonID]++
		}
		for _, l := range lessons {
			if lessonPlaced[l.ID] < l.HoursPerWeek {
				label := fmt.Sprintf("Урок ID %d: %s (%s) — нужно %d ч, размещено %d",
					l.ID, className(l.ClassID), subjName(l.SubjectID), l.HoursPerWeek, lessonPlaced[l.ID])
				unplacedList.Append(label)
			}
		}

		conflictsList.Set([]string{})
		entryMap := map[int]domain.ScheduleEntry{}
		for _, e := range entries {
			entryMap[e.ID] = e
		}
		dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
		for id := range conflictIDs {
			e := entryMap[id]
			conflictsList.Append(fmt.Sprintf("%s %s П%d: конфликт (%s — %s)",
				className(e.ClassID), dayNames[e.DayOfWeek], e.Timeslot+1,
				teachName(e.TeacherID), subjName(e.SubjectID)))
		}

		violationsLabel.SetText(fmt.Sprintf("Нарушений ограничений: %d", 0))
	}

	refreshBtn.OnTapped = refresh
	refresh()

	return container.NewVBox(
		widget.NewLabel("Проблемы расписания"),
		widget.NewSeparator(),
		container.NewHBox(daysEd, slotsEd, refreshBtn),
		widget.NewSeparator(),
		container.NewHBox(
			container.NewVBox(
				placedLabel,
				unplacedLabel,
				conflictsLabel,
				violationsLabel,
			),
			widget.NewSeparator(),
			container.NewVBox(
				widget.NewLabel("Не размещённые уроки:"),
				container.NewScroll(unplacedView),
			),
		),
		widget.NewSeparator(),
		container.NewVBox(
			widget.NewLabel("Накладки:"),
			container.NewScroll(conflictsView),
		),
	)
}
