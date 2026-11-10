package main

import (
	"testing"
	"time"

	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/view"
)

func TestBacklogKeepsTheNewest(t *testing.T) {
	var b backlog
	start := time.Unix(0, 0)
	for i := range view.History + 5 {
		b.add(reading{s: sample.Snapshot{Time: start.Add(time.Duration(i) * time.Second)}})
	}
	rs := b.take()
	if len(rs) != view.History || !rs[0].s.Time.Equal(start.Add(5*time.Second)) {
		t.Fatalf("kept %d from %v, want %d from 5 s", len(rs), rs[0].s.Time, view.History)
	}
	if len(b.take()) != 0 {
		t.Fatal("take left readings behind")
	}
}
