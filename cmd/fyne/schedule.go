package main

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
)

// buildScheduleView — сетка расписания. Список стоит в center у Border:
// он растягивается на всё окно и сам себя скроллит (widget.List
// виртуализованный), без дополнительного Scroll-контейнера.
func buildScheduleView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return container.NewVBox(
			widget.NewLabel("Расписание"),
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

	refreshBtn := widget.NewButton("Обновить", func() {})
	refreshBtn.Importance = widget.HighImportance

	var list *widget.List
	list = widget.NewList(
		func() int {
			entries, _ := store.ListSchedule(activeSchoolID)
			return len(entries)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			label := scheduleRowLabel(store, i)
			obj.(*widget.Label).SetText(label)
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
		_ = days
		_ = slots
		list.Refresh()
	}

	refreshBtn.OnTapped = refresh

	return container.NewBorder(
		// Шапка: заголовок + параметры + кнопка
		container.NewVBox(
			widget.NewLabelWithStyle("Расписание", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
			container.NewBorder(
				nil, nil,
				container.NewHBox(daysEd, slotsEd), nil,
				refreshBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		// Список на всё окно, скролл внутри него
		list,
	)
}

// scheduleRowLabel собирает текст строки i сетки расписания.
func scheduleRowLabel(store *db.Store, i widget.ListItemID) string {
	entries, err := store.ListSchedule(activeSchoolID)
	if err != nil || i >= len(entries) {
		return ""
	}
	e := entries[i]

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
	roomName := func(id int) string {
		for _, r := range rooms {
			if r.ID == id {
				return r.Name
			}
		}
		return "?"
	}

	dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	day := "Д?"
	if e.DayOfWeek >= 0 && e.DayOfWeek < len(dayNames) {
		day = dayNames[e.DayOfWeek]
	}
	return fmt.Sprintf("%s  %s  П%d: %s (%s) каб. %s",
		className(e.ClassID), day, e.Timeslot+1,
		subjName(e.SubjectID), teachName(e.TeacherID), roomName(e.RoomID))
}
