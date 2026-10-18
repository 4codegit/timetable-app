package pdf

import (
	"fmt"

	"github.com/signintech/gopdf"
)

// Единый aSc-стиль печати: сетка с чёрной рамкой, светло-серые шапка и
// колонка дней, ячейки цветом предмета с белым полным именем. Одна и та
// же отрисовка таблицы используется и для постраничных режимов
// (class/teacher/room), и для упакованной «вся школа».

// drawASCPrintTable paints one timetable in the unified aSc style:
// header row (period number over the bell time), day column with large
// bold captions, subject-colour lesson cells, heavy black outer frame.
// Font sizes scale with the cell dimensions, so the same function draws
// both a full-page table and a packed mini-table.
func drawASCPrintTable(pdf *gopdf.GoPdf, opts Options, row Row, x0, y0, dayColW, colW, rowH, hdrH float64, dayIdx []int, slots int) {
	black, dark, mid := "#111111", "#333333", "#555555"
	hdrBg, dayBg, dayText := "#f8fafc", "#f1f5f9", "#334155"
	if opts.BW {
		hdrBg, dayBg, dayText = "#ffffff", "#e8e8e8", "#111111"
	}
	daysN := len(dayIdx)

	ascFillRect(pdf, x0, y0, dayColW+colW*float64(slots), hdrH, hdrBg, black, 0.30)
	for si := 0; si < slots; si++ {
		// В шапке — только время звонка, жирным (без «П1..П7»).
		var lines []ascLine
		if si < len(opts.Periods) && opts.Periods[si].Start != "" {
			lines = append(lines, ascLine{
				Text: opts.Periods[si].Start + " - " + opts.Periods[si].End,
				Size: clampF(minF(8, hdrH*0.45), 4.5, 8), Bold: true, Color: dayText,
			})
		} else {
			// Звонки не заданы — возвращаемся к номеру урока.
			lines = append(lines, ascLine{
				Text: fmt.Sprintf("П%d", si+1),
				Size: clampF(minF(8, hdrH*0.45), 4.5, 8), Bold: true, Color: dayText,
			})
		}
		ascCenterLines(pdf, x0+dayColW+float64(si)*colW, y0, colW, hdrH, lines)
	}

	for i, di := range dayIdx {
		y := y0 + hdrH + float64(i)*rowH
		ascFillRect(pdf, x0, y, dayColW, rowH, dayBg, black, 0.30)
		ascCenterLines(pdf, x0, y, dayColW, rowH, []ascLine{
			{Text: dayName(di), Size: clampF(minF(16, rowH*0.45, dayColW*0.55), 6, 16), Bold: true, Color: dayText},
		})
		for si := 0; si < slots; si++ {
			cell, ok := opts.CellAt(row.ID, di, si)
			subs := []Cell{}
			if opts.CellSubs != nil {
				subs = opts.CellSubs(row.ID, di, si)
			}
			cx := x0 + dayColW + float64(si)*colW
			// Разделённая ячейка: полосы по числу уроков — целоклассовый
			// урок первой полосой, подгруппы следом (стиль aSc).
			bands := 1
			if ok || len(subs) > 0 {
				bands = 1 + len(subs)
			}
			bandH := rowH / float64(bands)
			if ok {
				drawASCPrintCell(pdf, opts, cell, true, cx, y, colW, bandH, black, dark, mid)
			} else {
				ascFillRect(pdf, cx, y, colW, bandH, "#ffffff", black, 0.30)
			}
			for bi, sc := range subs {
				drawASCPrintCell(pdf, opts, sc, true,
					cx, y+float64(bi+1)*bandH, colW, bandH, black, dark, mid)
			}
			for bi := 1; bi < bands; bi++ {
				ascHLine(pdf, cx, cx+colW, y+float64(bi)*bandH, black, 0.30)
			}
		}
	}

	// Жирная внешняя рамка — фирменный вид печати aSc.
	ascRectOutline(pdf, x0, y0, dayColW+colW*float64(slots), hdrH+rowH*float64(daysN), black, 0.70)
}

// drawASCPrintCell paints one lesson cell in the UI chip style on the
// aSc grid: subject-colour fill with the white bold full subject name
// and the teacher/room in a smaller white face beneath. BW keeps gray
// fills with dark text. Conflicts get a red fill plus a black marker.
func drawASCPrintCell(pdf *gopdf.GoPdf, opts Options, cell Cell, ok bool, x, y, w, h float64, black, dark, mid string) {
	if !ok {
		ascFillRect(pdf, x, y, w, h, "#ffffff", black, 0.30)
		return
	}
	if opts.BW {
		bg := "#ececec"
		if cell.Conflict {
			bg = "#d1d5db"
		}
		ascFillRect(pdf, x, y, w, h, bg, black, 0.30)
		drawPrintCellText(pdf, opts, cell, x, y, w, h, "#111111", "#333333", "#555555")
		if cell.Conflict {
			ascRectOutline(pdf, x+0.8, y+0.8, w-1.6, h-1.6, "#7f1d1d", 0.5)
		}
		return
	}
	bg := opts.SubjectColor(cell.SubjectID)
	if cell.Conflict {
		bg = "#dc2626"
	}
	ascFillRect(pdf, x, y, w, h, bg, black, 0.30)
	drawPrintCellText(pdf, opts, cell, x, y, w, h, "#ffffff", "#f1f5f9", "#e2e8f0")
	if cell.Conflict {
		ascRectOutline(pdf, x+0.8, y+0.8, w-1.6, h-1.6, black, 0.5)
	}
}

// drawPrintCellText stacks the cell content on three centered lines —
// the full subject name (bold) on top, then the teacher, then the room
// number, each on its own line. Every line shrinks just enough to fit
// the cell width on one line — no wrapping.
func drawPrintCellText(pdf *gopdf.GoPdf, opts Options, cell Cell, x, y, w, h float64, subjColor, teachColor, roomColor string) {
	subj := opts.SubjectName(cell.SubjectID)
	if subj == "" {
		subj = "?"
	}
	padW := w - 1.4

	// Подбор размера: строка сжимается шрифтом ровно до влезания.
	fitLine := func(s string, fam string, size, min float64) (string, float64) {
		setFont(pdf, fam, size)
		for size > min && textWidthMM(pdf, s) > padW {
			size -= 0.5
			setFont(pdf, fam, size)
		}
		if textWidthMM(pdf, s) > padW {
			s = truncate(pdf, s, padW)
		}
		return s, size
	}

	var lines []ascLine
	// Режим «по учителям»: страница принадлежит учителю, поэтому ячейка
	// показывает КЛАСС (первой строкой, жирным), затем предмет. Кабинет
	// не нужен — учитель ходит по кабинетам, но это их не касается.
	if opts.Mode == "teacher" {
		if cls := opts.ClassName(cell.ClassID); cls != "" && cls != "?" {
			s, sz := fitLine(cls, "DejaVu-Bold", 8.5, 5.0)
			lines = append(lines, ascLine{Text: s, Size: sz, Bold: true, Color: subjColor})
		}
		s, sz := fitLine(subj, "DejaVu", 6.5, 4.5)
		lines = append(lines, ascLine{Text: s, Size: sz, Color: teachColor})
		ascCenterLines(pdf, x, y, w, h, lines)
		return
	}
	s, sz := fitLine(subj, "DejaVu-Bold", 8.5, 5.0)
	lines = append(lines, ascLine{Text: s, Size: sz, Bold: true, Color: subjColor})
	if opts.ShowTeacher {
		if t := opts.TeacherName(cell.TeacherID); t != "" {
			s, sz := fitLine(t, "DejaVu", 6.5, 4.5)
			lines = append(lines, ascLine{Text: s, Size: sz, Color: teachColor})
		}
	}
	if opts.ShowRoom {
		if rn := opts.RoomName(cell.RoomID); rn != "" && rn != "?" {
			s, sz := fitLine(rn, "DejaVu", 6.5, 4.5)
			lines = append(lines, ascLine{Text: s, Size: sz, Color: roomColor})
		}
	}
	ascCenterLines(pdf, x, y, w, h, lines)
}

// renderASCPrintPages prints each row (class, teacher or room) on its
// own page in the unified aSc style. The grid is compact — the row
// height and column width are capped, the table is centered, and the
// rest of the sheet stays empty (like the aSc reference prints).
func renderASCPrintPages(pdf *gopdf.GoPdf, opts Options, th ascTheme, dayIdx []int, pageW, pageH float64) {
	const (
		margin  = 10.0
		footerH = 8.0
		dayColW = 18.0
		hdrH    = 10.0
		maxRowH = 11.0
		maxColW = 45.0
	)
	daysN := len(dayIdx)

	for pageNo, row := range opts.Rows {
		if pageNo > 0 {
			pdf.AddPage()
		}
		tableTop := ascPageHeader(pdf, opts, th, row.Label, opts.Title, pageW, margin)

		availW := pageW - margin*2
		tableH := pageH - tableTop - margin - footerH
		rowH := minF((tableH-hdrH)/float64(daysN), maxRowH)
		colW := minF((availW-dayColW)/float64(opts.Slots), maxColW)
		tableW := dayColW + colW*float64(opts.Slots)
		startX := margin + (availW-tableW)/2

		drawASCPrintTable(pdf, opts, row, startX, tableTop, dayColW, colW, rowH, hdrH, dayIdx, opts.Slots)
		ascPageFooter(pdf, opts, th, pageW, pageH, margin, pageNo+1, len(opts.Rows))
	}
}
