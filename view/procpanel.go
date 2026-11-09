package view

import (
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/theme"
)

// ProcPanel is the inside of the Processes card: the grouped table with
// its header, expandable groups and sort. The card frame around it is the
// View's. The zero value is ready to use.
type ProcPanel struct {
	sort     ppCol
	expanded map[string]bool              // by Group.Key, kept across tables
	clicks   map[string]*widget.Clickable // group rows, by Group.Key
	hovers   map[string]*gesture.Hover    // member rows, by Group.Key and comm
	heads    [ppNumCols]widget.Clickable
	list     widget.List
	threads  int // the machine's logical CPUs, what 100% CPU stands for
}

// ppCol is a numeric column. Its zero value, CPU, keeps the table's own
// order, which is busiest first.
type ppCol int

const (
	ppCPU ppCol = iota
	ppMem
	ppProcs
	ppThreads
	ppNumCols
)

// ppKidCap is how many member processes an expanded group lists.
const ppKidCap = 12

// ppNarrow is the panel width below which Procs and Threads are hidden.
const ppNarrow unit.Dp = 560

// ppCell is one numeric column as drawn: which, its title and its width.
type ppCell struct {
	col   ppCol
	title string
	w     unit.Dp
}

// ppCols is the numeric columns shown at a panel width, left to right.
func ppCols(width unit.Dp) []ppCell {
	if width < ppNarrow {
		return []ppCell{{ppCPU, "CPU", 96}, {ppMem, "Memory", 76}}
	}
	return []ppCell{{ppProcs, "Procs", 64}, {ppThreads, "Threads", 76}, {ppCPU, "CPU", 104}, {ppMem, "Memory", 84}}
}

// ppMatch reports whether g's name, agent, repo or branch contains filter,
// ignoring case. An empty filter matches every group.
func ppMatch(g procs.Group, filter string) bool {
	f := strings.ToLower(filter)
	for _, s := range []string{g.Name, g.Agent, g.Repo, g.Branch} {
		if strings.Contains(strings.ToLower(s), f) {
			return true
		}
	}
	return false
}

// ppSorted returns the groups ordered by col, descending, with Kind Other
// last whatever the column. CPU keeps the table's order.
func ppSorted(gs []procs.Group, col ppCol) []procs.Group {
	out := make([]procs.Group, 0, len(gs))
	var rest []procs.Group
	for _, g := range gs {
		if g.Kind == procs.Other {
			rest = append(rest, g)
		} else {
			out = append(out, g)
		}
	}
	if col != ppCPU {
		sort.SliceStable(out, func(i, j int) bool { return ppValue(out[i], col) > ppValue(out[j], col) })
	}
	return append(out, rest...)
}

func ppValue(g procs.Group, col ppCol) float64 {
	switch col {
	case ppMem:
		return float64(g.RSS)
	case ppProcs:
		return float64(len(g.Procs))
	case ppThreads:
		return float64(g.Threads)
	}
	return g.Cores
}

// ppRow is one line of the list: a group, one of its member rows, or the
// "+N more" line under a capped group.
type ppRow struct {
	g    *procs.Group
	kid  *ppKid
	more int // processes not listed
}

// ppKid is a member row: one process, or every process in the group with
// that comm, summed.
type ppKid struct {
	Comm    string
	PID     int // the process's pid; meaningful only when N is 1
	N       int // processes in the row
	Threads int
	Cores   float64
	RSS     uint64
}

// ppMerge folds ps into one row per comm, busiest first. Rows with equal
// CPU keep the order their first process had in ps.
func ppMerge(ps []procs.Proc) []ppKid {
	at := map[string]int{}
	var out []ppKid
	for _, p := range ps {
		i, ok := at[p.Comm]
		if !ok {
			i = len(out)
			at[p.Comm] = i
			out = append(out, ppKid{Comm: p.Comm, PID: p.PID})
		}
		k := &out[i]
		k.N++
		k.Threads += p.Threads
		k.Cores += p.Cores
		k.RSS += p.RSS
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Cores > out[j].Cores })
	return out
}

// ppKidNote is the quiet text after a member row's comm: the pid of a
// single process, else how many were merged, in the concept's words.
func ppKidNote(k ppKid) string {
	if k.N == 1 {
		return strconv.Itoa(k.PID)
	}
	return ppFormatInt(k.N) + " workers"
}

// ppBarScale is the percent of the machine a full CPU bar stands for: the
// busiest group's percent rounded up to 5, 10, 20, 50 or 100. The step
// keeps the scale still while load wobbles, the headroom keeps the busiest
// bar short of full (26% draws against 50), and the floor keeps an idle
// machine's bars near empty.
func ppBarScale(busiest float64) float64 {
	for _, m := range []float64{5, 10, 20, 50} {
		if !(busiest > m) { // NaN lands on the floor
			return m
		}
	}
	return 100
}

// rows flattens t into the lines to draw, after filter, sort and expansion.
func (p *ProcPanel) rows(t procs.Table, filter string) []ppRow {
	var rows []ppRow
	for _, g := range ppSorted(t.Groups, p.sort) {
		if !ppMatch(g, filter) {
			continue
		}
		g := g
		rows = append(rows, ppRow{g: &g})
		if !p.expanded[g.Key] {
			continue
		}
		kids := ppMerge(g.Procs)
		for i := range kids {
			if i == ppKidCap {
				n := 0
				for _, k := range kids[i:] {
					n += k.N
				}
				rows = append(rows, ppRow{g: &g, more: n})
				break
			}
			rows = append(rows, ppRow{g: &g, kid: &kids[i]})
		}
	}
	return rows
}

func (p *ProcPanel) toggle(key string) {
	if p.expanded == nil {
		p.expanded = map[string]bool{}
	}
	p.expanded[key] = !p.expanded[key]
}

// ppFormatPct is a percent of the machine without its sign, one decimal
// under 10: "3.2", "47".
func ppFormatPct(p float64) string {
	if p < 0.05 {
		p = 0 // no "-0.0"
	}
	if math.Round(p*10) < 100 {
		return strconv.FormatFloat(p, 'f', 1, 64)
	}
	return strconv.FormatFloat(p, 'f', 0, 64)
}

// cpuPct is cores as a percent of threads logical CPUs.
func cpuPct(cores float64, threads int) float64 { return cores * 100 / float64(max(1, threads)) }

// ppFormatMem is bytes as "1.2 GiB" from one GiB up, else "512 MiB".
func ppFormatMem(b uint64) string {
	mib := (b + 1<<19) >> 20
	if mib < 1024 {
		return strconv.FormatUint(mib, 10) + " MiB"
	}
	return strconv.FormatFloat(float64(b)/(1<<30), 'f', 1, 64) + " GiB"
}

// ppFormatInt is n with thousands separators: "4,812".
func ppFormatInt(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

// Layout draws t filling gtx.Constraints.Max, keeping only groups whose
// name, agent, repo or branch contains filter (case-insensitive) when filter is set.
// swatch is a group's color in the CPU graph, by Group.Key; the panel draws
// it beside the group's name so a row and its band read as one thing.
func (p *ProcPanel) Layout(gtx layout.Context, th *theme.Theme, t procs.Table, filter string, swatch func(key string) color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Max
	if p.clicks == nil {
		p.clicks = map[string]*widget.Clickable{}
		p.hovers = map[string]*gesture.Hover{}
		p.list.Axis = layout.Vertical
	}
	// Clicks first, so this frame already draws their result.
	for key, c := range p.clicks {
		for c.Clicked(gtx) {
			p.toggle(key)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	for i := range p.heads {
		if p.heads[i].Clicked(gtx) {
			p.sort = ppCol(i)
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	rows := p.rows(t, filter)
	p.prune(rows)
	cols := ppCols(gtx.Metric.PxToDp(size.X))
	// The bar scale comes from the whole table, so filtering doesn't rescale.
	var busiest float64
	for _, g := range t.Groups {
		busiest = max(busiest, cpuPct(g.Cores, p.threads))
	}
	d := &ppDraw{gtx: gtx, th: th, cols: cols, w: size.X, swatch: swatch, threads: p.threads, maxPct: ppBarScale(busiest)}

	headH, footH := gtx.Dp(32), gtx.Dp(30)
	p.header(d, headH)

	listH := size.Y - headH - footH
	if listH > 0 {
		st := op.Offset(image.Pt(0, headH)).Push(gtx.Ops)
		lg := gtx
		lg.Constraints = layout.Exact(image.Pt(size.X, listH))
		cl := clip.Rect{Max: lg.Constraints.Max}.Push(gtx.Ops)
		if len(rows) == 0 && filter != "" {
			ppText(lg, th, th.Font, theme.Size12, th.TextQuiet, "No groups match", gtx.Dp(12), size.X, gtx.Dp(8), gtx.Dp(30), false)
		}
		ls := material.List(&material.Theme{}, &p.list)
		ls.AnchorStrategy = material.Overlay
		ls.Track.MinorPadding = 2
		ls.Indicator = material.ScrollIndicatorStyle{
			MajorMinLen: 24, MinorWidth: 4, CornerRadius: 2,
			Color: theme.Mix(th.Surface, th.Muted, 0.22), HoverColor: theme.Mix(th.Surface, th.Muted, 0.45),
		}
		ls.Layout(lg, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
			d.gtx = gtx
			return p.row(d, rows[i])
		})
		cl.Pop()
		st.Pop()
	}

	// Footer: totals for the whole table, not just what the filter keeps.
	st := op.Offset(image.Pt(0, size.Y-footH)).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, th.BorderSubtle, clip.Rect{Max: image.Pt(size.X, gtx.Dp(1))}.Op())
	foot := ppFormatInt(t.Procs) + " processes · " + ppFormatInt(t.Threads) + " threads"
	ppText(gtx, th, th.Font, theme.Size12, th.TextQuiet, foot, gtx.Dp(12), size.X-gtx.Dp(24), 0, footH, false)
	st.Pop()
	return layout.Dimensions{Size: size}
}

// prune drops click and hover state for rows that are gone. Expanded state
// stays, so a group that leaves and comes back opens as it was.
func (p *ProcPanel) prune(rows []ppRow) {
	keys, kids := map[string]bool{}, map[string]bool{}
	for _, r := range rows {
		keys[r.g.Key] = true
		if r.kid != nil {
			kids[ppKidKey(r)] = true
		}
	}
	for k := range p.clicks {
		if !keys[k] {
			delete(p.clicks, k)
		}
	}
	for k := range p.hovers {
		if !kids[k] {
			delete(p.hovers, k)
		}
	}
}

// ppKidKey names a member row's hover state: its group and comm.
func ppKidKey(r ppRow) string { return r.g.Key + "\x00" + r.kid.Comm }

// ppDraw carries what every row needs.
type ppDraw struct {
	gtx     layout.Context
	th      *theme.Theme
	cols    []ppCell
	w       int
	swatch  func(string) color.NRGBA
	threads int
	maxPct  float64
}

// cell returns column i's left edge and width in px, right to left from
// the row's inner edge.
func (d *ppDraw) cell(i int) (x, w int) {
	gtx := d.gtx
	x = d.w - gtx.Dp(12)
	for j := len(d.cols) - 1; j >= i; j-- {
		w = gtx.Dp(d.cols[j].w)
		x -= w
		if j > i {
			x -= gtx.Dp(10)
		}
	}
	return x, w
}

// nameW is the width left for the name column.
func (d *ppDraw) nameW() int {
	x, _ := d.cell(0)
	return x - d.gtx.Dp(10) - d.gtx.Dp(12)
}

func (p *ProcPanel) header(d *ppDraw, h int) {
	gtx, th := d.gtx, d.th
	base := (h - gtx.Dp(4) + gtx.Sp(9)) / 2
	ppText(gtx, th, th.Font, theme.Size12, th.TextQuiet, "Group and processes", gtx.Dp(12), d.nameW(), base, 0, false)
	for i, c := range d.cols {
		x, w := d.cell(i)
		col, active := c.col, c.col == p.sort
		st := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		cg := gtx
		cg.Constraints = layout.Exact(image.Pt(w, h))
		p.heads[col].Layout(cg, func(gtx layout.Context) layout.Dimensions {
			fg, right := th.TextQuiet, w
			if active {
				fg = th.Fg
				right -= gtx.Dp(15)
				ppChevron(gtx, image.Pt(right+gtx.Dp(3), (h-gtx.Dp(4)-gtx.Dp(12))/2), gtx.Dp(12), true, fg)
			} else if p.heads[col].Hovered() {
				fg = th.Muted
			}
			ppText(gtx, th, th.Font, theme.Size12, fg, c.title, 0, right, base, 0, true)
			pointer.CursorPointer.Add(gtx.Ops)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		st.Pop()
	}
	paint.FillShape(gtx.Ops, th.BorderSubtle, clip.Rect{Min: image.Pt(0, h-gtx.Dp(1)), Max: image.Pt(d.w, h)}.Op())
}

func (p *ProcPanel) row(d *ppDraw, r ppRow) layout.Dimensions {
	gtx, th := d.gtx, d.th
	h := gtx.Dp(30)
	size := image.Pt(d.w, h)
	base := (h + gtx.Sp(9)) / 2
	bg := clip.UniformRRect(image.Rect(gtx.Dp(2), 0, d.w-gtx.Dp(2), h), gtx.Dp(6))
	x0 := gtx.Dp(12)
	indent := x0 + gtx.Dp(37)
	kidFg := theme.Mix(th.Muted, th.Fg, 0.53)
	mono := th.Mono

	switch {
	case r.more > 0:
		ppText(gtx, th, mono, theme.Size12, th.TextQuiet, "+"+ppFormatInt(r.more)+" more", indent, d.nameW()-indent+x0, base, 0, false)

	case r.kid != nil:
		hk := ppKidKey(r)
		hv := p.hovers[hk]
		if hv == nil {
			hv = new(gesture.Hover)
			p.hovers[hk] = hv
		}
		if hv.Update(gtx.Source) {
			paint.FillShape(gtx.Ops, th.SurfaceSecondary, bg.Op(gtx.Ops))
		}
		a := clip.Rect{Max: size}.Push(gtx.Ops)
		hv.Add(gtx.Ops)
		a.Pop()
		nw := d.nameW() - indent + x0
		used := ppText(gtx, th, mono, theme.Size12, kidFg, r.kid.Comm, indent, nw, base, 0, false)
		gap := gtx.Dp(7)
		ppText(gtx, th, mono, theme.Size12, th.TextQuiet, ppKidNote(*r.kid), indent+used+gap, nw-used-gap, base, 0, false)
		for i, c := range d.cols {
			x, w := d.cell(i)
			s := ""
			switch c.col {
			case ppProcs:
				s = ppFormatInt(r.kid.N)
			case ppThreads:
				s = ppFormatInt(r.kid.Threads)
			case ppCPU:
				s = ppFormatPct(cpuPct(r.kid.Cores, d.threads)) + "%"
			case ppMem:
				s = ppFormatMem(r.kid.RSS)
			}
			ppText(gtx, th, th.Font, theme.Size13, kidFg, s, x, w, base, 0, true)
		}

	default:
		g := r.g
		cl := p.clicks[g.Key]
		if cl == nil {
			cl = new(widget.Clickable)
			p.clicks[g.Key] = cl
		}
		if cl.Hovered() {
			paint.FillShape(gtx.Ops, th.SurfaceSecondary, bg.Op(gtx.Ops))
		}
		x := x0
		ppChevron(gtx, image.Pt(x, (h-gtx.Dp(14))/2), gtx.Dp(14), p.expanded[g.Key], th.TextQuiet)
		x += gtx.Dp(14 + 7)
		var sw color.NRGBA
		if d.swatch != nil {
			sw = d.swatch(g.Key)
		}
		if sw.A != 0 {
			s := gtx.Dp(8)
			paint.FillShape(gtx.Ops, sw, clip.UniformRRect(image.Rect(x, (h-s)/2, x+s, (h+s)/2), gtx.Dp(2)).Op(gtx.Ops))
		}
		x += gtx.Dp(8 + 7)
		ppIcon(gtx, th, *g, image.Pt(x, (h-gtx.Dp(14))/2), gtx.Dp(14))
		x += gtx.Dp(14 + 7)
		name, branch, kind := ppLabels(*g)
		nameFg := th.Fg
		if g.Kind == procs.Other {
			nameFg = th.Muted
		}
		end := x0 + d.nameW()
		bold := th.Font
		bold.Weight = font.SemiBold
		x += ppText(gtx, th, bold, theme.Size13, nameFg, name, x, end-x, base, 0, false)
		if branch != "" {
			x += gtx.Dp(7)
			x += ppText(gtx, th, mono, theme.Size12, th.TextQuiet, branch, x, end-x, base, 0, false)
		}
		if kind != "" {
			x += gtx.Dp(7)
			ppText(gtx, th, th.Font, theme.Size12, th.TextQuiet, kind, x, end-x, base, 0, false)
		}
		quiet := theme.Mix(th.Muted, th.Fg, 0.42)
		for i, c := range d.cols {
			x, w := d.cell(i)
			switch c.col {
			case ppProcs:
				ppText(gtx, th, th.Font, theme.Size13, quiet, ppFormatInt(len(g.Procs)), x, w, base, 0, true)
			case ppThreads:
				ppText(gtx, th, th.Font, theme.Size13, quiet, ppFormatInt(g.Threads), x, w, base, 0, true)
			case ppMem:
				ppText(gtx, th, bold, theme.Size13, th.Fg, ppFormatMem(g.RSS), x, w, base, 0, true)
			case ppCPU:
				gp := cpuPct(g.Cores, d.threads)
				tw := ppText(gtx, th, bold, theme.Size13, th.Fg, ppFormatPct(gp)+"%", x, w, base, 0, true)
				bw, bh := gtx.Dp(40), gtx.Dp(4)
				bx, by := x+w-tw-gtx.Dp(8)-bw, (h-bh)/2
				if bx >= x {
					paint.FillShape(gtx.Ops, th.SurfaceElevated, clip.UniformRRect(image.Rect(bx, by, bx+bw, by+bh), bh/2).Op(gtx.Ops))
					fill := sw
					if fill.A == 0 {
						fill = theme.Mix(th.Surface, th.Muted, 0.4)
					}
					if fw := min(bw, int(float64(bw)*gp/d.maxPct+0.5)); fw > 0 {
						paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rect(bx, by, bx+fw, by+bh), min(bh, fw)/2).Op(gtx.Ops))
					}
				}
			}
		}
		cg := gtx
		cg.Constraints = layout.Exact(size)
		cl.Layout(cg, func(gtx layout.Context) layout.Dimensions {
			pointer.CursorPointer.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		})
	}
	return layout.Dimensions{Size: size}
}

// ppLabels is a group row's bold name, its branch (drawn in mono) and a
// quiet kind word after them: "acme-api fix/login claude", "Chromium app".
func ppLabels(g procs.Group) (name, branch, kind string) {
	switch g.Kind {
	case procs.Agent:
		if g.Repo == "" {
			return g.Name, "", ""
		}
		return g.Repo, g.Branch, g.Agent
	case procs.App:
		return g.Name, "", "app"
	case procs.System:
		return g.Name, "", "system"
	case procs.Other:
		switch g.Folded {
		case 0:
		case 1:
			return g.Name, "", "1 group"
		default:
			return g.Name, "", ppFormatInt(g.Folded) + " groups"
		}
	}
	return g.Name, "", ""
}

// ppText draws s on one line, truncated to w, with its baseline at y
// (or centered in h when h is set), right-aligned in w when end is set.
// It returns the width drawn.
func ppText(gtx layout.Context, th *theme.Theme, f font.Font, size unit.Sp, c color.NRGBA, s string, x, w, y, h int, end bool) int {
	if w <= 0 || s == "" {
		return 0
	}
	mat := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paintC := mat.Stop()
	m := op.Record(gtx.Ops)
	gtx.Constraints = layout.Constraints{Max: image.Pt(w, 1<<20)}
	dims := widget.Label{MaxLines: 1, Truncator: "…"}.Layout(gtx, th.Shaper, f, size, s, paintC)
	call := m.Stop()
	if end {
		x += w - dims.Size.X
	}
	if h > 0 {
		y = (h - dims.Size.Y) / 2
	} else {
		y -= dims.Size.Y - dims.Baseline // Baseline counts from the bottom
	}
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
	return dims.Size.X
}

// ppChevron draws a chevron in an s by s box at at, pointing down when
// down is set, else right.
func ppChevron(gtx layout.Context, at image.Point, s int, down bool, c color.NRGBA) {
	f := float32(s) / 14
	pt := func(x, y float32) f32.Point { return f32.Pt(float32(at.X)+x*f, float32(at.Y)+y*f) }
	var path clip.Path
	path.Begin(gtx.Ops)
	if down {
		path.MoveTo(pt(3.5, 5.25))
		path.LineTo(pt(7, 8.75))
		path.LineTo(pt(10.5, 5.25))
	} else {
		path.MoveTo(pt(5.25, 3.5))
		path.LineTo(pt(8.75, 7))
		path.LineTo(pt(5.25, 10.5))
	}
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: path.End(), Width: 1.5 * f}.Op())
}
