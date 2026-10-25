package main

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
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

	daysEd := widget.NewEntry()
	daysEd.SetPlaceHolder("Дней")
	daysEd.SetText("6")
	slotsEd := widget.NewEntry()
	slotsEd.SetPlaceHolder("Слотов")
	slotsEd.SetText("8")

	refreshBtn := widget.NewButton("Обновить", func() {})
	refreshBtn.Importance = widget.HighImportance

	var listContainer *fyne.Container
	var refresh func()

	refresh = func() {
		listContainer.Objects = nil
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
			listContainer.Objects = append(listContainer.Objects, widget.NewEntry())
			listContainer.Objects[len(listContainer.Objects)-1].(*widget.Entry).SetText("Ошибка загрузки: " + err.Error())
			listContainer.Refresh()
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
						entry := widget.NewEntry()
						entry.SetText(label)
						entry.Disable()
						listContainer.Objects = append(listContainer.Objects, entry)
					}
				}
			}
		}
		listContainer.Refresh()
	}

	refreshBtn.OnTapped = refresh
	listContainer = container.NewVBox()
	refresh()

	return container.NewVBox(
		widget.NewLabelWithStyle("Расписание", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewBorder(
			nil, nil,
			container.NewHBox(
				container.NewVBox(daysEd, slotsEd),
				refreshBtn,
				layout.NewSpacer(),
			),
			nil,
		),
		widget.NewSeparator(),
		container.NewScroll(listContainer),
	)
}
