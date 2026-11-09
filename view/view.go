// Package view draws sector's dashboard into any Gio layout. sector's own
// window hosts it, and pitwall embeds it the same way: the host samples,
// calls Push, and calls Layout each frame.
package view

import (
	"image"
	"image/color"
	"sync/atomic"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/theme"
)

// History is how many samples the graphs keep.
const History = 120

// View is the dashboard: title bar, CPU by group, cores, memory, drives,
// network and the process groups.
type View struct {
	th    *theme.Theme
	last  sample.Snapshot
	table procs.Table
	procs ProcPanel
	// interval and paused are read by the host's sampling goroutine.
	interval atomic.Int64
	paused   atomic.Bool

	hist  []entry
	bands []string       // groups with a band in the CPU graph, busiest first
	slots map[string]int // palette slot by group key
	names map[string]string

	filter    widget.Editor
	seg       [3]widget.Clickable
	pause     widget.Clickable
	scroll    layout.List
	hover     hover
	measuring bool // a card is being sized, not drawn: read no events
}

// intervals are the choices of the title bar's segmented control.
var intervals = [3]time.Duration{time.Second, 2 * time.Second, 5 * time.Second}

// New returns a View drawn in th, sampling every second until the user
// picks another interval.
func New(th *theme.Theme) *View {
	v := &View{th: th, slots: map[string]int{}, names: map[string]string{}}
	v.interval.Store(int64(time.Second))
	v.filter.SingleLine = true
	v.scroll.Axis = layout.Vertical
	return v
}

// Push adds one sample and its process table. The host calls it once per
// Interval, and not while Paused.
func (v *View) Push(s sample.Snapshot, t procs.Table) {
	if s.Time.IsZero() {
		s.Time = time.Now()
	}
	v.last, v.table = s, t
	e := entry{t: s.Time, cores: map[string]float64{}, drives: map[string][2]float64{}, nets: map[string][2]float64{},
		used: float64(s.Mem.Used), cached: float64(stackedCache(s.Mem))}
	live := map[string]bool{}
	for _, g := range t.Groups {
		e.cores[g.Key] += g.Cores
		e.total += g.Cores
		live[g.Key] = true
		v.names[g.Key] = groupLabel(g)
	}
	if len(t.Groups) == 0 {
		e.total = s.CPU.Total * float64(len(s.CPU.Threads))
	}
	for _, d := range s.Drives {
		e.drives[d.Name] = [2]float64{d.ReadBps, d.WriteBps}
	}
	for _, n := range s.Nets {
		e.nets[n.Name] = [2]float64{n.RxBps, n.TxBps}
	}
	v.hist = keep(v.hist, e)
	v.bands = topBands(t)
	assign(v.slots, live, v.bands)
	for k := range v.names {
		if !live[k] {
			delete(v.names, k)
		}
	}
}

// Interval is the sampling period the user picked in the view.
func (v *View) Interval() time.Duration { return time.Duration(v.interval.Load()) }

// Paused reports whether the user paused sampling.
func (v *View) Paused() bool { return v.paused.Load() }

// groupColor is a group's band color in the CPU graph, gray when the group
// is folded into "Everything else".
func (v *View) groupColor(key string) color.NRGBA {
	for _, k := range v.bands {
		if k == key {
			return palette[v.slots[key]]
		}
	}
	return cOut
}

// Layout draws the dashboard filling gtx.Constraints.Max.
func (v *View) Layout(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, v.th.Bg, clip.Rect{Max: size}.Op())
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()

	now := time.Now()
	gtx.Execute(op.InvalidateCmd{At: now.Truncate(time.Second).Add(time.Second)})

	tb := v.titleBar(gtx, now)
	defer op.Offset(image.Pt(0, tb.Size.Y)).Push(gtx.Ops).Pop()
	avail := size.Y - tb.Size.Y
	gtx.Constraints = layout.Exact(image.Pt(size.X, avail))
	v.scroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions { return v.dash(gtx, avail) })
	return layout.Dimensions{Size: size}
}

// dash lays the cards out on the grid in avail px of height, or taller
// when even squeezed they need more and the host scrolls.
func (v *View) dash(gtx layout.Context, avail int) layout.Dimensions {
	w := gtx.Constraints.Max.X
	pad, gp := gtx.Dp(12), gtx.Dp(10)
	p, fr := places(float32(w) / gtx.Metric.PxPerDp)
	cards := [nCards]layout.Widget{v.cpuCard, v.coresCard, v.memCard, v.diskCard, v.netCard, v.procsCard}

	// Each card is measured as tall as it wants, and as short as it can
	// squeeze, which only the wide layout uses.
	measure := func(i int, maxY int) int {
		_, cw := colSpan(p[i], w, pad, gp)
		c := gtx
		c.Constraints = layout.Constraints{Min: image.Pt(cw, 0), Max: image.Pt(cw, maxY)}
		m := op.Record(gtx.Ops)
		defer m.Stop()
		return cards[i](c).Size.Y
	}
	var nat, floor [nCards]int
	v.measuring = true
	for i := range cards {
		nat[i] = measure(i, big)
		floor[i] = nat[i]
		if fr >= 0 {
			floor[i] = measure(i, 0)
		}
	}
	v.measuring = false
	if fr < 0 {
		nat[cardProcs] = max(nat[cardProcs], gtx.Dp(320))
	}

	rows := rowHeights(p, nat, floor, avail-2*pad, gp, fr)
	ys := make([]int, len(rows)+1)
	ys[0] = pad
	for r, h := range rows {
		ys[r+1] = ys[r] + h + gp
	}
	for i, card := range cards {
		x, cw := colSpan(p[i], w, pad, gp)
		h := ys[p[i].row+p[i].rows] - gp - ys[p[i].row]
		t := op.Offset(image.Pt(x, ys[p[i].row])).Push(gtx.Ops)
		c := gtx
		c.Constraints = layout.Exact(image.Pt(cw, h))
		card(c)
		t.Pop()
	}
	return layout.Dimensions{Size: image.Pt(w, ys[len(rows)]-gp+pad)}
}

// card draws the surface of a card and body inside its padding, clipped to
// the card. The card is gtx.Constraints.Min.Y tall, or as tall as body when
// that is 0.
func (v *View) card(gtx layout.Context, body layout.Widget) layout.Dimensions {
	pt, px, pb := gtx.Dp(11), gtx.Dp(12), gtx.Dp(12)
	w := gtx.Constraints.Max.X
	c := gtx
	c.Constraints = layout.Constraints{
		Min: image.Pt(w-2*px, max(0, gtx.Constraints.Min.Y-pt-pb)),
		Max: image.Pt(w-2*px, max(0, gtx.Constraints.Max.Y-pt-pb)),
	}
	m := op.Record(gtx.Ops)
	d := body(c)
	call := m.Stop()
	h := d.Size.Y + pt + pb
	if gtx.Constraints.Min.Y > 0 {
		h = gtx.Constraints.Min.Y
	}
	rect(gtx, image.Rect(0, 0, w, h), gtx.Dp(8), v.th.Surface)
	defer clip.Rect{Max: image.Pt(w, h)}.Push(gtx.Ops).Pop()
	t := op.Offset(image.Pt(px, pt)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	t.Pop()
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// narrow reports whether a card's content is under 340 dp, where the
// concept drops secondary labels and stacks pairs.
func narrow(gtx layout.Context) bool { return gtx.Constraints.Max.X < gtx.Dp(340) }

// head is a card's header row: bold title, a quiet subtitle that goes away
// on narrow cards, and right-aligned spans.
func (v *View) head(title, sub string, right ...span) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		th := v.th
		if narrow(gtx) {
			sub = ""
		}
		return spread(gtx, func(gtx layout.Context) layout.Dimensions {
			return v.rich(gtx, theme.Size12, span{s: title, c: th.Fg, w: font.SemiBold, size: theme.Size13}, span{s: sub, c: th.Muted, size: theme.Size12})
		}, func(gtx layout.Context) layout.Dimensions { return v.rich(gtx, theme.Size12, right...) })
	}
}

// procsCard frames the process panel, which is ProcPanel's to draw.
func (v *View) procsCard(gtx layout.Context) layout.Dimensions {
	t := v.table
	head := v.head("Processes", "by agent, repo and app",
		span{s: commas(t.Procs), c: cSoft}, span{s: "in", c: v.th.TextQuiet}, span{s: commas(len(t.Groups)) + " groups", c: cSoft})
	if v.measuring {
		return v.card(gtx, head)
	}
	return v.card(gtx, func(gtx layout.Context) layout.Dimensions {
		return column(gtx, rigid(head), gap(gtx.Dp(6)), flexed(0, func(gtx layout.Context) layout.Dimensions {
			// The table reaches 2 dp short of the card's edges.
			in := gtx.Dp(10)
			defer op.Offset(image.Pt(-in, 0)).Push(gtx.Ops).Pop()
			c := gtx
			c.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X+2*in, gtx.Constraints.Max.Y))
			v.procs.threads = v.threads()
			v.procs.Layout(c, v.th, v.table, v.filter.Text(), v.groupColor)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}))
	})
}
