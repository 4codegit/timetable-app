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

func buildScheduleView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return container.NewVBox(
			widget.NewLabel("Расписание"),
			widget.NewSeparator(),
			widget.NewLabel("Выберите школу в разделе «Справочники»"),
		)
	}

	daysEntry := widget.NewEntry()
	daysEntry.SetPlaceHolder("Дней")
	daysEntry.SetText("6")
	slotsEntry := widget.NewEntry()
	slotsEntry.SetPlaceHolder("Слотов")
	slotsEntry.SetText("8")

	refreshBtn := widget.NewButton("Обновить", func() {})
	listData := binding.NewStringList()
	list := widget.NewListWithData(
		listData,
		func() fyne.CanvasObject { return widget.NewEntry() },
		func(i binding.DataItem, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			text, _ := i.(binding.String).Get()
			entry.SetText(text)
			entry.Disable()
		},
	)

	refresh := func() {
		listData.Set([]string{})
		days, _ := strconv.Atoi(daysEntry.Text)
		if days < 1 || days > 7 {
			days = 6
		}
		slots, _ := strconv.Atoi(slotsEntry.Text)
		if slots < 1 || slots > 14 {
			slots = 8
		}

		entries, err := store.ListSchedule(activeSchoolID)
		if err != nil {
			listData.Append("Ошибка загрузки: " + err.Error())
			return
		}

		type cellKey struct{ day, slot int }
		grid := map[int]map[cellKey][]domain.ScheduleEntry{}
		for _, e := range entries {
			if grid[e.ClassID] == nil {
				grid[e.ClassID] = map[cellKey][]domain.ScheduleEntry{}
			}
			k := cellKey{e.DayOfWeek, e.Timeslot}
			grid[e.ClassID][k] = append(grid[e.ClassID][k], e)
		}

		classes, _ := store.ListClasses(activeSchoolID)
		subjects, _ := store.ListSubjects(activeSchoolID)
		teachers, _ := store.ListTeachers(activeSchoolID)
		rooms, _ := store.ListRooms(activeSchoolID)

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
		roomName := func(id int) string {
			for _, r := range rooms {
				if r.ID == id {
					return r.Name
				}
			}
			return "?"
		}

		dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

		for _, c := range classes {
			if c.SubgroupOf != nil {
				continue
			}
			for d := 0; d < days; d++ {
				for s := 0; s < slots; s++ {
					k := cellKey{d, s}
					ents := grid[c.ID][k]
					if len(ents) == 0 {
						continue
					}
					for _, e := range ents {
						label := fmt.Sprintf("%s %s П%d: %s (%s) %s",
							className(c.ID), dayNames[d], s+1,
							subjName(e.SubjectID), teachName(e.TeacherID), roomName(e.RoomID))
						listData.Append(label)
					}
				}
			}
		}
	}

	refreshBtn.OnTapped = refresh
	refresh()

	return container.NewVBox(
		widget.NewLabel("Расписание"),
		widget.NewSeparator(),
		container.NewHBox(daysEntry, slotsEntry, refreshBtn),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}
