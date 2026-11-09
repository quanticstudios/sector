package view

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/theme"
)

func TestNewSamplesEveryTwoSeconds(t *testing.T) {
	if got := New(nil).Interval(); got != 2*time.Second {
		t.Fatalf("default interval %v, want 2s", got)
	}
}

func TestSlotKeepsRowWidth(t *testing.T) {
	v := New(theme.Dark())
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1000, 100)}}
	row := func(val string) int {
		return v.rich(gtx, theme.Size12, span{s: "up"}, span{s: val, slot: "88:88:88"}, span{s: "·"}, span{s: "20:35:12"}).Size.X
	}
	if a, b := row("1:00:00"), row("23:59:58"); a != b {
		t.Fatalf("row is %d px wide at 1:00:00 and %d at 23:59:58", a, b)
	}
	if a, b := row("23:59:58"), row("11:11:11"); a != b {
		t.Fatalf("row is %d px wide at 23:59:58 and %d at 11:11:11", a, b)
	}
}
