package view

import (
	"slices"
	"testing"
)

func TestPlaces(t *testing.T) {
	wide, fr := places(1440)
	if fr != 1 || wide[cardProcs] != (place{6, 6, 1, 2}) || wide[cardMem] != (place{0, 3, 1, 1}) || wide[cardNet] != (place{0, 6, 2, 1}) {
		t.Fatalf("wide: %v fr %d", wide, fr)
	}
	mid, fr := places(1000)
	if fr != -1 || mid[cardMem] != (place{0, 6, 2, 1}) || mid[cardDisk] != (place{6, 6, 2, 1}) || mid[cardCores].span != 12 {
		t.Fatalf("1000: %v", mid)
	}
	narrow, _ := places(720)
	for i, c := range narrow {
		if c.col != 0 || c.span != 12 || c.row != i {
			t.Fatalf("720: card %d at %v", i, c)
		}
	}
}

func TestColSpan(t *testing.T) {
	// 12 columns of 100 with gaps of 10 and padding 12.
	w := 2*12 + 12*100 + 11*10
	if x, cw := colSpan(place{0, 6, 0, 1}, w, 12, 10); x != 12 || cw != 650 {
		t.Fatalf("left half at %d, %d wide", x, cw)
	}
	if x, cw := colSpan(place{6, 6, 0, 1}, w, 12, 10); x != 672 || cw != 650 {
		t.Fatalf("right half at %d, %d wide", x, cw)
	}
	if x, cw := colSpan(place{0, 12, 0, 1}, w, 12, 10); x != 12 || cw != w-24 {
		t.Fatalf("full at %d, %d wide", x, cw)
	}
}

func TestRowHeights(t *testing.T) {
	p, fr := places(1440)
	nat := [nCards]int{cardCPU: 240, cardCores: 260, cardMem: 200, cardDisk: 300, cardNet: 120, cardProcs: 100}
	floor := [nCards]int{cardCPU: 120, cardCores: 200, cardMem: 150, cardDisk: 180, cardNet: 100, cardProcs: 40}
	if h := rowHeights(p, nat, floor, 900, 10, fr); !slices.Equal(h, []int{260, 900 - 260 - 120 - 20, 120}) {
		t.Fatalf("spare height: %v", h)
	}
	// Short of the natural 700, each row gives up the same share of its
	// slack over the floors [200 180 100].
	if h := rowHeights(p, nat, floor, 640, 10, fr); !slices.Equal(h, []int{242, 264, 114}) {
		t.Fatalf("short window: %v", h)
	}
	// Under the floors the rows stop shrinking and the view scrolls.
	if h := rowHeights(p, nat, floor, 400, 10, fr); !slices.Equal(h, []int{200, 180, 100}) {
		t.Fatalf("under the floors: %v", h)
	}
	for avail := 300; avail <= 1000; avail++ {
		h := rowHeights(p, nat, floor, avail, 10, fr)
		total := 20
		for r, v := range h {
			total += v
			if lo := []int{200, 180, 100}[r]; v < lo {
				t.Fatalf("avail %d: row %d is %d, under its floor %d", avail, r, v, lo)
			}
		}
		if want := max(avail, 500); total != want {
			t.Fatalf("avail %d: rows %v add up to %d, want %d", avail, h, total, want)
		}
	}
	// A tall process card stretches the last row it spans.
	nat[cardProcs] = 600
	if h := rowHeights(p, nat, nat, 500, 10, fr); !slices.Equal(h, []int{260, 300, 290}) {
		t.Fatalf("tall processes: %v", h)
	}
	// Narrow layouts keep the natural heights and scroll.
	p, fr = places(700)
	if h := rowHeights(p, nat, floor, 400, 10, fr); !slices.Equal(h, []int{240, 260, 200, 300, 120, 600}) {
		t.Fatalf("narrow: %v", h)
	}
}

func TestSqueeze(t *testing.T) {
	nat, floor := []int{50, 10, 30}, []int{20, 10, 0}
	for _, c := range []struct {
		room int
		want []int
	}{{100, []int{50, 10, 30}}, {75, []int{43, 10, 22}}, {40, []int{25, 10, 5}}, {10, []int{20, 10, 0}}} {
		if h := squeeze(nat, floor, c.room); !slices.Equal(h, c.want) {
			t.Fatalf("room %d: %v, want %v", c.room, h, c.want)
		}
	}
}
