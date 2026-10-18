// Command pdfsample renders realistic sample PDFs of every export mode
// to /tmp/samples for visual inspection (convert with pdftoppm -png -r 150).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"timetable/internal/pdf"
)

func main() {
	outDir := "/tmp/samples"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	subjects := []string{"Алгебра", "Физика", "Химия", "Литература", "История", "Английский язык", "Физкультура", "Информатика"}
	palette := []string{"#3b82f6", "#10b981", "#f59e0b", "#8b5cf6", "#ec4899", "#06b6d4", "#6366f1", "#14b8a6"}
	classes := []string{"7А", "8А", "8Б", "9А", "9Б", "10А", "10Б", "11А", "11Б"}
	teachers := []string{"Иванова И.И.", "Петров П.П.", "Сидорова А.А.", "Кузнецов К.К.", "Смирнов С.С.", "Орлова О.О."}
	rooms := []string{"101", "102", "103", "201", "202", "203", "Спортзал", "ИТ-лаборатория"}
	periods := []pdf.Period{
		{Start: "08:00", End: "08:45"}, {Start: "08:55", End: "09:40"},
		{Start: "09:50", End: "10:35"}, {Start: "10:55", End: "11:40"},
		{Start: "11:50", End: "12:35"}, {Start: "12:45", End: "13:30"},
		{Start: "13:40", End: "14:25"},
	}

	// В ячейке может лежать второй урок — параллельная подгруппа класса.
	cellAt := func(mode string) func(rowID, day, slot int) (pdf.Cell, bool) {
		return func(rowID, day, slot int) (pdf.Cell, bool) {
			if (day+slot+rowID*2)%5 == 4 {
				return pdf.Cell{}, false
			}
			teacher := ((slot + rowID) % len(teachers)) + 1
			room := ((day + slot) % len(rooms)) + 1
			// In teacher/room modes the row IS that teacher/room, so the
			// sample grid shows their own (synthetic) occupancy.
			switch mode {
			case "teacher":
				teacher = rowID
			case "room":
				room = rowID
			}
			return pdf.Cell{
				SubjectID: ((day*3 + slot + rowID) % len(subjects)) + 1,
				TeacherID: teacher,
				RoomID:    room,
				ClassID:   (rowID % len(classes)) + 1,
				Conflict:  (rowID == 6 && day == 0 && slot == 2) || (rowID == 8 && day == 3 && slot == 4),
			}, true
		}
	}

	base := func(mode string, labels []string, ids []int) pdf.Options {
		rows := make([]pdf.Row, 0, len(labels))
		for i, l := range labels {
			rows = append(rows, pdf.Row{ID: ids[i], Label: l})
		}
		return pdf.Options{
			SchoolName: "МБОУ «Средняя школа №7 с углублённым изучением математики»",
			Title: map[string]string{
				"school": "вся школа", "class": "по классам",
				"teacher": "по учителям", "room": "по кабинетам",
			}[mode],
			Days:    6,
			Slots:   7,
			Periods: periods[:7],
			Mode:    mode,
			Rows:    rows,
			CellAt:  cellAt(mode),
			// У класса кабинет в подписи строки, у учителя — класс в ячейке:
			// как в реальном экспорте из app.go.
			ShowTeacher:  true,
			ShowRoom:     false,
			PageSize:     "A4",
			Orientation:  "portrait",
			SubjectName:  func(id int) string { return subjects[(id-1)%len(subjects)] },
			TeacherName:  func(id int) string { return teachers[(id-1)%len(teachers)] },
			RoomName:     func(id int) string { return rooms[(id-1)%len(rooms)] },
			ClassName:    func(id int) string { return classes[(id-1)%len(classes)] },
			SubjectColor: func(id int) string { return palette[(id-1)%len(palette)] },
		}
	}

	ids := func(n, from int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = from + i
		}
		return out
	}

	poster := base("school", classes, ids(len(classes), 1))
	poster.PageSize, poster.Orientation = "A2", "landscape"

	posterBW := poster
	posterBW.BW = true
	posterBW.PageSize, posterBW.Orientation = "A3", "landscape"

	classPages := base("class", []string{"10А", "11Б"}, []int{6, 8})
	classPages.PageSize, classPages.Orientation = "A4", "portrait"

	teacherPages := base("teacher", teachers[:2], []int{1, 3})
	teacherPages.PageSize, teacherPages.Orientation = "A4", "landscape"

	roomPages := base("room", rooms[:2], []int{1, 8})
	roomPages.PageSize, roomPages.Orientation = "A4", "portrait"

	wdOnly := classPages
	wdOnly.WeekdaysOnly = true

	cases := []struct {
		name string
		opts pdf.Options
	}{
		{"poster_A2_landscape", poster},
		{"poster_A3_landscape_bw", posterBW},
		{"class_A4_portrait", classPages},
		{"class_A4_portrait_weekdays", wdOnly},
		{"teacher_A4_landscape", teacherPages},
		{"room_A4_portrait", roomPages},
	}

	for _, c := range cases {
		b, err := pdf.Render(c.opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", c.name, err)
			os.Exit(1)
		}
		p := filepath.Join(outDir, c.name+".pdf")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("written %s (%d bytes)\n", p, len(b))
	}
}
