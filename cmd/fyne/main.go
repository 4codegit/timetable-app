package main

import (
	"log"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
)

const (
	appVersion = "1.11.3"
	appName    = "Timetable"
)

func main() {
	a := app.New()
	w := a.NewWindow(appName + " v" + appVersion)
	w.Resize(fyne.NewSize(1300, 850))
	w.CenterOnScreen()

	store, err := db.New(defaultDBPath())
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}

	title := widget.NewLabel(appName + " v" + appVersion)
	title.TextStyle.Bold = true

	nav := container.NewHBox(
		widget.NewButton("Школы", func() { w.SetContent(buildSchoolsView(store, w)) }),
		widget.NewButton("Справочники", func() { w.SetContent(buildRefsView(store, w)) }),
		widget.NewButton("Генерация", func() { w.SetContent(buildGenerationView(store, w)) }),
		widget.NewButton("Расписание", func() { w.SetContent(buildScheduleView(store, w)) }),
		widget.NewButton("Проблемы", func() { w.SetContent(buildIssuesView(store, w)) }),
		widget.NewButton("Экспорт", func() { w.SetContent(buildExportView(store, w)) }),
	)

	content := container.NewVBox(
		title,
		widget.NewSeparator(),
		nav,
		widget.NewSeparator(),
		buildWelcomeView(),
	)

	w.SetContent(content)
	w.ShowAndRun()
}

func defaultDBPath() string {
	dir, _ := filepath.Abs(".")
	return filepath.Join(dir, "timetable.db")
}

func buildWelcomeView() fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel("Добро пожаловать в Timetable v"+appVersion),
		widget.NewLabel("Настольное приложение для составления школьного расписания."),
		widget.NewLabel(""),
		widget.NewLabel("Возможности:"),
		widget.NewLabel("• справочники: школы, учителя, предметы, классы, кабинеты, уроки"),
		widget.NewLabel("• генерация расписания с живым прогресс-баром и настраиваемым таймаутом"),
		widget.NewLabel("• вкладка «Проблемы»: неразмещённые уроки, накладки, нарушения ограничений"),
		widget.NewLabel("• экспорт в PDF, CSV, JSON (резервная копия)"),
	)
}
