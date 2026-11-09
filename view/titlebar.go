package view

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/theme"
)

// titleBar draws the mark, the host chip with the clock and uptime, and the
// tools: filter, interval and pause. It returns its size.
func (v *View) titleBar(gtx layout.Context, now time.Time) layout.Dimensions {
	th := v.th
	for i := range v.seg {
		if v.seg[i].Clicked(gtx) {
			v.interval.Store(int64(intervals[i]))
		}
	}
	if v.pause.Clicked(gtx) {
		v.paused.Store(!v.paused.Load())
	}
	wdp := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	roomy, tiny := wdp >= 1100, wdp <= 720

	h := gtx.Dp(48)
	w := gtx.Constraints.Max.X
	hline(gtx, 0, w, h-1, th.BorderSubtle)

	gtx.Constraints = layout.Exact(image.Pt(w-gtx.Dp(30), h-1))
	defer op.Offset(image.Pt(gtx.Dp(16), 0)).Push(gtx.Ops).Pop()

	host := v.last.Host
	detail := fmt.Sprintf("%s · %d threads · %s", shortModel(host.Model), host.Threads, bytesStr(float64(v.last.Mem.Total)))
	if host.Model == "" {
		detail = ""
	}
	clock, up := v.clockAt(now)
	sp := layout.Spacer{Width: 10}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return v.lead(gtx, detail, roomy, clock, up) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			fw := 148
			if !roomy {
				fw = 112
			}
			return v.filterField(gtx, gtx.Dp(unit.Dp(fw)))
		}),
	}
	if !tiny {
		children = append(children, layout.Rigid(sp.Layout), layout.Rigid(v.segmented))
	}
	children = append(children, layout.Rigid(sp.Layout), layout.Rigid(v.pauseButton))
	layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	})
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// lead is the title bar's left side: sector's mark, which an Embedded
// view leaves out, then the host chip once a sample names the host.
func (v *View) lead(gtx layout.Context, detail string, roomy bool, clock time.Time, up time.Duration) layout.Dimensions {
	var items []layout.FlexChild
	if !v.opts.Embedded {
		items = append(items, layout.Rigid(v.mark), layout.Rigid(layout.Spacer{Width: 18}.Layout))
	}
	if name := v.last.Host.Name; name != "" {
		items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return v.chip(gtx, name, detail, roomy, clock, up) }))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, items...)
}

// mark is sector's logo and name.
func (v *View) mark(gtx layout.Context) layout.Dimensions {
	th := v.th
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(9)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return logo(gtx, gtx.Dp(18), th.Blue, th.Yellow, th.Green, th.Fg)
		}),
		layout.Rigid(v.text(theme.Size16-1, font.SemiBold, th.Fg, "sector")),
	)
}

// chip is the host: monitor icon, name, the CPU and memory when roomy, then
// the clock and uptime. Both tick in tabular digits. The uptime keeps room
// for a two-digit hour and comes last, where the chip's padding absorbs a
// day count growing.
func (v *View) chip(gtx layout.Context, name, detail string, roomy bool, now time.Time, up time.Duration) layout.Dimensions {
	th := v.th
	return v.boxed(gtx, gtx.Dp(6), gtx.Dp(10), gtx.Dp(6), th.Surface, func(gtx layout.Context) layout.Dimensions {
		spans := []span{
			{mark: func(gtx layout.Context) layout.Dimensions {
				d := icon(gtx, icDesktop, gtx.Dp(13), th.Fg)
				d.Baseline = gtx.Dp(2)
				return d
			}},
			{s: name, c: th.Fg, w: font.SemiBold},
		}
		if roomy && detail != "" {
			spans = append(spans, span{s: detail, c: th.Muted})
		}
		ups := uptimeStr(up)
		upSlot := "88:88:88"
		if i := strings.IndexByte(ups, 'd'); i >= 0 {
			upSlot = ups[:i+2] + upSlot
		}
		spans = append(spans, span{s: "·", c: th.Muted},
			span{s: now.Format("15:04:05"), c: th.Fg, w: font.SemiBold, tab: true},
			span{s: "·", c: th.Muted}, span{s: "up", c: th.Muted}, span{s: ups, c: th.Muted, tab: true, slot: upSlot})
		return v.rich(gtx, theme.Size12, spans...)
	})
}

// boxed draws w on a rounded fill with vertical padding py and horizontal
// padding px.
func (v *View) boxed(gtx layout.Context, py, px, rad int, bg color.NRGBA, w layout.Widget) layout.Dimensions {
	c := gtx
	c.Constraints.Min = image.Point{}
	m := op.Record(gtx.Ops)
	d := w(c)
	call := m.Stop()
	size := d.Size.Add(image.Pt(2*px, 2*py))
	rect(gtx, image.Rectangle{Max: size}, rad, bg)
	t := op.Offset(image.Pt(px, py)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	t.Pop()
	return layout.Dimensions{Size: size, Baseline: d.Baseline + py}
}

// filterField is the search box whose text filters the process panel.
func (v *View) filterField(gtx layout.Context, w int) layout.Dimensions {
	th := v.th
	h := gtx.Dp(28)
	r := image.Rect(0, 0, w, h)
	rect(gtx, r, gtx.Dp(6), th.BorderSubtle)
	rect(gtx, r.Inset(1), gtx.Dp(5), th.Surface)
	x := gtx.Dp(9)
	is := gtx.Dp(13)
	t := op.Offset(image.Pt(x, (h-is)/2)).Push(gtx.Ops)
	icon(gtx, icSearch, is, th.TextQuiet)
	t.Pop()
	x += is + gtx.Dp(6)

	c := gtx
	c.Constraints = layout.Exact(image.Pt(w-x-gtx.Dp(9), gtx.Sp(theme.Size12)*4/3))
	t = op.Offset(image.Pt(x, (h-c.Constraints.Min.Y)/2)).Push(gtx.Ops)
	if v.filter.Len() == 0 {
		v.th.Label(c, theme.Size12, font.Normal, th.TextQuiet, "Filter")
	}
	ink := op.Record(gtx.Ops)
	paint.ColorOp{Color: th.Fg}.Add(gtx.Ops)
	text := ink.Stop()
	sel := op.Record(gtx.Ops)
	paint.ColorOp{Color: theme.Alpha(th.Primary, 0.35)}.Add(gtx.Ops)
	selection := sel.Stop()
	v.filter.Layout(c, th.Shaper, th.Font, theme.Size12, text, selection)
	t.Pop()
	return layout.Dimensions{Size: r.Max}
}

// segmented is the 1, 2 and 5 second interval control.
func (v *View) segmented(gtx layout.Context) layout.Dimensions {
	th := v.th
	cur := v.Interval()
	pad := gtx.Dp(3)
	m := op.Record(gtx.Ops)
	x := pad
	bh := gtx.Dp(22)
	for i := range v.seg {
		on := intervals[i] == cur
		label := fmt.Sprintf("%d s", int(intervals[i]/time.Second))
		t := op.Offset(image.Pt(x, pad)).Push(gtx.Ops)
		d := v.seg[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lm := op.Record(gtx.Ops)
			col := th.Muted
			if on {
				col = th.Fg
			}
			ld := v.th.Label(gtx, theme.Size12, font.Normal, col, label)
			lc := lm.Stop()
			s := image.Pt(ld.Size.X+gtx.Dp(16), bh)
			switch {
			case on:
				rect(gtx, image.Rectangle{Max: s}, gtx.Dp(5), th.SurfaceElevated)
			case v.seg[i].Hovered():
				rect(gtx, image.Rectangle{Max: s}, gtx.Dp(5), th.SurfaceSecondary)
			}
			pointer.CursorPointer.Add(gtx.Ops)
			lt := op.Offset(image.Pt(gtx.Dp(8), (bh-ld.Size.Y)/2)).Push(gtx.Ops)
			lc.Add(gtx.Ops)
			lt.Pop()
			return layout.Dimensions{Size: s}
		})
		t.Pop()
		x += d.Size.X + gtx.Dp(2)
	}
	call := m.Stop()
	size := image.Pt(x-gtx.Dp(2)+pad, bh+2*pad)
	r := image.Rectangle{Max: size}
	rect(gtx, r, gtx.Dp(8), th.BorderSubtle)
	rect(gtx, r.Inset(1), gtx.Dp(7), th.Surface)
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}

// pauseButton stops and resumes sampling; it shows play while paused.
func (v *View) pauseButton(gtx layout.Context) layout.Dimensions {
	th := v.th
	return v.pause.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		s := gtx.Dp(28)
		if v.pause.Hovered() {
			rect(gtx, image.Rect(0, 0, s, s), gtx.Dp(6), th.SurfaceSecondary)
		}
		pointer.CursorPointer.Add(gtx.Ops)
		ic, col := icPause, th.Muted
		if v.Paused() {
			ic, col = icPlay, th.Yellow
		}
		is := gtx.Dp(16)
		defer op.Offset(image.Pt((s-is)/2, (s-is)/2)).Push(gtx.Ops).Pop()
		icon(gtx, ic, is, col)
		return layout.Dimensions{Size: image.Pt(s, s)}
	})
}
