package main

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

// constraintRow — шаблон строки списка ограничений.
type constraintRow struct {
	*fyne.Container
	label  *widget.Label
	button *widget.Button
}

func buildConstraintsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return emptySchoolView("Ограничения")
	}
	kind := widget.NewSelect([]string{"Учитель", "Класс", "Кабинет"}, nil)
	kind.SetSelected("Учитель")
	entity := widget.NewSelect([]string{}, nil)
	dayOptions := append([]string{"Любой день"}, weekdayNames...)
	day := widget.NewSelect(dayOptions, nil)
	day.SetSelected(dayOptions[0])
	settings := loadFyneSettings(store, activeSchoolID)
	slotOptions := []string{"Весь день"}
	for i := 0; i < settings.Slots; i++ {
		slotOptions = append(slotOptions, fmt.Sprintf("Урок %d", i+1))
	}
	start := widget.NewSelect(slotOptions, nil)
	start.SetSelected(slotOptions[0])
	end := widget.NewSelect(slotOptions, nil)
	end.SetSelected(slotOptions[0])
	entityIDs := map[string]int{}
	loadEntities := func() {
		entityIDs = make(map[string]int)
		var labels []string
		switch kind.Selected {
		case "Учитель":
			items, err := store.ListTeachers(activeSchoolID)
			if err != nil {
				showError(w, err.Error())
				return
			}
			for _, item := range items {
				label := fmt.Sprintf("%s (#%d)", item.Name, item.ID)
				labels = append(labels, label)
				entityIDs[label] = item.ID
			}
		case "Класс":
			items, err := store.ListClasses(activeSchoolID)
			if err != nil {
				showError(w, err.Error())
				return
			}
			for _, item := range items {
				label := fmt.Sprintf("%s (#%d)", item.Name, item.ID)
				labels = append(labels, label)
				entityIDs[label] = item.ID
			}
		case "Кабинет":
			items, err := store.ListRooms(activeSchoolID)
			if err != nil {
				showError(w, err.Error())
				return
			}
			for _, item := range items {
				label := fmt.Sprintf("%s (#%d)", item.Name, item.ID)
				labels = append(labels, label)
				entityIDs[label] = item.ID
			}
		}
		entity.Options = labels
		if len(labels) > 0 {
			entity.SetSelected(labels[0])
		} else {
			entity.ClearSelected()
		}
		entity.Refresh()
	}
	kind.OnChanged = func(string) { loadEntities() }
	loadEntities()
	var list *widget.List
	list = widget.NewList(
		func() int { cs, _ := store.ListConstraints(activeSchoolID); return len(cs) },
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			btn := widget.NewButton("Удалить", nil)
			return &constraintRow{
				Container: container.NewBorder(nil, nil, lbl, nil, btn),
				label:     lbl, button: btn,
			}
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			cs, err := store.ListConstraints(activeSchoolID)
			if err != nil || i >= len(cs) {
				return
			}
			c := cs[i]
			row := obj.(*constraintRow)
			label := row.label
			button := row.button
			label.SetText(describeConstraint(store, c))
			button.OnTapped = func() {
				dialog.NewConfirm("Удалить ограничение?", label.Text, func(ok bool) {
					if !ok {
						return
					}
					if err := store.DeleteConstraint(c.ID); err != nil {
						showError(w, "Не удалось удалить ограничение: "+err.Error())
						return
					}
					list.Refresh()
				}, w).Show()
			}
		},
	)
	addUnavailable := widget.NewButton("Добавить недоступность", func() {
		id := entityIDs[entity.Selected]
		if id == 0 {
			showError(w, "Сначала добавьте сущность в справочники")
			return
		}
		c := domain.Constraint{SchoolID: activeSchoolID, EntityID: id, IsHard: true, Weight: 100}
		switch kind.Selected {
		case "Учитель":
			c.Type = "teacher_unavailable"
			c.EntityType = "teacher"
		case "Класс":
			c.Type = "class_unavailable"
			c.EntityType = "class"
		case "Кабинет":
			c.Type = "room_unavailable"
			c.EntityType = "room"
		}
		if day.Selected != "Любой день" {
			for i, name := range weekdayNames {
				if name == day.Selected {
					v := i
					c.DayOfWeek = &v
					break
				}
			}
		}
		if start.Selected != "Весь день" || end.Selected != "Весь день" {
			if start.Selected == "Весь день" || end.Selected == "Весь день" {
				showError(w, "Выберите номер урока в обоих полях")
				return
			}
			a, _ := strconv.Atoi(strings.TrimPrefix(start.Selected, "Урок "))
			b, _ := strconv.Atoi(strings.TrimPrefix(end.Selected, "Урок "))
			if a < 1 || b < a {
				showError(w, "Проверьте диапазон уроков")
				return
			}
			a--
			b--
			c.TimeslotStart = &a
			c.TimeslotEnd = &b
		}
		if _, err := store.CreateConstraint(c); err != nil {
			showError(w, "Не удалось добавить ограничение: "+err.Error())
			return
		}
		list.Refresh()
	})
	addUnavailable.Importance = widget.HighImportance

	maxRun := widget.NewEntry()
	maxRun.SetText("2")
	maxRun.SetPlaceHolder("Например: 2")
	runScope := widget.NewSelect([]string{"Все предметы", "Отдельный предмет"}, nil)
	runScope.SetSelected("Все предметы")
	subjectSelect := widget.NewSelect([]string{}, nil)
	subjectIDs := map[string]int{}
	loadSubjects := func() {
		items, err := store.ListSubjects(activeSchoolID)
		if err != nil {
			return
		}
		subjectIDs = make(map[string]int)
		opts := []string{}
		for _, item := range items {
			label := fmt.Sprintf("%s (#%d)", item.Name, item.ID)
			opts = append(opts, label)
			subjectIDs[label] = item.ID
		}
		subjectSelect.Options = opts
		if len(opts) > 0 {
			subjectSelect.SetSelected(opts[0])
		} else {
			subjectSelect.ClearSelected()
		}
		subjectSelect.Refresh()
	}
	loadSubjects()
	subjectSelect.Disable()
	runScope.OnChanged = func(value string) {
		if value == "Отдельный предмет" {
			subjectSelect.Enable()
		} else {
			subjectSelect.Disable()
		}
	}
	addRun := widget.NewButton("Добавить правило подряд", func() {
		n, err := strconv.Atoi(strings.TrimSpace(maxRun.Text))
		if err != nil || n < 1 || n > 14 {
			showError(w, "Укажите число уроков подряд от 1 до 14")
			return
		}
		c := domain.Constraint{SchoolID: activeSchoolID, Type: "max_consecutive_subject", EntityType: "school", Weight: n, IsHard: true}
		if runScope.Selected == "Отдельный предмет" {
			id := subjectIDs[subjectSelect.Selected]
			if id == 0 {
				showError(w, "Выберите предмет")
				return
			}
			c.EntityType = "subject"
			c.EntityID = id
		}
		if _, err := store.CreateConstraint(c); err != nil {
			showError(w, "Не удалось добавить правило: "+err.Error())
			return
		}
		list.Refresh()
	})
	return container.NewBorder(
		container.NewVBox(widget.NewLabelWithStyle("Ограничения расписания", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator()),
		nil, nil, nil,
		container.NewVSplit(
			container.NewVScroll(container.NewVBox(
				widget.NewLabelWithStyle("Запрет времени", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewForm(widget.NewFormItem("Тип", kind), widget.NewFormItem("Сущность", entity), widget.NewFormItem("День", day), widget.NewFormItem("С урока", start), widget.NewFormItem("По урок", end)),
				addUnavailable,
				widget.NewSeparator(),
				widget.NewLabelWithStyle("Не более уроков предмета подряд", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				widget.NewForm(widget.NewFormItem("Применить", runScope), widget.NewFormItem("Предмет", subjectSelect), widget.NewFormItem("Максимум", maxRun)),
				addRun,
			)),
			container.NewBorder(widget.NewLabelWithStyle("Сохранённые правила", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, list),
		),
	)
}

func describeConstraint(store *db.Store, c domain.Constraint) string {
	entity := fmt.Sprintf("%s #%d", c.EntityType, c.EntityID)
	switch c.EntityType {
	case "teacher":
		if xs, err := store.ListTeachers(c.SchoolID); err == nil {
			for _, x := range xs {
				if x.ID == c.EntityID {
					entity = x.Name
					break
				}
			}
		}
	case "class":
		if xs, err := store.ListClasses(c.SchoolID); err == nil {
			for _, x := range xs {
				if x.ID == c.EntityID {
					entity = x.Name
					break
				}
			}
		}
	case "room":
		if xs, err := store.ListRooms(c.SchoolID); err == nil {
			for _, x := range xs {
				if x.ID == c.EntityID {
					entity = x.Name
					break
				}
			}
		}
	case "subject":
		if xs, err := store.ListSubjects(c.SchoolID); err == nil {
			for _, x := range xs {
				if x.ID == c.EntityID {
					entity = x.Name
					break
				}
			}
		}
	case "school":
		entity = "все предметы"
	}
	if c.Type == "max_consecutive_subject" {
		return fmt.Sprintf("Не более %d уроков одного предмета подряд · %s", c.Weight, entity)
	}
	label := "Недоступность"
	if c.DayOfWeek != nil && *c.DayOfWeek >= 0 && *c.DayOfWeek < len(weekdayNames) {
		label += " · " + weekdayNames[*c.DayOfWeek]
	}
	if c.TimeslotStart != nil && c.TimeslotEnd != nil {
		label += fmt.Sprintf(" · уроки %d–%d", *c.TimeslotStart+1, *c.TimeslotEnd+1)
	} else if c.TimeslotStart != nil {
		label += fmt.Sprintf(" · с урока %d", *c.TimeslotStart+1)
	} else if c.TimeslotEnd != nil {
		label += fmt.Sprintf(" · до урока %d", *c.TimeslotEnd+1)
	}
	return fmt.Sprintf("%s · %s", entity, label)
}
