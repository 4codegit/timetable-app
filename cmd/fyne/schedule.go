package main

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

func buildScheduleView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return emptySchoolView("Расписание")
	}
	schoolID := activeSchoolID
	settings := loadFyneSettings(store, schoolID)
	dayIndexes := make([]int, 0, 7)
	for d := 0; d < 7; d++ {
		if settings.DaysMask&(1<<d) != 0 {
			dayIndexes = append(dayIndexes, d)
		}
	}
	if len(dayIndexes) == 0 {
		dayIndexes = []int{0}
	}
	classes, _ := store.ListClasses(schoolID)
	teachers, _ := store.ListTeachers(schoolID)
	rooms, _ := store.ListRooms(schoolID)
	subjects, _ := store.ListSubjects(schoolID)
	classByID := map[int]domain.SchoolClass{}
	teacherByID := map[int]domain.Teacher{}
	roomByID := map[int]domain.Room{}
	subjectByID := map[int]domain.Subject{}
	for _, x := range classes {
		classByID[x.ID] = x
	}
	for _, x := range teachers {
		teacherByID[x.ID] = x
	}
	for _, x := range rooms {
		roomByID[x.ID] = x
	}
	for _, x := range subjects {
		subjectByID[x.ID] = x
	}
	mode := widget.NewSelect([]string{"Класс", "Учитель", "Кабинет"}, nil)
	mode.SetSelected("Класс")
	entity := widget.NewSelect([]string{}, nil)
	entityIDByLabel := map[string]int{}
	selectedMode, selectedID := "class", 0
	rowsForMode := func(m string) []struct {
		id   int
		name string
	} {
		rows := []struct {
			id   int
			name string
		}{}
		switch m {
		case "Класс":
			for _, x := range classes {
				rows = append(rows, struct {
					id   int
					name string
				}{x.ID, x.Name})
			}
		case "Учитель":
			for _, x := range teachers {
				rows = append(rows, struct {
					id   int
					name string
				}{x.ID, x.Name})
			}
		case "Кабинет":
			for _, x := range rooms {
				rows = append(rows, struct {
					id   int
					name string
				}{x.ID, x.Name})
			}
		}
		sort.Slice(rows, func(i, j int) bool { return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name) })
		return rows
	}
	loadEntities := func() {
		entityIDByLabel = make(map[string]int)
		rows := rowsForMode(mode.Selected)
		options := make([]string, 0, len(rows))
		for _, r := range rows {
			label := fmt.Sprintf("%s  (#%d)", r.name, r.id)
			options = append(options, label)
			entityIDByLabel[label] = r.id
		}
		entity.Options = options
		if len(options) > 0 {
			entity.SetSelected(options[0])
			selectedID = rows[0].id
		} else {
			entity.ClearSelected()
			selectedID = 0
		}
		selectedMode = map[string]string{"Класс": "class", "Учитель": "teacher", "Кабинет": "room"}[mode.Selected]
		entity.Refresh()
	}
	var entries []domain.ScheduleEntry
	loadSchedule := func() error { var err error; entries, err = store.ListSchedule(schoolID); return err }
	if err := loadSchedule(); err != nil {
		showError(w, "Не удалось загрузить расписание: "+err.Error())
	}
	status := widget.NewLabel("Выберите урок, затем ячейку назначения. Для удаления выберите урок и нажмите «Удалить».")
	selectedEntryID := 0
	var undoStack [][]domain.ScheduleEntry
	pushUndo := func() {
		cp := append([]domain.ScheduleEntry(nil), entries...)
		undoStack = append(undoStack, cp)
		if len(undoStack) > 30 {
			undoStack = undoStack[1:]
		}
	}
	undoButton := widget.NewButton("↶ Отменить", nil)
	undoButton.Disable()
	removeButton := widget.NewButton("Удалить урок", nil)
	removeButton.Disable()
	var table *widget.Table
	classMatches := func(classID int) bool {
		if selectedMode != "class" {
			return false
		}
		c, ok := classByID[classID]
		return classID == selectedID || ok && c.SubgroupOf != nil && *c.SubgroupOf == selectedID
	}
	cellEntries := func(day, slot int) []domain.ScheduleEntry {
		out := make([]domain.ScheduleEntry, 0, 2)
		for _, e := range entries {
			if e.DayOfWeek != day || e.Timeslot != slot {
				continue
			}
			switch selectedMode {
			case "class":
				if !classMatches(e.ClassID) {
					continue
				}
			case "teacher":
				if e.TeacherID != selectedID {
					continue
				}
			case "room":
				if e.RoomID != selectedID {
					continue
				}
			default:
				continue
			}
			out = append(out, e)
		}
		return out
	}
	entryLabel := func(e domain.ScheduleEntry) string {
		subject := subjectByID[e.SubjectID].Name
		if subject == "" {
			subject = "Предмет?"
		}
		teacher := teacherByID[e.TeacherID].ShortName
		if teacher == "" {
			teacher = teacherByID[e.TeacherID].Name
		}
		parts := []string{subject}
		if teacher != "" {
			parts = append(parts, teacher)
		}
		if room := roomByID[e.RoomID].Name; room != "" {
			parts = append(parts, "каб. "+room)
		}
		if selectedMode != "class" {
			parts = append(parts, classByID[e.ClassID].Name)
		}
		return strings.Join(parts, "\n")
	}
	refresh := func() {
		if err := loadSchedule(); err != nil {
			showError(w, "Не удалось обновить расписание: "+err.Error())
			return
		}
		selectedEntryID = 0
		removeButton.Disable()
		status.SetText("Расписание обновлено. Выберите урок и целевую ячейку для переноса.")
		if table != nil {
			table.Refresh()
		}
	}
	selectEntry := func(e domain.ScheduleEntry) {
		selectedEntryID = e.ID
		removeButton.Enable()
		status.SetText(fmt.Sprintf("Выбран урок: %s · %s, %s, урок %d. Выберите ячейку назначения.", subjectByID[e.SubjectID].Name, classByID[e.ClassID].Name, weekdayNames[e.DayOfWeek], e.Timeslot+1))
		if table != nil {
			table.Refresh()
		}
	}
	var chooseEntry func([]domain.ScheduleEntry, func(domain.ScheduleEntry))
	chooseEntry = func(matches []domain.ScheduleEntry, picked func(domain.ScheduleEntry)) {
		if len(matches) == 1 {
			picked(matches[0])
			return
		}
		options := make([]string, len(matches))
		byLabel := make(map[string]domain.ScheduleEntry, len(matches))
		for i, e := range matches {
			label := fmt.Sprintf("%s — %s (#%d)", subjectByID[e.SubjectID].Name, classByID[e.ClassID].Name, e.ID)
			options[i] = label
			byLabel[label] = e
		}
		selectWidget := widget.NewSelect(options, nil)
		if len(options) > 0 {
			selectWidget.SetSelected(options[0])
		}
		dialog.ShowForm("Выберите урок", "Выбрать", "Отмена", []*widget.FormItem{widget.NewFormItem("Уроки в ячейке", selectWidget)}, func(ok bool) {
			if ok {
				if e, found := byLabel[selectWidget.Selected]; found {
					picked(e)
				}
			}
		}, w)
	}
	moveTo := func(day, slot int) {
		sourceIndex := -1
		for i, e := range entries {
			if e.ID == selectedEntryID {
				sourceIndex = i
				break
			}
		}
		if sourceIndex < 0 {
			status.SetText("Сначала выберите урок для переноса.")
			return
		}
		src := entries[sourceIndex]
		if src.DayOfWeek == day && src.Timeslot == slot {
			var alternatives []domain.ScheduleEntry
			for _, candidate := range cellEntries(day, slot) {
				if candidate.ID != src.ID {
					alternatives = append(alternatives, candidate)
				}
			}
			if len(alternatives) > 0 {
				chooseEntry(alternatives, selectEntry)
			}
			return
		}
		targets := cellEntries(day, slot)
		apply := func(target *domain.ScheduleEntry) {
			pushUndo()
			var err error
			if target == nil {
				err = store.MoveEntry(src.ID, day, slot)
			} else {
				err = store.SwapEntries(src.ID, day, slot, target.ID, src.DayOfWeek, src.Timeslot)
			}
			if err != nil {
				undoStack = undoStack[:len(undoStack)-1]
				showError(w, "Не удалось изменить расписание: "+err.Error())
				return
			}
			undoButton.Enable()
			selectedEntryID = 0
			removeButton.Disable()
			if err := loadSchedule(); err != nil {
				showError(w, err.Error())
				return
			}
			if table != nil {
				table.Refresh()
			}
			if target == nil {
				status.SetText(fmt.Sprintf("Урок перенесён на %s, урок %d.", weekdayNames[day], slot+1))
			} else {
				status.SetText(fmt.Sprintf("Уроки поменяны местами: %s, урок %d.", weekdayNames[day], slot+1))
			}
		}
		if len(targets) == 0 {
			apply(nil)
			return
		}
		chooseEntry(targets, func(e domain.ScheduleEntry) { apply(&e) })
	}
	table = widget.NewTableWithHeaders(
		func() (int, int) { return settings.Slots, len(dayIndexes) },
		func() fyne.CanvasObject { l := widget.NewLabel(""); l.Wrapping = fyne.TextWrapWord; return l },
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			if id.Row < 0 || id.Col < 0 || id.Row >= settings.Slots || id.Col >= len(dayIndexes) {
				return
			}
			matches := cellEntries(dayIndexes[id.Col], id.Row)
			lines := make([]string, 0, len(matches))
			for _, e := range matches {
				lines = append(lines, entryLabel(e))
			}
			label := obj.(*widget.Label)
			label.SetText(strings.Join(lines, "\n────────\n"))
			if len(matches) > 0 && selectedEntryID > 0 {
				for _, e := range matches {
					if e.ID == selectedEntryID {
						label.TextStyle.Bold = true
						label.Refresh()
						return
					}
				}
			}
			label.TextStyle.Bold = false
		},
	)
	table.ShowHeaderRow, table.ShowHeaderColumn = true, true
	table.CreateHeader = func() fyne.CanvasObject {
		l := widget.NewLabel("")
		l.TextStyle.Bold = true
		return container.NewPadded(l)
	}
	table.UpdateHeader = func(id widget.TableCellID, obj fyne.CanvasObject) {
		label := obj.(*fyne.Container).Objects[0].(*widget.Label)
		if id.Row == -1 && id.Col == -1 {
			label.SetText("Урок")
		} else if id.Row == -1 && id.Col >= 0 && id.Col < len(dayIndexes) {
			label.SetText(weekdayNames[dayIndexes[id.Col]])
		} else if id.Col == -1 && id.Row >= 0 && id.Row < settings.Slots {
			p := settings.Periods[id.Row]
			text := fmt.Sprintf("%d-й урок", id.Row+1)
			if p.Start != "" {
				text += "\n" + p.Start
				if p.End != "" {
					text += "–" + p.End
				}
			}
			label.SetText(text)
		}
	}
	table.OnSelected = func(pos widget.TableCellID) {
		if pos.Row < 0 || pos.Col < 0 || pos.Row >= settings.Slots || pos.Col >= len(dayIndexes) {
			return
		}
		day, slot := dayIndexes[pos.Col], pos.Row
		if selectedEntryID > 0 {
			moveTo(day, slot)
			return
		}
		matches := cellEntries(day, slot)
		if len(matches) == 0 {
			status.SetText("В ячейке нет урока. Сначала выберите урок в другой ячейке.")
			return
		}
		chooseEntry(matches, selectEntry)
	}
	for i := range dayIndexes {
		table.SetColumnWidth(i, 190)
	}
	for i := 0; i < settings.Slots; i++ {
		table.SetRowHeight(i, 82)
	}
	entity.OnChanged = func(label string) {
		selectedID = entityIDByLabel[label]
		selectedEntryID = 0
		removeButton.Disable()
		table.Refresh()
	}
	mode.OnChanged = func(string) { loadEntities(); table.Refresh() }
	removeButton.OnTapped = func() {
		if selectedEntryID == 0 {
			return
		}
		dialog.NewConfirm("Удалить урок из расписания?", "Урок останется в учебном плане и может быть расставлен повторно.", func(ok bool) {
			if !ok {
				return
			}
			pushUndo()
			if err := store.DeleteScheduleEntry(selectedEntryID); err != nil {
				undoStack = undoStack[:len(undoStack)-1]
				showError(w, err.Error())
				return
			}
			undoButton.Enable()
			selectedEntryID = 0
			removeButton.Disable()
			refresh()
		}, w).Show()
	}
	undoButton.OnTapped = func() {
		if len(undoStack) == 0 {
			return
		}
		previous := undoStack[len(undoStack)-1]
		if err := store.ReplaceSchedule(schoolID, previous); err != nil {
			showError(w, "Не удалось отменить: "+err.Error())
			return
		}
		undoStack = undoStack[:len(undoStack)-1]
		if len(undoStack) == 0 {
			undoButton.Disable()
		}
		refresh()
		status.SetText("Последнее изменение отменено.")
	}
	refreshButton := widget.NewButton("Обновить", refresh)
	refreshButton.Importance = widget.LowImportance
	loadEntities()
	return container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Расписание", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(widget.NewLabel("Показывать:"), mode, entity, refreshButton, undoButton, removeButton),
			status,
			widget.NewSeparator(),
		), nil, nil, nil,
		table,
	)
}
