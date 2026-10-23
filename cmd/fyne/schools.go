package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

func buildSchoolsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название школы")

	var list *widget.List
	list = widget.NewList(
		func() int {
			schools, _ := store.ListSchools()
			return len(schools)
		},
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			btnSave := widget.NewButton("💾", nil)
			btnDel := widget.NewButton("✕", nil)
			return container.NewHBox(nameEd, btnSave, btnDel)
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			schools, err := store.ListSchools()
			if err != nil || i >= len(schools) {
				return
			}
			sc := schools[i]
			box := obj.(*fyne.Container)
			nameEd := box.Objects[0].(*widget.Entry)
			nameEd.SetText(sc.Name)
			nameEd.Disable()

			btnSave := box.Objects[1].(*widget.Button)
			btnSave.OnTapped = func() {
				nameEd.Enable()
				btnSave.SetText("💾")
				btnSave.OnTapped = func() {
					if err := store.UpdateSchool(domain.School{ID: sc.ID, Name: nameEd.Text}); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					nameEd.Disable()
					btnSave.SetText("✎")
					btnSave.OnTapped = func() {
						nameEd.Enable()
						btnSave.SetText("💾")
					}
				}
			}

			btnDel := box.Objects[2].(*widget.Button)
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

	return container.NewVBox(
		widget.NewLabel("Школы"),
		widget.NewSeparator(),
		container.NewHBox(nameEntry, addBtn),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}
