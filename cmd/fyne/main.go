package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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
	w.Resize(fyne.NewSize(1400, 900))
	w.CenterOnScreen()

	// Кастомная тема: тёмная, приятная для глаз
	a.Settings().SetTheme(theme.DarkTheme())

	store, err := db.New(defaultDBPath())
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}

	ui := NewUI(store, w, a)
	ui.Show()

	w.ShowAndRun()
}

func defaultDBPath() string {
	dir, _ := fyneAppDir()
	return joinPath(dir, "timetable.db")
}

func fyneAppDir() (string, error) {
	return ".", nil
}

func joinPath(elem ...string) string {
	result := ""
	for i, e := range elem {
		if i > 0 {
			result += "/"
		}
		result += e
	}
	return result
}

// UI содержит все состояния приложения.
type UI struct {
	store      *db.Store
	window     fyne.Window
	app        fyne.App
	activeTab  string
	activeView fyne.CanvasObject
	content    *fyne.Container
	schoolID   int
}

func NewUI(store *db.Store, w fyne.Window, a fyne.App) *UI {
	ui := &UI{
		store:  store,
		window: w,
		app:    a,
	}

	// Верхняя панель: версия + выбор школы + навигация
	topBar := container.NewBorder(
		nil, nil,
		widget.NewLabel(appName+" v"+appVersion),
		container.NewHBox(
			widget.NewLabel("Школа:"),
			widget.NewSelect([]string{}, func(s string) {}),
		),
	)

	// Боковое меню
	nav := container.NewVBox(
		widget.NewButton("Школы", func() { ui.showSchools() }),
		widget.NewButton("Справочники", func() { ui.showRefs() }),
		widget.NewButton("Генерация", func() { ui.showGeneration() }),
		widget.NewButton("Расписание", func() { ui.showSchedule() }),
		widget.NewButton("Проблемы", func() { ui.showIssues() }),
		widget.NewButton("Экспорт", func() { ui.showExport() }),
	)

	ui.content = container.NewVBox(ui.showWelcome())

	mainSplit := container.NewHSplit(nav, ui.content)
	mainSplit.Offset = 0.2

	w.SetContent(container.NewVBox(topBar, mainSplit))

	ui.showWelcome()
	return ui
}

func (ui *UI) SetContent(obj fyne.CanvasObject) {
	ui.content.Objects = []fyne.CanvasObject{obj}
	ui.content.Refresh()
}

func (ui *UI) Show() {}

func (ui *UI) showWelcome() fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel("Добро пожаловать в "+appName+" v"+appVersion),
		widget.NewLabel("Настольное приложение для составления школьного расписания"),
		widget.NewSeparator(),
		widget.NewLabel("Выберите раздел из меню слева:"),
		widget.NewLabel("• Школы — создание и управление школами"),
		widget.NewLabel("• Справочники — учителя, предметы, классы, кабинеты, уроки"),
		widget.NewLabel("• Генерация — автоматическое построение расписания"),
		widget.NewLabel("• Расписание — просмотр готовой сетки"),
		widget.NewLabel("• Проблемы — отчёт о конфликтах и неразмещённых уроках"),
		widget.NewLabel("• Экспорт — PDF, CSV, JSON резервная копия"),
	)
}

func (ui *UI) showSchools()    { ui.SetContent(buildSchoolsView(ui.store, ui.window)) }
func (ui *UI) showRefs()       { ui.SetContent(buildRefsView(ui.store, ui.window)) }
func (ui *UI) showGeneration() { ui.SetContent(buildGenerationView(ui.store, ui.window)) }
func (ui *UI) showSchedule()   { ui.SetContent(buildScheduleView(ui.store, ui.window)) }
func (ui *UI) showIssues()     { ui.SetContent(buildIssuesView(ui.store, ui.window)) }
func (ui *UI) showExport()     { ui.SetContent(buildExportView(ui.store, ui.window)) }
