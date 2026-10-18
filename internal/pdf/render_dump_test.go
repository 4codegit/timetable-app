package pdf

import (
	"os"
	"testing"
)

// TestDumpSamplesForVisualCheck renders a poster and a per-class sample to
// /tmp so a human (or CI debug job) can eyeball the layout. It always runs
// but never fails the build on file-write errors.
func TestDumpSamplesForVisualCheck(t *testing.T) {
	poster := Options{
		SchoolName: "МБОУ «Средняя школа №7»",
		Title:      "вся школа",
		Days:       6,
		Slots:      8,
		Mode:       "school",
		Rows: []Row{
			{ID: 1, Label: "5А"}, {ID: 2, Label: "5Б"}, {ID: 3, Label: "6А"},
			{ID: 4, Label: "7А"}, {ID: 5, Label: "8Б"},
		},
		Periods: []Period{
			{Start: "08:30", End: "09:15"}, {Start: "09:25", End: "10:10"},
			{Start: "10:25", End: "11:10"}, {Start: "11:25", End: "12:10"},
			{Start: "12:20", End: "13:05"}, {Start: "13:15", End: "14:00"},
			{Start: "14:10", End: "14:55"}, {Start: "15:05", End: "15:50"},
		},
		CellAt: func(rowID, day, slot int) (Cell, bool) {
			if (day+rowID+slot)%3 == 0 && slot < 6 {
				return Cell{SubjectID: (rowID*7 + day) % 6, TeacherID: 20, RoomID: 30, Conflict: rowID == 3 && day == 1 && slot == 2}, true
			}
			return Cell{}, false
		},
		ShowTeacher: true,
		ShowRoom:    true,
		PageSize:    "A2",
		Orientation: "landscape",
		SubjectName: func(int) string { return "Математика" },
		TeacherName: func(int) string { return "Иванова" },
		RoomName:    func(int) string { return "301" },
		ClassName:   func(int) string { return "5А" },
		SubjectColor: func(id int) string {
			return [6]string{"#dbeafe", "#dcfce7", "#fef9c3", "#fae8ff", "#ffedd5", "#cffafe"}[id%6]
		},
	}
	b, err := Render(poster)
	if err != nil {
		t.Fatalf("poster render: %v", err)
	}
	_ = os.WriteFile("/tmp/poster.pdf", b, 0644)

	one := poster
	one.Mode = "class"
	one.Title = "по классам"
	one.PageSize = "A4"
	one.Orientation = "landscape"
	b2, err := Render(one)
	if err != nil {
		t.Fatalf("one-per-page render: %v", err)
	}
	_ = os.WriteFile("/tmp/onepage.pdf", b2, 0644)
	t.Logf("poster=%d bytes, onepage=%d bytes", len(b), len(b2))
}
