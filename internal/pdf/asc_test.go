package pdf

import (
	"strings"
	"testing"
)

// ascTestCellAt produces a deterministic, mostly-filled schedule with a
// couple of conflict cells.
func ascTestCellAt(rowID, day, slot int) (Cell, bool) {
	if (day+slot+rowID*2)%5 == 4 {
		return Cell{}, false
	}
	return Cell{
		SubjectID: ((day*3 + slot + rowID) % 8) + 1,
		TeacherID: ((slot + rowID) % 6) + 1,
		RoomID:    ((day + slot) % 8) + 1,
		Conflict:  (rowID == 1 && day == 0 && slot == 0) || (rowID == 2 && day == 2 && slot == 3),
	}, true
}

func ascTestOptions(mode string, rows []Row, pageSize, orient string, days, slots int) Options {
	return Options{
		SchoolName: "МБОУ Тестовая школа №7",
		Title:      map[string]string{"school": "вся школа", "class": "по классам", "teacher": "по учителям", "room": "по кабинетам"}[mode],
		Days:       days,
		Slots:      slots,
		Periods: []Period{
			{Start: "08:00", End: "08:45"}, {Start: "08:55", End: "09:40"},
			{Start: "09:50", End: "10:35"}, {Start: "10:55", End: "11:40"},
			{Start: "11:50", End: "12:35"}, {Start: "12:45", End: "13:30"},
			{Start: "13:40", End: "14:25"}, {Start: "14:35", End: "15:20"},
			{Start: "15:25", End: "16:10"}, {Start: "16:20", End: "17:05"},
			{Start: "17:15", End: "18:00"}, {Start: "18:10", End: "18:55"},
			{Start: "19:05", End: "19:50"}, {Start: "20:00", End: "20:45"},
		}[:slots],
		Mode:        mode,
		Rows:        rows,
		CellAt:      ascTestCellAt,
		ShowTeacher: true,
		ShowRoom:    true,
		PageSize:    pageSize,
		Orientation: orient,
		SubjectName: func(id int) string {
			names := []string{"Алгебра", "Физика", "Химия", "Литература", "История", "Английский язык", "Физкультура", "Информатика"}
			return names[(id-1)%len(names)]
		},
		TeacherName: func(int) string { return "Иванова И.И." },
		RoomName:    func(int) string { return "301" },
		ClassName:   func(int) string { return "5А" },
		SubjectColor: func(id int) string {
			palette := []string{"#dbeafe", "#dcfce7", "#fef9c3", "#fae8ff", "#ffedd5", "#cffafe", "#fecaca", "#e0e7ff"}
			return palette[(id-1)%len(palette)]
		},
	}
}

func ascPageCount(b []byte) int {
	return strings.Count(string(b), "/Type /Page") - strings.Count(string(b), "/Type /Pages")
}

// TestASCPrintOnePagePerRow pins the aSc-print pagination contract: the
// black-and-white grid always fits, so every row takes exactly one page.
func TestASCPrintOnePagePerRow(t *testing.T) {
	rows := []Row{{ID: 1, Label: "10А"}, {ID: 2, Label: "10Б"}, {ID: 3, Label: "11А"}}
	b, err := Render(ascTestOptions("class", rows, "A4", "landscape", 5, 6))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := ascPageCount(b); got != len(rows) {
		t.Fatalf("expected %d pages (one per row), got %d", len(rows), got)
	}
	one := []Row{{ID: 1, Label: "10А"}}
	b, err = Render(ascTestOptions("teacher", one, "A4", "landscape", 5, 6))
	if err != nil {
		t.Fatalf("Render single: %v", err)
	}
	if got := ascPageCount(b); got != 1 {
		t.Fatalf("single row: expected 1 page, got %d", got)
	}
}

// TestASCPrintUnifiedModes pins the unified-style contract: the "school"
// mode packs per-class aSc-style tables onto few sheets (a class never
// gets a whole sheet), other row modes take one page per row.
func TestASCPrintUnifiedModes(t *testing.T) {
	mkRows := func(n int) []Row {
		rows := make([]Row, 0, n)
		for i := 1; i <= n; i++ {
			rows = append(rows, Row{ID: i, Label: string(rune('0'+i/10)) + string(rune('0'+i%10)) + "А"})
		}
		return rows
	}
	b, err := Render(ascTestOptions("school", mkRows(12), "A3", "landscape", 5, 8))
	if err != nil {
		t.Fatalf("Render school A3: %v", err)
	}
	if got := ascPageCount(b); got < 1 || got >= 12 {
		t.Fatalf("school packed: expected few pages for 12 classes, got %d", got)
	}
	cls := []Row{{ID: 1, Label: "10А"}, {ID: 2, Label: "10Б"}, {ID: 3, Label: "11А"}}
	b, err = Render(ascTestOptions("class", cls, "A4", "landscape", 5, 6))
	if err != nil {
		t.Fatalf("Render class: %v", err)
	}
	if got := ascPageCount(b); got != len(cls) {
		t.Fatalf("class mode: expected %d pages (one per row), got %d", len(cls), got)
	}
}

// TestASCDaysMask makes sure a school-day mask renders exactly the
// masked weekdays and survives a degenerate (empty) mask.
func TestASCDaysMask(t *testing.T) {
	// activeDays unit behavior.
	if got := activeDays(6, 0); len(got) != 6 || got[5] != 5 {
		t.Fatalf("activeDays(6,0) = %v", got)
	}
	if got := activeDays(6, 0b11111); len(got) != 5 || got[4] != 4 {
		t.Fatalf("activeDays(6,0b11111) = %v", got)
	}
	if got := activeDays(5, 0b100000); len(got) != 1 || got[0] != 5 {
		t.Fatalf("activeDays(5,0b100000) = %v", got)
	}
	if got := activeDays(5, 0); len(got) != 5 || got[4] != 4 {
		t.Fatalf("activeDays(5,0) = %v", got)
	}
	// Пн, Вт, Чт, Пт, Сб (без среды) — полный рендер без паники.
	for _, mode := range []string{"class", "school"} {
		rows := []Row{{ID: 1, Label: "10А"}, {ID: 2, Label: "10Б"}}
		opts := ascTestOptions(mode, rows, "A4", "landscape", 6, 8)
		opts.DaysMask = 0b1111011
		b, err := Render(opts)
		if err != nil {
			t.Fatalf("%s: Render: %v", mode, err)
		}
		if len(b) < 1000 {
			t.Fatalf("%s: suspiciously small PDF", mode)
		}
	}
	// Вырожденная маска без единого дня — не должна ломать рендер.
	opts := ascTestOptions("class", []Row{{ID: 1, Label: "10А"}}, "A4", "landscape", 5, 6)
	opts.DaysMask = 0b100000 // только суббота при Days=5 → едиственная колонка
	if _, err := Render(opts); err != nil {
		t.Fatalf("saturday-only mask: %v", err)
	}
}

// TestASCEdgeCases makes sure the renderer survives empty input, a 14-slot
// day, BW mode and weekdays-only without panicking.
func TestASCEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"empty school", ascTestOptions("school", nil, "A4", "landscape", 5, 6)},
		{"14 slots", ascTestOptions("class", []Row{{ID: 1, Label: "10А"}}, "A4", "portrait", 5, 14)},
		{"bw poster", func() Options {
			o := ascTestOptions("school", []Row{{ID: 1, Label: "10А"}, {ID: 2, Label: "10Б"}}, "A3", "landscape", 6, 8)
			o.BW = true
			return o
		}()},
		{"weekdays only", func() Options {
			o := ascTestOptions("class", []Row{{ID: 1, Label: "10А"}}, "A4", "portrait", 6, 8)
			o.WeekdaysOnly = true
			return o
		}()},
		{"no teacher no room", func() Options {
			o := ascTestOptions("teacher", []Row{{ID: 1, Label: "Иванова И.И."}}, "A4", "portrait", 5, 8)
			o.ShowTeacher = false
			o.ShowRoom = false
			return o
		}()},
	}
	for _, tc := range cases {
		b, err := Render(tc.opts)
		if err != nil {
			t.Fatalf("%s: Render: %v", tc.name, err)
		}
		if len(b) < 1000 || !strings.HasPrefix(string(b[:8]), "%PDF") {
			t.Fatalf("%s: not a plausible PDF (%d bytes)", tc.name, len(b))
		}
	}
}
