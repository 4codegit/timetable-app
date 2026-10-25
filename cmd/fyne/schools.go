package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
)

func buildSchoolsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название школы")

	var listContainer *fyne.Container
	var refreshList func()

	refreshList = func() {
		schools, _ := store.ListSchools()
		items := make([]fyne.CanvasObject, 0, len(schools))
		for _, sc := range schools {
			nameEd := widget.NewEntry()
			nameEd.SetText(sc.Name)
			nameEd.Disable()

			btnSave := widget.NewButton("Сохранить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance

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
					btnSave.OnTapped = func() {
						nameEd.Enable()
						btnSave.SetText("Сохранить")
					}
				}
			}

			btnDel.OnTapped = func() {
				if err := store.DeleteSchool(sc.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				refreshList()
			}

			row := container.NewBorder(
				nil, nil, nameEd, nil,
				container.NewHBox(btnSave, btnDel),
			)
			items = append(items, row)
		}
		listContainer.Objects = items
		listContainer.Refresh()
	}

	listContainer = container.NewVBox()
	refreshList()

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
		refreshList()
	})
	addBtn.Importance = widget.HighImportance

	return container.NewVBox(
		widget.NewLabelWithStyle("Школы", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewBorder(
			nil, nil,
			container.NewHBox(
				container.NewVBox(nameEntry),
				addBtn,
				layout.NewSpacer(),
			),
			nil,
		),
		widget.NewSeparator(),
		container.NewScroll(listContainer),
	)
}
