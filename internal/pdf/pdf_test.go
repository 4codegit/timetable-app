package pdf

import (
	"strings"
	"testing"
)

// TestRenderPosterSmoke renders a small poster and verifies the output is a
// syntactically plausible, non-empty PDF.
func TestRenderPosterSmoke(t *testing.T) {
	opts := Options{
		SchoolName: "Тестовая школа",
		Title:      "вся школа",
		Days:       6,
		Slots:      8,
		Mode:       "school",
		Rows: []Row{
			{ID: 1, Label: "10А"},
			{ID: 2, Label: "11Б"},
		},
		CellAt: func(rowID, day, slot int) (Cell, bool) {
			if slot == 0 && day == 0 {
				return Cell{SubjectID: 10, TeacherID: 20, RoomID: 30, Conflict: rowID == 2}, true
			}
			return Cell{}, false
		},
		ShowTeacher: true,
		ShowRoom:    true,
		PageSize:    "A3",
		Orientation: "landscape",
		SubjectName: func(int) string {
			return "Математика очень длинное название предмета"
		},
		TeacherName:  func(int) string { return "Иванова И.И." },
		RoomName:     func(int) string { return "301" },
		ClassName:    func(int) string { return "5А" },
		SubjectColor: func(int) string { return "#dbeafe" },
	}
	b, err := Render(opts)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(b) < 1000 {
		t.Fatalf("PDF suspiciously small: %d bytes", len(b))
	}
	if !strings.HasPrefix(string(b[:8]), "%PDF") {
		t.Fatalf("not a PDF: %q", string(b[:8]))
	}
}

// TestRenderCompactSmoke renders the diary per-class pages and checks
// pagination plus the page-number footer do not crash the renderer.
func TestRenderCompactSmoke(t *testing.T) {
	opts := Options{
		SchoolName: "Тестовая школа",
		Title:      "по классам",
		Days:       5,
		Slots:      6,
		Mode:       "class",
		Rows: []Row{
			{ID: 1, Label: "10А"},
			{ID: 2, Label: "11Б"},
			{ID: 3, Label: "9В"},
		},
		CellAt: func(rowID, day, slot int) (Cell, bool) {
			return Cell{SubjectID: rowID, TeacherID: 20, RoomID: 30}, true
		},
		ShowTeacher:  true,
		ShowRoom:     true,
		PageSize:     "A4",
		Orientation:  "landscape",
		SubjectName:  func(int) string { return "Физика" },
		TeacherName:  func(int) string { return "Петров П.П." },
		RoomName:     func(int) string { return "203" },
		ClassName:    func(int) string { return "9В" },
		SubjectColor: func(int) string { return "#dcfce7" },
	}
	b, err := Render(opts)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(b) < 1000 {
		t.Fatalf("PDF suspiciously small: %d bytes", len(b))
	}
	// A PDF with N pages contains N "/Type /Page" objects (not /Pages).
	// aSc print style: exactly one page per row.
	pages := strings.Count(string(b), "/Type /Page") - strings.Count(string(b), "/Type /Pages")
	if pages != len(opts.Rows) {
		t.Fatalf("expected %d pages (one per row), got %d", len(opts.Rows), pages)
	}
}
