package main

import (
	"encoding/json"
)

// ---- School settings (grid size + bell schedule) ----

type period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type schoolSettings struct {
	Days     int      `json:"days"`
	Slots    int      `json:"slots"`
	DaysMask int      `json:"days_mask"` // bit 0 = Пн … bit 6 = Вс; 0 = derive from Days
	Periods  []period `json:"periods"`
}

func defaultPeriods(n int) []period {
	out := make([]period, n)
	for i := 0; i < n; i++ {
		startMin := 8*60 + i*45
		out[i] = period{
			Start: formatTime(startMin),
			End:   formatTime(startMin + 45),
		}
	}
	return out
}

func formatTime(minutes int) string {
	return format2(minutes / 60) + ":" + format2(minutes%60)
}

func format2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (a *App) loadSettings(schoolID int) schoolSettings {
	raw, _ := a.store.GetSchoolSettings(schoolID)
	st := schoolSettings{Days: 6, Slots: 8}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &st)
	}
	if st.Days <= 0 {
		st.Days = 6
	}
	if st.Slots <= 0 {
		st.Slots = 8
	}
	if st.DaysMask == 0 {
		// Legacy settings (or fresh school): first Days days from Monday.
		for d := 0; d < st.Days && d < 7; d++ {
			st.DaysMask |= 1 << d
		}
	}
	if len(st.Periods) != st.Slots {
		st.Periods = defaultPeriods(st.Slots)
	}
	return st
}

func (a *App) saveSettings(schoolID int, st schoolSettings) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return a.store.UpdateSchoolSettings(schoolID, string(b))
}

// GetSchoolSettings returns the raw settings JSON for a school.
func (a *App) GetSchoolSettings(schoolID int) (string, error) {
	return a.store.GetSchoolSettings(schoolID)
}

// UpdateSchoolSettings persists the raw settings JSON for a school.
func (a *App) UpdateSchoolSettings(schoolID int, settings string) error {
	return a.store.UpdateSchoolSettings(schoolID, settings)
}
