package view

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/quanticstudios/sector/sample"
	"github.com/quanticstudios/sector/theme"
)

// die is one die's cores as the chip map draws them.
type die struct {
	id    int
	tempC float64
	cores []sample.Core
}

// dies groups the cores by die. A CPU that reports no cores gets one core
// per thread on a single die.
func dies(c sample.CPU) []die {
	cores := c.Cores
	if len(cores) == 0 {
		for i := range c.Threads {
			cores = append(cores, sample.Core{ID: i, Threads: []int{i}})
		}
	}
	out := make([]die, max(1, len(c.Dies)))
	for i := range out {
		out[i].id = i
		if i < len(c.Dies) {
			out[i].id, out[i].tempC = c.Dies[i].ID, c.Dies[i].TempC
		}
	}
	for _, k := range cores {
		i := min(max(0, k.Die), len(out)-1)
		out[i].cores = append(out[i].cores, k)
	}
	return out
}

// coresCard is the chip map: a column per physical core, grouped by die,
// its threads stacked as tiles tinted by load.
func (v *View) coresCard(gtx layout.Context) layout.Dimensions {
	th := v.th
	cpu := v.last.CPU
	var right []span
	if cpu.MHz > 0 {
		right = append(right, span{s: strconv.FormatFloat(cpu.MHz/1000, 'f', 1, 64) + " GHz", c: cSoft})
	}
	if cpu.TempC > 0 {
		if len(right) > 0 {
			right = append(right, span{s: "·", c: th.TextQuiet})
		}
		right = append(right, v.temp(cpu.TempC)...)
	}
	if cpu.PowerW > 0 {
		if len(right) > 0 {
			right = append(right, span{s: "·", c: th.TextQuiet})
		}
		right = append(right, span{s: fmt.Sprintf("%.0f W", cpu.PowerW), c: cSoft})
	}
	ds := dies(cpu)
	smt := 0
	for _, d := range ds {
		for _, k := range d.cores {
			smt = max(smt, len(k.Threads))
		}
	}
	sub := "a column per core"
	switch {
	case smt == 2:
		sub += ", its two threads stacked"
	case smt > 2:
		sub += ", its threads stacked"
	}
	parts := []part{rigid(v.head("Cores", sub, right...))}
	for _, d := range ds {
		parts = append(parts, gap(gtx.Dp(10)).to(gtx.Dp(5)), rigid(v.dieHead(d, len(ds), smt)), gap(gtx.Dp(6)).to(gtx.Dp(3)),
			rigid(v.chipMap(d, cpu.Threads)).to(chipFloor(gtx, d)))
	}
	parts = append(parts, flexed(gtx.Dp(10), func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }).to(0))
	parts = append(parts, v.rule(gtx.Dp(10), gtx.Dp(8))...)
	parts = append(parts, rigid(v.loadFooter))
	return v.card(gtx, func(gtx layout.Context) layout.Dimensions { return column(gtx, parts...) })
}

// dieHead names a die, its core range and its temperature.
func (v *View) dieHead(d die, n, smt int) layout.Widget {
	th := v.th
	return func(gtx layout.Context) layout.Dimensions {
		name := "CCD " + strconv.Itoa(d.id)
		if n == 1 {
			name = "Package"
		}
		sub := ""
		if len(d.cores) > 0 && !narrow(gtx) {
			sub = fmt.Sprintf("cores %d to %d", d.cores[0].ID, d.cores[len(d.cores)-1].ID)
			if smt == 2 {
				sub += ", both threads"
			}
		}
		return spread(gtx, func(gtx layout.Context) layout.Dimensions {
			return v.rich(gtx, theme.Size12, span{s: name, c: cDim, w: font.SemiBold}, span{s: sub, c: th.Muted})
		}, func(gtx layout.Context) layout.Dimensions {
			if d.tempC <= 0 {
				return layout.Dimensions{}
			}
			return v.rich(gtx, theme.Size12, v.temp(d.tempC)...)
		})
	}
}

// temp is a thermometer and a temperature, green at 50 °C to red at 85 °C.
func (v *View) temp(c float64) []span {
	col := heat(v.th, (c-50)/35)
	return []span{{mark: func(gtx layout.Context) layout.Dimensions {
		d := icon(gtx, icThermo, gtx.Dp(13), col)
		d.Baseline = gtx.Dp(2)
		return d
	}}, {s: fmt.Sprintf("%.0f °C", c), c: col, w: font.SemiBold}}
}

// chipCols is how many cores a chip map row holds, and chipLines how many
// rows a die takes and how many threads its tallest core stacks.
func chipCols(d die) int { return min(8, max(1, len(d.cores))) }

func chipLines(d die) (lines, smt int) {
	for _, k := range d.cores {
		smt = max(smt, len(k.Threads))
	}
	return (len(d.cores) + chipCols(d) - 1) / chipCols(d), smt
}

// Chip map metrics in dp: the core number row, a tile at full height and
// at its floor, and the gaps between tiles and between rows of cores.
const chipNum, chipTile, chipTileMin, chipGap, chipRowGap = 15, 24, 18, 3, 6

// chipFloor is the chip map's height at its floor: no core numbers, the
// shortest tiles.
func chipFloor(gtx layout.Context, d die) int {
	lines, smt := chipLines(d)
	return lines*smt*(gtx.Dp(chipTileMin)+gtx.Dp(chipGap)) + max(0, lines-1)*gtx.Dp(chipRowGap)
}

// chipMap draws a die's cores eight columns to a row: the core number,
// then one tile per thread. Given an exact height under its natural one, it
// shortens the tiles to chipTileMin, then drops the core numbers.
func (v *View) chipMap(d die, load []float64) layout.Widget {
	th := v.th
	return func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		g := gtx.Dp(chipGap)
		numH, tileH := gtx.Dp(chipNum), gtx.Dp(chipTile)
		if h := gtx.Constraints.Min.Y; h > 0 {
			lines, smt := chipLines(d)
			per := (h - max(0, lines-1)*gtx.Dp(chipRowGap)) / max(1, lines)
			fit := func(numH int) int { return (per-numH)/max(1, smt) - g }
			if fit(numH) < gtx.Dp(chipTileMin) {
				numH = 0
			}
			tileH = max(gtx.Dp(chipTileMin), min(tileH, fit(numH)))
		}
		cols := chipCols(d)
		colX := func(i int) int { return (w + g) * i / cols }
		y := 0
		for at := 0; at < len(d.cores); at += cols {
			row := d.cores[at:min(len(d.cores), at+cols)]
			rows := 0
			for i, k := range row {
				x0, x1 := colX(i), colX(i+1)-g
				if numH > 0 {
					v.placeLabel(gtx, (x0+x1)/2, y+numH/2, 0.5, 0.5, th.TextQuiet, strconv.Itoa(k.ID))
				}
				for j, t := range k.Threads {
					l := 0.0
					if t >= 0 && t < len(load) {
						l = load[t]
					}
					ty := y + numH + g + j*(tileH+g)
					v.tile(gtx, image.Rect(x0, ty, x1, ty+tileH), l)
				}
				rows = max(rows, len(k.Threads))
			}
			y += numH + rows*(tileH+g)
			if at+cols < len(d.cores) {
				y += gtx.Dp(chipRowGap)
			}
		}
		return layout.Dimensions{Size: image.Pt(w, y)}
	}
}

// tile is one thread: the tint deepens with load, and the number carries
// the color, lifted toward white so it reads.
func (v *View) tile(gtx layout.Context, r image.Rectangle, load float64) {
	th := v.th
	pct := math.Max(0, math.Min(100, load*100))
	c := heat(th, load)
	rect(gtx, r, gtx.Dp(4), theme.Mix(th.SurfaceSecondary, c, float32(0.08+pct*0.0016)))
	ink := theme.Mix(theme.Hex("#ffffff"), c, 0.65)
	c2 := gtx
	c2.Constraints = layout.Constraints{Max: image.Pt(big, big)}
	m := op.Record(gtx.Ops)
	d := th.Label(c2, theme.Size12, font.Medium, ink, strconv.Itoa(int(math.Round(pct))))
	call := m.Stop()
	t := op.Offset(image.Pt((r.Min.X+r.Max.X-d.Size.X)/2, (r.Min.Y+r.Max.Y-d.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	t.Pop()
}

// loadFooter is the load average and the color key.
func (v *View) loadFooter(gtx layout.Context) layout.Dimensions {
	th := v.th
	label := "Load average, 1 · 5 · 15 min"
	if narrow(gtx) {
		label = "Load"
	}
	l := v.last.Host.Load
	return spread(gtx, v.text(theme.Size12, font.Normal, th.Muted, label), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Baseline, Gap: gtx.Dp(16)}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.rich(gtx, theme.Size12, span{s: "0", c: th.TextQuiet}, span{mark: v.heatKey}, span{s: "100%", c: th.TextQuiet})
			}),
			layout.Rigid(v.text(theme.Size12, font.Normal, th.Fg, fmt.Sprintf("%.1f · %.1f · %.1f", l[0], l[1], l[2]))),
		)
	})
}

// heatKey is the green to yellow to red key, 56 by 6 dp.
func (v *View) heatKey(gtx layout.Context) layout.Dimensions {
	th := v.th
	w, h := gtx.Dp(56), gtx.Dp(6)
	defer clip.UniformRRect(image.Rect(0, 0, w, h), h/2).Push(gtx.Ops).Pop()
	for i, s := range [][2]color.NRGBA{{th.Green, th.Yellow}, {th.Yellow, th.Red}} {
		x0, x1 := float32(w*i/2), float32(w*(i+1)/2)
		half := clip.Rect{Min: image.Pt(int(x0), 0), Max: image.Pt(int(x1), h)}.Push(gtx.Ops)
		paint.LinearGradientOp{Stop1: f32.Pt(x0, 0), Color1: s[0], Stop2: f32.Pt(x1, 0), Color2: s[1]}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		half.Pop()
	}
	// The bar sits 2 dp over the baseline, centered on the digits.
	return layout.Dimensions{Size: image.Pt(w, h+gtx.Dp(2))}
}
