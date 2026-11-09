package procs

import (
	"cmp"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// procRoot is where procfs is mounted; tests point it at a fake tree.
var procRoot = "/proc"

// clkTck is USER_HZ, the unit of stat's CPU times: 100 on every Linux Go
// runs on.
const clkTck = 100

// Folding: an App or System group under both limits joins "Everything else".
const (
	foldCores = 0.01
	foldRSS   = 256 << 20
)

// apps names an app by its root process's comm, lower-cased.
var apps = map[string]string{
	"chromium": "Chromium",
	"chrome":   "Chrome",
	"brave":    "Brave",
	"firefox":  "Firefox",
	"spotify":  "Spotify",
	"slack":    "Slack",
	"code":     "VS Code",
	"code-oss": "VS Code",
	"discord":  "Discord",
	"signal":   "Signal",
	"obsidian": "Obsidian",
	"zoom":     "Zoom",
}

type procKey struct {
	pid   int
	start uint64 // starttime in ticks since boot: a reused pid gets a new one
}

// seen is what a scan keeps about a process for the next one.
type seen struct {
	ticks uint64 // utime + stime
	comm  string // an exec changes it, and with it the agent
	agent string
	uid   int
}

type scanner struct {
	seen   map[procKey]seen
	uptime float64           // seconds since boot at the last scan
	repos  map[string]gitDir // by cwd
}

// gitDir is where an agent's cwd lies: the repo's name, and the HEAD file
// of a linked worktree, "" for a plain checkout.
type gitDir struct{ repo, head string }

// proc is a Proc and what grouping needs about it.
type proc struct {
	Proc
	start uint64
	agent string
	uid   int
}

func (s *Scanner) scan() (Table, error) {
	up, err := readUptime()
	if err != nil {
		return Table{}, err
	}
	ents, err := os.ReadDir(procRoot)
	if err != nil {
		return Table{}, err
	}
	first, elapsed := s.seen == nil, up-s.uptime
	next := make(map[procKey]seen, len(s.seen))
	ps := make([]*proc, 0, len(ents))
	page := uint64(os.Getpagesize())
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		p, ticks, ok := readStat(pid)
		if !ok {
			continue // gone mid-read
		}
		p.RSS *= page
		k := procKey{pid, p.start}
		old, had := s.seen[k]
		if !had || old.comm != p.Comm {
			old.comm = p.Comm
			if old.uid, ok = readUID(pid); !ok {
				continue
			}
			old.agent = identifyProc(pid, p.Comm)
		}
		p.agent, p.uid = old.agent, old.uid
		if !first && elapsed > 0 {
			if had && ticks >= old.ticks {
				p.Cores = float64(ticks-old.ticks) / clkTck / elapsed
			} else if !had {
				// New since the last scan: all its time fell in the window,
				// unless it hid from that scan, when its lifetime is the window.
				p.Cores = float64(ticks) / clkTck / max(elapsed, up-float64(p.start)/clkTck)
			}
		}
		old.ticks = ticks
		next[k] = old
		ps = append(ps, p)
	}
	s.seen, s.uptime = next, up
	return s.group(ps), nil
}

// group sorts processes into agent, app and System groups and folds the
// small ones into "Everything else".
func (s *Scanner) group(ps []*proc) Table {
	byPID := make(map[int]*proc, len(ps))
	t := Table{Procs: len(ps)}
	for _, p := range ps {
		byPID[p.PID] = p
		t.Threads += p.Threads
	}
	self := os.Getuid()
	system := func(p *proc) bool { return p.PID == 2 || p.PPID == 2 || p.uid != self }

	// agentOf is the nearest agent at or above p, nil when there is none.
	agentMemo := map[int]*proc{}
	var agentOf func(p *proc, depth int) *proc
	agentOf = func(p *proc, depth int) *proc {
		if p.agent != "" {
			return p
		}
		if a, ok := agentMemo[p.PID]; ok {
			return a
		}
		var a *proc
		if parent := byPID[p.PPID]; parent != nil && parent != p && depth < 256 {
			a = agentOf(parent, depth+1)
		}
		agentMemo[p.PID] = a
		return a
	}
	// rootOf is p's app root: the topmost ancestor whose parent is pid 1, a
	// systemd (the user manager) or a System process.
	rootMemo := map[int]*proc{}
	var rootOf func(p *proc, depth int) *proc
	rootOf = func(p *proc, depth int) *proc {
		if r, ok := rootMemo[p.PID]; ok {
			return r
		}
		r := p
		if parent := byPID[p.PPID]; parent != nil && parent != p && parent.PID != 1 &&
			parent.Comm != "systemd" && !system(parent) && depth < 256 {
			r = rootOf(parent, depth+1)
		}
		rootMemo[p.PID] = r
		return r
	}

	groups := map[string]*Group{}
	repos := make(map[string]gitDir)
	add := func(key string, g Group, p *proc) {
		gp := groups[key]
		if gp == nil {
			g.Key = key
			gp = &g
			groups[key] = gp
		}
		gp.Procs = append(gp.Procs, p.Proc)
		gp.Cores += p.Cores
		gp.RSS += p.RSS
		gp.Threads += p.Threads
	}
	for _, p := range ps {
		switch a := agentOf(p, 0); {
		case a != nil:
			key := "agent:" + strconv.Itoa(a.PID) + ":" + strconv.FormatUint(a.start, 10)
			g := Group{Kind: Agent, Name: a.agent, Agent: a.agent}
			if groups[key] == nil {
				g.Repo, g.Branch = s.repo(a.PID, repos)
			}
			add(key, g, p)
		case system(p):
			add("system", Group{Kind: System, Name: "System"}, p)
		default:
			name := rootOf(p, 0).Comm
			if n := apps[strings.ToLower(name)]; n != "" {
				name = n
			}
			add("app:"+name, Group{Kind: App, Name: name}, p)
		}
	}
	s.repos = repos

	other := Group{Key: "other", Kind: Other, Name: "Everything else"}
	for _, g := range groups {
		if g.Kind != Agent && g.Cores < foldCores && g.RSS < foldRSS {
			other.Procs = append(other.Procs, g.Procs...)
			other.Cores += g.Cores
			other.RSS += g.RSS
			other.Threads += g.Threads
			continue
		}
		t.Groups = append(t.Groups, *g)
	}
	slices.SortFunc(t.Groups, func(a, b Group) int {
		return cmp.Or(cmp.Compare(b.Cores, a.Cores), cmp.Compare(b.RSS, a.RSS), strings.Compare(a.Key, b.Key))
	})
	if len(other.Procs) > 0 {
		t.Groups = append(t.Groups, other)
	}
	for _, g := range t.Groups {
		slices.SortFunc(g.Procs, func(a, b Proc) int {
			return cmp.Or(cmp.Compare(b.Cores, a.Cores), cmp.Compare(b.RSS, a.RSS), cmp.Compare(a.PID, b.PID))
		})
	}
	return t
}

// repo names the repo agent pid runs in and, in a linked worktree, its
// branch. The repo is cached by cwd; the branch is read fresh each scan.
// An unreadable cwd (another user's process) leaves both empty.
func (s *Scanner) repo(pid int, keep map[string]gitDir) (repo, branch string) {
	cwd, err := os.Readlink(procRoot + "/" + strconv.Itoa(pid) + "/cwd")
	if err != nil {
		return "", ""
	}
	cwd = strings.TrimSuffix(cwd, " (deleted)")
	d, ok := s.repos[cwd]
	if !ok {
		d = findGitDir(cwd)
	}
	keep[cwd] = d
	if d.head != "" {
		b, _ := os.ReadFile(d.head)
		if br, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "ref: refs/heads/"); ok {
			branch = br // a detached HEAD leaves it ""
		}
	}
	return d.repo, branch
}

// findGitDir walks up from dir to the folder holding .git. A .git file is
// a linked worktree when its gitdir lies in <main>/.git/worktrees, and the
// repo is then named after <main>.
func findGitDir(dir string) gitDir {
	for {
		git := filepath.Join(dir, ".git")
		if fi, err := os.Stat(git); err == nil {
			if fi.IsDir() {
				return gitDir{repo: filepath.Base(dir)}
			}
			b, _ := os.ReadFile(git)
			gd := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
			if !filepath.IsAbs(gd) {
				gd = filepath.Join(dir, gd)
			}
			if filepath.Base(filepath.Dir(gd)) == "worktrees" {
				main := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(gd))))
				return gitDir{repo: strings.TrimSuffix(main, ".git"), head: filepath.Join(gd, "HEAD")}
			}
			return gitDir{repo: filepath.Base(dir)} // a submodule
		}
		up := filepath.Dir(dir)
		if up == dir {
			return gitDir{}
		}
		dir = up
	}
}

// readStat parses /proc/<pid>/stat into a proc, RSS still in pages, and
// returns its CPU time in ticks. comm may hold spaces and parentheses, so
// fields are counted from after the last ')'.
func readStat(pid int) (*proc, uint64, bool) {
	b, err := os.ReadFile(procRoot + "/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil, 0, false
	}
	s := string(b)
	open, end := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return nil, 0, false
	}
	f := strings.Fields(s[end+1:])
	if len(f) < 22 || f[0] == "" {
		return nil, 0, false
	}
	// f[i] is field i+3 in proc(5)'s numbering.
	num := func(i int) uint64 { v, _ := strconv.ParseUint(f[i], 10, 64); return v }
	p := &proc{
		Proc: Proc{
			PID:     pid,
			PPID:    int(num(1)),
			Comm:    s[open+1 : end],
			State:   f[0][0],
			RSS:     num(21),
			Threads: int(num(17)),
		},
		start: num(19),
	}
	return p, num(11) + num(12), true
}

// readUID is the real uid on the Uid line of /proc/<pid>/status.
func readUID(pid int) (int, bool) {
	b, err := os.ReadFile(procRoot + "/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, false
	}
	_, rest, ok := strings.Cut(string(b), "\nUid:")
	if !ok {
		return 0, false
	}
	f := strings.Fields(rest)
	if len(f) == 0 {
		return 0, false
	}
	uid, err := strconv.Atoi(f[0])
	return uid, err == nil
}

func readUptime() (float64, error) {
	b, err := os.ReadFile(procRoot + "/uptime")
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, os.ErrInvalid
	}
	return strconv.ParseFloat(f[0], 64)
}

// Adapted from pitwall: internal/daemon/proc_linux.go

// identifyProc names the agent process pid runs, by its comm and exe, or,
// for an interpreter, by the script its argv starts with.
func identifyProc(pid int, comm string) string {
	exe, _ := os.Readlink(procRoot + "/" + strconv.Itoa(pid) + "/exe")
	exe = strings.TrimSuffix(exe, " (deleted)")
	if p := identify(comm, exe); p != "" || !interpreter(comm, exe) {
		return p
	}
	return script(readArgs(pid))
}

// argsRead bounds the bytes of an argv readArgs reads: a script's path
// comes first, and the rest is never looked at.
const argsRead = 4096

// readArgs is the start of pid's argv, at most argsRead bytes of it. The
// only caller passes it to script and drops it: arguments carry secrets,
// so it is never kept, logged or returned further.
func readArgs(pid int) []string {
	f, err := os.Open(procRoot + "/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, argsRead))
	args := strings.Split(string(b), "\x00")
	if len(args) > 0 && args[len(args)-1] == "" {
		args = args[:len(args)-1]
	}
	return args
}
