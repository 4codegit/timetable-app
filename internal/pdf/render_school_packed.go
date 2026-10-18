package pdf

import (
	"github.com/signintech/gopdf"
)

// renderSchoolPacked prints the whole school in the same unified aSc
// style as the other modes, but packs several per-class tables onto
// every page: a grid (cols × per page column) is chosen to fit all
// classes on as few sheets as possible with the largest cells. Each
// table carries a small caption with the class name above it.
func renderSchoolPacked(pdf *gopdf.GoPdf, opts Options, th ascTheme, dayIdx []int, pageW, pageH float64) {
	const (
		margin  = 10.0
		footerH = 8.0
		dayColW = 12.0
		hdrH    = 9.0
		capH    = 5.0
		gap     = 6.0
		minColW = 14.0
		maxColW = 40.0 // колонка не растягивается на весь лист (стиль aSc)
		minRowH = 6.0
	)
	daysN := len(dayIdx)
	n := len(opts.Rows)

	subtitle := "классов: " + itoa(n) + " · дней: " + itoa(daysN) + " · уроков в день: " + itoa(opts.Slots)
	tableTop := ascPageHeader(pdf, opts, th, opts.Title, subtitle, pageW, margin)
	availW := pageW - margin*2
	availH := pageH - tableTop - margin - footerH

	// Подбор сетки: минимум страниц, затем максимум площади ячейки
	// (min() сравнивал неверно: после насыщения rowH выигрывал вариант
	// с большим числом узких колонок), при равенстве — меньше колонок.
	// colW ограничен сверху maxColW: при малом числе классов таблица
	// не должна распираться на всю ширину страницы.
	type gridCand struct {
		cols, perCol, pages int
		score               float64
	}
	var cands []gridCand
	for cols := 1; cols <= 4; cols++ {
		colW := minF((availW-float64(cols)*dayColW-float64(cols-1)*gap)/float64(cols*opts.Slots), maxColW)
		if colW < minColW {
			break
		}
		for perCol := 1; perCol <= n; perCol++ {
			rowH := minF((availH/float64(perCol)-capH-hdrH-gap)/float64(daysN), 11.0)
			if rowH < minRowH {
				continue
			}
			capacity := cols * perCol
			cands = append(cands, gridCand{
				cols:   cols,
				perCol: perCol,
				pages:  (n + capacity - 1) / capacity,
				score:  colW * rowH,
			})
		}
	}
	if len(cands) == 0 {
		cands = append(cands, gridCand{cols: 1, perCol: 1, pages: n, score: minRowH})
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.pages < best.pages ||
			(c.pages == best.pages && c.score > best.score+1e-9) ||
			(c.pages == best.pages && c.score > best.score-1e-9 && c.cols < best.cols) {
			best = c
		}
	}

	cols, perCol := best.cols, best.perCol
	colW := minF((availW-float64(cols)*dayColW-float64(cols-1)*gap)/float64(cols*opts.Slots), maxColW)
	rowH := minF((availH/float64(perCol)-capH-hdrH-gap)/float64(daysN), 11.0)
	tableW := dayColW + colW*float64(opts.Slots)
	tableH := capH + hdrH + rowH*float64(daysN)
	perPage := cols * perCol
	totalPages := (n + perPage - 1) / perPage

	startX := margin + (availW-(float64(cols)*tableW+float64(cols-1)*gap))/2
	// Таблицы прижаты к шапке: вертикальное центрирование оставляло
	// пустоту в пол-листа, когда классов мало.
	startY := tableTop

	firstPage := true
	globalPage := 0
	for tp := 0; tp < totalPages; tp++ {
		globalPage++
		if firstPage {
			firstPage = false // Render() уже добавил первую страницу
		} else {
			pdf.AddPage()
		}
		ascPageHeader(pdf, opts, th, opts.Title, subtitle, pageW, margin)
		for i := 0; i < perPage; i++ {
			idx := tp*perPage + i
			if idx >= n {
				break
			}
			tc, tr := i%cols, i/cols
			x0 := startX + float64(tc)*(tableW+gap)
			y0 := startY + float64(tr)*(tableH+gap)

			// Подпись класса над таблицей — по центру.
			setFont(pdf, "DejaVu-Bold", 8)
			r, g, b := hexToRGB("#334155")
			pdf.SetTextColor(r, g, b)
			lw := textWidthMM(pdf, opts.Rows[idx].Label)
			pdf.SetX(x0 + (tableW-lw)/2)
			pdf.SetY(y0)
			_ = pdf.Text(opts.Rows[idx].Label)

			drawASCPrintTable(pdf, opts, opts.Rows[idx], x0, y0+capH, dayColW, colW, rowH, hdrH, dayIdx, opts.Slots)
		}
		ascPageFooter(pdf, opts, th, pageW, pageH, margin, globalPage, totalPages)
	}
}
