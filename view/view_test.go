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

func TestTabularDigitsShareAWidth(t *testing.T) {
	v := New(theme.Dark())
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	w := func(s string) int { return v.tabular(gtx, theme.Size12, 400, v.th.Fg, s).Size.X }
	if a, b := w("11.1 GiB"), w("88.8 GiB"); a != b {
		t.Fatalf("11.1 GiB is %d px, 88.8 GiB %d", a, b)
	}
	if a, b := w("9.9%"), w("10.0%"); a >= b {
		t.Fatalf("one more digit should widen: 9.9%% is %d px, 10.0%% %d", a, b)
	}
}

func TestSlotKeepsRowWidth(t *testing.T) {
	v := New(theme.Dark())
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1000, 100)}}
	row := func(val string) int {
		return v.rich(gtx, theme.Size12, span{s: "up"}, span{s: val, tab: true, slot: "88:88:88"}, span{s: "·"}, span{s: "20:35:12"}).Size.X
	}
	if a, b := row("1:00:00"), row("23:59:58"); a != b {
		t.Fatalf("row is %d px wide at 1:00:00 and %d at 23:59:58", a, b)
	}
	if a, b := row("23:59:58"), row("11:11:11"); a != b {
		t.Fatalf("row is %d px wide at 23:59:58 and %d at 11:11:11", a, b)
	}
}
