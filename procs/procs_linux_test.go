package procs

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fake is one fictional process in a fake /proc.
type fake struct {
	ppid         int
	comm         string
	other        bool // owned by another user
	ticks, start uint64
	rss          uint64 // pages
	threads      int
	exe, cwd     string
	argv         []string
}

// writeProc writes a fake /proc holding ps at uptime seconds, and points
// procRoot at it.
func writeProc(t *testing.T, root string, up float64, ps map[int]fake) {
	t.Helper()
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	w := func(name, s string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	w("uptime", fmt.Sprintf("%.2f 0.00\n", up))
	for pid, p := range ps {
		d := fmt.Sprint(pid)
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
		w(d+"/stat", fmt.Sprintf("%d (%s) S %d 0 0 0 0 0 0 0 0 0 %d 0 0 0 20 0 %d 0 %d 0 %d 0 0\n",
			pid, p.comm, p.ppid, p.ticks, max(p.threads, 1), p.start, p.rss))
		uid := os.Getuid()
		if p.other {
			uid++
		}
		w(d+"/status", fmt.Sprintf("Name:\t%s\nUmask:\t0022\nState:\tS\nUid:\t%d\t%d\t%d\t%d\n", p.comm, uid, uid, uid, uid))
		w(d+"/cmdline", strings.Join(p.argv, "\x00")+"\x00")
		if p.exe != "" {
			link(p.exe, d+"/exe")
		}
		if p.cwd != "" {
			link(p.cwd, d+"/cwd")
		}
	}
	old := procRoot
	t.Cleanup(func() { procRoot = old })
	procRoot = root
}

// repos makes acme-api, a plain checkout, and web-app with a linked
// worktree on fix/login, and returns a dir in each.
func repos(t *testing.T) (acme, webWT string) {
	t.Helper()
	base := t.TempDir()
	mk := func(p string) {
		if err := os.MkdirAll(filepath.Join(base, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w := func(p, s string) {
		if err := os.WriteFile(filepath.Join(base, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("acme-api/.git")
	mk("acme-api/src/lib")
	mk("web-app/.git/worktrees/login")
	w("web-app/.git/HEAD", "ref: refs/heads/main\n")
	w("web-app/.git/worktrees/login/HEAD", "ref: refs/heads/fix/login\n")
	mk("web-app/.worktrees/login/pkg")
	w("web-app/.worktrees/login/.git", "gitdir: "+filepath.Join(base, "web-app/.git/worktrees/login")+"\n")
	return filepath.Join(base, "acme-api/src/lib"), filepath.Join(base, "web-app/.worktrees/login/pkg")
}

const mib = 1 << 20

// machine is the fictional machine: a terminal running two claude sessions
// (one with a codex under it), Chromium, kernel threads, root services and
// a tiny idle app.
func machine(t *testing.T) map[int]fake {
	acme, web := repos(t)
	page := uint64(os.Getpagesize())
	return map[int]fake{
		1:   {comm: "systemd", other: true, rss: 300 * mib / page},
		2:   {comm: "kthreadd"}, // kernel threads: System by pid, not owner
		50:  {ppid: 2, comm: "kworker/0:1"},
		90:  {ppid: 1, comm: "sshd", other: true},
		100: {ppid: 1, comm: "systemd"},
		200: {ppid: 100, comm: "ghostty", rss: 10 * mib / page},
		210: {ppid: 200, comm: "zsh"},
		211: {ppid: 210, comm: "claude", exe: "/opt/claude/2.1.0", cwd: acme, start: 1000, rss: 400 * mib / page, threads: 12},
		212: {ppid: 211, comm: "node", exe: "/usr/bin/node", argv: []string{"node", "server.js"}},
		213: {ppid: 211, comm: "vitest", start: 5000},
		214: {ppid: 211, comm: "codex", exe: "/usr/bin/codex"},
		215: {ppid: 214, comm: "node", exe: "/usr/bin/node", argv: []string{"node", "tool.js"}},
		220: {ppid: 200, comm: "zsh"},
		221: {ppid: 220, comm: "node", exe: "/usr/bin/node", cwd: web, start: 2000,
			argv: []string{"node", "--no-warnings", "/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js"}},
		222: {ppid: 221, comm: "node", exe: "/usr/bin/node", argv: []string{"node", "worker.js"}},
		223: {ppid: 222, comm: "vitest"},
		300: {ppid: 100, comm: "chromium", rss: 200 * mib / page},
		301: {ppid: 300, comm: "chromium", rss: 300 * mib / page},
		400: {ppid: 100, comm: "tinyapp", rss: 1 * mib / page},
	}
}

func find(tb Table, key string) *Group {
	for i := range tb.Groups {
		if tb.Groups[i].Key == key {
			return &tb.Groups[i]
		}
	}
	return nil
}

func pids(g *Group) map[int]bool {
	m := map[int]bool{}
	for _, p := range g.Procs {
		m[p.PID] = true
	}
	return m
}

func TestScanGroups(t *testing.T) {
	ps := machine(t)
	writeProc(t, filepath.Join(t.TempDir(), "proc"), 100, ps)
	tb, err := NewScanner().Scan()
	if err != nil {
		t.Fatal(err)
	}
	if tb.Procs != len(ps) || tb.Threads != len(ps)+11 {
		t.Errorf("Procs, Threads = %d, %d, want %d, %d", tb.Procs, tb.Threads, len(ps), len(ps)+11)
	}
	for _, g := range tb.Groups {
		if g.Cores != 0 {
			t.Errorf("first scan: %s has %v cores", g.Key, g.Cores)
		}
	}
	want := []struct {
		key, name, repo, branch string
		kind                    Kind
		pids                    []int
	}{
		{"agent:211:1000", "claude", "acme-api", "", Agent, []int{211, 212, 213}},
		{"agent:214:0", "codex", "", "", Agent, []int{214, 215}},
		{"agent:221:2000", "claude", "web-app", "fix/login", Agent, []int{221, 222, 223}},
		{"app:Chromium", "Chromium", "", "", App, []int{300, 301}},
		{"system", "System", "", "", System, []int{1, 2, 50, 90}},
		{"other", "Everything else", "", "", Other, []int{100, 200, 210, 220, 400}},
	}
	if len(tb.Groups) != len(want) {
		t.Errorf("got %d groups, want %d", len(tb.Groups), len(want))
	}
	for _, w := range want {
		g := find(tb, w.key)
		if g == nil {
			t.Errorf("no group %s", w.key)
			continue
		}
		if g.Name != w.name || g.Repo != w.repo || g.Branch != w.branch || g.Kind != w.kind {
			t.Errorf("%s = %q %q %q %d, want %q %q %q %d", w.key, g.Name, g.Repo, g.Branch, g.Kind, w.name, w.repo, w.branch, w.kind)
		}
		if w.kind == Agent && g.Agent != w.name {
			t.Errorf("%s: Agent = %q", w.key, g.Agent)
		}
		got := pids(g)
		if len(got) != len(w.pids) {
			t.Errorf("%s: members %v, want %v", w.key, got, w.pids)
		}
		for _, p := range w.pids {
			if !got[p] {
				t.Errorf("%s: missing pid %d", w.key, p)
			}
		}
	}
	if last := tb.Groups[len(tb.Groups)-1]; last.Key != "other" {
		t.Errorf("last group is %s, want other", last.Key)
	}
	// By RSS on a scan with no CPU: Chromium (500 MiB) leads.
	if tb.Groups[0].Key != "app:Chromium" {
		t.Errorf("first group is %s", tb.Groups[0].Key)
	}
	if g := find(tb, "agent:211:1000"); g.RSS != 400*mib || g.Threads != 14 {
		t.Errorf("claude 211: RSS %d, Threads %d", g.RSS, g.Threads)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestScanRates(t *testing.T) {
	ps := machine(t)
	root := filepath.Join(t.TempDir(), "proc")
	writeProc(t, root, 100, ps)
	s := NewScanner()
	if _, err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	bump := func(pid int, ticks uint64) { p := ps[pid]; p.ticks += ticks; ps[pid] = p }
	bump(223, 300) // vitest: 1.5 threads over 2 s
	bump(301, 100) // chromium: 0.5
	bump(200, 4)   // ghostty: 0.02, over the fold line
	bump(400, 1)   // tinyapp: 0.005, under it
	// A vitest that started half a second ago and used 0.4 s.
	ps[224] = fake{ppid: 211, comm: "vitest", start: 10150, ticks: 40}
	delete(ps, 212) // gone between scans
	writeProc(t, root, 102, ps)
	tb, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	g := find(tb, "agent:221:2000")
	if g == nil || !near(g.Cores, 1.5) {
		t.Fatalf("web-app claude: %+v", g)
	}
	if g.Procs[0].PID != 223 {
		t.Errorf("busiest member is %d, want 223", g.Procs[0].PID)
	}
	if g := find(tb, "agent:211:1000"); g == nil || !near(g.Cores, 0.2) || pids(g)[212] {
		t.Errorf("acme-api claude: %+v", g)
	}
	if g := find(tb, "app:Chromium"); g == nil || !near(g.Cores, 0.5) {
		t.Errorf("Chromium: %+v", g)
	}
	if g := find(tb, "app:ghostty"); g == nil || !near(g.Cores, 0.02) {
		t.Errorf("ghostty did not unfold: %+v", g)
	}
	if find(tb, "app:tinyapp") != nil || !pids(find(tb, "other"))[400] {
		t.Error("tinyapp did not fold")
	}
	order := []string{"agent:221:2000", "app:Chromium", "agent:211:1000"}
	for i, k := range order {
		if tb.Groups[i].Key != k {
			t.Errorf("group %d is %s, want %s", i, tb.Groups[i].Key, k)
		}
	}
}

func TestPIDReuse(t *testing.T) {
	ps := machine(t)
	root := filepath.Join(t.TempDir(), "proc")
	ps[213] = fake{ppid: 211, comm: "vitest", start: 5000, ticks: 900}
	writeProc(t, root, 100, ps)
	s := NewScanner()
	if _, err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	// 213 exits and its pid goes to a new process that ran 0.5 s of 1 s.
	ps[213] = fake{ppid: 211, comm: "vitest", start: 10100, ticks: 50}
	writeProc(t, root, 102, ps)
	tb, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	g := find(tb, "agent:211:1000")
	if g == nil || !near(g.Cores, 0.25) {
		t.Errorf("reused pid: %+v", g)
	}
}

func TestFindGitDirSubmodule(t *testing.T) {
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "acme-api/.git/modules/vendor"), 0o755)
	os.MkdirAll(filepath.Join(base, "acme-api/vendor"), 0o755)
	os.WriteFile(filepath.Join(base, "acme-api/vendor/.git"), []byte("gitdir: ../.git/modules/vendor\n"), 0o644)
	if d := findGitDir(filepath.Join(base, "acme-api/vendor")); d.repo != "vendor" || d.head != "" {
		t.Errorf("submodule: %+v", d)
	}
}

func TestScript(t *testing.T) {
	for argv, want := range map[string]string{
		"claude": "claude",
		"node --no-warnings /x/@openai/codex/bin.js": "codex",
		"node /x/vitest.mjs":                         "",
		"python3 -m aider":                           "aider",
		"deno run /x/gemini.ts":                      "gemini",
	} {
		if got := script(strings.Fields(argv)); got != want {
			t.Errorf("script(%q) = %q, want %q", argv, got, want)
		}
	}
}

// TestLive scans this machine twice and checks counts and invariants only.
// It prints group names and sizes, never anything about a process.
func TestLive(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the live /proc")
	}
	s := NewScanner()
	if _, err := s.Scan(); err != nil {
		t.Fatal(err)
	}
	tb, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if tb.Procs == 0 || tb.Threads < tb.Procs {
		t.Fatalf("Procs %d, Threads %d", tb.Procs, tb.Threads)
	}
	n, keys := 0, map[string]bool{}
	for i, g := range tb.Groups {
		n += len(g.Procs)
		if keys[g.Key] {
			t.Errorf("duplicate key %s", g.Key)
		}
		keys[g.Key] = true
		if g.Kind == Other && i != len(tb.Groups)-1 {
			t.Error("Other is not last")
		}
		if g.Cores < 0 {
			t.Errorf("%s: negative cores", g.Name)
		}
		t.Logf("%d %-20s %3d procs", g.Kind, g.Name, len(g.Procs))
	}
	if n != tb.Procs {
		t.Errorf("groups hold %d procs, scan saw %d", n, tb.Procs)
	}
}
