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

func buildRefsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	schoolSelect := widget.NewSelect([]string{}, func(s string) {
		schools, err := store.ListSchools()
		if err != nil {
			return
		}
		for _, sc := range schools {
			if sc.Name == s {
				setActiveSchoolID(sc.ID)
				break
			}
		}
	})
	refreshSchoolsSelect(store, schoolSelect)

	tabs := container.NewAppTabs(
		container.NewTabItem("Учителя", buildTeachersView(store, w)),
		container.NewTabItem("Предметы", buildSubjectsView(store, w)),
		container.NewTabItem("Классы", buildClassesView(store, w)),
		container.NewTabItem("Кабинеты", buildRoomsView(store, w)),
		container.NewTabItem("Уроки", buildLessonsView(store, w)),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	return container.NewVBox(
		widget.NewLabel("Справочники"),
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabel("Школа:"),
			schoolSelect,
		),
		widget.NewSeparator(),
		tabs,
	)
}

func refreshSchoolsSelect(store *db.Store, selectWidget *widget.Select) {
	schools, err := store.ListSchools()
	if err != nil {
		return
	}
	var names []string
	for _, sc := range schools {
		names = append(names, sc.Name)
	}
	selectWidget.Options = names
	if len(names) > 0 && selectWidget.Selected == "" {
		selectWidget.Selected = names[0]
		selectWidget.Refresh()
		setActiveSchoolID(schools[0].ID)
	}
}

// ---------- Учителя ----------

func buildTeachersView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	listData := binding.NewStringList()
	list := widget.NewListWithData(
		listData,
		func() fyne.CanvasObject { return widget.NewEntry() },
		func(i binding.DataItem, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			name, _ := i.(binding.String).Get()
			entry.SetText(name)
			entry.Disable()
		},
	)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Имя")
	shortEntry := widget.NewEntry()
	shortEntry.SetPlaceHolder("Кратко")

	refresh := func() {
		listData.Set([]string{})
		if activeSchoolID == 0 {
			return
		}
		teachers, err := store.ListTeachers(activeSchoolID)
		if err != nil {
			return
		}
		for _, t := range teachers {
			listData.Append(t.Name)
		}
	}

	add := func() {
		if activeSchoolID == 0 || nameEntry.Text == "" {
			return
		}
		if _, err := store.CreateTeacher(domain.Teacher{SchoolID: activeSchoolID, Name: nameEntry.Text, ShortName: shortEntry.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEntry.SetText("")
		shortEntry.SetText("")
		refresh()
	}

	del := func() {
		sel, _ := listData.Get()
		if len(sel) == 0 || activeSchoolID == 0 {
			return
		}
		teachers, err := store.ListTeachers(activeSchoolID)
		if err != nil {
			return
		}
		for _, t := range teachers {
			if len(sel) > 0 && t.Name == sel[0] {
				if err := store.DeleteTeacher(t.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				break
			}
		}
		refresh()
	}

	refresh()

	return container.NewVBox(
		widget.NewLabel("Учителя"),
		widget.NewSeparator(),
		container.NewHBox(nameEntry, shortEntry, widget.NewButton("+", add), widget.NewButton("Удалить", del)),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}

// ---------- Предметы ----------

func buildSubjectsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	listData := binding.NewStringList()
	list := widget.NewListWithData(
		listData,
		func() fyne.CanvasObject { return widget.NewEntry() },
		func(i binding.DataItem, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			name, _ := i.(binding.String).Get()
			entry.SetText(name)
			entry.Disable()
		},
	)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название предмета")
	shortEntry := widget.NewEntry()
	shortEntry.SetPlaceHolder("Кратко")

	refresh := func() {
		listData.Set([]string{})
		if activeSchoolID == 0 {
			return
		}
		subs, err := store.ListSubjects(activeSchoolID)
		if err != nil {
			return
		}
		for _, s := range subs {
			listData.Append(s.Name)
		}
	}

	add := func() {
		if activeSchoolID == 0 || nameEntry.Text == "" {
			return
		}
		if _, err := store.CreateSubject(domain.Subject{SchoolID: activeSchoolID, Name: nameEntry.Text, ShortName: shortEntry.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEntry.SetText("")
		shortEntry.SetText("")
		refresh()
	}

	del := func() {
		sel, _ := listData.Get()
		if len(sel) == 0 || activeSchoolID == 0 {
			return
		}
		subs, err := store.ListSubjects(activeSchoolID)
		if err != nil {
			return
		}
		for _, s := range subs {
			if len(sel) > 0 && s.Name == sel[0] {
				if err := store.DeleteSubject(s.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				break
			}
		}
		refresh()
	}

	refresh()

	return container.NewVBox(
		widget.NewLabel("Предметы"),
		widget.NewSeparator(),
		container.NewHBox(nameEntry, shortEntry, widget.NewButton("+", add), widget.NewButton("Удалить", del)),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}

// ---------- Классы ----------

func buildClassesView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	listData := binding.NewStringList()
	list := widget.NewListWithData(
		listData,
		func() fyne.CanvasObject { return widget.NewEntry() },
		func(i binding.DataItem, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			name, _ := i.(binding.String).Get()
			entry.SetText(name)
			entry.Disable()
		},
	)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название класса (например, 10А)")

	refresh := func() {
		listData.Set([]string{})
		if activeSchoolID == 0 {
			return
		}
		classes, err := store.ListClasses(activeSchoolID)
		if err != nil {
			return
		}
		for _, c := range classes {
			label := c.Name
			if c.SubgroupOf != nil {
				label += " (подгруппа)"
			}
			listData.Append(label)
		}
	}

	add := func() {
		if activeSchoolID == 0 || nameEntry.Text == "" {
			return
		}
		if _, err := store.CreateClass(domain.SchoolClass{SchoolID: activeSchoolID, Name: nameEntry.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEntry.SetText("")
		refresh()
	}

	del := func() {
		sel, _ := listData.Get()
		if len(sel) == 0 || activeSchoolID == 0 {
			return
		}
		classes, err := store.ListClasses(activeSchoolID)
		if err != nil {
			return
		}
		for _, c := range classes {
			label := c.Name
			if c.SubgroupOf != nil {
				label += " (подгруппа)"
			}
			if len(sel) > 0 && label == sel[0] {
				if err := store.DeleteClass(c.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				break
			}
		}
		refresh()
	}

	refresh()

	return container.NewVBox(
		widget.NewLabel("Классы"),
		widget.NewSeparator(),
		container.NewHBox(nameEntry, widget.NewButton("+", add), widget.NewButton("Удалить", del)),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}

// ---------- Кабинеты ----------

func buildRoomsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	listData := binding.NewStringList()
	list := widget.NewListWithData(
		listData,
		func() fyne.CanvasObject { return widget.NewEntry() },
		func(i binding.DataItem, obj fyne.CanvasObject) {
			entry := obj.(*widget.Entry)
			name, _ := i.(binding.String).Get()
			entry.SetText(name)
			entry.Disable()
		},
	)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название кабинета")

	refresh := func() {
		listData.Set([]string{})
		if activeSchoolID == 0 {
			return
		}
		rooms, err := store.ListRooms(activeSchoolID)
		if err != nil {
			return
		}
		for _, r := range rooms {
			listData.Append(r.Name)
		}
	}

	add := func() {
		if activeSchoolID == 0 || nameEntry.Text == "" {
			return
		}
		if _, err := store.CreateRoom(domain.Room{SchoolID: activeSchoolID, Name: nameEntry.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEntry.SetText("")
		refresh()
	}

	del := func() {
		sel, _ := listData.Get()
		if len(sel) == 0 || activeSchoolID == 0 {
			return
		}
		rooms, err := store.ListRooms(activeSchoolID)
		if err != nil {
			return
		}
		for _, r := range rooms {
			if len(sel) > 0 && r.Name == sel[0] {
				if err := store.DeleteRoom(r.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				break
			}
		}
		refresh()
	}

	refresh()

	return container.NewVBox(
		widget.NewLabel("Кабинеты"),
		widget.NewSeparator(),
		container.NewHBox(nameEntry, widget.NewButton("+", add), widget.NewButton("Удалить", del)),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}

// ---------- Уроки ----------

func buildLessonsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	var classes []domain.SchoolClass
	var subjects []domain.Subject
	var teachers []domain.Teacher

	loadRefs := func() {
		var err error
		classes, err = store.ListClasses(activeSchoolID)
		if err != nil {
			return
		}
		subjects, err = store.ListSubjects(activeSchoolID)
		if err != nil {
			return
		}
		teachers, err = store.ListTeachers(activeSchoolID)
		if err != nil {
			return
		}
	}

	classSelect := widget.NewSelect([]string{}, func(string) {})
	subjectSelect := widget.NewSelect([]string{}, func(string) {})
	teacherSelect := widget.NewSelect([]string{}, func(string) {})
	hoursEntry := widget.NewEntry()
	hoursEntry.SetPlaceHolder("Часов/нед (1-20)")

	loadRefs()
	updateSelects := func() {
		classOpts := make([]string, 0, len(classes))
		for _, c := range classes {
			classOpts = append(classOpts, c.Name)
		}
		classSelect.Options = classOpts
		if len(classOpts) > 0 {
			classSelect.Selected = classOpts[0]
		}
		subjOpts := make([]string, 0, len(subjects))
		for _, s := range subjects {
			subjOpts = append(subjOpts, s.Name)
		}
		subjectSelect.Options = subjOpts
		if len(subjOpts) > 0 {
			subjectSelect.Selected = subjOpts[0]
		}
		teachOpts := make([]string, 0, len(teachers))
		for _, t := range teachers {
			teachOpts = append(teachOpts, t.Name)
		}
		teacherSelect.Options = teachOpts
		if len(teachOpts) > 0 {
			teacherSelect.Selected = teachOpts[0]
		}
	}
	updateSelects()

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
		if activeSchoolID == 0 {
			return
		}
		lessons, err := store.ListLessons(activeSchoolID)
		if err != nil {
			return
		}
		for _, l := range lessons {
			listData.Append(fmt.Sprintf("ID %d: %d ч/нед", l.ID, l.HoursPerWeek))
		}
	}

	add := func() {
		if activeSchoolID == 0 || classSelect.Selected == "" || subjectSelect.Selected == "" || teacherSelect.Selected == "" {
			return
		}
		hours, err := strconv.Atoi(hoursEntry.Text)
		if err != nil || hours < 1 {
			hours = 1
		}
		if hours > 20 {
			hours = 20
		}
		var classID, subjectID, teacherID int
		for _, c := range classes {
			if c.Name == classSelect.Selected {
				classID = c.ID
				break
			}
		}
		for _, s := range subjects {
			if s.Name == subjectSelect.Selected {
				subjectID = s.ID
				break
			}
		}
		for _, t := range teachers {
			if t.Name == teacherSelect.Selected {
				teacherID = t.ID
				break
			}
		}
		if _, err := store.CreateLesson(domain.Lesson{
			SchoolID:     activeSchoolID,
			ClassID:      classID,
			SubjectID:    subjectID,
			TeacherID:    teacherID,
			HoursPerWeek: hours,
		}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		hoursEntry.SetText("")
		refresh()
	}

	del := func() {
		sel, _ := listData.Get()
		if len(sel) == 0 || activeSchoolID == 0 {
			return
		}
		lessons, err := store.ListLessons(activeSchoolID)
		if err != nil {
			return
		}
		for _, l := range lessons {
			label := fmt.Sprintf("ID %d: %d ч/нед", l.ID, l.HoursPerWeek)
			if len(sel) > 0 && label == sel[0] {
				if err := store.DeleteLesson(l.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				break
			}
		}
		refresh()
	}

	refresh()

	return container.NewVBox(
		widget.NewLabel("Уроки"),
		widget.NewSeparator(),
		container.NewHBox(classSelect, subjectSelect, teacherSelect, hoursEntry, widget.NewButton("+", add), widget.NewButton("Удалить", del)),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}
