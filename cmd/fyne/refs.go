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

// buildRefsView — вкладки справочников. AppTabs стоит в center у Border:
// активная вкладка растягивается на всё окно, а внутри каждой —
// виртуализованный widget.NewList, который скроллится сам.
func buildRefsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
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
		),
		nil, nil, nil,
		tabs,
	)
}

// ---------- Учителя ----------

// teacherRow — шаблон строки списка учителей с типизированными ссылками
// (позиционное индексирование Objects у Border ненадёжно).
type teacherRow struct {
	*fyne.Container
	name, short *widget.Entry
	save, del   *widget.Button
}

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
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			shortEd := widget.NewEntry()
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return &teacherRow{
				Container: container.NewBorder(
					nil, nil,
					container.NewHBox(nameEd, shortEd), nil,
					container.NewHBox(btnSave, btnDel),
				),
				name: nameEd, short: shortEd, save: btnSave, del: btnDel,
			}
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
			row := obj.(*teacherRow)
			nameEd := row.name
			shortEd := row.short
			btnSave := row.save
			btnDel := row.del

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
				dialog.NewConfirm("Удалить учителя?", "Связанные уроки и записи расписания тоже будут удалены.", func(ok bool) {
					if !ok {
						return
					}
					if err := store.DeleteTeacher(t.ID); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					list.Refresh()
				}, w).Show()
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

// subjectRow — шаблон строки списка предметов.
type subjectRow struct {
	*fyne.Container
	name, short, roomType *widget.Entry
	save, del             *widget.Button
}

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
			roomTypeEd := widget.NewEntry()
			roomTypeEd.SetPlaceHolder("тип кабинета / any")
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return &subjectRow{
				Container: container.NewBorder(
					nil, nil,
					container.NewHBox(nameEd, shortEd, roomTypeEd), nil,
					container.NewHBox(btnSave, btnDel),
				),
				name: nameEd, short: shortEd, roomType: roomTypeEd, save: btnSave, del: btnDel,
			}
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
			row := obj.(*subjectRow)
			nameEd := row.name
			shortEd := row.short
			roomTypeEd := row.roomType
			btnSave := row.save
			btnDel := row.del

			nameEd.SetText(s.Name)
			shortEd.SetText(s.ShortName)
			roomTypeEd.SetText(s.RequiresRoomType)
			nameEd.Disable()
			shortEd.Disable()
			roomTypeEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				shortEd.Enable()
				roomTypeEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if strings.TrimSpace(nameEd.Text) == "" {
						showError(w, "Укажите название предмета")
						return
					}
					if err := store.UpdateSubject(domain.Subject{ID: s.ID, SchoolID: s.SchoolID, Name: strings.TrimSpace(nameEd.Text), ShortName: strings.TrimSpace(shortEd.Text), RequiresRoomType: strings.TrimSpace(roomTypeEd.Text)}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					shortEd.Disable()
					roomTypeEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				dialog.NewConfirm("Удалить предмет?", "Связанные уроки и записи расписания тоже будут удалены.", func(ok bool) {
					if !ok {
						return
					}
					if err := store.DeleteSubject(s.ID); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					list.Refresh()
				}, w).Show()
			}
		},
	)

	nameEd := widget.NewEntry()
	nameEd.SetPlaceHolder("Название предмета")
	shortEd := widget.NewEntry()
	shortEd.SetPlaceHolder("Кратко")
	roomTypeEd := widget.NewEntry()
	roomTypeEd.SetText("any")
	roomTypeEd.SetPlaceHolder("тип кабинета / any")

	addBtn := widget.NewButton("Добавить предмет", func() {
		if activeSchoolID == 0 || strings.TrimSpace(nameEd.Text) == "" {
			showError(w, "Выберите школу и введите название")
			return
		}
		if _, err := store.CreateSubject(domain.Subject{SchoolID: activeSchoolID, Name: strings.TrimSpace(nameEd.Text), ShortName: strings.TrimSpace(shortEd.Text), RequiresRoomType: strings.TrimSpace(roomTypeEd.Text)}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEd.SetText("")
		shortEd.SetText("")
		roomTypeEd.SetText("any")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				container.NewHBox(nameEd, shortEd, roomTypeEd), nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
	)
}

// ---------- Классы ----------

// classRow — шаблон строки списка классов.
type classRow struct {
	*fyne.Container
	name         *widget.Entry
	room, parent *widget.Select
	save, del    *widget.Button
}

func buildClassesView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	roomOptions := []string{"Без кабинета"}
	roomIDs := map[string]int{"Без кабинета": 0}
	if rooms, err := store.ListRooms(activeSchoolID); err == nil {
		for _, room := range rooms {
			label := fmt.Sprintf("%s (#%d)", room.Name, room.ID)
			roomOptions = append(roomOptions, label)
			roomIDs[label] = room.ID
		}
	}
	parentOptions := []string{"Обычный класс"}
	parentIDs := map[string]int{"Обычный класс": 0}
	if existing, err := store.ListClasses(activeSchoolID); err == nil {
		for _, parent := range existing {
			if parent.SubgroupOf == nil {
				label := fmt.Sprintf("Подгруппа класса %s (#%d)", parent.Name, parent.ID)
				parentOptions = append(parentOptions, label)
				parentIDs[label] = parent.ID
			}
		}
	}
	roomSelect := widget.NewSelect(roomOptions, nil)
	roomSelect.SetSelected(roomOptions[0])
	parentSelect := widget.NewSelect(parentOptions, nil)
	parentSelect.SetSelected(parentOptions[0])
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
			roomEd := widget.NewSelect([]string{}, nil)
			parentEd := widget.NewSelect([]string{}, nil)
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return &classRow{
				Container: container.NewBorder(
					nil, nil,
					container.NewHBox(nameEd, roomEd, parentEd), nil,
					container.NewHBox(btnSave, btnDel),
				),
				name: nameEd, room: roomEd, parent: parentEd, save: btnSave, del: btnDel,
			}
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
			rooms, _ := store.ListRooms(activeSchoolID)
			row := obj.(*classRow)
			nameEd := row.name
			roomEd := row.room
			parentEd := row.parent
			btnSave := row.save
			btnDel := row.del

			nameEd.SetText(c.Name)
			roomIDs := map[string]int{"Без кабинета": 0}
			roomOpts := []string{"Без кабинета"}
			for _, room := range rooms {
				label := fmt.Sprintf("%s (#%d)", room.Name, room.ID)
				roomOpts = append(roomOpts, label)
				roomIDs[label] = room.ID
			}
			roomEd.Options = roomOpts
			roomEd.SetSelected("Без кабинета")
			for label, id := range roomIDs {
				if id == c.RoomID {
					roomEd.SetSelected(label)
					break
				}
			}
			parentOpts := []string{"Обычный класс"}
			parentIDs := map[string]int{"Обычный класс": 0}
			hasChildren := false
			for _, child := range classes {
				if child.SubgroupOf != nil && *child.SubgroupOf == c.ID {
					hasChildren = true
					break
				}
			}
			for _, candidate := range classes {
				if candidate.SubgroupOf != nil || candidate.ID == c.ID || hasChildren {
					continue
				}
				label := fmt.Sprintf("Подгруппа класса %s (#%d)", candidate.Name, candidate.ID)
				parentOpts = append(parentOpts, label)
				parentIDs[label] = candidate.ID
			}
			parentEd.Options = parentOpts
			parentEd.SetSelected(parentOpts[0])
			if c.SubgroupOf != nil {
				for label, id := range parentIDs {
					if id == *c.SubgroupOf {
						parentEd.SetSelected(label)
						break
					}
				}
			}
			roomEd.Refresh()
			parentEd.Refresh()
			nameEd.Disable()
			roomEd.Disable()
			parentEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				roomEd.Enable()
				parentEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if strings.TrimSpace(nameEd.Text) == "" {
						showError(w, "Укажите название класса")
						return
					}
					updated := domain.SchoolClass{ID: c.ID, SchoolID: c.SchoolID, Name: strings.TrimSpace(nameEd.Text), RoomID: roomIDs[roomEd.Selected]}
					if parentID := parentIDs[parentEd.Selected]; parentID != 0 {
						updated.SubgroupOf = &parentID
					}
					if err := store.UpdateClass(updated); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					roomEd.Disable()
					parentEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				dialog.NewConfirm("Удалить класс?", "Уроки, подгруппы и записи расписания этого класса будут удалены.", func(ok bool) {
					if !ok {
						return
					}
					if err := store.DeleteClass(c.ID); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					list.Refresh()
				}, w).Show()
			}
		},
	)

	nameEd := widget.NewEntry()
	nameEd.SetPlaceHolder("Название класса (например, 10А)")

	addBtn := widget.NewButton("Добавить класс", func() {
		if activeSchoolID == 0 || strings.TrimSpace(nameEd.Text) == "" {
			showError(w, "Выберите школу и введите название класса")
			return
		}
		class := domain.SchoolClass{SchoolID: activeSchoolID, Name: strings.TrimSpace(nameEd.Text), RoomID: roomIDs[roomSelect.Selected]}
		if parentID := parentIDs[parentSelect.Selected]; parentID != 0 {
			class.SubgroupOf = &parentID
		}
		if _, err := store.CreateClass(class); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEd.SetText("")
		roomSelect.SetSelected(roomOptions[0])
		parentSelect.SetSelected(parentOptions[0])
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				container.NewHBox(nameEd, roomSelect, parentSelect), nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
	)
}

// ---------- Кабинеты ----------

// roomRow — шаблон строки списка кабинетов.
type roomRow struct {
	*fyne.Container
	name, roomType *widget.Entry
	save, del      *widget.Button
}

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
			roomTypeEd := widget.NewEntry()
			roomTypeEd.SetPlaceHolder("тип / any")
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return &roomRow{
				Container: container.NewBorder(
					nil, nil,
					container.NewHBox(nameEd, roomTypeEd), nil,
					container.NewHBox(btnSave, btnDel),
				),
				name: nameEd, roomType: roomTypeEd, save: btnSave, del: btnDel,
			}
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
			row := obj.(*roomRow)
			nameEd := row.name
			roomTypeEd := row.roomType
			btnSave := row.save
			btnDel := row.del

			nameEd.SetText(r.Name)
			roomTypeEd.SetText(r.RoomType)
			nameEd.Disable()
			roomTypeEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				roomTypeEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if strings.TrimSpace(nameEd.Text) == "" {
						showError(w, "Укажите название кабинета")
						return
					}
					if err := store.UpdateRoom(domain.Room{ID: r.ID, SchoolID: r.SchoolID, Name: strings.TrimSpace(nameEd.Text), RoomType: strings.TrimSpace(roomTypeEd.Text)}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					roomTypeEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				dialog.NewConfirm("Удалить кабинет?", "Связанные записи расписания будут удалены.", func(ok bool) {
					if !ok {
						return
					}
					if err := store.DeleteRoom(r.ID); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					list.Refresh()
				}, w).Show()
			}
		},
	)

	nameEd := widget.NewEntry()
	nameEd.SetPlaceHolder("Название кабинета")
	roomTypeEd := widget.NewEntry()
	roomTypeEd.SetText("any")
	roomTypeEd.SetPlaceHolder("тип / any")

	addBtn := widget.NewButton("Добавить кабинет", func() {
		if activeSchoolID == 0 || strings.TrimSpace(nameEd.Text) == "" {
			showError(w, "Выберите школу и введите название кабинета")
			return
		}
		if _, err := store.CreateRoom(domain.Room{SchoolID: activeSchoolID, Name: strings.TrimSpace(nameEd.Text), RoomType: strings.TrimSpace(roomTypeEd.Text)}); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEd.SetText("")
		roomTypeEd.SetText("any")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		container.NewVBox(
			container.NewBorder(
				nil, nil,
				container.NewHBox(nameEd, roomTypeEd), nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		list,
	)
}

// ---------- Уроки ----------

// lessonRow — шаблон строки списка уроков.
type lessonRow struct {
	*fyne.Container
	name  *widget.Label
	hours *widget.Entry
	save  *widget.Button
	del   *widget.Button
}

func buildLessonsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	schoolID := activeSchoolID
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
	classIDs, subjectIDs, teacherIDs := map[string]int{}, map[string]int{}, map[string]int{}
	hoursEd := widget.NewEntry()
	hoursEd.SetPlaceHolder("Часов/нед")

	loadRefs()
	updateSelects := func() {
		classIDs = make(map[string]int, len(classes))
		classOpts := make([]string, 0, len(classes))
		for _, c := range classes {
			label := fmt.Sprintf("%s (#%d)", c.Name, c.ID)
			classOpts = append(classOpts, label)
			classIDs[label] = c.ID
		}
		classSelect.Options = classOpts
		if len(classOpts) > 0 {
			classSelect.Selected = classOpts[0]
		}
		subjectIDs = make(map[string]int, len(subjects))
		subjOpts := make([]string, 0, len(subjects))
		for _, s := range subjects {
			label := fmt.Sprintf("%s (#%d)", s.Name, s.ID)
			subjOpts = append(subjOpts, label)
			subjectIDs[label] = s.ID
		}
		subjectSelect.Options = subjOpts
		if len(subjOpts) > 0 {
			subjectSelect.Selected = subjOpts[0]
		}
		teacherIDs = make(map[string]int, len(teachers))
		teachOpts := make([]string, 0, len(teachers))
		for _, t := range teachers {
			label := fmt.Sprintf("%s (#%d)", t.Name, t.ID)
			teachOpts = append(teachOpts, label)
			teacherIDs[label] = t.ID
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
			if schoolID == 0 {
				return 0
			}
			lessons, _ := store.ListLessons(schoolID)
			return len(lessons)
		},
		func() fyne.CanvasObject {
			nameEd := widget.NewLabel("")
			hoursInput := widget.NewEntry()
			hoursInput.SetPlaceHolder("ч/нед")
			btnSave := widget.NewButton("Сохранить", nil)
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return &lessonRow{
				Container: container.NewBorder(nil, nil, container.NewHBox(nameEd, hoursInput), nil, container.NewHBox(btnSave, btnDel)),
				name:      nameEd, hours: hoursInput, save: btnSave, del: btnDel,
			}
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if schoolID == 0 {
				return
			}
			lessons, err := store.ListLessons(schoolID)
			if err != nil || i >= len(lessons) {
				return
			}
			l := lessons[i]
			row := obj.(*lessonRow)
			nameEd := row.name
			hoursInput := row.hours
			btnSave := row.save
			btnDel := row.del
			nameEd.SetText(fmt.Sprintf("%s → %s · %s", getClassName(classes, l.ClassID), getSubjName(subjects, l.SubjectID), getTeacherName(teachers, l.TeacherID)))
			hoursInput.SetText(fmt.Sprintf("%d", l.HoursPerWeek))
			btnSave.OnTapped = func() {
				hours, err := strconv.Atoi(strings.TrimSpace(hoursInput.Text))
				if err != nil || hours < 1 || hours > 40 {
					showError(w, "Часов в неделю должно быть от 1 до 40")
					return
				}
				err = store.WithTx(func(tx *db.Store) error {
					if err := tx.UpdateLesson(domain.Lesson{ID: l.ID, SchoolID: schoolID, ClassID: l.ClassID, SubjectID: l.SubjectID, TeacherID: l.TeacherID, HoursPerWeek: hours}); err != nil {
						return err
					}
					if hours >= l.HoursPerWeek {
						return nil
					}
					entries, err := tx.ListSchedule(schoolID)
					if err != nil {
						return err
					}
					kept, count := make([]domain.ScheduleEntry, 0, len(entries)), 0
					for _, e := range entries {
						if e.LessonID != l.ID || count < hours {
							kept = append(kept, e)
							if e.LessonID == l.ID {
								count++
							}
						}
					}
					return tx.ReplaceSchedule(schoolID, kept)
				})
				if err != nil {
					showError(w, "Не удалось обновить урок: "+err.Error())
					return
				}
				list.Refresh()
			}

			btnDel.OnTapped = func() {
				dialog.NewConfirm("Удалить урок из учебного плана?", "Урок и все его записи в расписании будут удалены.", func(ok bool) {
					if !ok {
						return
					}
					if err := store.DeleteLesson(l.ID); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					list.Refresh()
				}, w).Show()
			}
		},
	)

	addBtn := widget.NewButton("Добавить урок", func() {
		if schoolID == 0 || classSelect.Selected == "" || subjectSelect.Selected == "" || teacherSelect.Selected == "" {
			showError(w, "Выберите все поля")
			return
		}
		hours := 1
		fmt.Sscanf(hoursEd.Text, "%d", &hours)
		if hours < 1 {
			hours = 1
		}
		classID, subjectID, teacherID := classIDs[classSelect.Selected], subjectIDs[subjectSelect.Selected], teacherIDs[teacherSelect.Selected]
		if classID == 0 || subjectID == 0 || teacherID == 0 {
			showError(w, "Не удалось определить класс, предмет или учителя")
			return
		}
		if _, err := store.CreateLesson(domain.Lesson{SchoolID: schoolID, ClassID: classID, SubjectID: subjectID, TeacherID: teacherID, HoursPerWeek: hours}); err != nil {
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

func getTeacherName(teachers []domain.Teacher, id int) string {
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
