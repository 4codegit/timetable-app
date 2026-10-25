package main

import (
	"fmt"
	"runtime"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
	"timetable/internal/solver"
)

func buildGenerationView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	daysEd := widget.NewEntry()
	daysEd.SetPlaceHolder("Дней (1-7)")
	daysEd.SetText("6")

	slotsEd := widget.NewEntry()
	slotsEd.SetPlaceHolder("Слотов в день (1-14)")
	slotsEd.SetText("8")

	timeoutPreset := widget.NewSelect([]string{"none", "30", "90", "custom"}, func(string) {})
	timeoutPreset.SetSelected("none")
	timeoutCustom := widget.NewEntry()
	timeoutCustom.SetPlaceHolder("сек")
	timeoutCustom.Disable()
	timeoutPreset.OnChanged = func(s string) {
		if s == "custom" {
			timeoutCustom.Enable()
		} else {
			timeoutCustom.Disable()
		}
	}

	usePrecise := widget.NewCheck("Точный CP-SAT (OR-Tools)", func(bool) {})

	generateBtn := widget.NewButton("Сгенерировать расписание", func() {})
	generateBtn.Importance = widget.HighImportance

	undoBtn := widget.NewButton("Отменить", func() {})
	undoBtn.Importance = widget.LowImportance

	progressBar := widget.NewProgressBar()
	progressBar.Min = 0
	progressBar.Max = 100
	progressBar.Value = 0

	statusLabel := widget.NewLabel("Готово")

	generateBtn.OnTapped = func() {
		if activeSchoolID == 0 {
			showError(w, "Выберите школу в разделе «Справочники»")
			return
		}

		days, _ := strconv.Atoi(daysEd.Text)
		if days < 1 || days > 7 {
			days = 6
		}
		slots, _ := strconv.Atoi(slotsEd.Text)
		if slots < 1 || slots > 14 {
			slots = 8
		}

		timeoutSec := 0
		preset := timeoutPreset.Selected
		if preset == "custom" {
			if v, err := strconv.Atoi(timeoutCustom.Text); err == nil && v >= 0 {
				timeoutSec = v
			}
		} else if preset != "none" {
			if v, err := strconv.Atoi(preset); err == nil {
				timeoutSec = v
			}
		}

		generateBtn.Disable()
		progressBar.Value = 0
		statusLabel.SetText("Запуск генерации...")

		go runGeneration(store, activeSchoolID, days, slots, timeoutSec, usePrecise.Checked,
			func(placed, total int, status string) {
				if total > 0 {
					progressBar.Max = float64(total)
				}
				progressBar.Value = float64(placed)
				statusLabel.SetText(fmt.Sprintf("%s: %d/%d", status, placed, total))
			},
			func() {
				generateBtn.Enable()
				statusLabel.SetText("Готово")
			},
		)
	}

	undoBtn.OnTapped = func() {
		progressBar.Value = 0
		statusLabel.SetText("Готово")
		generateBtn.Enable()
	}

	return container.NewVBox(
		widget.NewLabelWithStyle("Генерация расписания", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		container.NewBorder(
			nil, nil,
			container.NewHBox(
				container.NewGridWithColumns(2,
					container.NewVBox(daysEd, slotsEd),
					container.NewVBox(
						container.NewHBox(widget.NewLabel("Таймаут CP-SAT:"), timeoutPreset),
						timeoutCustom,
					),
					usePrecise,
				),
				layout.NewSpacer(),
				container.NewVBox(generateBtn, undoBtn),
			),
			nil,
		),
		widget.NewSeparator(),
		container.NewVBox(
			container.NewHBox(
				progressBar,
				statusLabel,
			),
		),
	)
}

func runGeneration(store *db.Store, schoolID, days, slots, timeoutSec int, usePrecise bool,
	onProgress func(placed, total int, status string), onDone func()) {

	lessons, err := store.ListLessons(schoolID)
	if err != nil {
		onDone()
		return
	}
	ts, _ := store.ListTeachers(schoolID)
	cs, _ := store.ListClasses(schoolID)
	rs, _ := store.ListRooms(schoolID)
	subs, _ := store.ListSubjects(schoolID)
	cons, _ := store.ListConstraints(schoolID)

	in := solver.SolveInput{
		SchoolID:    schoolID,
		Lessons:     lessons,
		Teachers:    toTeacherMap(ts),
		Classes:     toClassMap(cs),
		Rooms:       rs,
		Subjects:    toSubjMap(subs),
		Constraints: cons,
		Config: domain.SchedulingConfig{
			DaysPerWeek: days,
			SlotsPerDay: slots,
		},
	}

	total := 0
	for _, l := range lessons {
		if h := l.HoursPerWeek; h > 0 {
			total += h
		}
	}

	if usePrecise {
		timeout := time.Duration(timeoutSec) * time.Second
		if timeout <= 0 {
			timeout = 0
		}
		id := genState.newID()
		genState.set(id, 0, total, "running", "")

		go func() {
			res := solver.SolvePrecise(nil, in, runtime.NumCPU(), timeout, func(placed int) {
				genState.set(id, placed, total, "running", "")
			})
			if err := store.ReplaceSchedule(schoolID, res.Entries); err != nil {
				genState.set(id, res.Placed, res.Total, "error", err.Error())
				onProgress(res.Placed, res.Total, "error")
				onDone()
				return
			}
			genState.set(id, res.Placed, res.Total, "done", "")
			onProgress(res.Placed, res.Total, "done")
			onDone()
		}()

		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				p, ok := genState.get(id)
				if !ok {
					return
				}
				onProgress(p.Placed, p.Total, p.Status)
				if p.Status == "done" || p.Status == "error" {
					return
				}
			}
		}()
	} else {
		go func() {
			res := solver.Solve(nil, in, runtime.NumCPU(), 30*time.Second, nil)
			if err := store.ReplaceSchedule(schoolID, res.Entries); err != nil {
				onDone()
				return
			}
			onProgress(res.Placed, res.Total, "done")
			onDone()
		}()
	}
}

func toTeacherMap(ts []domain.Teacher) map[int]domain.Teacher {
	m := make(map[int]domain.Teacher, len(ts))
	for _, t := range ts {
		m[t.ID] = t
	}
	return m
}

func toClassMap(cs []domain.SchoolClass) map[int]domain.SchoolClass {
	m := make(map[int]domain.SchoolClass, len(cs))
	for _, c := range cs {
		m[c.ID] = c
	}
	return m
}

func toSubjMap(subs []domain.Subject) map[int]domain.Subject {
	m := make(map[int]domain.Subject, len(subs))
	for _, s := range subs {
		m[s.ID] = s
	}
	return m
}
