// Package view draws sector's dashboard into any Gio layout. sector's own
// window hosts it, and pitwall embeds it the same way: the host samples,
// calls Push, and calls Layout each frame.
package view

import (
	"image"
	"image/color"
	"maps"
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

// History is how many samples the graphs keep: the window at the shortest
// interval, 1 s, and a few before it.
const History = 125

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

	opts     Options
	pushedAt time.Time // the local clock at the last Push, to tick the host chip's clock
	labels   map[string]string
	stale    time.Time // SetStale's since, zero while the data is fresh

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

// Options change how a View draws.
type Options struct {
	// Embedded leaves sector's mark and name out of the title bar, for a
	// host whose own chrome already says what the view is.
	Embedded bool
}

// New returns a View drawn in th, sampling every 2 seconds until the user
// picks another interval.
func New(th *theme.Theme) *View { return NewWith(th, Options{}) }

// NewWith is New with opts.
func NewWith(th *theme.Theme, opts Options) *View {
	v := &View{th: th, opts: opts, slots: map[string]int{}, names: map[string]string{}}
	v.interval.Store(int64(2 * time.Second))
	v.filter.SingleLine = true
	v.scroll.Axis = layout.Vertical
	return v
}

// Push adds one sample and its process table, and clears SetStale. The
// host calls it once per Interval, and not while Paused.
//
// The title bar's clock is s.Time, ticking on from when Push ran, in
// s.Time's location: sample stamps it in the sampled machine's local zone,
// and gob keeps that zone's UTC offset, so a remote machine's samples show
// its own wall clock. A zero s.Time is stamped with the local clock.
func (v *View) Push(s sample.Snapshot, t procs.Table) {
	v.pushedAt = time.Now()
	if s.Time.IsZero() {
		s.Time = v.pushedAt
	}
	v.last, v.table, v.stale = s, t, time.Time{}
	e := entry{t: s.Time, cores: map[string]float64{}, drives: map[string]diskRate{}, nets: map[string][2]float64{},
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
		e.drives[d.Name] = diskRate{d.ReadBps, d.WriteBps, jamOf(d)}
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

// SetLabels names groups by Group.Key, such as an agent by the host's tab
// it runs in. A label replaces the group's repo and agent in the CPU
// legend and tooltip and its name in the process table. A nil map clears
// them. The view keeps a copy.
func (v *View) SetLabels(m map[string]string) { v.labels = maps.Clone(m) }

// SetStale says no sample has come since since, as when the sampled
// machine is out of reach. The graphs stay as they are under a quiet "No
// data since 20:41" until the next Push. A zero since clears it.
func (v *View) SetStale(since time.Time) { v.stale = since }

// Clicked returns the Group.Key of the group row the user clicked in the
// process table since the last call, once. A click also opens or closes
// the row.
func (v *View) Clicked() (key string, ok bool) {
	key, v.procs.clicked = v.procs.clicked, ""
	return key, key != ""
}

// label is a group's name in the CPU legend and tooltip: the host's label
// when it set one, else repo and agent.
func (v *View) label(key string) string {
	if l := v.labels[key]; l != "" {
		return l
	}
	return v.names[key]
}

// clockAt is the sampled machine's clock and uptime at local time now,
// run on from the last sample by the local clock.
func (v *View) clockAt(now time.Time) (time.Time, time.Duration) {
	if v.pushedAt.IsZero() {
		return now, 0
	}
	ran := now.Sub(v.pushedAt)
	up := v.last.Host.Uptime
	if up > 0 {
		up += ran
	}
	return v.last.Time.Add(ran), up
}

// staleText is SetStale's line, its time on the sampled machine's clock.
func (v *View) staleText() string {
	s := v.stale
	if !v.last.Time.IsZero() {
		s = s.In(v.last.Time.Location())
	}
	return "No data since " + s.Format("15:04")
}

// staleVeil dims r of a graph while the data is stale and, with say set,
// writes staleText in its middle when it fits there. Only the CPU graph
// says it: on the small graphs it would cover their tags.
func (v *View) staleVeil(gtx layout.Context, r image.Rectangle, say bool) {
	if v.stale.IsZero() {
		return
	}
	paint.FillShape(gtx.Ops, theme.Alpha(v.th.Surface, 0.72), clip.Rect(r).Op())
	if !say {
		return
	}
	c := r.Min.Add(r.Max).Div(2)
	v.placeText(gtx, theme.Size12, c.X, c.Y, 0.5, 0.5, v.th.Muted, v.staleText(), func(t image.Rectangle) bool { return t.In(r) })
}

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
	} else {
		// The wide layout grows the network graph from 48 to 92 dp, tall
		// enough for spark to label every gridline. The floor stays, so
		// a short window squeezes it back first.
		nat[cardNet] += gtx.Dp(44)
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
		span{s: commas(t.Procs), c: cSoft, tab: true}, span{s: "in", c: v.th.TextQuiet}, span{s: commas(len(t.Groups)) + " groups", c: cSoft, tab: true})
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
			v.procs.threads, v.procs.labels = v.threads(), v.labels
			v.procs.Layout(c, v.th, v.table, v.filter.Text(), v.groupColor)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}))
	})
}
