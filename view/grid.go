package view

// The dashboard is the concept's 12-column CSS grid, laid out by hand.

type cardID int

const (
	cardCPU cardID = iota
	cardCores
	cardMem
	cardDisk
	cardNet
	cardProcs
	nCards
)

// place is a card's cell: first column and span out of 12, first row and
// rows spanned.
type place struct{ col, span, row, rows int }

// places puts the shown cards for a dashboard wdp dp wide, like the
// concept's container queries. A hidden card gets the zero place, which
// spans no row. fr is the row that takes spare height, -1 for none:
// narrow layouts scroll instead.
//
// Wide, the cards stand in two columns: CPU, memory beside disks, and
// network on the left, cores and processes on the right. A column's last
// card stretches down to the other column's bottom, and when one column
// is empty the other takes the full width. Narrower, the same rows stack
// at full width, memory beside disks until 720 dp.
func places(wdp float32, hidden [nCards]bool) (p [nCards]place, fr int) {
	// rows keeps the shown cards of each row and drops the empty rows.
	rows := func(rs ...[]cardID) [][]cardID {
		var out [][]cardID
		for _, r := range rs {
			var keep []cardID
			for _, c := range r {
				if !hidden[c] {
					keep = append(keep, c)
				}
			}
			if len(keep) > 0 {
				out = append(out, keep)
			}
		}
		return out
	}
	// put lays rs into the columns from col, span wide, from row 0; its
	// last row reaches down to row end. Cards in a row share it evenly.
	put := func(rs [][]cardID, col, span, end int) {
		for i, r := range rs {
			n := 1
			if i == len(rs)-1 {
				n = max(1, end-i)
			}
			w := span / len(r)
			for j, c := range r {
				p[c] = place{col + j*w, w, i, n}
			}
		}
	}
	rowOf := func(c cardID) int {
		if hidden[c] {
			return -1
		}
		return p[c].row
	}
	switch {
	case wdp > 1000:
		left := rows([]cardID{cardCPU}, []cardID{cardMem, cardDisk}, []cardID{cardNet})
		right := rows([]cardID{cardCores}, []cardID{cardProcs})
		n := max(len(left), len(right))
		switch {
		case len(right) == 0:
			put(left, 0, 12, n)
		case len(left) == 0:
			put(right, 0, 12, n)
		default:
			put(left, 0, 6, n)
			put(right, 6, 6, n)
		}
		// The row with the graphs that grow best takes the spare height.
		for _, c := range []cardID{cardMem, cardDisk, cardProcs, cardCPU, cardNet, cardCores} {
			if r := rowOf(c); r >= 0 {
				return p, r
			}
		}
		return p, -1
	case wdp > 720:
		put(rows([]cardID{cardCPU}, []cardID{cardCores}, []cardID{cardMem, cardDisk}, []cardID{cardNet}, []cardID{cardProcs}), 0, 12, 0)
	default:
		put(rows([]cardID{cardCPU}, []cardID{cardCores}, []cardID{cardMem}, []cardID{cardDisk}, []cardID{cardNet}, []cardID{cardProcs}), 0, 12, 0)
	}
	return p, -1
}

// colSpan is the x and width in px of a card at p, in a grid w px wide
// with pad around it and gap between columns.
func colSpan(p place, w, pad, gap int) (x, cw int) {
	inner := w - 2*pad - 11*gap
	at := func(c int) int { return pad + c*gap + inner*c/12 }
	x = at(p.col)
	return x, at(p.col+p.span) - gap - x
}

// rowHeights sizes the rows: each row fits its single-row cards' natural
// heights, a card spanning rows stretches the last of them if it must, and
// row fr takes whatever is left of avail. When the natural rows are taller
// than avail and fr is set, the rows shrink toward the heights the cards'
// floors need, so the dashboard fits; only below those does it scroll.
func rowHeights(p [nCards]place, nat, floor [nCards]int, avail, gap, fr int) []int {
	h := fitRows(p, nat, gap)
	if fr < 0 {
		return h
	}
	total := gap * (len(h) - 1)
	for _, v := range h {
		total += v
	}
	if avail > total {
		h[fr] += avail - total
		return h
	}
	lo := fitRows(p, floor, gap)
	for i := range lo {
		lo[i] = min(lo[i], h[i])
	}
	return squeeze(h, lo, avail-gap*(len(h)-1))
}

// fitRows is the height of each row that fits the cards at heights hs.
func fitRows(p [nCards]place, hs [nCards]int, gap int) []int {
	n := 0
	for _, c := range p {
		n = max(n, c.row+c.rows)
	}
	h := make([]int, n)
	for i, c := range p {
		if c.rows == 1 {
			h[c.row] = max(h[c.row], hs[i])
		}
	}
	for i, c := range p {
		if c.rows > 1 {
			sum := gap * (c.rows - 1)
			for r := c.row; r < c.row+c.rows; r++ {
				sum += h[r]
			}
			if sum < hs[i] {
				h[c.row+c.rows-1] += hs[i] - sum
			}
		}
	}
	return h
}
