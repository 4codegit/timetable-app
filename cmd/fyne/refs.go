package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

// buildRefsView — вкладки справочников. AppTabs стоит в center у Border:
// активная вкладка растягивается на всё окно, а внутри каждой —
// виртуализованный widget.NewList, который скроллится сам.
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

	return container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Справочники", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
			container.NewBorder(
				nil, nil,
				widget.NewLabel("Школа:"), nil,
				schoolSelect,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
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
	var list *widget.List
	list = widget.NewList(
		func() int {
			if activeSchoolID == 0 {
				return 0
			}
			ts, _ := store.ListTeachers(activeSchoolID)
			return len(ts)
		},
		// Шаблон: Border(left=поля, center=кнопки).
		// Objects[0] — контейнер полей, Objects[1] — контейнер кнопок.
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			shortEd := widget.NewEntry()
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return container.NewBorder(
				nil, nil,
				container.NewHBox(nameEd, shortEd), nil,
				container.NewHBox(btnSave, btnDel),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if activeSchoolID == 0 {
				return
			}
			ts, err := store.ListTeachers(activeSchoolID)
			if err != nil || i >= len(ts) {
				return
			}
			t := ts[i]
			box := obj.(*fyne.Container)
			fields := box.Objects[0].(*fyne.Container)
			btns := box.Objects[1].(*fyne.Container)
			nameEd := fields.Objects[0].(*widget.Entry)
			shortEd := fields.Objects[1].(*widget.Entry)
			btnSave := btns.Objects[0].(*widget.Button)
			btnDel := btns.Objects[1].(*widget.Button)

			nameEd.SetText(t.Name)
			shortEd.SetText(t.ShortName)
			nameEd.Disable()
			shortEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				shortEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if err := store.UpdateTeacher(domain.Teacher{ID: t.ID, Name: nameEd.Text, ShortName: shortEd.Text}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					shortEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				if err := store.DeleteTeacher(t.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				list.Refresh()
			}
		},
	)

	nameEd := widget.NewEntry()
	nameEd.SetPlaceHolder("Имя")
	shortEd := widget.NewEntry()
	shortEd.SetPlaceHolder("Кратко")

	addBtn := widget.NewButton("Добавить учителя", func() {
		if activeSchoolID == 0 || nameEd.Text == "" {
			showError(w, "Выберите школу и введите имя")
			return
		}
		if _, err := store.CreateTeacher(domain.Teacher{SchoolID: activeSchoolID, Name: nameEd.Text, ShortName: shortEd.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEd.SetText("")
		shortEd.SetText("")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				container.NewHBox(nameEd, shortEd), nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
	)
}

// ---------- Предметы ----------

func buildSubjectsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	var list *widget.List
	list = widget.NewList(
		func() int {
			if activeSchoolID == 0 {
				return 0
			}
			subs, _ := store.ListSubjects(activeSchoolID)
			return len(subs)
		},
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			shortEd := widget.NewEntry()
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return container.NewBorder(
				nil, nil,
				container.NewHBox(nameEd, shortEd), nil,
				container.NewHBox(btnSave, btnDel),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if activeSchoolID == 0 {
				return
			}
			subs, err := store.ListSubjects(activeSchoolID)
			if err != nil || i >= len(subs) {
				return
			}
			s := subs[i]
			box := obj.(*fyne.Container)
			fields := box.Objects[0].(*fyne.Container)
			btns := box.Objects[1].(*fyne.Container)
			nameEd := fields.Objects[0].(*widget.Entry)
			shortEd := fields.Objects[1].(*widget.Entry)
			btnSave := btns.Objects[0].(*widget.Button)
			btnDel := btns.Objects[1].(*widget.Button)

			nameEd.SetText(s.Name)
			shortEd.SetText(s.ShortName)
			nameEd.Disable()
			shortEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				shortEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if err := store.UpdateSubject(domain.Subject{ID: s.ID, Name: nameEd.Text, ShortName: shortEd.Text}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					shortEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				if err := store.DeleteSubject(s.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				list.Refresh()
			}
		},
	)

	nameEd := widget.NewEntry()
	nameEd.SetPlaceHolder("Название предмета")
	shortEd := widget.NewEntry()
	shortEd.SetPlaceHolder("Кратко")

	addBtn := widget.NewButton("Добавить предмет", func() {
		if activeSchoolID == 0 || nameEd.Text == "" {
			showError(w, "Выберите школу и введите название")
			return
		}
		if _, err := store.CreateSubject(domain.Subject{SchoolID: activeSchoolID, Name: nameEd.Text, ShortName: shortEd.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEd.SetText("")
		shortEd.SetText("")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				container.NewHBox(nameEd, shortEd), nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
	)
}

// ---------- Классы ----------

func buildClassesView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	var list *widget.List
	list = widget.NewList(
		func() int {
			if activeSchoolID == 0 {
				return 0
			}
			classes, _ := store.ListClasses(activeSchoolID)
			return len(classes)
		},
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return container.NewBorder(
				nil, nil,
				nameEd, nil,
				container.NewHBox(btnSave, btnDel),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if activeSchoolID == 0 {
				return
			}
			classes, err := store.ListClasses(activeSchoolID)
			if err != nil || i >= len(classes) {
				return
			}
			c := classes[i]
			box := obj.(*fyne.Container)
			nameEd := box.Objects[0].(*widget.Entry)
			btns := box.Objects[1].(*fyne.Container)
			btnSave := btns.Objects[0].(*widget.Button)
			btnDel := btns.Objects[1].(*widget.Button)

			nameEd.SetText(c.Name)
			nameEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if err := store.UpdateClass(domain.SchoolClass{ID: c.ID, Name: nameEd.Text}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				if err := store.DeleteClass(c.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				list.Refresh()
			}
		},
	)

	nameEd := widget.NewEntry()
	nameEd.SetPlaceHolder("Название класса (например, 10А)")

	addBtn := widget.NewButton("Добавить класс", func() {
		if activeSchoolID == 0 || nameEd.Text == "" {
			showError(w, "Выберите школу и введите название класса")
			return
		}
		if _, err := store.CreateClass(domain.SchoolClass{SchoolID: activeSchoolID, Name: nameEd.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEd.SetText("")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				nameEd, nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
	)
}

// ---------- Кабинеты ----------

func buildRoomsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	var list *widget.List
	list = widget.NewList(
		func() int {
			if activeSchoolID == 0 {
				return 0
			}
			rooms, _ := store.ListRooms(activeSchoolID)
			return len(rooms)
		},
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return container.NewBorder(
				nil, nil,
				nameEd, nil,
				container.NewHBox(btnSave, btnDel),
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if activeSchoolID == 0 {
				return
			}
			rooms, err := store.ListRooms(activeSchoolID)
			if err != nil || i >= len(rooms) {
				return
			}
			r := rooms[i]
			box := obj.(*fyne.Container)
			nameEd := box.Objects[0].(*widget.Entry)
			btns := box.Objects[1].(*fyne.Container)
			btnSave := btns.Objects[0].(*widget.Button)
			btnDel := btns.Objects[1].(*widget.Button)

			nameEd.SetText(r.Name)
			nameEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if err := store.UpdateRoom(domain.Room{ID: r.ID, Name: nameEd.Text}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				if err := store.DeleteRoom(r.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				list.Refresh()
			}
		},
	)

	nameEd := widget.NewEntry()
	nameEd.SetPlaceHolder("Название кабинета")

	addBtn := widget.NewButton("Добавить кабинет", func() {
		if activeSchoolID == 0 || nameEd.Text == "" {
			showError(w, "Выберите школу и введите название кабинета")
			return
		}
		if _, err := store.CreateRoom(domain.Room{SchoolID: activeSchoolID, Name: nameEd.Text}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEd.SetText("")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				nameEd, nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
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
	hoursEd := widget.NewEntry()
	hoursEd.SetPlaceHolder("Часов/нед")

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

	var list *widget.List
	list = widget.NewList(
		func() int {
			if activeSchoolID == 0 {
				return 0
			}
			lessons, _ := store.ListLessons(activeSchoolID)
			return len(lessons)
		},
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return container.NewBorder(
				nil, nil,
				nameEd, nil,
				btnDel,
			)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if activeSchoolID == 0 {
				return
			}
			lessons, err := store.ListLessons(activeSchoolID)
			if err != nil || i >= len(lessons) {
				return
			}
			l := lessons[i]
			box := obj.(*fyne.Container)
			nameEd := box.Objects[0].(*widget.Entry)
			btnDel := box.Objects[1].(*widget.Button)

			nameEd.SetText(fmt.Sprintf("ID %d: %s → %s, %d ч/нед", l.ID, getClassName(classes, l.ClassID), getSubjName(subjects, l.SubjectID), l.HoursPerWeek))
			nameEd.Disable()

			btnDel.OnTapped = func() {
				if err := store.DeleteLesson(l.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				list.Refresh()
			}
		},
	)

	addBtn := widget.NewButton("Добавить урок", func() {
		if activeSchoolID == 0 || classSelect.Selected == "" || subjectSelect.Selected == "" || teacherSelect.Selected == "" {
			showError(w, "Выберите все поля")
			return
		}
		hours := 1
		fmt.Sscanf(hoursEd.Text, "%d", &hours)
		if hours < 1 {
			hours = 1
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
		if _, err := store.CreateLesson(domain.Lesson{SchoolID: activeSchoolID, ClassID: classID, SubjectID: subjectID, TeacherID: teacherID, HoursPerWeek: hours}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		hoursEd.SetText("")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				container.NewHBox(classSelect, subjectSelect, teacherSelect, hoursEd), nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
	)
}

func getClassName(classes []domain.SchoolClass, id int) string {
	for _, c := range classes {
		if c.ID == id {
			return c.Name
		}
	}
	return "?"
}

func getSubjName(subjects []domain.Subject, id int) string {
	for _, s := range subjects {
		if s.ID == id {
			return s.Name
		}
	}
	return "?"
}
