package view

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/theme"
)

func ppFixture() procs.Table {
	kids := make([]procs.Proc, 15)
	for i := range kids {
		kids[i] = procs.Proc{PID: 100 + i, Comm: "w" + strconv.Itoa(i), Threads: 4}
	}
	return procs.Table{
		Groups: []procs.Group{
			{Key: "a", Kind: procs.Agent, Name: "claude", Agent: "claude", Repo: "acme-api", Branch: "fix/login", Cores: 3, RSS: 1 << 30, Threads: 10, Procs: kids[:2]},
			{Key: "b", Kind: procs.App, Name: "Chromium", Cores: 2, RSS: 4 << 30, Threads: 300, Procs: kids},
			{Key: "c", Kind: procs.Agent, Name: "codex", Agent: "codex", Repo: "billing", Cores: 1, RSS: 2 << 30, Threads: 50, Procs: kids[:3]},
			{Key: "o", Kind: procs.Other, Name: "Everything else", Cores: 0.5, RSS: 9 << 30, Threads: 900, Procs: kids},
		},
		Procs: 1071, Threads: 4812,
	}
}

func ppKeys(gs []procs.Group) string {
	s := ""
	for _, g := range gs {
		s += g.Key
	}
	return s
}

func TestPPMatch(t *testing.T) {
	g := ppFixture().Groups[0]
	for f, want := range map[string]bool{"": true, "ACME": true, "fix/LOG": true, "Claude": true, "chromium": false} {
		if got := ppMatch(g, f); got != want {
			t.Errorf("ppMatch(%q) = %v, want %v", f, got, want)
		}
	}
}

func TestPPSortedPinsOtherLast(t *testing.T) {
	gs := ppFixture().Groups
	// Other first in the input, and the busiest by memory and threads.
	gs = append([]procs.Group{gs[3]}, gs[:3]...)
	for col, want := range map[ppCol]string{ppCPU: "abco", ppMem: "bcao", ppThreads: "bcao", ppProcs: "bcao"} {
		if got := ppKeys(ppSorted(gs, col)); got != want {
			t.Errorf("sort by %d = %s, want %s", col, got, want)
		}
	}
}

func TestPPExpandedSurvivesNewTable(t *testing.T) {
	var p ProcPanel
	p.toggle("b")
	next := ppFixture()
	next.Groups[1].Cores = 9 // a new scan, numbers moved
	rows := p.rows(next, "")
	// a, b, 12 of b's 15 kids, "+3 more", c, o
	if len(rows) != 17 {
		t.Fatalf("got %d rows, want 17", len(rows))
	}
	if rows[2].kid == nil || rows[2].g.Key != "b" || rows[14].more != 3 {
		t.Errorf("b's members not listed under it: %+v %+v", rows[2], rows[14])
	}
	p.toggle("b")
	if n := len(p.rows(next, "")); n != 4 {
		t.Errorf("collapsed: got %d rows, want 4", n)
	}
}

func TestPPFormat(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{ppFormatPct(3.24), "3.2"},
		{ppFormatPct(0.01), "0.0"},
		{ppFormatPct(9.94), "9.9"},
		{ppFormatPct(9.96), "10"},
		{ppFormatPct(47.4), "47"},
		{ppFormatPct(cpuPct(8, 32)), "25"},
		{ppFormatPct(cpuPct(0.5, 0)), "50"},
		{ppFormatMem(512 << 20), "512 MiB"},
		{ppFormatMem(1288490189), "1.2 GiB"},
		{ppFormatMem(1023<<20 + 1<<19), "1.0 GiB"},
		{ppFormatInt(1071), "1,071"},
		{ppFormatInt(999), "999"},
		{ppFormatInt(1234567), "1,234,567"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestPPNarrowHidesProcsAndThreads(t *testing.T) {
	has := func(w unit.Dp, col ppCol) bool {
		for _, c := range ppCols(w) {
			if c.col == col {
				return true
			}
		}
		return false
	}
	if !has(560, ppProcs) || !has(560, ppThreads) {
		t.Error("560 dp should show Procs and Threads")
	}
	if has(559, ppProcs) || has(559, ppThreads) || !has(559, ppCPU) || !has(559, ppMem) {
		t.Error("559 dp should show only CPU and Memory")
	}
}

func TestPPLayoutDraws(t *testing.T) {
	var p ProcPanel
	p.toggle("b")
	th := theme.Dark()
	for _, size := range []image.Point{{800, 600}, {400, 200}, {100, 20}} {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
		dims := p.Layout(gtx, th, ppFixture(), "", func(string) color.NRGBA { return th.Blue })
		if dims.Size != size {
			t.Errorf("size %v, want %v", dims.Size, size)
		}
		p.Layout(gtx, th, ppFixture(), "nothing", nil)
	}
}

func TestPPMergeSumsSameComm(t *testing.T) {
	ps := []procs.Proc{
		{PID: 11, Comm: "claude", Cores: 0.6, Threads: 45, RSS: 400},
		{PID: 12, Comm: "vitest", Cores: 0.5, Threads: 14, RSS: 100},
		{PID: 13, Comm: "node", Cores: 0.4, Threads: 20, RSS: 50},
		{PID: 14, Comm: "vitest", Cores: 0.5, Threads: 14, RSS: 100},
		{PID: 15, Comm: "vitest", Cores: 0.5, Threads: 14, RSS: 100},
	}
	got := ppMerge(ps)
	want := []ppKid{
		{Comm: "vitest", PID: 12, N: 3, Threads: 42, Cores: 1.5, RSS: 300},
		{Comm: "claude", PID: 11, N: 1, Threads: 45, Cores: 0.6, RSS: 400},
		{Comm: "node", PID: 13, N: 1, Threads: 20, Cores: 0.4, RSS: 50},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if n := ppKidNote(got[0]); n != "3 workers" {
		t.Errorf("merged note = %q", n)
	}
	if n := ppKidNote(got[1]); n != "11" {
		t.Errorf("single note = %q, want its pid", n)
	}
}

func TestPPOtherShowsFolded(t *testing.T) {
	for folded, want := range map[int]string{0: "", 1: "1 group", 31: "31 groups", 1200: "1,200 groups"} {
		g := procs.Group{Kind: procs.Other, Name: "Everything else", Folded: folded}
		if name, _, kind := ppLabels(g); name != "Everything else" || kind != want {
			t.Errorf("Folded %d: %q %q, want kind %q", folded, name, kind, want)
		}
	}
}

func TestPPBarScale(t *testing.T) {
	for _, c := range []struct{ busiest, want float64 }{
		{0, 5}, {0.3, 5}, {5, 5}, {5.1, 10}, {8.4, 10}, {10.1, 20}, {26, 50}, {50.1, 100}, {100, 100}, {math.NaN(), 5},
	} {
		if got := ppBarScale(c.busiest); got != c.want {
			t.Errorf("ppBarScale(%v) = %v, want %v", c.busiest, got, c.want)
		}
	}
}

func TestPPSVGPoints(t *testing.T) {
	subs := ppSVGPoints("M1 2l1.3.9-3 1H4v-2ZM3 3h1")
	if len(subs) != 2 || !subs[0].closed || subs[1].closed {
		t.Fatalf("got %+v", subs)
	}
	got := subs[0].pts
	want := []f32.Point{{X: 1, Y: 2}, {X: 2.3, Y: 2.9}, {X: -0.7, Y: 3.9}, {X: 4, Y: 3.9}, {X: 4, Y: 1.9}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if d := got[i].Sub(want[i]); math.Abs(float64(d.X)) > 1e-4 || math.Abs(float64(d.Y)) > 1e-4 {
			t.Errorf("point %d = %v, want %v", i, got[i], want[i])
		}
	}
	if p := subs[1].pts; len(p) != 2 || p[1] != (f32.Point{X: 4, Y: 3}) {
		t.Errorf("relative h: got %v, want a line to (4, 3)", p)
	}
	if n := len(ppSVGPoints(ppClaudeD)); n != 1 {
		t.Errorf("claude mark parses to %d subpaths, want 1", n)
	}
}
