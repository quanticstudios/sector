package view

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/quanticstudios/sector/procs"
)

func TestKeep(t *testing.T) {
	var h []entry
	t0 := time.Unix(0, 0)
	for i := range History + 10 {
		h = keep(h, entry{t: t0.Add(time.Duration(i) * time.Second)})
		if len(h) != min(i+1, History) {
			t.Fatalf("after %d pushes: %d entries", i+1, len(h))
		}
	}
	if len(h) != History || h[0].t != t0.Add(10*time.Second) {
		t.Fatalf("len %d, oldest %v", len(h), h[0].t)
	}
	want := int(window/time.Second) + 2
	if s := since(h, h[len(h)-1].t.Add(-window)); len(s) != want {
		t.Fatalf("since: %d entries, want %d: the window and one before", len(s), want)
	}
}

func TestStack(t *testing.T) {
	h := []entry{
		{total: 10, cores: map[string]float64{"a": 4, "b": 1, "x": 5}},
		{total: 6, cores: map[string]float64{"a": 3}},
	}
	s := bandSeries(h, []string{"a", "b"})
	if !slices.Equal(s[0], []float64{4, 3}) || !slices.Equal(s[1], []float64{1, 0}) || !slices.Equal(s[2], []float64{5, 3}) {
		t.Fatalf("series %v", s)
	}
	st := stack(s)
	if !slices.Equal(st[1], []float64{5, 3}) || !slices.Equal(st[2], []float64{10, 6}) {
		t.Fatalf("stacked %v", st)
	}
}

func TestTopBands(t *testing.T) {
	tb := procs.Table{Groups: []procs.Group{
		{Key: "a", Kind: procs.Agent}, {Key: "b", Kind: procs.App}, {Key: "s", Kind: procs.System}, {Key: "c", Kind: procs.Agent}, {Key: "d", Kind: procs.Agent}, {Key: "o", Kind: procs.Other},
	}}
	if got := topBands(tb); !slices.Equal(got, []string{"a", "b", "s", "c"}) {
		t.Fatalf("bands %v", got)
	}
}

func TestAssignKeepsColors(t *testing.T) {
	slots := map[string]int{}
	live := func(keys ...string) map[string]bool {
		m := map[string]bool{}
		for _, k := range keys {
			m[k] = true
		}
		return m
	}
	assign(slots, live("a", "b", "c", "d", "e"), []string{"a", "b", "c", "d"})
	want := map[string]int{"a": 0, "b": 1, "c": 2, "d": 3}
	for k, s := range want {
		if slots[k] != s {
			t.Fatalf("first pass: %v", slots)
		}
	}
	// Ranks swap and e climbs into the bands: nobody changes color, e
	// takes a fresh one, d (still live) keeps its own.
	assign(slots, live("a", "b", "c", "d", "e"), []string{"c", "e", "a", "b"})
	if slots["a"] != 0 || slots["b"] != 1 || slots["c"] != 2 || slots["d"] != 3 || slots["e"] != 4 {
		t.Fatalf("after swap: %v", slots)
	}
	// d comes back and gets its color again.
	assign(slots, live("a", "b", "c", "d", "e"), []string{"d", "a", "b", "c"})
	if slots["d"] != 3 {
		t.Fatalf("d came back as %d", slots["d"])
	}
	// Gone groups free their slots.
	assign(slots, live("f"), []string{"f"})
	if len(slots) != 1 || slots["f"] != 0 {
		t.Fatalf("after exits: %v", slots)
	}
	// With every slot held by a live group, a newcomer takes one from a
	// group without a band.
	for i := range palette {
		slots[string(rune('p'+i))] = i
	}
	all := live("p", "q", "r", "s", "t", "u", "v", "w", "z")
	assign(slots, all, []string{"z", "q"})
	if slots["z"] != 0 || slots["q"] != 1 {
		t.Fatalf("steal: %v", slots)
	}
}

func TestRuns(t *testing.T) {
	at := func(secs ...int) []entry {
		var h []entry
		for _, s := range secs {
			h = append(h, entry{t: time.Unix(int64(s), 0)})
		}
		return h
	}
	for _, c := range []struct {
		h    []entry
		want [][2]int
	}{
		{nil, nil},
		{at(0), [][2]int{{0, 1}}},
		{at(0, 5, 10, 15), [][2]int{{0, 4}}},            // 5 s apart: the slowest interval, no gap
		{at(0, 2, 4, 50, 52), [][2]int{{0, 3}, {3, 5}}}, // a hidden tab for 46 s
		{at(0, 2, 30), [][2]int{{0, 2}, {2, 3}}},
	} {
		if got := runs(c.h); !reflect.DeepEqual(got, c.want) {
			t.Errorf("runs at %v: %v, want %v", len(c.h), got, c.want)
		}
	}
}
