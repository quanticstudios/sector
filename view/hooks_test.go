package view

import (
	"bytes"
	"encoding/gob"
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/theme"
)

// boxTime is 03:04:05 on a machine nine hours east of UTC.
var boxTime = time.Date(2026, 11, 9, 3, 4, 5, 0, time.FixedZone("", 9*3600))

func TestSetLabels(t *testing.T) {
	v := New(theme.Dark())
	tb := ppFixture()
	v.Push(sample.Snapshot{}, tb)
	if got := v.label("a"); got != "acme-api, claude" {
		t.Fatalf("legend without labels: %q", got)
	}
	v.SetLabels(map[string]string{"a": "Login flow"})
	if got := v.label("a"); got != "Login flow" {
		t.Errorf("legend with a label: %q", got)
	}
	if got := v.label("b"); got != "Chromium" {
		t.Errorf("a group with no label: %q", got)
	}
	if name, branch, kind := ppLabels(tb.Groups[0], "Login flow"); name != "Login flow" || branch != "fix/login" || kind != "claude" {
		t.Errorf("table row with a label: %q %q %q", name, branch, kind)
	}
	if name, _, kind := ppLabels(tb.Groups[1], "Browser"); name != "Browser" || kind != "app" {
		t.Errorf("app row with a label: %q %q", name, kind)
	}
	v.SetLabels(nil)
	if got := v.label("a"); got != "acme-api, claude" {
		t.Errorf("after SetLabels(nil): %q", got)
	}
}

func TestClickedGroupRow(t *testing.T) {
	v := New(theme.Dark())
	v.Push(sample.Snapshot{Host: sample.Host{Threads: 8}}, ppFixture())
	var r input.Router
	ops := new(op.Ops)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: ops, Now: time.Now(), Source: r.Source(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 400))}
		v.procs.Layout(gtx, v.th, v.table, "", nil)
		r.Frame(ops)
	}
	frame()
	at := f32.Pt(100, 32+15) // the first group row, under the 32 dp header
	r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
	)
	frame()
	if key, ok := v.Clicked(); !ok || key != "a" {
		t.Fatalf("Clicked = %q %v, want a", key, ok)
	}
	if key, ok := v.Clicked(); ok {
		t.Errorf("a second Clicked = %q, want none", key)
	}
	if !v.procs.expanded["a"] {
		t.Error("the click did not open the row")
	}
}

// TestRemoteClock checks the title bar shows the sampled machine's clock,
// in its zone after gob, ticking on by the local clock.
func TestRemoteClock(t *testing.T) {
	s := sample.Snapshot{Time: boxTime, Host: sample.Host{Name: "devbox", Uptime: time.Hour}}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(s); err != nil {
		t.Fatal(err)
	}
	var got sample.Snapshot
	if err := gob.NewDecoder(&buf).Decode(&got); err != nil {
		t.Fatal(err)
	}
	v := New(theme.Dark())
	v.Push(got, procs.Table{})
	clock, up := v.clockAt(v.pushedAt.Add(10 * time.Second))
	if c := clock.Format("15:04:05"); c != "03:04:15" {
		t.Errorf("clock %s, want 03:04:15", c)
	}
	if up != time.Hour+10*time.Second {
		t.Errorf("uptime %v, want 1h0m10s", up)
	}
}

func TestStale(t *testing.T) {
	v := New(theme.Dark())
	v.Push(sample.Snapshot{Time: boxTime}, procs.Table{})
	v.SetStale(time.Date(2026, 11, 8, 11, 41, 30, 0, time.UTC)) // 20:41 on the sampled machine
	if v.stale.IsZero() {
		t.Fatal("SetStale did not take")
	}
	if got := v.staleText(); got != "No data since 20:41" {
		t.Errorf("stale line %q", got)
	}
	v.Push(sample.Snapshot{Time: boxTime.Add(time.Minute)}, procs.Table{})
	if !v.stale.IsZero() {
		t.Error("Push did not clear the stale state")
	}
}

func TestEmbeddedLeavesOutTheMark(t *testing.T) {
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1000, 48)}}
	lead := func(o Options) int { return NewWith(theme.Dark(), o).lead(gtx, "", true, time.Now(), 0).Size.X }
	if w := lead(Options{}); w == 0 {
		t.Error("the mark is missing from a standalone view")
	}
	if w := lead(Options{Embedded: true}); w != 0 {
		t.Errorf("an embedded view with no host is %d px wide on the left, want 0", w)
	}
}
