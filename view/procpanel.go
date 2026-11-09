package view

import (
	"image/color"

	"gioui.org/layout"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/theme"
)

// ProcPanel is the inside of the Processes card: the grouped table with
// its header, expandable groups and sort. The card frame around it is the
// View's.
type ProcPanel struct{}

// Layout draws t filling gtx.Constraints.Max, keeping only groups whose
// name, repo or branch contains filter (case-insensitive) when filter is set.
// swatch is a group's color in the CPU graph, by Group.Key; the panel draws
// it beside the group's name so a row and its band read as one thing.
func (p *ProcPanel) Layout(gtx layout.Context, th *theme.Theme, t procs.Table, filter string, swatch func(key string) color.NRGBA) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
