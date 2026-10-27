package main

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
	"timetable/internal/domain"
	"timetable/internal/solver"
)

func buildGenerationView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return emptySchoolView("Генерация")
	}
	schoolID := activeSchoolID
	settings := loadFyneSettings(store, schoolID)
	daysEd := widget.NewEntry()
	daysEd.SetText(strconv.Itoa(settings.Days))
	daysEd.SetPlaceHolder("1–7")
	slotsEd := widget.NewEntry()
	slotsEd.SetText(strconv.Itoa(settings.Slots))
	slotsEd.SetPlaceHolder("1–14")
	daysEd.Disable()
	slotsEd.Disable()
	timeoutPreset := widget.NewSelect([]string{"Без лимита", "30 секунд", "90 секунд", "Другое"}, nil)
	if solver.HasPreciseSolver() {
		timeoutPreset.SetSelected("90 секунд")
	} else {
		timeoutPreset.SetSelected("Без лимита")
	}
	timeoutCustom := widget.NewEntry()
	timeoutCustom.SetPlaceHolder("секунды")
	timeoutCustom.Disable()
	timeoutPreset.OnChanged = func(value string) {
		if value == "Другое" {
			timeoutCustom.Enable()
		} else {
			timeoutCustom.Disable()
		}
	}
	usePrecise := widget.NewCheck("Точный CP-SAT (OR-Tools)", nil)
	preciseInfo := widget.NewLabel("")
	// Лимит времени относится к ЛЮБОМУ решателю (CP-SAT и эвристическому)
	// и поэтому всегда доступен; «Остановить» работает для эвристического.
	if solver.HasPreciseSolver() {
		preciseInfo.SetText("В этой сборке доступен OR-Tools CP-SAT.")
		// Large schedules need CP-SAT when the native solver is available.
		// Keep it enabled by default so generation does not silently use the
		// weaker pure-Go fallback.
		usePrecise.SetChecked(true)
	} else {
		usePrecise.Disable()
		preciseInfo.SetText("OR-Tools не включён в эту сборку; используется стандартный решатель (лимит и «Остановить» действуют для него).")
	}
	generationButton := widget.NewButton("Сгенерировать расписание", nil)
	generationButton.Importance = widget.HighImportance
	cancelButton := widget.NewButton("Остановить", nil)
	cancelButton.Disable()
	undoButton := widget.NewButton("Вернуть прошлое расписание", nil)
	undoButton.Disable()
	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 1
	status := widget.NewLabel("Готово")
	var cancel context.CancelFunc
	var previous []domain.ScheduleEntry
	var generationDone bool
	generationButton.OnTapped = func() {
		if activeGeneration != nil {
			showError(w, "Генерация уже выполняется — дождитесь завершения или остановите её.")
			return
		}
		days, slots := settings.Days, settings.Slots
		if timeoutPreset.Selected == "Другое" {
			n, err := strconv.Atoi(strings.TrimSpace(timeoutCustom.Text))
			if err != nil || n < 0 {
				showError(w, "Введите неотрицательный лимит времени в секундах")
				return
			}
		}
		timeoutSec := selectedTimeout(timeoutPreset.Selected, timeoutCustom.Text)
		if timeoutSec > 86400 {
			showError(w, "Максимальный лимит точного поиска — 24 часа")
			return
		}
		if usePrecise.Checked && solver.HasPreciseSolver() && timeoutSec == 0 {
			showError(w, "Укажите лимит времени: CP-SAT нельзя прервать до завершения поиска.")
			return
		}
		lessons, err := store.ListLessons(schoolID)
		if err != nil {
			showError(w, "Не удалось загрузить учебный план: "+err.Error())
			return
		}
		if len(lessons) == 0 {
			showError(w, "Сначала добавьте уроки в учебный план.")
			return
		}
		previous, err = store.ListSchedule(schoolID)
		if err != nil {
			showError(w, "Не удалось загрузить текущее расписание: "+err.Error())
			return
		}
		generationDone = false
		progress.Value = 0
		progress.Max = float64(totalLessonHours(lessons))
		if progress.Max < 1 {
			progress.Max = 1
		}
		status.SetText("Подготовка генерации…")
		generationButton.Disable()
		usePrecise.Disable()
		timeoutPreset.Disable()
		timeoutCustom.Disable()
		cancelButton.Enable()
		undoButton.Disable()
		if usePrecise.Checked && solver.HasPreciseSolver() {
			cancelButton.Disable()
			status.SetText("CP-SAT выполняется до достижения лимита времени…")
		}
		ctx, stop := context.WithCancel(context.Background())
		cancel = stop
		activeGeneration = &runningGeneration{schoolID: schoolID, cancel: stop}
		go runGeneration(ctx, store, schoolID, days, slots, settings.DaysMask, timeoutSec, usePrecise.Checked,
			func(placed, total int, state string) {
				fyne.Do(func() {
					if total > 0 {
						progress.Max = float64(total)
					}
					progress.Value = float64(placed)
					if state == "running" {
						status.SetText(fmt.Sprintf("Поиск расписания: %d из %d", placed, total))
					}
				})
			},
			func(placed, total int, state string, err error) {
				fyne.Do(func() {
					if cancel != nil {
						cancel()
					}
					cancel = nil
					activeGeneration = nil
					generationButton.Enable()
					cancelButton.Disable()
					if solver.HasPreciseSolver() {
						usePrecise.Enable()
					} else {
						usePrecise.Disable()
					}
					// Лимит действует для любого решателя — возвращаем
					// доступ к нему после завершения генерации.
					timeoutPreset.Enable()
					if timeoutPreset.Selected == "Другое" {
						timeoutCustom.Enable()
					} else {
						timeoutCustom.Disable()
					}
					generationDone = true
					if state == "done" {
						undoButton.Enable()
						status.SetText(fmt.Sprintf("Готово: размещено %d из %d уроков", placed, total))
					} else if state == "cancelled" {
						status.SetText(fmt.Sprintf("Генерация остановлена. Текущее расписание сохранено без изменений (%d из %d)", placed, total))
						undoButton.Disable()
					} else {
						status.SetText("Ошибка генерации")
						if err != nil {
							showError(w, "Ошибка генерации: "+err.Error())
						}
					}
				})
			},
		)
	}
	cancelButton.OnTapped = func() {
		if cancel != nil {
			cancel()
			cancelButton.Disable()
			status.SetText("Останавливаю генератор…")
		}
	}
	undoButton.OnTapped = func() {
		if !generationDone {
			return
		}
		if err := store.ReplaceSchedule(schoolID, previous); err != nil {
			showError(w, "Не удалось вернуть расписание: "+err.Error())
			return
		}
		undoButton.Disable()
		status.SetText("Предыдущее расписание восстановлено.")
	}
	// Вид пересобрали, пока генерация ещё идёт (пользователь уходил с
	// страницы и вернулся): кнопка запуска недоступна до её завершения,
	// «Остановить» подключается к cancel активной генерации.
	if activeGeneration != nil {
		generationButton.Disable()
		status.SetText("Идёт генерация (запущена ранее)…")
		if activeGeneration.schoolID == schoolID && activeGeneration.cancel != nil {
			cancelButton.Enable()
			runningCancel := activeGeneration.cancel
			cancelButton.OnTapped = func() {
				runningCancel()
				cancelButton.Disable()
				status.SetText("Останавливаю генератор…")
			}
		} else {
			cancelButton.Disable()
		}
	}
	return container.NewBorder(
		container.NewVBox(widget.NewLabelWithStyle("Генерация расписания", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator()),
		nil, nil, nil,
		container.NewVScroll(container.NewVBox(
			widget.NewLabel("Используются выбранные учебные дни и звонки из настроек школы."),
			widget.NewForm(widget.NewFormItem("Учебных дней", daysEd), widget.NewFormItem("Уроков в день", slotsEd), widget.NewFormItem("Лимит времени поиска", timeoutPreset), widget.NewFormItem("Другой лимит", timeoutCustom)),
			usePrecise,
			preciseInfo,
			container.NewHBox(generationButton, cancelButton, undoButton),
			container.NewBorder(nil, nil, nil, status, progress),
		)),
	)
}

func totalLessonHours(lessons []domain.Lesson) int {
	total := 0
	for _, l := range lessons {
		if l.HoursPerWeek > 0 {
			total += l.HoursPerWeek
		}
	}
	return total
}

func selectedTimeout(preset, custom string) int {
	switch preset {
	case "30 секунд":
		return 30
	case "90 секунд":
		return 90
	case "Другое":
		n, _ := strconv.Atoi(strings.TrimSpace(custom))
		if n > 0 {
			return n
		}
	}
	return 0
}

func runGeneration(ctx context.Context, store *db.Store, schoolID, days, slots, daysMask, timeoutSec int, usePrecise bool,
	onProgress func(placed, total int, status string), onDone func(placed, total int, status string, err error)) {
	finish := func(placed, total int, status string, err error) { onDone(placed, total, status, err) }
	if usePrecise && !solver.HasPreciseSolver() {
		finish(0, 0, "error", fmt.Errorf("эта сборка не содержит OR-Tools CP-SAT; пересоберите с тегом -tags ortools"))
		return
	}
	lessons, err := store.ListLessons(schoolID)
	if err != nil {
		finish(0, 0, "error", err)
		return
	}
	ts, err := store.ListTeachers(schoolID)
	if err != nil {
		finish(0, 0, "error", err)
		return
	}
	cs, err := store.ListClasses(schoolID)
	if err != nil {
		finish(0, 0, "error", err)
		return
	}
	rs, err := store.ListRooms(schoolID)
	if err != nil {
		finish(0, 0, "error", err)
		return
	}
	subs, err := store.ListSubjects(schoolID)
	if err != nil {
		finish(0, 0, "error", err)
		return
	}
	cons, err := store.ListConstraints(schoolID)
	if err != nil {
		finish(0, 0, "error", err)
		return
	}
	if len(lessons) == 0 {
		finish(0, 0, "error", fmt.Errorf("учебный план пуст"))
		return
	}
	in := solver.SolveInput{SchoolID: schoolID, Lessons: lessons, Teachers: toTeacherMap(ts), Classes: toClassMap(cs), Rooms: rs, Subjects: toSubjMap(subs), Constraints: cons,
		Config: domain.SchedulingConfig{DaysPerWeek: days, SlotsPerDay: slots, DaysMask: daysMask}}
	total := totalLessonHours(lessons)
	var result solver.Result
	if usePrecise {
		result = solver.SolvePrecise(ctx, in, runtime.NumCPU(), time.Duration(timeoutSec)*time.Second, func(placed int) { onProgress(placed, total, "running") })
	} else {
		// Эвристический решатель получает выбранный пользователем лимит
		// (0 = без лимита; «Остановить» доступна). Раньше здесь был
		// жёстко зашитый 30 с — молча, вопреки выбору в поле лимита.
		result = solver.Solve(ctx, in, runtime.NumCPU(), time.Duration(timeoutSec)*time.Second, func(placed int) { onProgress(placed, total, "running") })
	}
	if ctx.Err() != nil {
		finish(result.Placed, result.Total, "cancelled", nil)
		return
	}
	if err := store.ReplaceSchedule(schoolID, result.Entries); err != nil {
		finish(result.Placed, result.Total, "error", err)
		return
	}
	finish(result.Placed, result.Total, "done", nil)
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
