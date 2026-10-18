package pdf

import (
	"github.com/signintech/gopdf"
)

// ascTheme — минимальная палитра страниц печати: приглушённый серый для
// служебного текста, цвет линии под заголовком и синий заголовка.
// Остальные цвета (сетка, ячейки, конфликты) задаются локально в
// drawASCPrintTable/drawASCPrintCell, потому что они едины для всех
// режимов и не зависят от ч/б-переключателя (ч/б обрабатывает opts.BW).
type ascTheme struct {
	grayText   string // school name, subtitle, footer
	ruleColor  string // rule under the page title
	titleColor string // page title blue
}

func newTheme(bw bool) ascTheme {
	if bw {
		return ascTheme{grayText: "#555555", ruleColor: "#333333", titleColor: "#111111"}
	}
	return ascTheme{grayText: "#94a3b8", ruleColor: "#e2e8f0", titleColor: "#1d4ed8"}
}

// ascLine is one line of text inside a centered text block.
type ascLine struct {
	Text  string
	Size  float64
	Bold  bool
	Color string
}

// ascFillRect paints a filled rectangle with an outline in one primitive.
func ascFillRect(pdf *gopdf.GoPdf, x, y, w, h float64, bg, lineColor string, lineW float64) {
	r, g, b := hexToRGB(bg)
	pdf.SetFillColor(r, g, b)
	lr, lg, lb := hexToRGB(lineColor)
	pdf.SetStrokeColor(lr, lg, lb)
	pdf.SetLineWidth(lineW)
	pdf.RectFromUpperLeftWithStyle(x, y, w, h, "DF")
}

// ascRectOutline strokes a rectangle without filling it.
func ascRectOutline(pdf *gopdf.GoPdf, x, y, w, h float64, color string, lineW float64) {
	r, g, b := hexToRGB(color)
	pdf.SetStrokeColor(r, g, b)
	pdf.SetLineWidth(lineW)
	pdf.RectFromUpperLeftWithStyle(x, y, w, h, "D")
}

func ascHLine(pdf *gopdf.GoPdf, x1, x2, y float64, color string, lineW float64) {
	r, g, b := hexToRGB(color)
	pdf.SetStrokeColor(r, g, b)
	pdf.SetLineWidth(lineW)
	pdf.Line(x1, y, x2, y)
}

// ascCenterLines draws a block of lines centered horizontally within
// [x, x+w] and vertically within [y, y+h]. gopdf's Text() places the
// text BASELINE at the current Y, so glyphs render above it — to make
// the visual top of each line land on ty we move the baseline down by
// the font ascent (~0.93 em for DejaVu). Without this the whole block
// hugs the top of the cell.
func ascCenterLines(pdf *gopdf.GoPdf, x, y, w, h float64, lines []ascLine) {
	total := 0.0
	for _, ln := range lines {
		total += lineHeightMM(ln.Size)
	}
	ty := y + (h-total)/2
	if ty < y+0.3 {
		ty = y + 0.3
	}
	for _, ln := range lines {
		if ln.Text == "" {
			continue
		}
		fam := "DejaVu"
		if ln.Bold {
			fam = "DejaVu-Bold"
		}
		setFont(pdf, fam, ln.Size)
		r, g, b := hexToRGB(ln.Color)
		pdf.SetTextColor(r, g, b)
		lw := textWidthMM(pdf, ln.Text)
		if lw > w-0.4 {
			// Should not happen (wrapping ellipsizes to fit), but if it
			// does, left-anchor instead of spilling out on both sides.
			pdf.SetX(x + 0.2)
		} else {
			pdf.SetX(x + (w-lw)/2)
		}
		pdf.SetY(ty + float64(ln.Size)*0.3528*0.93)
		_ = pdf.Text(ln.Text)
		ty += lineHeightMM(ln.Size)
	}
}

// ascPageHeader draws the page header: school name top-left, the big
// bold title centered, an optional muted subtitle under it, and a rule
// across the page. Returns the Y where the table may start.
func ascPageHeader(pdf *gopdf.GoPdf, opts Options, th ascTheme, bigTitle, subtitle string, pageW, margin float64) float64 {
	y := margin
	setFont(pdf, "DejaVu", 8)
	r, g, b := hexToRGB(th.grayText)
	pdf.SetTextColor(r, g, b)
	pdf.SetX(margin)
	pdf.SetY(y)
	_ = pdf.Text(truncate(pdf, opts.SchoolName, pageW*0.55))

	tSize := 16.5
	setFont(pdf, "DejaVu-Bold", tSize)
	tr, tg, tb := hexToRGB(th.titleColor)
	pdf.SetTextColor(tr, tg, tb)
	tw := textWidthMM(pdf, bigTitle)
	pdf.SetX((pageW - tw) / 2)
	pdf.SetY(y + 4.4)
	_ = pdf.Text(bigTitle)

	if subtitle != "" {
		setFont(pdf, "DejaVu", 8.5)
		g2r, g2g, g2b := hexToRGB(th.grayText)
		pdf.SetTextColor(g2r, g2g, g2b)
		sw := textWidthMM(pdf, subtitle)
		pdf.SetX((pageW - sw) / 2)
		pdf.SetY(y + 10.9)
		_ = pdf.Text(subtitle)
	}

	ruleY := y + 15.4
	ascHLine(pdf, margin, pageW-margin, ruleY, th.ruleColor, 0.45)
	return ruleY + 2.8
}

// ascPageFooter prints "стр. N из M" at the right, below the content
// area. Одностраничные документы нумерации не получают.
func ascPageFooter(pdf *gopdf.GoPdf, opts Options, th ascTheme, pageW, pageH, margin float64, pageNo, total int) {
	if total <= 1 {
		return
	}
	y := pageH - margin + 1.6
	setFont(pdf, "DejaVu", 8)
	r, g, b := hexToRGB(th.grayText)
	pdf.SetTextColor(r, g, b)
	right := "стр. " + itoa(pageNo) + " из " + itoa(total)
	w := textWidthMM(pdf, right)
	pdf.SetX(pageW - margin - w)
	pdf.SetY(y)
	_ = pdf.Text(right)
}

// itoa is a tiny helper to keep footer formatting allocation-light.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}
