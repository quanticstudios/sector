package view

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/quanticstudios/sector/procs"
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
	tag   string // a label at the line's right end, shown where it fits
}

// recent is the history the lower cards draw and the time at its right edge.
func (v *View) recent() ([]entry, time.Time) {
	if len(v.hist) == 0 {
		return nil, time.Time{}
	}
	end := v.hist[len(v.hist)-1].t
	return since(v.hist, end.Add(-window)), end
}

// spark draws lines over the window, scaled so top is the upper rule.
// The axis on the right labels top, 0 and a gridline every step, in the
// unit div bytes called unit; a short graph labels every second gridline,
// or none. shade, when set, tints the plot behind the lines from each
// entry back to the one before it. It fills gtx.Constraints.Min.Y, or
// 48 dp when that is 0.
func (v *View) spark(top, step, div float64, unit string, shade func(entry) color.NRGBA, lines ...line) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h, end := v.recent()
		w := gtx.Constraints.Max.X
		sh := gtx.Constraints.Min.Y
		if sh == 0 {
			sh = gtx.Dp(48)
		}
		if top <= 0 {
			top = 1
		}
		y := func(val float64) float32 {
			return float32(sh-1) - float32(math.Min(1, math.Max(0, val/top)))*float32(sh-3)
		}
		ts := ticks(top, step)
		room := gtx.Dp(22)
		every := 1
		for every <= len(ts) && float32(sh-3)*float32(step*float64(every)/top) < float32(room) {
			every *= 2
		}
		type tick struct {
			y int
			s string
		}
		marks := []tick{{sh, "0"}}
		for i, t := range ts {
			if yy := int(y(t)); (i+1)%every == 0 && yy >= room {
				marks = append(marks, tick{yy, tickStr(t / div)})
			}
		}
		// Every label stays in the gutter, which fits the widest of them as
		// the CPU graph's does: the top number on the top rule, its unit on
		// the line below when the graph is tall enough, the other numbers on
		// their gridlines.
		quiet := v.th.TextQuiet
		measure := func(s string) image.Rectangle {
			r, _ := v.placeText(gtx, theme.Size11, 0, 0, 0, 0, quiet, s, func(image.Rectangle) bool { return false })
			return r
		}
		topStr := tickStr(top / div)
		withUnit := sh >= gtx.Dp(64)
		gw := measure(topStr).Dx()
		if withUnit {
			gw = max(gw, measure(unit).Dx())
		}
		for _, m := range marks {
			gw = max(gw, measure(m.s).Dx())
		}
		pw := w - gw - gtx.Dp(8)
		x := func(t time.Time) float32 { return float32(pw) * (1 - float32(end.Sub(t))/float32(window)) }
		if shade != nil {
			px := func(t time.Time) int { return int(math.Round(float64(x(t)))) }
			for _, r := range runs(h) {
				for j := r[0] + 1; j < r[1]; j++ {
					if c := shade(h[j]); c.A > 0 {
						rect(gtx, image.Rect(max(0, px(h[j-1].t)), 0, min(pw, px(h[j].t)), sh), 0, c)
					}
				}
			}
		}
		hline(gtx, 0, pw, 0, cSparkTop)
		hline(gtx, 0, pw, sh-1, cSparkBot)
		tr, _ := v.placeText(gtx, theme.Size11, w, 0, 1, 0.5, quiet, topStr, nil)
		unitEnd := 0
		if withUnit {
			ur, _ := v.placeText(gtx, theme.Size11, w, tr.Max.Y, 1, 0, quiet, unit, nil)
			unitEnd = ur.Max.Y
		}
		for _, m := range marks {
			if m.y < sh {
				hline(gtx, 0, pw, m.y, cGridLine)
			}
			if m.y < sh && m.y-tr.Dy()/2 < unitEnd+gtx.Dp(2) {
				continue
			}
			v.placeText(gtx, theme.Size11, w, m.y, 1, 0.5, quiet, m.s, nil)
		}
		func() {
			defer clip.Rect{Min: image.Pt(0, -sh), Max: image.Pt(pw, 2*sh)}.Push(gtx.Ops).Pop()
			v.sparkLines(gtx, h, x, y, sh, lines)
		}()
		v.sparkTags(gtx, h, x, y, sh, lines)
		v.staleVeil(gtx, image.Rect(0, 0, pw, sh), false)
		return layout.Dimensions{Size: image.Pt(w, sh)}
	}
}

// sparkLines strokes and fills the lines of a sparkline sh px tall.
func (v *View) sparkLines(gtx layout.Context, h []entry, x func(time.Time) float32, y func(float64) float32, sh int, lines []line) {
	for _, r := range runs(h) {
		v.sparkRun(gtx, h[r[0]:r[1]], x, y, sh, lines)
	}
}

// sparkRun draws the lines over h, which has no gap.
func (v *View) sparkRun(gtx layout.Context, h []entry, x func(time.Time) float32, y func(float64) float32, sh int, lines []line) {
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
}

// sparkTags writes each line's tag inside the plot at its right end, above
// the line or else below it, and leaves out a tag that fits neither way
// without leaving the plot, covering another tag or crossing another line's
// end.
func (v *View) sparkTags(gtx layout.Context, h []entry, x func(time.Time) float32, y func(float64) float32, sh int, lines []line) {
	if len(h) == 0 {
		return
	}
	var taken []image.Rectangle
	gap := gtx.Dp(2)
	e := h[len(h)-1]
	for i, l := range lines {
		if l.tag == "" {
			continue
		}
		px, py := int(x(e.t))-gtx.Dp(4), int(y(l.val(e)))
		for _, at := range []struct {
			y  int
			ay float32
		}{{py - gap, 1}, {py + gap, 0}} {
			r, ok := v.placeText(gtx, theme.Size11, px, at.y, 1, at.ay, v.th.TextQuiet, l.tag, func(r image.Rectangle) bool {
				if r.Min.Y < 0 || r.Max.Y > sh || r.Min.X < 0 {
					return false
				}
				for _, t := range taken {
					if r.Overlaps(t) {
						return false
					}
				}
				for j, o := range lines {
					if oy := int(y(o.val(e))); j != i && oy >= r.Min.Y && oy <= r.Max.Y {
						return false
					}
				}
				return true
			})
			if ok {
				taken = append(taken, r)
				break
			}
		}
	}
}

// ticks are the gridline values under top, step apart, leaving out one
// within half a step of top.
func ticks(top, step float64) []float64 {
	var out []float64
	for k := 1.0; step > 0 && k*step < top-step/2+1e-9*top; k++ {
		out = append(out, k*step)
	}
	return out
}

// niceStep rounds v to a half step of its leading digit: 15.1 to 15, 7.8
// to 8, 0.48 to 0.5.
func niceStep(v float64) float64 {
	if v <= 0 {
		return 0
	}
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	return math.Round(v/mag*2) / 2 * mag
}

// tickStr is an axis value with about three significant digits and no
// trailing zeros: 60.5, 32, 0.25.
func tickStr(v float64) string {
	s := num3(v)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
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

// memCard is memory over the window, a bar of used and cached, the
// four free(1) numbers and swap.
func (v *View) memCard(gtx layout.Context) layout.Dimensions {
	th := v.th
	m := v.last.Mem
	div, unit := unitOf(float64(m.Total))
	head := v.head("Memory", "last "+windowStr,
		span{s: inUnit(float64(m.Used), div), c: cSoft, tab: true}, span{s: "of", c: th.TextQuiet}, span{s: inUnit(float64(m.Total), div) + " " + unit, c: cSoft})
	graph := v.spark(float64(m.Total), niceStep(float64(m.Total)/div/4)*div, div, unit, nil,
		line{val: func(e entry) float64 { return e.used + e.cached }, c: th.MemCache, w: 1, fill: true, tag: "cached " + inUnit(float64(m.Cached), div)},
		line{val: func(e entry) float64 { return e.used }, c: th.MemUsed, fill: true, tag: "used " + inUnit(float64(m.Used), div)})
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
	num := func(n uint64) span { return span{s: bytesStr(float64(n)), c: th.Fg, tab: true} }
	cells := []layout.Widget{
		v.kv([]span{{mark: swatch(th.MemUsed)}, {s: "Used", c: th.Muted}, {s: pct(share(m.Used, m.Total)), c: th.TextQuiet, tab: true}}, num(m.Used)),
		v.kv([]span{{s: "Available", c: th.Muted}}, num(m.Available)),
		v.kv([]span{{mark: swatch(th.MemCache)}, {s: "Cached", c: th.Muted}}, num(m.Cached)),
		v.kv([]span{{s: "Free", c: th.Muted}}, num(m.Free)),
	}
	sdiv, sunit := unitOf(float64(m.SwapTotal))
	swap := v.kv([]span{{s: "Swap", c: th.Muted}},
		span{s: inUnit(float64(m.SwapUsed), sdiv), c: th.Fg, tab: true}, span{s: "of", c: th.TextQuiet}, span{s: inUnit(float64(m.SwapTotal), sdiv) + " " + sunit, c: th.Fg})
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
		{mark: ioMark(cIOOut, true)}, {s: "read", c: th.TextQuiet}, {s: r, c: th.Fg, w: wt, tab: true},
		{mark: ioMark(th.Blue, false)}, {s: "write", c: th.TextQuiet}, {s: w, c: th.Fg, w: wt, tab: true}, {s: "MB/s", c: th.TextQuiet},
	}
}

// jam is how long a drive keeps the programs using it waiting.
type jam uint8

const (
	jamOK     jam = iota
	jamSlow       // programs that touch the drive notice
	jamJammed     // they stall
)

// jamOf grades a drive by how long its average request took, counting
// only a drive with a request or more in flight on average, so one slow
// flush on an idle drive does not count. A flash drive that keeps up
// answers in well under a millisecond; a spinning disk needs tens of
// milliseconds to seek, so it gets more room.
func jamOf(d sample.Drive) jam {
	slow, jammed := 0.003, 0.010
	if d.Rotational {
		slow, jammed = 0.040, 0.150
	}
	switch {
	case d.Queue < 1 || d.Wait < slow:
		return jamOK
	case d.Wait < jammed:
		return jamSlow
	}
	return jamJammed
}

// jamWord and jamColor name and color a jam in the drive's head, and
// jamShade tints the stretch of its graph where it lasted.
func (v *View) jamWord(j jam) (string, color.NRGBA) {
	if j == jamJammed {
		return "jammed", v.th.Red
	}
	return "slow", v.th.Yellow
}

func (v *View) jamShade(j jam) color.NRGBA {
	switch j {
	case jamJammed:
		return theme.Alpha(v.th.Red, 0.16)
	case jamSlow:
		return theme.Alpha(v.th.Yellow, 0.10)
	}
	return color.NRGBA{}
}

// topDisk is the process moving the most bytes to and from storage and
// its group.
func topDisk(t procs.Table) (procs.Proc, procs.Group, bool) {
	var bp procs.Proc
	var bg procs.Group
	for _, g := range t.Groups {
		for _, p := range g.Procs {
			if p.DiskBps > bp.DiskBps {
				bp, bg = p, g
			}
		}
	}
	return bp, bg, bp.DiskBps > 0
}

// diskCard is one block per drive: its mounts, how long it keeps programs
// waiting, a read and write sparkline on its own scale tinted where the
// drive was slow or jammed, and how full it is. While a drive is slow it
// names the process moving the most bytes.
func (v *View) diskCard(gtx layout.Context) layout.Dimensions {
	th := v.th
	drives := v.last.Drives
	var rd, wr float64
	worst := jamOK
	for _, d := range drives {
		rd, wr = rd+d.ReadBps, wr+d.WriteBps
		worst = max(worst, jamOf(d))
	}
	sub := strconv.Itoa(len(drives)) + " drives, last " + windowStr
	if len(drives) == 1 {
		sub = "1 drive, last " + windowStr
	}
	head := v.head("Disks", sub, v.ioPair(mbStr(rd), mbStr(wr), false)...)
	return v.card(gtx, func(gtx layout.Context) layout.Dimensions {
		parts := []part{rigid(head)}
		if p, g, ok := topDisk(v.table); ok && worst > jamOK {
			// The rate before the names, so a long group name is what gets cut.
			who := []span{{s: "most disk I/O", c: th.Muted}, {s: mbStr(p.DiskBps), c: th.Fg, w: font.SemiBold, tab: true},
				{s: "MB/s", c: th.TextQuiet}, {s: p.Comm, c: th.Fg, w: font.SemiBold}}
			if g.Kind == procs.Agent || g.Kind == procs.App {
				who = append(who, span{s: "in " + v.label(g.Key), c: th.Muted})
			}
			parts = append(parts, gap(gtx.Dp(6)), rigid(func(gtx layout.Context) layout.Dimensions {
				return v.rich(gtx, theme.Size12, who...)
			}))
		}
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
		peak = max(peak, rw.r, rw.w)
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
			if d.Wait == 0 && d.Queue == 0 {
				return v.rich(gtx, theme.Size12, span{s: "idle", c: th.Muted})
			}
			wait := []span{{s: "waits", c: th.Muted}, {s: waitStr(d.Wait), c: th.Fg, w: font.SemiBold, tab: true}}
			j := jamOf(d)
			if j == jamOK {
				return v.rich(gtx, theme.Size12, wait...)
			}
			word, c := v.jamWord(j)
			return v.rich(gtx, theme.Size12, append([]span{{s: word, c: c, w: font.SemiBold}}, append(wait,
				span{s: "·", c: th.TextQuiet}, span{s: strconv.Itoa(int(math.Round(d.Queue))), c: th.Fg, w: font.SemiBold, tab: true}, span{s: "queued", c: th.Muted})...)...)
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
	graph := v.spark(top*1e6, top*1e6/4, 1e6, "MB/s", func(e entry) color.NRGBA { return v.jamShade(e.drives[d.Name].jam) },
		line{val: func(e entry) float64 { return e.drives[d.Name].w }, c: th.Blue, fill: true},
		line{val: func(e entry) float64 { return e.drives[d.Name].r }, c: cIOOut, dashd: true})
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
	ndiv, nunit := unitOf(top)
	graph := v.spark(top, top/4, ndiv, nunit+"/s", nil,
		line{val: func(e entry) float64 { return e.nets[n.Name][0] }, c: th.Blue, fill: true},
		line{val: func(e entry) float64 { return e.nets[n.Name][1] }, c: cIOOut, dashd: true})
	row := func(name string, dash bool, c color.NRGBA, rate, peak float64, total uint64) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			left := []span{{mark: ioMark(c, dash)}, {s: name, c: th.Muted}, {s: rateStr(rate), c: th.Fg, tab: true}}
			if narrow(gtx) {
				return v.rich(gtx, theme.Size12, left...)
			}
			return v.kv(left, span{s: "peak", c: th.TextQuiet}, span{s: bitsStr(peak), c: cSoft, tab: true},
				span{s: "· total", c: th.TextQuiet}, span{s: bytesStr(float64(total)), c: cSoft, tab: true})(gtx)
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
