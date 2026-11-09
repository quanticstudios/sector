package view

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/theme"
)

// hover is the pointer over the CPU graph. While it is there the graph
// stops scrolling: its right edge stays at end.
type hover struct {
	on  bool
	x   float32
	end time.Time
}

// band is one series of the CPU graph as drawn: its color, legend name and
// cores per entry. The card shows cores as a percent of the machine.
type band struct {
	c    color.NRGBA
	name string
	vals []float64
}

// threads is the machine's logical CPU count, the graph's top.
func (v *View) threads() int {
	if n := v.last.Host.Threads; n > 0 {
		return n
	}
	return max(1, len(v.last.CPU.Threads))
}

func (v *View) hoverEvents(gtx layout.Context) {
	if v.measuring {
		return
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &v.hover, Kinds: pointer.Enter | pointer.Move | pointer.Leave | pointer.Cancel})
		if !ok {
			return
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Enter, pointer.Move:
			if !v.hover.on && len(v.hist) > 0 {
				v.hover.on, v.hover.end = true, v.hist[len(v.hist)-1].t
			}
			v.hover.x = pe.Position.X
		default:
			v.hover.on = false
		}
	}
}

// cpuCard is CPU by group: a stacked area of cores per group over the last
// minute, with a legend and a hover readout.
func (v *View) cpuCard(gtx layout.Context) layout.Dimensions {
	v.hoverEvents(gtx)
	th := v.th
	h := v.hist
	var end time.Time
	if len(h) > 0 {
		end = h[len(h)-1].t
	}
	if v.hover.on {
		end = v.hover.end
		for len(h) > 0 && h[len(h)-1].t.After(end) {
			h = h[:len(h)-1]
		}
	}
	h = since(h, end.Add(-window))
	series := bandSeries(h, v.bands)
	bands := make([]band, len(series))
	total := 0.0
	for i, s := range series {
		b := band{c: cOut, name: "Everything else", vals: s}
		if i < len(v.bands) {
			b.c, b.name = palette[v.slots[v.bands[i]]], v.label(v.bands[i])
		}
		bands[i] = b
		if len(s) > 0 {
			total += s[len(s)-1]
		}
	}
	threads := v.threads()

	head := v.head("CPU by group", "last 60 s",
		span{s: ppFormatPct(cpuPct(total, threads)) + "%", c: cSoft, tab: true}, span{s: "of", c: th.TextQuiet}, span{s: strconv.Itoa(threads) + " threads", c: cSoft})
	legend := func(gtx layout.Context) layout.Dimensions {
		items := make([]layout.Widget, len(bands))
		for i, b := range bands {
			now := 0.0
			if len(b.vals) > 0 {
				now = b.vals[len(b.vals)-1]
			}
			items[i] = func(gtx layout.Context) layout.Dimensions {
				return v.rich(gtx, theme.Size12, span{mark: swatch(b.c)}, span{s: b.name, c: th.Muted}, span{s: ppFormatPct(cpuPct(now, threads)) + "%", c: cDim, tab: true})
			}
		}
		return flow(gtx, gtx.Dp(14), gtx.Dp(4), items)
	}
	return v.card(gtx, func(gtx layout.Context) layout.Dimensions {
		return column(gtx, rigid(head), gap(gtx.Dp(6)), rigid(legend), gap(gtx.Dp(8)),
			flexed(gtx.Dp(150), func(gtx layout.Context) layout.Dimensions {
				return v.cpuGraph(gtx, h, end, bands, float64(threads))
			}).to(gtx.Dp(56)))
	})
}

// cpuGraph draws the stacked bands in the constraints' exact size, scaled
// so top cores is 100%, percent labels in a 36 dp gutter on the right and
// time labels underneath.
func (v *View) cpuGraph(gtx layout.Context, h []entry, end time.Time, bands []band, top float64) layout.Dimensions {
	th := v.th
	size := gtx.Constraints.Min
	pw := size.X - gtx.Dp(36)
	y0, y1 := float32(size.Y-gtx.Dp(16)), float32(gtx.Dp(4))
	y := func(c float64) float32 { return y0 - float32(c/top)*(y0-y1) }
	x := func(t time.Time) float32 { return float32(pw) * (1 - float32(end.Sub(t))/float32(window)) }

	// A short graph labels every second grid line, or only the top one.
	every := 1
	for every < 4 && (y0-y1)*float32(every)/4 < float32(gtx.Dp(18)) {
		every *= 2
	}
	for k := 1; k <= 4; k++ {
		c := top * float64(k) / 4
		yy := int(y(c))
		hline(gtx, 0, pw, yy, cGridLine)
		if k%every == 0 {
			v.placeLabel(gtx, size.X, yy, 1, 0.5, th.TextQuiet, strconv.Itoa(25*k)+"%")
		}
	}

	st := stack(seriesOf(bands))
	tops := make([][]f32.Point, len(st))
	for k, s := range st {
		tops[k] = make([]f32.Point, len(s))
		for j, c := range s {
			tops[k][j] = f32.Pt(x(h[j].t), y(c))
		}
	}
	plot := clip.Rect{Max: image.Pt(pw, size.Y)}.Push(gtx.Ops)
	for k := range tops {
		lower := make([]f32.Point, len(tops[k]))
		for j, p := range tops[k] {
			lower[j] = f32.Pt(p.X, y0)
			if k > 0 {
				lower[j] = tops[k-1][j]
			}
		}
		area(gtx, tops[k], lower, theme.Alpha(bands[k].c, 0.26))
	}
	for k := range tops {
		polyline(gtx, tops[k], float32(gtx.Dp(1.5)), bands[k].c)
	}
	plot.Pop()
	hline(gtx, 0, pw, int(y0), cAxis)
	v.staleVeil(gtx, image.Rect(0, 0, pw, int(y0)), true)

	ly := size.Y + gtx.Dp(2)
	v.placeLabel(gtx, 0, ly, 0, 1, th.TextQuiet, "60 s ago")
	v.placeLabel(gtx, pw/2, ly, 0.5, 1, th.TextQuiet, "30 s")
	v.placeLabel(gtx, pw, ly, 1, 1, th.TextQuiet, "now")

	if !v.measuring {
		a := clip.Rect{Max: size}.Push(gtx.Ops)
		event.Op(gtx.Ops, &v.hover)
		a.Pop()
	}
	if v.hover.on && len(h) > 0 {
		i := 0
		for j := range h {
			if math.Abs(float64(x(h[j].t)-v.hover.x)) < math.Abs(float64(x(h[i].t)-v.hover.x)) {
				i = j
			}
		}
		cx := int(x(h[i].t))
		rect(gtx, image.Rect(cx, 0, cx+1, int(y0)), 0, cDim)
		v.tooltip(gtx, h[i], end, i, bands, int(top), cx, pw)
	}
	return layout.Dimensions{Size: size}
}

func seriesOf(bands []band) [][]float64 {
	s := make([][]float64, len(bands))
	for i, b := range bands {
		s[i] = b.vals
	}
	return s
}

// placeLabel draws s with its anchor point (ax, ay, as fractions of its
// size) at x, y.
func (v *View) placeLabel(gtx layout.Context, x, y int, ax, ay float32, c color.NRGBA, s string) {
	v.placeText(gtx, theme.Size12, x, y, ax, ay, c, s, nil)
}

// placeText is placeLabel at size, and returns where the text went. When
// ok is set it draws only if ok accepts that rectangle.
func (v *View) placeText(gtx layout.Context, size unit.Sp, x, y int, ax, ay float32, c color.NRGBA, s string, ok func(image.Rectangle) bool) (image.Rectangle, bool) {
	c2 := gtx
	c2.Constraints = layout.Constraints{Max: image.Pt(big, big)}
	m := op.Record(gtx.Ops)
	d := v.th.Label(c2, size, font.Normal, c, s)
	call := m.Stop()
	at := image.Pt(x-int(ax*float32(d.Size.X)), y-int(ay*float32(d.Size.Y)))
	r := image.Rectangle{Min: at, Max: at.Add(d.Size)}
	if ok != nil && !ok(r) {
		return r, false
	}
	t := op.Offset(at).Push(gtx.Ops)
	call.Add(gtx.Ops)
	t.Pop()
	return r, true
}

// tooltip lists the groups at entry e, busiest first, beside the crosshair,
// in percent of threads logical CPUs.
func (v *View) tooltip(gtx layout.Context, e entry, end time.Time, i int, bands []band, threads, cx, pw int) {
	th := v.th
	ago := "now"
	if s := int(end.Sub(e.t).Round(time.Second) / time.Second); s > 0 {
		ago = strconv.Itoa(s) + " s ago"
	}
	order := make([]int, len(bands))
	sum := 0.0
	for k := range order {
		order[k] = k
		sum += bands[k].vals[i]
	}
	slices.SortStableFunc(order, func(a, b int) int {
		switch va, vb := bands[a].vals[i], bands[b].vals[i]; {
		case va > vb:
			return -1
		case va < vb:
			return 1
		}
		return 0
	})

	c := gtx
	c.Constraints = layout.Constraints{Max: image.Pt(big, big)}
	rec := func(w layout.Widget) (op.CallOp, layout.Dimensions) {
		m := op.Record(gtx.Ops)
		d := w(c)
		return m.Stop(), d
	}
	type line struct {
		name, num   op.CallOp
		nameD, numD layout.Dimensions
	}
	headCall, headD := rec(func(gtx layout.Context) layout.Dimensions {
		return v.rich(gtx, theme.Size12, span{s: ago, c: th.Fg, w: font.SemiBold}, span{s: "·", c: th.Muted},
			span{s: ppFormatPct(cpuPct(sum, threads)) + "%", c: th.Fg})
	})
	lines := make([]line, len(order))
	wName, wNum := 0, 0
	for j, k := range order {
		b := bands[k]
		lines[j].name, lines[j].nameD = rec(func(gtx layout.Context) layout.Dimensions {
			return v.rich(gtx, theme.Size12, span{mark: swatch(b.c)}, span{s: b.name, c: th.Muted})
		})
		lines[j].num, lines[j].numD = rec(v.text(theme.Size12, font.Normal, th.Fg, ppFormatPct(cpuPct(b.vals[i], threads))+"%"))
		wName, wNum = max(wName, lines[j].nameD.Size.X), max(wNum, lines[j].numD.Size.X)
	}
	px, py, lh := gtx.Dp(10), gtx.Dp(8), gtx.Dp(18)
	inner := max(headD.Size.X, wName+gtx.Dp(14)+wNum)
	box := image.Pt(inner+2*px, lh*(len(lines)+1)+2*py)
	left := cx + gtx.Dp(14)
	if left+box.X > pw {
		left = cx - box.X - gtx.Dp(14)
	}
	// A short graph lifts the box over the legend rather than past the card.
	defer op.Offset(image.Pt(left, min(0, gtx.Constraints.Min.Y-box.Y))).Push(gtx.Ops).Pop()
	r := image.Rectangle{Max: box}
	for _, s := range []struct {
		grow, drop int
		a          uint8
	}{{gtx.Dp(2), gtx.Dp(2), 0x59}, {gtx.Dp(10), gtx.Dp(10), 0x30}} {
		rect(gtx, r.Inset(-s.grow).Add(image.Pt(0, s.drop)), gtx.Dp(8)+s.grow, color.NRGBA{A: s.a})
	}
	rect(gtx, r, gtx.Dp(8), cTipRing)
	rect(gtx, r.Inset(1), gtx.Dp(7), th.SurfaceElevated)
	at := func(call op.CallOp, d layout.Dimensions, x, row int) {
		t := op.Offset(image.Pt(x, py+row*lh+(lh-d.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		t.Pop()
	}
	at(headCall, headD, px, 0)
	for j, l := range lines {
		at(l.name, l.nameD, px, j+1)
		at(l.num, l.numD, px+inner-l.numD.Size.X, j+1)
	}
}
