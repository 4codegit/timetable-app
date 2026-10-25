package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

// buildSchoolsView — список школ на widget.NewList (виртуализация: плавный
// скролл и один шаблон строки на все данные).
func buildSchoolsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название школы")

	var list *widget.List
	list = widget.NewList(
		func() int {
			schools, _ := store.ListSchools()
			return len(schools)
		},
		// Шаблон строки: Border(left=entry, center=кнопки).
		// Objects[0] — entry, Objects[1] — контейнер кнопок.
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
			schools, err := store.ListSchools()
			if err != nil || i >= len(schools) {
				return
			}
			sc := schools[i]
			box := obj.(*fyne.Container)
			nameEd := box.Objects[0].(*widget.Entry)
			btns := box.Objects[1].(*fyne.Container)
			btnSave := btns.Objects[0].(*widget.Button)
			btnDel := btns.Objects[1].(*widget.Button)

			nameEd.SetText(sc.Name)
			nameEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					if err := store.UpdateSchool(domain.School{ID: sc.ID, Name: nameEd.Text}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				if err := store.DeleteSchool(sc.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				list.Refresh()
			}
		},
	)

	addBtn := widget.NewButton("Добавить школу", func() {
		if nameEntry.Text == "" {
			showError(w, "Введите название школы")
			return
		}
		if _, err := store.CreateSchool(nameEntry.Text); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEntry.SetText("")
		list.Refresh()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewBorder(
		// Верх: заголовок + форма добавления
		container.NewVBox(
			widget.NewLabelWithStyle("Школы", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
			container.NewBorder(
				nil, nil,
				nameEntry, nil,
				addBtn,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		// Центр: список на весь остаток окна — скролл внутри списка
		list,
	)
}
