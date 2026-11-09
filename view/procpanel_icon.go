package view

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/theme"
)

// Icon path data from the concept's sprite (lucide's shapes, 24 by 24).
const (
	ppClaudeD = "M11.2 2h1.6l.5 7.4L17.6 3l1.3.9-3.6 6.6 6.4-3.4.7 1.4-6.7 2.9 7.3 1v1.6l-7.3.5 6.4 3.6-.8 1.4-6.4-3.7 3.6 6.4-1.3.9-4.3-6.2-.4 7.5h-1.6l-.5-7.4-4.3 6.3-1.3-.9 3.6-6.4-6.4 3.7-.8-1.4 6.4-3.6L1.3 13v-1.6l7.3-1L1.9 7.5l.7-1.4L9 9.5 5.4 3l1.3-.9 4.4 7.3Z"
	ppCodexD  = "m8 9.5 3 2.5-3 2.5M12.5 15h4"
	ppTermD   = "m5 17 5-5-5-5M12 19h7"
	ppAppD    = "M2 8h20M6 4v4M10 4v4"  // lucide app-window, less its frame
	ppServerD = "M7 6.5h.01M7 17.5h.01" // the server's two lights, less its frames
	ppLayersD = "m12 2 10 5-10 5L2 7ZM2 12l10 5 10-5M2 17l10 5 10-5"
)

// ppIcon draws g's icon in an s by s box at at: the claude or codex mark
// for those agents, a terminal for other agents, a window for apps, a
// server for System and a quiet stack for Everything else.
func ppIcon(gtx layout.Context, th *theme.Theme, g procs.Group, at image.Point, s int) {
	defer op.Offset(at).Push(gtx.Ops).Pop()
	f := float32(s) / 24
	w := float32(gtx.Dp(1.5))
	rr := func(x, y, wd, ht, r float32) clip.RRect {
		return clip.UniformRRect(image.Rect(int(x*f+.5), int(y*f+.5), int((x+wd)*f+.5), int((y+ht)*f+.5)), int(r*f+.5))
	}
	stroke := func(c color.NRGBA, w float32, d string, rects ...clip.RRect) {
		var p clip.Path
		p.Begin(gtx.Ops)
		for _, r := range rects {
			ppRRectPath(&p, r)
		}
		ppSVGPath(&p, d, f)
		paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p.End(), Width: w}.Op())
	}
	switch g.Kind {
	case procs.Agent:
		switch g.Agent {
		case "claude":
			var p clip.Path
			p.Begin(gtx.Ops)
			ppSVGPath(&p, ppClaudeD, f)
			paint.FillShape(gtx.Ops, theme.Hex("#d97757"), clip.Outline{Path: p.End()}.Op())
		case "codex":
			paint.FillShape(gtx.Ops, theme.Hex("#e8e9ec"), rr(3, 3, 18, 18, 5).Op(gtx.Ops))
			stroke(th.Surface, 2*f, ppCodexD)
		default:
			stroke(th.Muted, w, ppTermD)
		}
	case procs.App:
		stroke(th.Muted, w, ppAppD, rr(2, 4, 20, 16, 2))
	case procs.System:
		stroke(th.Muted, w, ppServerD, rr(3, 3, 18, 7, 2), rr(3, 14, 18, 7, 2))
	default:
		stroke(th.TextQuiet, w, ppLayersD)
	}
}

// ppRRectPath adds r's outline to p as its own closed subpath.
func ppRRectPath(p *clip.Path, r clip.RRect) {
	x0, y0, x1, y1 := float32(r.Rect.Min.X), float32(r.Rect.Min.Y), float32(r.Rect.Max.X), float32(r.Rect.Max.Y)
	k := float32(r.NW)
	p.MoveTo(f32.Pt(x0+k, y0))
	p.LineTo(f32.Pt(x1-k, y0))
	p.QuadTo(f32.Pt(x1, y0), f32.Pt(x1, y0+k))
	p.LineTo(f32.Pt(x1, y1-k))
	p.QuadTo(f32.Pt(x1, y1), f32.Pt(x1-k, y1))
	p.LineTo(f32.Pt(x0+k, y1))
	p.QuadTo(f32.Pt(x0, y1), f32.Pt(x0, y1-k))
	p.LineTo(f32.Pt(x0, y0+k))
	p.QuadTo(f32.Pt(x0, y0), f32.Pt(x0+k, y0))
	p.Close()
}

// ppSVGPath adds SVG path data d, scaled by f, to p. It knows the line
// commands only (M L H V Z, absolute and relative), which is all the
// icons above use; it stops at anything else.
func ppSVGPath(p *clip.Path, d string, f float32) {
	pts := ppSVGPoints(d)
	for _, sp := range pts {
		for i, q := range sp.pts {
			q = q.Mul(f)
			if i == 0 {
				p.MoveTo(q)
			} else {
				p.LineTo(q)
			}
		}
		if sp.closed {
			p.Close()
		}
	}
}

// ppSub is one subpath in absolute 24 by 24 coordinates.
type ppSub struct {
	pts    []f32.Point
	closed bool
}

// ppSVGPoints parses d into subpaths; see ppSVGPath.
func ppSVGPoints(d string) []ppSub {
	var (
		out      []ppSub
		cur      f32.Point
		start    f32.Point
		cmd      byte
		i        int
		haveCmd  bool
		badInput bool
	)
	num := func() (float32, bool) {
		for i < len(d) && (d[i] == ' ' || d[i] == ',') {
			i++
		}
		j, dot := i, false
		if j < len(d) && (d[j] == '-' || d[j] == '+') {
			j++
		}
		for j < len(d) && (d[j] >= '0' && d[j] <= '9' || d[j] == '.' && !dot) {
			dot = dot || d[j] == '.'
			j++
		}
		v, err := strconv.ParseFloat(d[i:j], 32)
		if err != nil {
			return 0, false
		}
		i = j
		return float32(v), true
	}
	add := func(q f32.Point) {
		cur = q
		out[len(out)-1].pts = append(out[len(out)-1].pts, q)
	}
	for !badInput {
		for i < len(d) && (d[i] == ' ' || d[i] == ',') {
			i++
		}
		if i == len(d) {
			break
		}
		if c := d[i]; c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			cmd, haveCmd = c, true
			i++
		} else if !haveCmd {
			break
		}
		rel := cmd >= 'a'
		base := cur
		if !rel {
			base = f32.Point{}
		}
		switch cmd {
		case 'Z', 'z':
			if len(out) > 0 {
				out[len(out)-1].closed = true
			}
			cur = start
			haveCmd = false
		case 'M', 'm':
			x, ok1 := num()
			y, ok2 := num()
			if badInput = !ok1 || !ok2; badInput {
				break
			}
			out = append(out, ppSub{})
			add(base.Add(f32.Pt(x, y)))
			start = cur
			cmd -= 'M' - 'L' // later pairs are lines
		case 'L', 'l':
			x, ok1 := num()
			y, ok2 := num()
			if badInput = !ok1 || !ok2 || len(out) == 0; badInput {
				break
			}
			add(base.Add(f32.Pt(x, y)))
		case 'H', 'h':
			x, ok := num()
			if badInput = !ok || len(out) == 0; badInput {
				break
			}
			if rel {
				x += cur.X
			}
			add(f32.Pt(x, cur.Y))
		case 'V', 'v':
			y, ok := num()
			if badInput = !ok || len(out) == 0; badInput {
				break
			}
			if rel {
				y += cur.Y
			}
			add(f32.Pt(cur.X, y))
		default:
			badInput = true
		}
	}
	return out
}
