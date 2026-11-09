package procs

import (
	"bytes"
	"encoding/gob"
	"path/filepath"
	"reflect"
	"testing"
)

// TestGobRoundTrip checks a Table crosses an embedder's gob wire unchanged.
func TestGobRoundTrip(t *testing.T) {
	ps := machine(t)
	root := filepath.Join(t.TempDir(), "proc")
	writeProc(t, root, 100, ps)
	s := NewScanner()
	if _, err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	claude := ps[211]
	claude.ticks = 150
	ps[211] = claude
	writeProc(t, root, 101, ps) // a second scan, so rates are not zero
	tb, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(tb); err != nil {
		t.Fatal(err)
	}
	var got Table
	if err := gob.NewDecoder(&buf).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, tb) {
		t.Errorf("after gob:\n%+v\nwant\n%+v", got, tb)
	}
	if g := find(got, "agent:211:1000"); g == nil || g.PID != 211 || g.Cores == 0 {
		t.Errorf("claude 211 after gob: %+v", g)
	}
}
