package view

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/theme"
)

// line is one series of a sparkline: a value per history entry, drawn
// solid with a fill under it, or dashed without one.
type line struct {
	val   func(e entry) float64
	c     color.NRGBA
	w     float32 // stroke in dp, 1.5 when 0
	fill  bool
	dashd bool
}

// recent is the history the lower cards draw and the time at its right edge.
func (v *View) recent() ([]entry, time.Time) {
	if len(v.hist) == 0 {
		return nil, time.Time{}
	}
	end := v.hist[len(v.hist)-1].t
	return since(v.hist, end.Add(-window)), end
}

// spark draws lines over the last minute, scaled so top is the upper rule,
// with label beside that rule and 0 beside the lower one. It fills
// gtx.Constraints.Min.Y, or 48 dp when that is 0.
func (v *View) spark(top float64, label string, lines ...line) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h, end := v.recent()
		w := gtx.Constraints.Max.X
		sh := gtx.Constraints.Min.Y
		if sh == 0 {
			sh = gtx.Dp(48)
		}
		pw := w - gtx.Dp(60)
		hline(gtx, 0, pw, 0, cSparkTop)
		hline(gtx, 0, pw, sh-1, cSparkBot)
		v.placeLabel(gtx, w, 0, 1, 0.5, v.th.TextQuiet, label)
		v.placeLabel(gtx, w, sh, 1, 0.5, v.th.TextQuiet, "0")
		if top <= 0 {
			top = 1
		}
		x := func(t time.Time) float32 { return float32(pw) * (1 - float32(end.Sub(t))/float32(window)) }
		y := func(val float64) float32 {
			return float32(sh-1) - float32(math.Min(1, math.Max(0, val/top)))*float32(sh-3)
		}
		defer clip.Rect{Min: image.Pt(0, -sh), Max: image.Pt(pw, 2*sh)}.Push(gtx.Ops).Pop()
		for _, l := range lines {
			pts := make([]f32.Point, len(h))
			base := make([]f32.Point, len(h))
			for i, e := range h {
				pts[i] = f32.Pt(x(e.t), y(l.val(e)))
				base[i] = f32.Pt(pts[i].X, float32(sh))
			}
			sw := l.w
			if sw == 0 {
				sw = 1.5
			}
			if l.fill {
				area(gtx, pts, base, theme.Alpha(l.c, 0.14))
			}
			if l.dashd {
				dashed(gtx, pts, float32(gtx.Dp(1))*sw, float32(gtx.Dp(4)), float32(gtx.Dp(3)), l.c)
			} else {
				polyline(gtx, pts, float32(gtx.Dp(1))*sw, l.c)
			}
		}
		return layout.Dimensions{Size: image.Pt(w, sh)}
	}
}

// kv is a row with label spans at the left and value spans at the right.
func (v *View) kv(left []span, right ...span) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return spread(gtx, func(gtx layout.Context) layout.Dimensions { return v.rich(gtx, theme.Size12, left...) },
			func(gtx layout.Context) layout.Dimensions { return v.rich(gtx, theme.Size12, right...) })
	}
}

// thin is a 4 dp bar filled to share in c.
func thin(share float64, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return bar(gtx, gtx.Dp(4), share, c) }
}

// rule is a one pixel line between gaps of above and below px, which
// halve when the card is short.
func (v *View) rule(above, below int) []part {
	line := func(gtx layout.Context) layout.Dimensions {
		hline(gtx, 0, gtx.Constraints.Max.X, 0, v.th.BorderSubtle)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 1)}
	}
	return []part{gap(above).to(above / 2), rigid(line), gap(below).to(below / 2)}
}

// sparkFloor is how short a sparkline gets in a squeezed card.
const sparkFloor = 24

func share(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func pct(f float64) string { return strconv.Itoa(int(math.Round(f*100))) + "%" }

// stackedCache is how much of Cached the graph and bar stack on Used.
// Used is Total minus Available, and Cached counts shmem and other pages
// that are not reclaimable, so the two can add up past Total; the stack
// stops at Total and the Cached row keeps the full number.
func stackedCache(m sample.Mem) uint64 {
	if m.Used >= m.Total {
		return 0
	}
	return min(m.Cached, m.Total-m.Used)
}

// memCard is memory over the last minute, a bar of used and cached, the
// four free(1) numbers and swap.
func (v *View) memCard(gtx layout.Context) layout.Dimensions {
	th := v.th
	m := v.last.Mem
	div, unit := unitOf(float64(m.Total))
	head := v.head("Memory", "last 60 s",
		span{s: inUnit(float64(m.Used), div), c: cSoft}, span{s: "of", c: th.TextQuiet}, span{s: inUnit(float64(m.Total), div) + " " + unit, c: cSoft})
	graph := v.spark(float64(m.Total), bytesStr(float64(m.Total)),
		line{val: func(e entry) float64 { return e.used + e.cached }, c: th.MemCache, w: 1, fill: true},
		line{val: func(e entry) float64 { return e.used }, c: th.MemUsed, fill: true})
	memrow := func(gtx layout.Context) layout.Dimensions {
		w, h, g := gtx.Constraints.Max.X, gtx.Dp(8), gtx.Dp(2)
		defer clip.UniformRRect(image.Rect(0, 0, w, h), gtx.Dp(3)).Push(gtx.Ops).Pop()
		u := int(float64(w) * share(m.Used, m.Total))
		c := int(float64(w) * share(stackedCache(m), m.Total))
		rect(gtx, image.Rect(0, 0, u, h), 0, th.MemUsed)
		rect(gtx, image.Rect(u+g, 0, min(w, u+g+c), h), 0, th.MemCache)
		rect(gtx, image.Rect(min(w, u+c+2*g), 0, w, h), 0, cTrack)
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
	num := func(n uint64) span { return span{s: bytesStr(float64(n)), c: th.Fg} }
	cells := []layout.Widget{
		v.kv([]span{{mark: swatch(th.MemUsed)}, {s: "Used", c: th.Muted}, {s: pct(share(m.Used, m.Total)), c: th.TextQuiet}}, num(m.Used)),
		v.kv([]span{{s: "Available", c: th.Muted}}, num(m.Available)),
		v.kv([]span{{mark: swatch(th.MemCache)}, {s: "Cached", c: th.Muted}}, num(m.Cached)),
		v.kv([]span{{s: "Free", c: th.Muted}}, num(m.Free)),
	}
	sdiv, sunit := unitOf(float64(m.SwapTotal))
	swap := v.kv([]span{{s: "Swap", c: th.Muted}},
		span{s: inUnit(float64(m.SwapUsed), sdiv), c: th.Fg}, span{s: "of", c: th.TextQuiet}, span{s: inUnit(float64(m.SwapTotal), sdiv) + " " + sunit, c: th.Fg})
	return v.card(gtx, func(gtx layout.Context) layout.Dimensions {
		parts := []part{rigid(head), gap(gtx.Dp(8)), flexed(gtx.Dp(48), graph).to(gtx.Dp(sparkFloor)), gap(gtx.Dp(10)), rigid(memrow), gap(gtx.Dp(9)), rigid(v.grid2(cells))}
		if m.SwapTotal > 0 {
			parts = append(parts, v.rule(gtx.Dp(9), gtx.Dp(8))...)
			parts = append(parts, rigid(swap), gap(gtx.Dp(5)), rigid(thin(share(m.SwapUsed, m.SwapTotal), th.MemUsed)))
		}
		return column(gtx, parts...)
	})
}

// grid2 lays cells two to a row, 20 dp apart, or one to a row on a narrow
// card.
func (v *View) grid2(cells []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		cols := 2
		if narrow(gtx) {
			cols = 1
		}
		w, g := gtx.Constraints.Max.X, gtx.Dp(20)
		cw := (w - (cols-1)*g) / cols
		y, rowH := 0, 0
		for i, cell := range cells {
			c := gtx
			c.Constraints = layout.Constraints{Min: image.Pt(cw, 0), Max: image.Pt(cw, big)}
			t := op.Offset(image.Pt((i%cols)*(cw+g), y)).Push(gtx.Ops)
			rowH = max(rowH, cell(c).Size.Y)
			t.Pop()
			if i%cols == cols-1 || i == len(cells)-1 {
				y += rowH + gtx.Dp(4)
				rowH = 0
			}
		}
		return layout.Dimensions{Size: image.Pt(w, max(0, y-gtx.Dp(4)))}
	}
}

// ioPair is the "read r, write w MB/s" key: dashed gray read, solid blue
// write.
func (v *View) ioPair(r, w string, strong bool) []span {
	th := v.th
	wt := font.Normal
	if strong {
		wt = font.Medium
	}
	return []span{
		{mark: ioMark(cIOOut, true)}, {s: "read", c: th.TextQuiet}, {s: r, c: th.Fg, w: wt},
		{mark: ioMark(th.Blue, false)}, {s: "write", c: th.TextQuiet}, {s: w, c: th.Fg, w: wt}, {s: "MB/s", c: th.TextQuiet},
	}
}

// diskCard is one block per drive: its mounts, busy share, a read and
// write sparkline on its own scale, and how full it is.
func (v *View) diskCard(gtx layout.Context) layout.Dimensions {
	drives := v.last.Drives
	var rd, wr float64
	for _, d := range drives {
		rd, wr = rd+d.ReadBps, wr+d.WriteBps
	}
	sub := strconv.Itoa(len(drives)) + " drives, last 60 s"
	if len(drives) == 1 {
		sub = "1 drive, last 60 s"
	}
	head := v.head("Disks", sub, v.ioPair(mbStr(rd), mbStr(wr), false)...)
	return v.card(gtx, func(gtx layout.Context) layout.Dimensions {
		parts := []part{rigid(head)}
		for i, d := range drives {
			if i > 0 {
				parts = append(parts, v.rule(gtx.Dp(10), gtx.Dp(10))...)
			} else {
				parts = append(parts, gap(gtx.Dp(10)).to(gtx.Dp(5)))
			}
			parts = append(parts, v.drive(gtx, d)...)
		}
		return column(gtx, parts...)
	})
}

// diskTop is the ceiling of a drive's sparkline in MB/s: the smallest power
// of two from 4 above its busiest second in the window, with 10% headroom.
func diskTop(h []entry, name string) float64 {
	peak := 0.0
	for _, e := range h {
		rw := e.drives[name]
		peak = max(peak, rw[0], rw[1])
	}
	return ceilPow2(peak/1e6*1.1, 4)
}

// drive is one drive's block in the disk card. A short card leaves out the
// used and free line first, the bar still shows how full the drive is,
// then the drive's read and write line, which the card's header sums.
func (v *View) drive(gtx layout.Context, d sample.Drive) []part {
	th := v.th
	h, _ := v.recent()
	top := diskTop(h, d.Name)
	head := func(gtx layout.Context) layout.Dimensions {
		busy := func(gtx layout.Context) layout.Dimensions {
			return v.rich(gtx, theme.Size12, span{s: "busy", c: th.Muted}, span{s: strconv.Itoa(int(math.Round(d.Busy*100))) + "%", c: th.Fg, w: font.SemiBold})
		}
		mounts := mountNames(d.Mounts)
		if narrow(gtx) || mounts == "" {
			return column(gtx, rigid(func(gtx layout.Context) layout.Dimensions {
				return spread(gtx, v.text(theme.Size12, font.SemiBold, th.Fg, d.Name), busy)
			}), gap(gtx.Dp(2)), rigid(v.text(theme.Size12, font.Normal, th.Muted, mounts)))
		}
		return spread(gtx, func(gtx layout.Context) layout.Dimensions {
			return v.rich(gtx, theme.Size12, span{s: d.Name, c: th.Fg, w: font.SemiBold}, span{s: mounts, c: th.Muted})
		}, busy)
	}
	graph := v.spark(top*1e6, strconv.FormatFloat(top, 'f', -1, 64)+" MB/s",
		line{val: func(e entry) float64 { return e.drives[d.Name][1] }, c: th.Blue, fill: true},
		line{val: func(e entry) float64 { return e.drives[d.Name][0] }, c: cIOOut, dashd: true})
	div, unit := unitOf(float64(d.Size))
	size := v.kv([]span{
		{s: inUnit(float64(d.Used), div) + " " + unit, c: th.Fg}, {s: "used ·", c: th.TextQuiet},
		{s: inUnit(float64(d.Free), div) + " " + unit, c: th.Fg}, {s: "free", c: th.TextQuiet},
	}, span{s: pct(share(d.Used, d.Size)), c: th.Fg}, span{s: "of", c: th.TextQuiet}, span{s: inUnit(float64(d.Size), div) + " " + unit, c: th.Fg})
	return []part{
		rigid(head), gap(gtx.Dp(7)), flexed(gtx.Dp(36), graph).to(gtx.Dp(sparkFloor)), gap(gtx.Dp(7)),
		rigid(v.kv(v.ioPair(mbStr(d.ReadBps), mbStr(d.WriteBps), true))).opt(1), gap(gtx.Dp(5)).opt(1),
		rigid(size).opt(2), gap(gtx.Dp(5)).opt(2), rigid(thin(share(d.Used, d.Size), cBarGray)),
	}
}

// netCard is the busiest interface: a sparkline of down and up, then each
// direction's rate, peak over the history and total since boot.
func (v *View) netCard(gtx layout.Context) layout.Dimensions {
	th := v.th
	n, ok := mainNet(v.last.Nets)
	if !ok {
		return v.card(gtx, v.head("Network", "no interface up"))
	}
	var peakRx, peakTx float64
	for _, e := range v.hist {
		rt := e.nets[n.Name]
		peakRx, peakTx = max(peakRx, rt[0]), max(peakTx, rt[1])
	}
	h, _ := v.recent()
	top := 0.0
	for _, e := range h {
		rt := e.nets[n.Name]
		top = max(top, rt[0], rt[1])
	}
	top = ceilPow2(top*1.1, 1<<20)
	graph := v.spark(top, ceilStr(top),
		line{val: func(e entry) float64 { return e.nets[n.Name][0] }, c: th.Blue, fill: true},
		line{val: func(e entry) float64 { return e.nets[n.Name][1] }, c: cIOOut, dashd: true})
	row := func(name string, dash bool, c color.NRGBA, rate, peak float64, total uint64) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			left := []span{{mark: ioMark(c, dash)}, {s: name, c: th.Muted}, {s: rateStr(rate), c: th.Fg}}
			if narrow(gtx) {
				return v.rich(gtx, theme.Size12, left...)
			}
			return v.kv(left, span{s: "peak", c: th.TextQuiet}, span{s: bitsStr(peak), c: cSoft},
				span{s: "· total", c: th.TextQuiet}, span{s: bytesStr(float64(total)), c: cSoft})(gtx)
		}
	}
	return v.card(gtx, func(gtx layout.Context) layout.Dimensions {
		return column(gtx, rigid(v.head("Network", n.Name, span{s: n.Addr, c: cSoft})), gap(gtx.Dp(8)), flexed(gtx.Dp(48), graph).to(gtx.Dp(sparkFloor)), gap(gtx.Dp(8)),
			rigid(row("Down", false, th.Blue, n.RxBps, peakRx, n.RxTotal)), gap(gtx.Dp(6)),
			rigid(row("Up", true, cIOOut, n.TxBps, peakTx, n.TxTotal)))
	})
}

// mainNet is the interface that has moved the most bytes since boot.
func mainNet(nets []sample.Net) (sample.Net, bool) {
	best := -1
	for i, n := range nets {
		if best < 0 || n.RxTotal+n.TxTotal > nets[best].RxTotal+nets[best].TxTotal {
			best = i
		}
	}
	if best < 0 {
		return sample.Net{}, false
	}
	return nets[best], true
}
