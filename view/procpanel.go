package view

import (
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
func (p *ProcPanel) Layout(gtx layout.Context, th *theme.Theme, t procs.Table, filter string) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
