package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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
	store         *db.Store
	win           fyne.Window
	sidebar       *fyne.Container
	content       *fyne.Container
	topBar        *fyne.Container
	root          *fyne.Container
	schoolID      int
	page          string
	schoolSelect  *widget.Select
	schoolByLabel map[string]int
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
	defer store.Close()

	ui := NewUI(store, w)
	w.SetContent(ui.Root())
	w.ShowAndRun()
}

func defaultDBPath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if writableDir(dir) {
			return filepath.Join(dir, "timetable.db")
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "timetable.db")
	}
	dir := filepath.Join(base, "Timetable")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return filepath.Join(".", "timetable.db")
	}
	return filepath.Join(dir, "timetable.db")
}

func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".timetable-write-check-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func NewUI(store *db.Store, w fyne.Window) *UI {
	ui := &UI{
		store:         store,
		win:           w,
		schoolID:      0,
		page:          "welcome",
		schoolByLabel: make(map[string]int),
	}

	versionLbl := widget.NewLabel(appName + " v" + appVersion)
	versionLbl.TextStyle.Bold = true

	ui.schoolSelect = widget.NewSelect([]string{}, func(label string) {
		if id := ui.schoolByLabel[label]; id != 0 && id != activeSchoolID {
			ui.schoolID = id
			setActiveSchoolID(id)
			ui.showPage(ui.page)
		}
	})

	ui.topBar = container.NewBorder(
		nil, nil,
		container.NewHBox(
			versionLbl,
			layout.NewSpacer(),
			widget.NewLabel("Школа:"),
			ui.schoolSelect,
		),
		nil,
	)

	ui.sidebar = container.NewVBox(
		widget.NewLabelWithStyle("МЕНЮ", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		navBtn("Школы", func() { ui.showSchools() }),
		navBtn("Справочники", func() { ui.showRefs() }),
		navBtn("Ограничения", func() { ui.showConstraints() }),
		navBtn("Настройки", func() { ui.showSettings() }),
		navBtn("Генерация", func() { ui.showGeneration() }),
		navBtn("Расписание", func() { ui.showSchedule() }),
		navBtn("Проблемы", func() { ui.showIssues() }),
		navBtn("Экспорт", func() { ui.showExport() }),
	)

	ui.content = container.NewMax(ui.showWelcome())

	split := container.NewHSplit(ui.sidebar, ui.content)
	split.Offset = 0.18

	ui.root = container.NewBorder(ui.topBar, nil, nil, nil, split)

	ui.refreshSchoolSelect(0, false)
	ui.showWelcome()
	return ui
}

func (ui *UI) refreshSchoolSelect(preferredID int, refreshPage bool) {
	schools, err := ui.store.ListSchools()
	if err != nil {
		showError(ui.win, "Не удалось загрузить школы: "+err.Error())
		return
	}
	ui.schoolByLabel = make(map[string]int, len(schools))
	options := make([]string, 0, len(schools))
	selected := activeSchoolID
	if preferredID > 0 {
		selected = preferredID
	}
	found := false
	selectedLabel := ""
	for _, sc := range schools {
		label := fmt.Sprintf("%s  (#%d)", sc.Name, sc.ID)
		options = append(options, label)
		ui.schoolByLabel[label] = sc.ID
		if sc.ID == selected {
			found = true
			selectedLabel = label
		}
	}
	if len(schools) == 0 {
		selected = 0
	} else if !found {
		selected = schools[0].ID
		selectedLabel = options[0]
	}
	ui.schoolSelect.Options = options
	onChanged := ui.schoolSelect.OnChanged
	ui.schoolSelect.OnChanged = nil
	if selectedLabel != "" {
		ui.schoolSelect.SetSelected(selectedLabel)
	} else {
		ui.schoolSelect.ClearSelected()
	}
	ui.schoolSelect.OnChanged = onChanged
	ui.schoolSelect.Refresh()
	ui.schoolID = selected
	setActiveSchoolID(selected)
	if refreshPage {
		ui.showPage(ui.page)
	}
}

func (ui *UI) showPage(page string) {
	ui.page = page
	switch page {
	case "schools":
		ui.SetContent(buildSchoolsView(ui.store, ui.win, func(id int) { ui.refreshSchoolSelect(id, true) }))
	case "refs":
		ui.SetContent(buildRefsView(ui.store, ui.win))
	case "constraints":
		ui.SetContent(buildConstraintsView(ui.store, ui.win))
	case "settings":
		ui.SetContent(buildSettingsView(ui.store, ui.win))
	case "generation":
		ui.SetContent(buildGenerationView(ui.store, ui.win))
	case "schedule":
		ui.SetContent(buildScheduleView(ui.store, ui.win))
	case "issues":
		ui.SetContent(buildIssuesView(ui.store, ui.win))
	case "export":
		ui.SetContent(buildExportView(ui.store, ui.win, func(id int) { ui.refreshSchoolSelect(id, true) }))
	default:
		ui.page = "welcome"
		ui.SetContent(ui.showWelcome())
	}
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
	dialog.ShowError(fmt.Errorf("%s", msg), w)
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

func (ui *UI) showSchools()     { ui.showPage("schools") }
func (ui *UI) showRefs()        { ui.showPage("refs") }
func (ui *UI) showConstraints() { ui.showPage("constraints") }
func (ui *UI) showSettings()    { ui.showPage("settings") }
func (ui *UI) showGeneration()  { ui.showPage("generation") }
func (ui *UI) showSchedule()    { ui.showPage("schedule") }
func (ui *UI) showIssues()      { ui.showPage("issues") }
func (ui *UI) showExport()      { ui.showPage("export") }
