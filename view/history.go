package view

import (
	"image/color"
	"time"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/theme"
)

// window is how much time the graphs show, and windowStr names it.
const window = 2 * time.Minute

const windowStr = "2 min"

// bandCount is how many groups get their own band in the CPU graph; the
// rest share "Everything else".
const bandCount = 4

// palette is the groups' colors: the concept's four validated series
// first, then four more so a group that drops out of the top keeps its
// color while a newcomer takes another.
var palette = []color.NRGBA{
	theme.Hex("#3987e5"), theme.Hex("#d95926"), theme.Hex("#199e70"), theme.Hex("#c98500"),
	theme.Hex("#8f6ad8"), theme.Hex("#d4477e"), theme.Hex("#2a9fb0"), theme.Hex("#8c8f2e"),
}

// cOut is the "Everything else" band and every group without a band.
var cOut = theme.Hex("#4a4f58")

// entry is one Push, reduced to what the graphs draw.
type entry struct {
	t            time.Time
	cores        map[string]float64 // by group key
	total        float64            // cores in use, all groups
	used, cached float64            // memory bytes
	drives       map[string][2]float64
	nets         map[string][2]float64
}

// keep appends e to h and drops the oldest past History.
func keep(h []entry, e entry) []entry {
	h = append(h, e)
	if len(h) > History {
		h = h[len(h)-History:]
	}
	return h
}

// since is the tail of h from the last entry before start on, so a graph
// clipped at start still reaches its left edge.
func since(h []entry, start time.Time) []entry {
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].t.Before(start) {
			return h[i:]
		}
	}
	return h
}

// topBands is the keys of the busiest groups, busiest first, leaving out
// the table's own "Everything else".
func topBands(t procs.Table) []string {
	var keys []string
	for _, g := range t.Groups {
		if g.Kind != procs.Other && len(keys) < bandCount {
			keys = append(keys, g.Key)
		}
	}
	return keys
}

// assign gives every banded key a palette slot. A key keeps its slot for
// as long as it is live; a newcomer takes the first slot no live key holds,
// or else one held by a key without a band.
func assign(slots map[string]int, live map[string]bool, banded []string) {
	for k := range slots {
		if !live[k] {
			delete(slots, k)
		}
	}
	isBand := map[string]bool{}
	for _, k := range banded {
		isBand[k] = true
	}
	for _, k := range banded {
		if _, ok := slots[k]; ok {
			continue
		}
		holder := map[int]string{}
		for hk, s := range slots {
			holder[s] = hk
		}
		pick := -1
		for s := range palette {
			if _, held := holder[s]; !held {
				pick = s
				break
			}
		}
		if pick < 0 {
			for s := range palette {
				if !isBand[holder[s]] {
					delete(slots, holder[s])
					pick = s
					break
				}
			}
		}
		slots[k] = pick
	}
}

// bandSeries is each band's cores per entry, then "Everything else": what
// the entry's total leaves after the bands.
func bandSeries(h []entry, bands []string) [][]float64 {
	out := make([][]float64, len(bands)+1)
	for i := range out {
		out[i] = make([]float64, len(h))
	}
	for j, e := range h {
		rest := e.total
		for i, k := range bands {
			out[i][j] = e.cores[k]
			rest -= e.cores[k]
		}
		out[len(bands)][j] = max(0, rest)
	}
	return out
}

// stack turns series into cumulative tops, the first series at the bottom.
func stack(series [][]float64) [][]float64 {
	out := make([][]float64, len(series))
	for i, s := range series {
		out[i] = make([]float64, len(s))
		for j, v := range s {
			out[i][j] = v
			if i > 0 {
				out[i][j] += out[i-1][j]
			}
		}
	}
	return out
}

// groupLabel names a group in the legend: "acme-api, claude" for an agent
// in a repo, else the group's name.
func groupLabel(g procs.Group) string {
	if g.Kind == procs.Agent && g.Repo != "" {
		return g.Repo + ", " + g.Agent
	}
	return g.Name
}
