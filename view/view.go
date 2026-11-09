// Package view draws sector's dashboard into any Gio layout. sector's own
// window hosts it, and pitwall embeds it the same way: the host samples,
// calls Push, and calls Layout each frame.
package view

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/theme"
)

// History is how many samples the graphs keep.
const History = 120

// View is the dashboard: title bar, CPU by group, cores, memory, drives,
// network and the process groups.
type View struct {
	th       *theme.Theme
	last     sample.Snapshot
	table    procs.Table
	procs    ProcPanel
	interval time.Duration
	paused   bool
}

// New returns a View drawn in th, sampling every second until the user
// picks another interval.
func New(th *theme.Theme) *View { return &View{th: th, interval: time.Second} }

// Push adds one sample and its process table. The host calls it once per
// Interval, and not while Paused.
func (v *View) Push(s sample.Snapshot, t procs.Table) { v.last, v.table = s, t }

// Interval is the sampling period the user picked in the view.
func (v *View) Interval() time.Duration { return v.interval }

// Paused reports whether the user paused sampling.
func (v *View) Paused() bool { return v.paused }

// Layout draws the dashboard filling gtx.Constraints.Max.
func (v *View) Layout(gtx layout.Context) layout.Dimensions {
	paint.FillShape(gtx.Ops, v.th.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
