package view

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Icons from the concept, lucide style, as SVG path data in a 24x24 box.
const (
	icSearch  = "M4 11a7 7 0 1 0 14 0a7 7 0 1 0-14 0M20 20l-3.5-3.5"
	icPause   = "M7 5h2a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM15 5h2a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-2a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Z"
	icPlay    = "M7 4.5v15a.8.8 0 0 0 1.2.7l12-7.5a.8.8 0 0 0 0-1.4l-12-7.5A.8.8 0 0 0 7 4.5Z"
	icDesktop = "M4 3h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2ZM8 21h8M12 17v4"
	icThermo  = "M14 4v10.5a4 4 0 1 1-4 0V4a2 2 0 0 1 4 0Z"
	// The sector mark's three arcs, from assets/logo.svg.
	icArcBlue   = "M5.6 16.6A8 8 0 0 1 7.4 5.4"
	icArcYellow = "M10.7 4.1a8 8 0 0 1 9 6.3"
	icArcGreen  = "M19.4 14.1a8 8 0 0 1-10.6 5.5"
)

// Adapted from pitwall: internal/ui/sidebar/icons.go (parsePath, arcCenter, iconPath).

// seg is one absolute path command: 'M', 'L', 'C' (ctrl0, ctrl1, end),
// 'A' (center, end; angle is the sweep) or 'Z'.
type seg struct {
	op    byte
	pts   [3]f32.Point
	angle float32
}

var iconCache = map[string][]seg{}

// parsePath turns SVG path data into absolute segments: M L H V C A Z and
// their relative forms, circular arcs only.
func parsePath(d string) []seg {
	var out []seg
	i := 0
	skip := func() {
		for i < len(d) && (d[i] == ' ' || d[i] == ',') {
			i++
		}
	}
	num := func() float32 {
		skip()
		j := i
		if j < len(d) && (d[j] == '-' || d[j] == '+') {
			j++
		}
		dot := false
		for j < len(d) && (d[j] >= '0' && d[j] <= '9' || d[j] == '.' && !dot) {
			if d[j] == '.' {
				dot = true
			}
			j++
		}
		v, _ := strconv.ParseFloat(d[i:j], 32)
		i = j
		return float32(v)
	}
	flag := func() bool {
		skip()
		i++
		return d[i-1] == '1'
	}
	var pen, start f32.Point
	var cmd byte
	for skip(); i < len(d); skip() {
		if c := d[i]; c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			cmd = c
			i++
			if cmd == 'Z' || cmd == 'z' {
				out = append(out, seg{op: 'Z'})
				pen = start
				continue
			}
		}
		rel := cmd >= 'a'
		pt := func() f32.Point {
			p := f32.Pt(num(), num())
			if rel {
				p = p.Add(pen)
			}
			return p
		}
		switch cmd | 0x20 {
		case 'm':
			pen = pt()
			start = pen
			out = append(out, seg{op: 'M', pts: [3]f32.Point{pen}})
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'l':
			pen = pt()
			out = append(out, seg{op: 'L', pts: [3]f32.Point{pen}})
		case 'h':
			x := num()
			if rel {
				x += pen.X
			}
			pen.X = x
			out = append(out, seg{op: 'L', pts: [3]f32.Point{pen}})
		case 'v':
			y := num()
			if rel {
				y += pen.Y
			}
			pen.Y = y
			out = append(out, seg{op: 'L', pts: [3]f32.Point{pen}})
		case 'c':
			c0, c1, to := pt(), pt(), pt()
			pen = to
			out = append(out, seg{op: 'C', pts: [3]f32.Point{c0, c1, to}})
		case 'a':
			r := num()
			num() // ry: circular arcs only
			num() // x-axis rotation
			large, sweep := flag(), flag()
			to := pt()
			c, a := arcCenter(pen, to, r, large, sweep)
			pen = to
			out = append(out, seg{op: 'A', pts: [3]f32.Point{c, to}, angle: a})
		default:
			panic("view: unsupported path command " + string(cmd))
		}
	}
	return out
}

// arcCenter converts an SVG circular arc from p0 to p1 into its center and
// signed sweep angle (SVG spec F.6.5 with rx == ry and no rotation).
func arcCenter(p0, p1 f32.Point, r float32, large, sweep bool) (f32.Point, float32) {
	hx, hy := float64(p0.X-p1.X)/2, float64(p0.Y-p1.Y)/2
	rr := float64(r)
	if l := (hx*hx + hy*hy) / (rr * rr); l > 1 {
		rr *= math.Sqrt(l)
	}
	k := math.Sqrt(math.Max(0, (rr*rr-hx*hx-hy*hy)/(hx*hx+hy*hy)))
	if large == sweep {
		k = -k
	}
	cx := k*hy + float64(p0.X+p1.X)/2
	cy := -k*hx + float64(p0.Y+p1.Y)/2
	a0 := math.Atan2(float64(p0.Y)-cy, float64(p0.X)-cx)
	a1 := math.Atan2(float64(p1.Y)-cy, float64(p1.X)-cx)
	da := a1 - a0
	if sweep && da < 0 {
		da += 2 * math.Pi
	} else if !sweep && da > 0 {
		da -= 2 * math.Pi
	}
	return f32.Pt(float32(cx), float32(cy)), float32(da)
}

// iconPath is path d scaled from its 24x24 box to size px.
func iconPath(gtx layout.Context, d string, size int) clip.PathSpec {
	segs, ok := iconCache[d]
	if !ok {
		segs = parsePath(d)
		iconCache[d] = segs
	}
	s := float32(size) / 24
	var p clip.Path
	p.Begin(gtx.Ops)
	for _, sg := range segs {
		a, b, c := sg.pts[0].Mul(s), sg.pts[1].Mul(s), sg.pts[2].Mul(s)
		switch sg.op {
		case 'M':
			p.MoveTo(a)
		case 'L':
			p.LineTo(a)
		case 'C':
			p.CubeTo(a, b, c)
		case 'A':
			p.ArcTo(a, a, sg.angle)
		case 'Z':
			p.Close()
		}
	}
	return p.End()
}

// icon strokes d in a size px box, at the concept's 1.75 stroke.
func icon(gtx layout.Context, d string, size int, c color.NRGBA) layout.Dimensions {
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: iconPath(gtx, d, size), Width: 1.75 * float32(size) / 24}.Op())
	return layout.Dimensions{Size: image.Pt(size, size)}
}

// logo draws the sector mark: three arcs around a light dot.
func logo(gtx layout.Context, size int, blue, yellow, green, dot color.NRGBA) layout.Dimensions {
	w := 3.2 * float32(size) / 24
	for _, a := range []struct {
		d string
		c color.NRGBA
	}{{icArcBlue, blue}, {icArcYellow, yellow}, {icArcGreen, green}} {
		paint.FillShape(gtx.Ops, a.c, clip.Stroke{Path: iconPath(gtx, a.d, size), Width: w}.Op())
	}
	r := 2.2 * float32(size) / 24
	c := float32(size) / 2
	paint.FillShape(gtx.Ops, dot, clip.Ellipse{Min: image.Pt(int(c-r+.5), int(c-r+.5)), Max: image.Pt(int(c+r+.5), int(c+r+.5))}.Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(size, size)}
}
