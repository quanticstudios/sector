package view

import (
	"testing"

	"github.com/quanticstudios/sector/sample"
)

// An Intel hybrid reports core ids 0, 4, 8 for its performance cores and
// 32 up for its efficiency cores; the map numbers them 0 up.
func TestDiesNumbersCoresInOrder(t *testing.T) {
	cpu := sample.CPU{Threads: make([]float64, 5), Cores: []sample.Core{
		{ID: 0, Threads: []int{0, 1}}, {ID: 4, Threads: []int{2, 3}}, {ID: 32, Threads: []int{4}},
	}}
	d := dies(cpu)
	if len(d) != 1 || len(d[0].cores) != 3 {
		t.Fatalf("dies = %+v", d)
	}
	for i, k := range d[0].cores {
		if k.ID != i {
			t.Errorf("core %d numbered %d", i, k.ID)
		}
	}
	if cpu.Cores[1].ID != 4 {
		t.Errorf("dies changed the sample's core ids")
	}
	if n := twoThreaded(d[0]); n != 2 {
		t.Errorf("twoThreaded = %d, want 2", n)
	}
}
