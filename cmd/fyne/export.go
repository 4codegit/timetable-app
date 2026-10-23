package main

import (
	"encoding/json"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/io"
	"timetable/internal/pdf"
)

func buildExportView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return container.NewVBox(
			widget.NewLabel("Экспорт"),
			widget.NewSeparator(),
			widget.NewLabel("Выберите школу в разделе «Справочники»"),
		)
	}

	modeSelect := widget.NewSelect([]string{"school", "class", "teacher", "room"}, func(string) {})
	modeSelect.SetSelected("school")

	pageSizeSelect := widget.NewSelect([]string{"A0", "A1", "A2", "A3", "A4"}, func(string) {})
	pageSizeSelect.SetSelected("A2")

	orientationSelect := widget.NewSelect([]string{"landscape", "portrait"}, func(string) {})
	orientationSelect.SetSelected("landscape")

	showTeacher := widget.NewCheck("Показывать учителей", func(bool) {})
	showTeacher.SetChecked(true)
	showRoom := widget.NewCheck("Показывать кабинеты", func(bool) {})
	showRoom.SetChecked(true)
	weekdaysOnly := widget.NewCheck("Только будни", func(bool) {})
	bw := widget.NewCheck("Ч/б", func(bool) {})

	exportPDFBtn := widget.NewButton("Экспорт PDF", func() {
		opts := pdf.Options{
			Mode:         modeSelect.Selected,
			PageSize:     pageSizeSelect.Selected,
			Orientation:  orientationSelect.Selected,
			ShowTeacher:  showTeacher.Checked,
			ShowRoom:     showRoom.Checked,
			WeekdaysOnly: weekdaysOnly.Checked,
			BW:           bw.Checked,
			Days:         6,
			Slots:        8,
		}

		pdfBytes, err := renderPDF(store, activeSchoolID, opts)
		if err != nil {
			showError(w, "Ошибка PDF: "+err.Error())
			return
		}

		saveDialog := dialog.NewFileSave(func(uri fyne.URI, err error) {
			if err != nil || uri == nil {
				return
			}
			if err := os.WriteFile(uri.Path(), pdfBytes, 0644); err != nil {
				showError(w, "Ошибка записи: "+err.Error())
			}
		}, w)
		saveDialog.SetFileName("timetable.pdf")
		saveDialog.Show()
	})

	exportJSONBtn := widget.NewButton("Экспорт JSON (резервная копия)", func() {
		snap, err := io.ExportAll(store, activeSchoolID)
		if err != nil {
			showError(w, "Ошибка экспорта: "+err.Error())
			return
		}
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			showError(w, "Ошибка сериализации: "+err.Error())
			return
		}

		saveDialog := dialog.NewFileSave(func(uri fyne.URI, err error) {
			if err != nil || uri == nil {
				return
			}
			if err := os.WriteFile(uri.Path(), data, 0644); err != nil {
				showError(w, "Ошибка записи: "+err.Error())
			}
		}, w)
		saveDialog.SetFileName("timetable_backup.json")
		saveDialog.Show()
	})

	importJSONBtn := widget.NewButton("Импорт JSON", func() {
		fileDialog := dialog.NewFileOpen(func(uri fyne.URI, err error) {
			if err != nil || uri == nil {
				return
			}
			data, err := os.ReadFile(uri.Path())
			if err != nil {
				showError(w, "Ошибка чтения: "+err.Error())
				return
			}
			snap, err := io.ParseSnapshot(data)
			if err != nil {
				showError(w, "Ошибка разбора JSON: "+err.Error())
				return
			}
			if _, err := io.ImportAll(store, snap); err != nil {
				showError(w, "Ошибка импорта: "+err.Error())
				return
			}
			widget.NewPopUp(widget.NewLabel("Импорт завершён успешно"), w.Canvas()).Show()
		}, w)
		fileDialog.Show()
	})

	return container.NewVBox(
		widget.NewLabel("Экспорт / Импорт"),
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabel("Режим:"), modeSelect,
			widget.NewLabel("Формат:"), pageSizeSelect,
			widget.NewLabel("Ориентация:"), orientationSelect,
		),
		container.NewHBox(showTeacher, showRoom, weekdaysOnly, bw),
		widget.NewSeparator(),
		container.NewHBox(exportPDFBtn, exportJSONBtn, importJSONBtn),
		widget.NewSeparator(),
		widget.NewLabel("Резервная копия сохраняет всю школу: настройки, подгруппы, ограничения и готовое расписание."),
	)
}

func renderPDF(store *db.Store, schoolID int, opts pdf.Options) ([]byte, error) {
	sc, err := store.GetSchool(schoolID)
	if err != nil {
		return nil, err
	}

	ts, err := store.ListTeachers(schoolID)
	if err != nil {
		return nil, err
	}
	cs, err := store.ListClasses(schoolID)
	if err != nil {
		return nil, err
	}
	rs, err := store.ListRooms(schoolID)
	if err != nil {
		return nil, err
	}
	subs, err := store.ListSubjects(schoolID)
	if err != nil {
		return nil, err
	}
	entries, err := store.ListSchedule(schoolID)
	if err != nil {
		return nil, err
	}

	opts.SchoolName = sc.Name
	if opts.Title == "" {
		opts.Title = "Расписание"
	}
	if opts.ShowTeacher && opts.Mode == "teacher" {
		opts.ShowTeacher = false
	}
	opts.ShowRoom = false

	return pdf.Render(opts)
}
