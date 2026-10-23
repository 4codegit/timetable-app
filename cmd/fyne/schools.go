package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
)

func buildSchoolsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	schoolList := binding.NewStringList()

	list := widget.NewListWithData(
		schoolList,
		func() fyne.CanvasObject {
			return widget.NewButton("", func() {})
		},
		func(i binding.DataItem, obj fyne.CanvasObject) {
			btn := obj.(*widget.Button)
			name, _ := i.(binding.String).Get()
			btn.SetText(name)
			btn.OnTapped = func() {
				setActiveSchoolByName(store, name)
			}
		},
	)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название школы")

	addBtn := widget.NewButton("Добавить школу", func() {
		name := nameEntry.Text
		if name == "" {
			return
		}
		if _, err := store.CreateSchool(name); err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEntry.SetText("")
		refreshSchools(store, schoolList)
	})

	delBtn := widget.NewButton("Удалить выбранную", func() {
		sel, _ := schoolList.Get()
		if len(sel) == 0 {
			return
		}
		schools, err := store.ListSchools()
		if err != nil {
			return
		}
		for _, sc := range schools {
			if sc.Name == sel[0] {
				if err := store.DeleteSchool(sc.ID); err != nil {
					showError(w, "Ошибка: "+err.Error())
					return
				}
				break
			}
		}
		refreshSchools(store, schoolList)
	})

	refreshBtn := widget.NewButton("Обновить", func() {
		refreshSchools(store, schoolList)
	})

	refreshSchools(store, schoolList)

	return container.NewVBox(
		widget.NewLabel("Школы"),
		widget.NewSeparator(),
		container.NewHBox(nameEntry, addBtn, delBtn, refreshBtn),
		widget.NewSeparator(),
		container.NewScroll(list),
	)
}

func refreshSchools(store *db.Store, list binding.StringList) {
	list.Set([]string{})
	schools, err := store.ListSchools()
	if err != nil {
		return
	}
	for _, sc := range schools {
		list.Append(sc.Name)
	}
}

func setActiveSchoolByName(store *db.Store, name string) {
	schools, err := store.ListSchools()
	if err != nil {
		return
	}
	for _, sc := range schools {
		if sc.Name == name {
			setActiveSchoolID(sc.ID)
			return
		}
	}
}

func showError(w fyne.Window, msg string) {
	dlg := widget.NewPopUp(widget.NewLabel(msg), w.Canvas())
	dlg.Show()
}
