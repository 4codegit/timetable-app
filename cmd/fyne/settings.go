package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"timetable/internal/db"
)

type fynePeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type fyneSchoolSettings struct {
	Days     int          `json:"days"`
	Slots    int          `json:"slots"`
	DaysMask int          `json:"days_mask"`
	Periods  []fynePeriod `json:"periods"`
}

var weekdayNames = []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"}

func loadFyneSettings(store *db.Store, schoolID int) fyneSchoolSettings {
	st := fyneSchoolSettings{Days: 6, Slots: 8}
	if schoolID > 0 {
		if raw, err := store.GetSchoolSettings(schoolID); err == nil && strings.TrimSpace(raw) != "" {
			_ = json.Unmarshal([]byte(raw), &st)
		}
	}
	if st.Days < 1 || st.Days > 7 {
		st.Days = 6
	}
	if st.Slots < 1 || st.Slots > 14 {
		st.Slots = 8
	}
	if st.DaysMask == 0 {
		st.DaysMask = (1 << st.Days) - 1
	}
	if st.DaysMask&0x7f == 0 {
		st.DaysMask = 1
	}
	if len(st.Periods) != st.Slots {
		st.Periods = defaultFynePeriods(st.Slots)
	}
	return st
}

func defaultFynePeriods(n int) []fynePeriod {
	out := make([]fynePeriod, n)
	for i := range out {
		start := 8*60 + i*45
		out[i] = fynePeriod{Start: fmt.Sprintf("%02d:%02d", start/60, start%60), End: fmt.Sprintf("%02d:%02d", (start+45)/60, (start+45)%60)}
	}
	return out
}

func saveFyneSettings(store *db.Store, schoolID int, st fyneSchoolSettings) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return store.UpdateSchoolSettings(schoolID, string(data))
}

func buildSettingsView(store *db.Store, w fyne.Window) fyne.CanvasObject {
	if activeSchoolID == 0 {
		return emptySchoolView("Настройки")
	}
	st := loadFyneSettings(store, activeSchoolID)
	schoolID := activeSchoolID
	daysEntry := widget.NewEntry()
	daysEntry.SetText(strconv.Itoa(st.Days))
	daysEntry.SetPlaceHolder("1–7")
	slotsEntry := widget.NewEntry()
	slotsEntry.SetText(strconv.Itoa(st.Slots))
	slotsEntry.SetPlaceHolder("1–14")
	dayChecks := make([]*widget.Check, len(weekdayNames))
	dayObjects := make([]fyne.CanvasObject, len(weekdayNames))
	for d, name := range weekdayNames {
		dayChecks[d] = widget.NewCheck(name, nil)
		dayChecks[d].SetChecked(st.DaysMask&(1<<d) != 0)
		dayObjects[d] = dayChecks[d]
	}
	periodsBox := container.NewVBox()
	periodFields := make([][2]*widget.Entry, 0, st.Slots)
	fillPeriods := func(n int, periods []fynePeriod) {
		periodsBox.Objects = nil
		periodFields = make([][2]*widget.Entry, n)
		for i := 0; i < n; i++ {
			p := fynePeriod{}
			if i < len(periods) {
				p = periods[i]
			}
			start, end := widget.NewEntry(), widget.NewEntry()
			start.SetText(p.Start)
			start.SetPlaceHolder("08:00")
			end.SetText(p.End)
			end.SetPlaceHolder("08:45")
			periodFields[i] = [2]*widget.Entry{start, end}
			periodsBox.Add(container.NewBorder(nil, nil, widget.NewLabel(fmt.Sprintf("Урок %d", i+1)), nil, container.NewHBox(start, widget.NewLabel("—"), end)))
		}
		periodsBox.Refresh()
	}
	fillPeriods(st.Slots, st.Periods)
	applySlots := widget.NewButton("Обновить звонки", func() {
		n, err := strconv.Atoi(strings.TrimSpace(slotsEntry.Text))
		if err != nil || n < 1 || n > 14 {
			showError(w, "Укажите число уроков от 1 до 14")
			return
		}
		if n == len(periodFields) {
			return
		}
		periods := make([]fynePeriod, len(periodFields))
		for i, fields := range periodFields {
			periods[i] = fynePeriod{Start: fields[0].Text, End: fields[1].Text}
		}
		for len(periods) < n {
			periods = append(periods, fynePeriod{})
		}
		fillPeriods(n, periods[:n])
	})
	save := widget.NewButton("Сохранить настройки", func() {
		days, err := strconv.Atoi(strings.TrimSpace(daysEntry.Text))
		if err != nil || days < 1 || days > 7 {
			showError(w, "Укажите число учебных дней от 1 до 7")
			return
		}
		slots, err := strconv.Atoi(strings.TrimSpace(slotsEntry.Text))
		if err != nil || slots < 1 || slots > 14 || slots != len(periodFields) {
			showError(w, "Укажите число уроков от 1 до 14 и нажмите «Обновить звонки»")
			return
		}
		mask := 0
		for d, check := range dayChecks {
			if check.Checked {
				mask |= 1 << d
			}
		}
		if mask == 0 {
			showError(w, "Выберите хотя бы один учебный день")
			return
		}
		periods := make([]fynePeriod, len(periodFields))
		previousEnd := -1
		for i, fields := range periodFields {
			periods[i] = fynePeriod{Start: normalizeFyneTime(fields[0].Text), End: normalizeFyneTime(fields[1].Text)}
			if fields[0].Text != "" && periods[i].Start == "" || fields[1].Text != "" && periods[i].End == "" {
				showError(w, fmt.Sprintf("Проверьте время звонка для урока %d", i+1))
				return
			}
			if periods[i].Start != "" && periods[i].End != "" {
				startMinute := fyneMinutes(periods[i].Start)
				endMinute := fyneMinutes(periods[i].End)
				if endMinute <= startMinute || previousEnd >= 0 && startMinute < previousEnd {
					showError(w, fmt.Sprintf("Проверьте порядок звонков для урока %d", i+1))
					return
				}
				previousEnd = endMinute
			}
		}
		if err := saveFyneSettings(store, schoolID, fyneSchoolSettings{Days: days, Slots: slots, DaysMask: mask, Periods: periods}); err != nil {
			showError(w, "Не удалось сохранить настройки: "+err.Error())
			return
		}
		showInfo(w, "Настройки сохранены")
	})
	save.Importance = widget.HighImportance
	return container.NewBorder(
		container.NewVBox(widget.NewLabelWithStyle("Настройки школы", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator()),
		nil, nil, nil,
		container.NewVScroll(container.NewVBox(
			widget.NewForm(widget.NewFormItem("Дней в неделе", daysEntry), widget.NewFormItem("Уроков в день", container.NewBorder(nil, nil, nil, applySlots, slotsEntry))),
			widget.NewLabelWithStyle("Учебные дни", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewGridWithColumns(4, dayObjects...),
			widget.NewLabelWithStyle("Время звонков", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			periodsBox,
			save,
		)),
	)
}

func normalizeFyneTime(value string) string {
	s := strings.TrimSpace(strings.NewReplacer(".", ":", ",", ":", ";", ":", " ", ":").Replace(value))
	if s == "" {
		return ""
	}
	if !strings.Contains(s, ":") {
		if len(s) == 3 || len(s) == 4 {
			s = s[:len(s)-2] + ":" + s[len(s)-2:]
		}
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return ""
	}
	h, e1 := strconv.Atoi(parts[0])
	m, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}

func fyneMinutes(value string) int {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return -1
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m
}

func emptySchoolView(title string) fyne.CanvasObject {
	return container.NewVBox(widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator(), widget.NewLabel("Сначала создайте или выберите школу в верхней панели."))
}

func showInfo(w fyne.Window, message string) {
	dialog.ShowInformation("Готово", message, w)
}
