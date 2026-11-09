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

// places puts the cards for a dashboard wdp dp wide, like the concept's
// container queries. fr is the row that takes spare height, -1 for none:
// narrow layouts scroll instead.
func places(wdp float32) (p [nCards]place, fr int) {
	switch {
	case wdp > 1000:
		return [nCards]place{
			cardCPU: {0, 6, 0, 1}, cardCores: {6, 6, 0, 1},
			cardMem: {0, 3, 1, 1}, cardDisk: {3, 3, 1, 1},
			cardNet: {0, 6, 2, 1}, cardProcs: {6, 6, 1, 2},
		}, 1
	case wdp > 720:
		return [nCards]place{
			cardCPU: {0, 12, 0, 1}, cardCores: {0, 12, 1, 1},
			cardMem: {0, 6, 2, 1}, cardDisk: {6, 6, 2, 1},
			cardNet: {0, 12, 3, 1}, cardProcs: {0, 12, 4, 1},
		}, -1
	}
	return [nCards]place{
		cardCPU: {0, 12, 0, 1}, cardCores: {0, 12, 1, 1},
		cardMem: {0, 12, 2, 1}, cardDisk: {0, 12, 3, 1},
		cardNet: {0, 12, 4, 1}, cardProcs: {0, 12, 5, 1},
	}, -1
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
