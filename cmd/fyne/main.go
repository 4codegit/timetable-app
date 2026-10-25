package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
)

const (
	appVersion = "1.11.3"
	appName    = "Timetable"
)

type UI struct {
	store    *db.Store
	win      fyne.Window
	sidebar  *fyne.Container
	content  *fyne.Container
	topBar   *fyne.Container
	root     *fyne.Container
	schoolID int
}

func main() {
	a := app.New()
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow(appName + " v" + appVersion)
	w.Resize(fyne.NewSize(1400, 900))
	w.CenterOnScreen()
	w.SetPadded(false)

	store, err := db.New(defaultDBPath())
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}

	ui := NewUI(store, w)
	w.SetContent(ui.Root())
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

func NewUI(store *db.Store, w fyne.Window) *UI {
	ui := &UI{
		store:    store,
		win:      w,
		schoolID: 0,
	}

	versionLbl := widget.NewLabel(appName + " v" + appVersion)
	versionLbl.TextStyle.Bold = true

	schoolSelect := widget.NewSelect([]string{}, func(s string) {
		schools, err := store.ListSchools()
		if err != nil {
			return
		}
		for _, sc := range schools {
			if sc.Name == s {
				ui.schoolID = sc.ID
				break
			}
		}
	})

	ui.topBar = container.NewBorder(
		nil, nil,
		container.NewHBox(
			versionLbl,
			layout.NewSpacer(),
			widget.NewLabel("Школа:"),
			schoolSelect,
		),
		nil,
	)

	ui.sidebar = container.NewVBox(
		widget.NewLabelWithStyle("МЕНЮ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		navBtn("Школы", func() { ui.showSchools() }),
		navBtn("Справочники", func() { ui.showRefs() }),
		navBtn("Генерация", func() { ui.showGeneration() }),
		navBtn("Расписание", func() { ui.showSchedule() }),
		navBtn("Проблемы", func() { ui.showIssues() }),
		navBtn("Экспорт", func() { ui.showExport() }),
	)

	ui.content = container.NewMax(ui.showWelcome())

	split := container.NewHSplit(ui.sidebar, ui.content)
	split.Offset = 0.18

	ui.root = container.NewVBox(ui.topBar, split)

	refreshSchoolsSelect(store, schoolSelect)

	ui.showWelcome()
	return ui
}

// SetContent переключает активный вид; NewMax растягивает его на всё
// свободное пространство окна — иначе VBox сжимает вид в маленькую
// область и скролл крутится внутри неё.
func (ui *UI) SetContent(obj fyne.CanvasObject) {
	ui.content.Objects = []fyne.CanvasObject{container.NewMax(obj)}
	ui.content.Refresh()
}

func (ui *UI) Root() fyne.CanvasObject {
	return ui.root
}

func (ui *UI) Show() {}

func navBtn(label string, onTap func()) *widget.Button {
	b := widget.NewButton(label, onTap)
	b.Importance = widget.LowImportance
	return b
}

func showError(w fyne.Window, msg string) {
	dlg := widget.NewPopUp(widget.NewLabel(msg), w.Canvas())
	dlg.Show()
}

func (ui *UI) showWelcome() fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabelWithStyle("Добро пожаловать в "+appName+" v"+appVersion, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Настольное приложение для составления школьного расписания"),
		widget.NewSeparator(),
		container.NewGridWithColumns(3,
			card("🏫", "Школы", "Создание и управление школами", func() { ui.showSchools() }),
			card("📚", "Справочники", "Учителя, предметы, классы, кабинеты, уроки", func() { ui.showRefs() }),
			card("⚙", "Генерация", "Автоматическое построение расписания", func() { ui.showGeneration() }),
			card("🗓", "Расписание", "Просмотр готовой сетки уроков", func() { ui.showSchedule() }),
			card("⚠", "Проблемы", "Конфликты, неразмещённые уроки", func() { ui.showIssues() }),
			card("💾", "Экспорт", "PDF, CSV, JSON резервная копия", func() { ui.showExport() }),
		),
	)
}

func card(icon, title, desc string, onTap func()) *widget.Button {
	b := widget.NewButton(icon+"  "+title+"\n"+desc, onTap)
	b.Importance = widget.LowImportance
	return b
}

func (ui *UI) showSchools()    { ui.SetContent(buildSchoolsView(ui.store, ui.win)) }
func (ui *UI) showRefs()       { ui.SetContent(buildRefsView(ui.store, ui.win)) }
func (ui *UI) showGeneration() { ui.SetContent(buildGenerationView(ui.store, ui.win)) }
func (ui *UI) showSchedule()   { ui.SetContent(buildScheduleView(ui.store, ui.win)) }
func (ui *UI) showIssues()     { ui.SetContent(buildIssuesView(ui.store, ui.win)) }
func (ui *UI) showExport()     { ui.SetContent(buildExportView(ui.store, ui.win)) }
