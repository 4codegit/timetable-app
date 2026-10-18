// Package pdf renders the school timetable to a PDF using a pure-Go PDF
// library and an embedded Unicode (Cyrillic-capable) font. Moving PDF
// generation off the frontend (which used jsPDF inside the Wails WebKit
// webview, where datauristring output was unreliable and the default
// fonts lacked Cyrillic glyphs) eliminates a whole class of export bugs.
//
// v1.9.0 — UI-style print:
//   - page header: school name top-left, print date in the footer, the
//     mode as the centered bold title over a thin rule;
//   - every mode ("school", "class", "teacher", "room") renders compact
//     per-row mini-tables — the exact look of the on-screen overview —
//     packed as densely as each page allows, so no row wastes a sheet;
//   - lesson cells are chips in the subject colour with white text;
//   - the subject-colour legend and the conflict list go under the last
//     page of tables;
//   - every page carries a footer: print date left, "стр. N из M" right.
package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/signintech/gopdf"
)

//go:embed fonts/DejaVuSans.ttf
var fontRegular []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var fontBold []byte

// Row is one "row" of the timetable grid: a class, a teacher, or a room.
// The Label is what gets printed at the left of the row.
type Row struct {
	ID    int
	Label string
}

// Cell is the contents of one timetable cell.
type Cell struct {
	SubjectID int
	TeacherID int
	RoomID    int
	ClassID   int // needed by the "teacher" mode (cell shows the class)
	Conflict  bool
}

// Options is the full description of one PDF export request.
//
// All strings are already user-facing (the caller is responsible for any
// localization). The PDF library only does layout.
type Options struct {
	SchoolName string
	Title      string // e.g. "по классам", "вся школа"

	// Grid geometry.
	Days  int // total days in week (1..7)
	Slots int // total slots per day

	// DaysMask marks which weekdays are school days (bit 0 = Monday …
	// bit 6 = Sunday). 0 prints Days days from Monday. Days outside the
	// mask get no column at all — like aSc Timetables, which does not
	// print non-teaching days.
	DaysMask int

	// Bell schedule (length == Slots). Empty start/end means "no label".
	Periods []Period

	// What to render.
	Mode string // "school" (per-class mini-tables), "class", "teacher", "room"
	Rows []Row

	// Lookup of a cell by (rowID, day, slot) -> Cell. Return ok=false for empty.
	CellAt func(rowID, day, slot int) (Cell, bool)

	// CellSubs returns ADDITIONAL lessons in the same cell — parallel
	// subgroups of the class (или параллельные подгруппы у учителя).
	// Ячейка печатается разделённой на полосы по числу уроков.
	// Nil = ячейки всегда одиночные.
	CellSubs func(rowID, day, slot int) []Cell

	// Display flags (mirror the frontend checkboxes).
	ShowTeacher  bool
	ShowRoom     bool
	WeekdaysOnly bool // hide Sat/Sun even if Days > 5
	BW           bool // black & white: gray fill for any subject, red for conflicts

	// Page setup.
	PageSize    string // "A0".."A4"
	Orientation string // "landscape" or "portrait"

	// Label resolvers — the caller passes these in so the PDF package
	// does not depend on the domain layer.
	SubjectName  func(id int) string
	TeacherName  func(id int) string // short name preferred by caller
	RoomName     func(id int) string
	ClassName    func(id int) string // used by the "teacher" mode cells
	SubjectColor func(id int) string // hex like "#dbeafe"
}

// Period is one bell slot.
type Period struct {
	Start string // "08:00" or ""
	End   string // "08:45" or ""
}

// Render produces the PDF bytes for the given options.
func Render(opts Options) ([]byte, error) {
	if err := validate(&opts); err != nil {
		return nil, err
	}

	wMM := pageWidthMM(opts.PageSize, opts.Orientation)
	hMM := pageHeightMM(opts.PageSize, opts.Orientation)

	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{
		Unit:     gopdf.UnitMM,
		PageSize: gopdf.Rect{W: wMM, H: hMM},
	})

	// Add the regular and bold faces. TTF bytes are embedded into the
	// binary via go:embed above, so the .exe/AppImage has no external
	// font dependency.
	if err := pdf.AddTTFFontDataWithOption("DejaVu", fontRegular, gopdf.TtfOption{Style: gopdf.Regular}); err != nil {
		return nil, fmt.Errorf("add regular font: %w", err)
	}
	// Register the bold face as its OWN family with the default (regular)
	// style flag. gopdf matches SetFont(family, "", size) against fonts whose
	// TtfOption.Style == Regular — a bold face registered with Style: Bold can
	// never be selected through SetFont and SetFont silently fails with
	// ErrMissingFontFamily. (The old renderer loaded the bold face with
	// Style: Bold and never actually used it, which is why this never blew up
	// before.)
	if err := pdf.AddTTFFontDataWithOption("DejaVu-Bold", fontBold, gopdf.TtfOption{}); err != nil {
		return nil, fmt.Errorf("add bold font: %w", err)
	}
	// Create the first page so SetFont / Text have a current page to
	// write into. (The grid renderer calls AddPage itself for each page
	// after the first, and reuses this one for page 1.)
	pdf.AddPage()
	if err := pdf.SetFont("DejaVu", "", 11); err != nil {
		return nil, fmt.Errorf("set regular font: %w", err)
	}

	mask := opts.DaysMask
	if opts.WeekdaysOnly {
		mask &= 0x1F // only Пн..Пт
	}
	dayIdx := activeDays(opts.Days, mask)

	th := newTheme(opts.BW)

	switch opts.Mode {
	case "school":
		// «Вся школа» — те же aSc-таблицы в едином стиле, но плотно
		// упакованные: несколько классов на одном листе.
		renderSchoolPacked(&pdf, opts, th, dayIdx, wMM, hMM)
	case "class", "teacher", "room":
		// Остальные режимы — одна страница на строку, та же отрисовка.
		renderASCPrintPages(&pdf, opts, th, dayIdx, wMM, hMM)
	default:
		return nil, fmt.Errorf("unknown mode %q", opts.Mode)
	}

	var buf bytes.Buffer
	if err := pdf.Write(&buf); err != nil {
		return nil, fmt.Errorf("write pdf: %w", err)
	}
	return buf.Bytes(), nil
}

// activeDays returns the weekday indexes to print: 0..daysN-1 for an
// empty mask, otherwise exactly the masked school days (bit 0 = Monday …
// bit 6 = Sunday). A mask that names no day falls back to the plain
// range — an empty table helps nobody.
func activeDays(daysN, mask int) []int {
	if mask == 0 {
		if daysN < 1 {
			daysN = 1
		}
		if daysN > 7 {
			daysN = 7
		}
		idx := make([]int, daysN)
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	var out []int
	for d := 0; d < 7; d++ {
		if mask&(1<<d) != 0 {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return activeDays(daysN, 0)
	}
	return out
}

func validate(o *Options) error {
	if o.Days <= 0 || o.Days > 7 {
		return fmt.Errorf("Days must be 1..7, got %d", o.Days)
	}
	if o.Slots <= 0 || o.Slots > 14 {
		return fmt.Errorf("Slots must be 1..14, got %d", o.Slots)
	}
	if o.CellAt == nil {
		return fmt.Errorf("CellAt is required")
	}
	if o.SubjectName == nil || o.TeacherName == nil || o.RoomName == nil || o.ClassName == nil {
		return fmt.Errorf("SubjectName/TeacherName/RoomName/ClassName are required")
	}
	if o.SubjectColor == nil {
		o.SubjectColor = func(int) string { return "#e5e7eb" }
	}
	if len(o.Periods) != o.Slots {
		// Tolerate missing periods; just don't print bell labels.
		o.Periods = make([]Period, o.Slots)
	}
	return nil
}

// pageWidthMM/pageHeightMM return the W/H pair for the chosen page in mm.
// gopdf uses millimeters as the unit (UnitMM) throughout.
func pageWidthMM(size, orient string) float64 {
	w, h := paperSizeMM(size)
	if orient == "landscape" {
		w, h = h, w
	}
	return w
}

func pageHeightMM(size, orient string) float64 {
	w, h := paperSizeMM(size)
	if orient == "landscape" {
		w, h = h, w
	}
	return h
}

// paperSizeMM returns (W, H) in millimeters for the given ISO A-series page.
func paperSizeMM(size string) (float64, float64) {
	switch strings.ToUpper(size) {
	case "A0":
		return 841, 1189
	case "A1":
		return 594, 841
	case "A2":
		return 420, 594
	case "A3":
		return 297, 420
	case "A4":
		return 210, 297
	default:
		return 297, 420 // A3 default — same as the old jsPDF default
	}
}

// mmToPt converts millimeters to PDF points (1 pt = 1/72 inch; 1 inch = 25.4 mm).
// Only used when we need a point measurement (e.g. MeasureTextWidth returns pt).
func mmToPt(mm float64) float64 {
	return mm * 72.0 / 25.4
}

// ptToMM converts points to millimeters.
func ptToMM(pt float64) float64 {
	return pt * 25.4 / 72.0
}

// lineHeightMM is the vertical advance (in mm) for one line of `size` pt
// text, including comfortable leading.
func lineHeightMM(size float64) float64 {
	return float64(size) * 0.3528 * 1.18
}

// minF returns the smallest of its arguments.
func minF(vals ...float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

// maxF returns the largest of its arguments.
func maxF(vals ...float64) float64 {
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

// clampF constrains v to [lo, hi].
func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// setFont switches the font family/size, ignoring lookup errors (the
// previous face stays active — a missing family must never truncate the
// whole document mid-render).
func setFont(pdf *gopdf.GoPdf, family string, size float64) {
	_ = pdf.SetFont(family, "", size)
}

// setText sets the text colour and font size in one call.
func setText(pdf *gopdf.GoPdf, r, g, b uint8, size float64) {
	pdf.SetTextColor(r, g, b)
	_ = pdf.SetFontSize(size)
}

// truncate shortens a string so it fits within maxMM millimeters (using
// the current font). Used for row labels that might otherwise overflow.
func truncate(pdf *gopdf.GoPdf, s string, maxMM float64) string {
	if s == "" {
		return ""
	}
	wMM := textWidthMM(pdf, s)
	if wMM <= maxMM {
		return s
	}
	runes := []rune(s)
	for n := len(runes); n > 1; n-- {
		candidate := string(runes[:n-1]) + "…"
		if textWidthMM(pdf, candidate) <= maxMM {
			return candidate
		}
	}
	return "…"
}

// textWidthMM measures the rendered width of s (in millimeters) using
// the currently-set font. gopdf's MeasureTextWidth already converts its
// internal PDF-point measurement into the configured unit (UnitMM here),
// so the value comes back in millimeters directly — converting it again
// (the v1.8.0 behaviour) under-measured every string by ~2.8x, which
// made wrapped lines, truncation and the legend flow all overflow.
func textWidthMM(pdf *gopdf.GoPdf, s string) float64 {
	wMM, err := pdf.MeasureTextWidth(s)
	if err != nil || wMM == 0 {
		return float64(len([]rune(s))) * 1.5
	}
	return wMM
}

func hexToRGB(hex string) (uint8, uint8, uint8) {
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string(h[0]) + string(h[0]) + string(h[1]) + string(h[1]) + string(h[2]) + string(h[2])
	}
	if len(h) != 6 {
		return 229, 231, 235 // #e5e7eb default gray
	}
	var r, g, b int
	fmt.Sscanf(h[:2], "%02x", &r)
	fmt.Sscanf(h[2:4], "%02x", &g)
	fmt.Sscanf(h[4:], "%02x", &b)
	return uint8(r), uint8(g), uint8(b)
}

func dayName(d int) string {
	names := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	if d >= 0 && d < len(names) {
		return names[d]
	}
	return fmt.Sprintf("Д%d", d+1)
}
