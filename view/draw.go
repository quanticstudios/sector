package view

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/theme"
)

// Colors the concept uses beyond the theme's tokens.
var (
	cTrack    = theme.Hex("#23262d") // empty part of a bar
	cSoft     = theme.Hex("#b8bcc3") // numbers in a card header
	cDim      = theme.Hex("#c9ccd2") // legend numbers, die names
	cGridLine = theme.Hex("#22252c")
	cAxis     = theme.Hex("#3a3e46")
	cSparkTop = theme.Hex("#24272e")
	cSparkBot = theme.Hex("#2e3138")
	cIOOut    = theme.Hex("#c3c6cc") // read and up: a dashed light gray line
	cTipRing  = theme.Hex("#343944")
	cBarGray  = theme.Hex("#8e939c")
)

// big is the height a measuring pass offers: as much as a card wants.
const big = 1 << 20

// span is one run of text in a row, or a small drawn mark when mark is set.
type span struct {
	s    string
	c    color.NRGBA
	w    font.Weight
	size unit.Sp // 0 for the row's size
	mark layout.Widget
	// tab draws digits as tabular figures, so a value's width changes
	// only when its digit count does.
	tab bool
	// slot is the widest form of a tab value, like "88:88:88": the span
	// takes that much width even when its digit count drops.
	slot string
}

// rich lays spans side by side on one baseline, a space apart.
func (v *View) rich(gtx layout.Context, size unit.Sp, spans ...span) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	children := make([]layout.FlexChild, len(spans))
	for i, s := range spans {
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.mark != nil {
				return s.mark(gtx)
			}
			sz := s.size
			if sz == 0 {
				sz = size
			}
			if !s.tab && s.slot == "" {
				return v.th.Label(gtx, sz, s.w, s.c, s.s)
			}
			d := v.tabular(gtx, sz, s.w, s.c, s.s)
			if s.slot != "" {
				m := op.Record(gtx.Ops)
				d.Size.X = max(d.Size.X, v.tabular(gtx, sz, s.w, s.c, s.slot).Size.X)
				m.Stop()
			}
			return d
		})
	}
	return layout.Flex{Alignment: layout.Baseline, Gap: gtx.Sp(size) / 4}.Layout(gtx, children...)
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// tabular draws s with each digit centered in a cell as wide as the widest
// digit. Geist's digits are proportional, so without it a value's width
// changes with the digits it shows.
// ponytail: measures the ten digits on every call; cache the cell width per
// size and weight if frames get slow.
func (v *View) tabular(gtx layout.Context, size unit.Sp, w font.Weight, c color.NRGBA, s string) layout.Dimensions {
	gtx.Constraints = layout.Constraints{Max: image.Pt(big, big)}
	label := func(t string) (layout.Dimensions, op.CallOp) {
		m := op.Record(gtx.Ops)
		d := v.th.Label(gtx, size, w, c, t)
		return d, m.Stop()
	}
	if s == "" {
		d, _ := label(s)
		return d
	}
	cell := 0
	for r := '0'; r <= '9'; r++ {
		d, _ := label(string(r))
		cell = max(cell, d.Size.X)
	}
	var out layout.Dimensions
	for s != "" {
		end := 1
		if !isDigit(rune(s[0])) {
			if end = strings.IndexFunc(s, isDigit); end < 0 {
				end = len(s)
			}
		}
		d, call := label(s[:end])
		x, adv := out.Size.X, d.Size.X
		if isDigit(rune(s[0])) {
			x, adv = x+(cell-d.Size.X)/2, cell
		}
		t := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		t.Pop()
		out.Size = image.Pt(out.Size.X+adv, max(out.Size.Y, d.Size.Y))
		out.Baseline = d.Baseline
		s = s[end:]
	}
	return out
}

// text is one label as a widget.
func (v *View) text(size unit.Sp, w font.Weight, c color.NRGBA, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return v.th.Label(gtx, size, w, c, s) }
}

// spread lays left at the left edge and right at the right edge of the
// width, on one baseline. left is truncated when they meet.
func spread(gtx layout.Context, left, right layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	return layout.Flex{Alignment: layout.Baseline, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			d := left(gtx)
			d.Size.X = gtx.Constraints.Max.X
			return d
		}),
		layout.Rigid(right),
	)
}

// part is one piece of a column: rigid at its own height, or flex sharing
// the height left over, never under min px. When the column is shorter
// than its parts, each part shrinks toward its floor, and parts with a drop
// rank are left out whole, highest rank first, while even the floors do
// not fit.
type part struct {
	w     layout.Widget
	flex  bool
	min   int
	floor int // -1: no shrinking
	drop  int // 0: never left out
}

func rigid(w layout.Widget) part           { return part{w: w, floor: -1} }
func flexed(min int, w layout.Widget) part { return part{w: w, flex: true, min: min, floor: -1} }
func gap(h int) part {
	return rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(0, h)} })
}

// to lets p shrink to floor px. The widget is then laid out at an exact
// height between the two and must fit itself into it.
func (p part) to(floor int) part { p.floor = floor; return p }

// opt lets the column leave p out when short, rank by rank.
func (p part) opt(rank int) part { p.drop = rank; return p }

// squeeze fits heights into room: nat when they fit, floor when even
// that does not, and in between each one gives up the same share of
// its slack.
func squeeze(nat, floor []int, room int) []int {
	h := slices.Clone(nat)
	total, slack := 0, 0
	for i := range nat {
		total += nat[i]
		slack += max(0, nat[i]-floor[i])
	}
	over := total - room
	if over <= 0 || slack == 0 {
		return h
	}
	over = min(over, slack)
	run := 0
	for i := range h {
		s := max(0, nat[i]-floor[i])
		h[i] -= over*(run+s)/slack - over*run/slack
		run += s
	}
	return h
}

// column stacks parts in the full width and fills gtx.Constraints.Min.Y,
// giving the spare height to the flex parts in equal shares. When
// gtx.Constraints.Max.Y is less than the parts need, it squeezes them.
func column(gtx layout.Context, parts ...part) layout.Dimensions {
	w := gtx.Constraints.Max.X
	calls := make([]op.CallOp, len(parts))
	nat := make([]int, len(parts))
	floor := make([]int, len(parts))
	for i, p := range parts {
		if p.flex {
			nat[i] = p.min
		} else {
			c := gtx
			c.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, big)}
			m := op.Record(gtx.Ops)
			nat[i] = p.w(c).Size.Y
			calls[i] = m.Stop()
		}
		floor[i] = nat[i]
		if p.floor >= 0 {
			floor[i] = min(p.floor, nat[i])
		}
	}
	room := gtx.Constraints.Max.Y
	sum := func(hs []int) (n int) {
		for _, h := range hs {
			n += h
		}
		return n
	}
	for sum(nat) > room && sum(floor) > room {
		rank := 0
		for i, p := range parts {
			if nat[i] > 0 {
				rank = max(rank, p.drop)
			}
		}
		if rank == 0 {
			break
		}
		for i, p := range parts {
			if p.drop == rank {
				nat[i], floor[i] = 0, 0
			}
		}
	}
	hs := squeeze(nat, floor, room)
	extra := max(0, gtx.Constraints.Min.Y-sum(hs))
	nflex := 0
	for _, p := range parts {
		if p.flex {
			nflex++
		}
	}
	for i, p := range parts {
		if p.flex {
			share := extra / nflex
			extra -= share
			nflex--
			hs[i] += share
		}
		if p.flex || hs[i] != nat[i] && hs[i] > 0 {
			c := gtx
			c.Constraints = layout.Exact(image.Pt(w, hs[i]))
			m := op.Record(gtx.Ops)
			p.w(c)
			calls[i] = m.Stop()
		}
	}
	y := 0
	for i := range parts {
		if hs[i] == 0 && !parts[i].flex {
			continue
		}
		t := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		calls[i].Add(gtx.Ops)
		t.Pop()
		y += hs[i]
	}
	return layout.Dimensions{Size: image.Pt(w, y)}
}

// flow lays items left to right, wrapping to a new line when the width
// runs out, like CSS flex-wrap.
func flow(gtx layout.Context, gapX, gapY int, items []layout.Widget) layout.Dimensions {
	w := gtx.Constraints.Max.X
	x, y, lineH := 0, 0, 0
	for _, it := range items {
		c := gtx
		c.Constraints = layout.Constraints{Max: image.Pt(w, big)}
		m := op.Record(gtx.Ops)
		d := it(c)
		call := m.Stop()
		if x > 0 && x+d.Size.X > w {
			x, y, lineH = 0, y+lineH+gapY, 0
		}
		t := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		t.Pop()
		x += d.Size.X + gapX
		lineH = max(lineH, d.Size.Y)
	}
	return layout.Dimensions{Size: image.Pt(w, y+lineH)}
}

// rect fills r with corner radius rad.
func rect(gtx layout.Context, r image.Rectangle, rad int, c color.NRGBA) {
	paint.FillShape(gtx.Ops, c, clip.UniformRRect(r, rad).Op(gtx.Ops))
}

// hline is a one pixel rule across w at y.
func hline(gtx layout.Context, x0, x1, y int, c color.NRGBA) {
	paint.FillShape(gtx.Ops, c, clip.Rect{Min: image.Pt(x0, y), Max: image.Pt(x1, y+1)}.Op())
}

// polyline strokes the line through pts.
func polyline(gtx layout.Context, pts []f32.Point, width float32, c color.NRGBA) {
	if len(pts) < 2 {
		return
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(pts[0])
	for _, q := range pts[1:] {
		p.LineTo(q)
	}
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p.End(), Width: width}.Op())
}

// dashed strokes pts as dashes on px long with off px gaps. Gio's caps
// are round, so the caller allows for half the width at each end.
func dashed(gtx layout.Context, pts []f32.Point, width, on, off float32, c color.NRGBA) {
	if len(pts) < 2 {
		return
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	pos, drawing := float32(0), true // how far into the current dash or gap
	p.MoveTo(pts[0])
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		d := b.Sub(a)
		l := float32(math.Hypot(float64(d.X), float64(d.Y)))
		for t := float32(0); t < l; {
			n := on
			if !drawing {
				n = off
			}
			step := min(n-pos, l-t)
			t += step
			pos += step
			if q := a.Add(d.Mul(t / l)); drawing {
				p.LineTo(q)
			} else {
				p.MoveTo(q)
			}
			if pos >= n {
				drawing, pos = !drawing, 0
			}
		}
	}
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p.End(), Width: width}.Op())
}

// area fills between top and bottom, two lines over the same x values.
func area(gtx layout.Context, top, bottom []f32.Point, c color.NRGBA) {
	if len(top) < 2 {
		return
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(top[0])
	for _, q := range top[1:] {
		p.LineTo(q)
	}
	for i := len(bottom) - 1; i >= 0; i-- {
		p.LineTo(bottom[i])
	}
	p.Close()
	paint.FillShape(gtx.Ops, c, clip.Outline{Path: p.End()}.Op())
}

// swatch is a group's 8 dp color square, sitting on the text baseline.
func swatch(c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		s := gtx.Dp(8)
		rect(gtx, image.Rect(0, 0, s, s), gtx.Dp(2), c)
		return layout.Dimensions{Size: image.Pt(s, s)}
	}
}

// ioMark is the 12 dp line that keys a series: solid, or dashed for read
// and up.
func ioMark(c color.NRGBA, dash bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Dp(12), gtx.Dp(2)
		y := gtx.Dp(4)
		if !dash {
			rect(gtx, image.Rect(0, y, w, y+h), 0, c)
		} else {
			for x := 0; x < w; x += gtx.Dp(6) {
				rect(gtx, image.Rect(x, y, min(w, x+gtx.Dp(4)), y+h), 0, c)
			}
		}
		return layout.Dimensions{Size: image.Pt(w, y+h)}
	}
}

// bar is a 4 dp track filled to share (0 to 1) in c.
func bar(gtx layout.Context, h int, share float64, c color.NRGBA) layout.Dimensions {
	w := gtx.Constraints.Max.X
	rect(gtx, image.Rect(0, 0, w, h), h/2, cTrack)
	if f := int(float64(w)*math.Min(1, math.Max(0, share)) + .5); f > 0 {
		rect(gtx, image.Rect(0, 0, f, h), h/2, c)
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// heat is pitwall's status scale at k from 0 to 1: green, yellow, red.
func heat(th *theme.Theme, k float64) color.NRGBA {
	k = math.Max(0, math.Min(1, k)) * 2
	a, b := th.Green, th.Yellow
	if k > 1 {
		a, b, k = th.Yellow, th.Red, k-1
	}
	return theme.Mix(a, b, float32(k))
}
