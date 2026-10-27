package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
)

// schoolRow — шаблон строки списка школ: контейнер + типизированные ссылки
// на виджеты (позиционное индексирование Objects у Border ненадёжно).
type schoolRow struct {
	*fyne.Container
	name *widget.Entry
	save *widget.Button
	del  *widget.Button
}

// buildSchoolsView — список школ на widget.NewList (виртуализация: плавный
// скролл и один шаблон строки на все данные).
func buildSchoolsView(store *db.Store, w fyne.Window, onChange func(preferredID int)) fyne.CanvasObject {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("Название школы")

	var list *widget.List
	list = widget.NewList(
		func() int {
			schools, _ := store.ListSchools()
			return len(schools)
		},
		// Шаблон строки: schoolRow хранит типизированные ссылки на виджеты —
		// порядок Objects у container.NewBorder зависеть от позиции нельзя.
		func() fyne.CanvasObject {
			nameEd := widget.NewEntry()
			btnSave := widget.NewButton("Изменить", nil)
			btnSave.Importance = widget.LowImportance
			btnDel := widget.NewButton("Удалить", nil)
			btnDel.Importance = widget.DangerImportance
			return &schoolRow{
				Container: container.NewBorder(
					nil, nil,
					nameEd, nil,
					container.NewHBox(btnSave, btnDel),
				),
				name: nameEd, save: btnSave, del: btnDel,
			}
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			schools, err := store.ListSchools()
			if err != nil || i >= len(schools) {
				return
			}
			sc := schools[i]
			row := obj.(*schoolRow)
			nameEd := row.name
			btnSave := row.save
			btnDel := row.del

			nameEd.SetText(sc.Name)
			nameEd.Disable()
			btnSave.SetText("Изменить")
			btnSave.OnTapped = func() {
				nameEd.Enable()
				btnSave.SetText("Сохранить")
				btnSave.OnTapped = func() {
					name := strings.TrimSpace(nameEd.Text)
					if name == "" {
						showError(w, "Введите название школы")
						return
					}
					updated, err := store.GetSchool(sc.ID)
					if err != nil {
						showError(w, "Не удалось загрузить школу: "+err.Error())
						return
					}
					updated.Name = name
					if err := store.UpdateSchool(*updated); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					if onChange != nil {
						onChange(sc.ID)
					}
					nameEd.Disable()
					btnSave.SetText("Изменить")
				}
			}

			btnDel.OnTapped = func() {
				dialog.NewConfirm("Удалить школу?", "Будут удалены все её справочники, уроки и расписание.", func(ok bool) {
					if !ok {
						return
					}
					if err := store.DeleteSchool(sc.ID); err != nil {
						showError(w, "Ошибка: "+err.Error())
						return
					}
					if onChange != nil {
						onChange(0)
					}
					list.Refresh()
				}, w).Show()
			}
		},
	)

	addBtn := widget.NewButton("Добавить школу", func() {
		if strings.TrimSpace(nameEntry.Text) == "" {
			showError(w, "Введите название школы")
			return
		}
		sc, err := store.CreateSchool(strings.TrimSpace(nameEntry.Text))
		if err != nil {
			showError(w, "Ошибка: "+err.Error())
			return
		}
		nameEntry.SetText("")
		if onChange != nil {
			onChange(sc.ID)
		}
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
