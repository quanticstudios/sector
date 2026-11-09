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
	unit         string // its systemd unit; "" writes no cgroup file
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
		if p.unit != "" {
			w(d+"/cgroup", "0::/user.slice/user-1000.slice/user@1000.service/app.slice/"+p.unit+"\n")
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

type wantGroup struct {
	key, name, repo, branch string
	kind                    Kind
	pids                    []int
}

// checkGroups checks tb holds exactly the groups in want.
func checkGroups(t *testing.T, tb Table, want []wantGroup) {
	t.Helper()
	if len(tb.Groups) != len(want) {
		keys := []string{}
		for _, g := range tb.Groups {
			keys = append(keys, g.Key)
		}
		t.Errorf("got groups %v, want %d", keys, len(want))
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
		wantPID := 0
		if w.kind == Agent {
			wantPID = w.pids[0] // an agent's own pid comes first in want
		}
		if g.PID != wantPID {
			t.Errorf("%s: PID = %d, want %d", w.key, g.PID, wantPID)
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
}

// TestScanDesktop scans a Hyprland session started through uwsm, its
// apps in systemd user scopes.
func TestScanDesktop(t *testing.T) {
	_, web := repos(t)
	big := uint64(300*mib) / uint64(os.Getpagesize()) // keeps a group from folding
	wm := "wayland-wm@hyprland.desktop.service"
	spotify1, spotify2 := "app-flatpak-com.spotify.Client-1234.scope", "app-flatpak-com.spotify.Client-5678.scope"
	script := "app-Hyprland-autostart.sh-c0ffee.scope"
	term := "app-Hyprland-org.acme.Term-ab12cd.scope"
	gtkScope, setsidScope := `app-Hyprland-gtk\x2dlaunch-beef01.scope`, "app-Hyprland-setsid-beef02.scope"
	ps := map[int]fake{
		1:   {comm: "systemd", other: true, rss: big},
		100: {ppid: 1, comm: "systemd", unit: "init.scope"},
		101: {ppid: 100, comm: "(sd-pam)", unit: "init.scope"},
		// The compositor and what it starts itself.
		110: {ppid: 100, comm: "start-hyprland", unit: wm},
		111: {ppid: 110, comm: "Hyprland", unit: wm, rss: big},
		112: {ppid: 111, comm: "Xwayland", unit: wm, rss: big},
		113: {ppid: 111, comm: "launch-bar", exe: "/usr/bin/bash", unit: wm},
		114: {ppid: 113, comm: "statusbar", unit: wm, rss: big},
		109: {ppid: 114, comm: "wl-paste", unit: wm}, // a lower pid than its parent
		// One Flatpak app in two scopes, bwrap at each root.
		120: {ppid: 100, comm: "bwrap", unit: spotify1},
		121: {ppid: 120, comm: "xdg-dbus-proxy", unit: spotify1},
		122: {ppid: 120, comm: "bwrap", unit: spotify1},
		123: {ppid: 122, comm: "spotify", unit: spotify1, rss: big},
		130: {ppid: 100, comm: "bwrap", unit: spotify2},
		131: {ppid: 130, comm: "spotify", unit: spotify2},
		// An autostart script's scope: the script is gone, its apps remain.
		140: {ppid: 100, comm: "chromium", unit: script, rss: big},
		141: {ppid: 140, comm: "chromium", unit: script},
		145: {ppid: 100, comm: "notes", unit: script, rss: big},
		// A terminal in its scope with an agent in it.
		150: {ppid: 111, comm: "term", unit: term, rss: big},
		151: {ppid: 150, comm: "zsh", unit: term},
		152: {ppid: 151, comm: "claude", exe: "/opt/claude/2.1.0", cwd: web, unit: term, start: 3000},
		153: {ppid: 152, comm: "node", unit: term},
		// Apps the launcher started through gtk-launch, and through setsid
		// and nohup, in scopes named after the launcher.
		170: {ppid: 111, comm: "gtk-launch", unit: gtkScope},
		171: {ppid: 170, comm: "editor", unit: gtkScope, rss: big},
		180: {ppid: 111, comm: "setsid", unit: setsidScope},
		181: {ppid: 180, comm: "nohup", unit: setsidScope},
		182: {ppid: 181, comm: "billing", unit: setsidScope, rss: big},
		// A user service.
		160: {ppid: 100, comm: "acme-sync-dae", exe: "/usr/bin/bash", unit: "acme-sync.service", rss: big},
	}
	writeProc(t, filepath.Join(t.TempDir(), "proc"), 100, ps)
	tb, err := NewScanner().Scan()
	if err != nil {
		t.Fatal(err)
	}
	checkGroups(t, tb, []wantGroup{
		{"agent:152:3000", "claude", "web-app", "fix/login", Agent, []int{152, 153}},
		{"app:Hyprland", "Hyprland", "", "", App, []int{110, 111}},
		{"app:Xwayland", "Xwayland", "", "", App, []int{112}},
		{"app:statusbar", "statusbar", "", "", App, []int{109, 113, 114}},
		{"app:Spotify", "Spotify", "", "", App, []int{120, 121, 122, 123, 130, 131}},
		{"app:Chromium", "Chromium", "", "", App, []int{140, 141}},
		{"app:notes", "notes", "", "", App, []int{145}},
		{"app:Term", "Term", "", "", App, []int{150, 151}},
		{"app:acme-sync", "acme-sync", "", "", App, []int{160}},
		{"app:editor", "editor", "", "", App, []int{170, 171}},
		{"app:billing", "billing", "", "", App, []int{180, 181, 182}},
		{"system", "System", "", "", System, []int{1, 100, 101}},
	})
}

func TestAppOfUnit(t *testing.T) {
	for unit, want := range map[string]string{
		"app-flatpak-com.spotify.Client-1234.scope":      "Spotify",
		`app-Hyprland-acme\x2dnotes-0a1b2c3d.scope`:      "acme-notes",
		"app-org.chromium.Chromium-4214.scope":           "Chromium",
		"app-gnome-org.acme.Editor-99.scope":             "Editor",
		"app-billing@autostart.service":                  "billing",
		"app-com.acme.Mail@0f1e2d.service":               "Mail",
		"app-billing.scope":                              "billing",
		"snap.billing.daemon-6f1d2e3a.scope":             "billing",
		"acme-sync.service":                              "acme-sync",
		"dbus-:1.26-org.acme.Registry@0.service":         "Registry",
		"app-Hyprland-autostart.sh-c0ffee.scope":         "",
		"app-Hyprland-uwsm-c0ffee.scope":                 "",
		`app-Hyprland-gtk\x2dlaunch-c0ffee.scope`:        "",
		"app-Hyprland-xdg-open-c0ffee.scope":             "",
		"app-dbus\\x2d:1.26\\x2dorg.acme.Registry.slice": "",
		"session-1.scope":                                "",
		"init.scope":                                     "",
		"":                                               "",
	} {
		if got := appOfUnit(unit); got != want {
			t.Errorf("appOfUnit(%q) = %q, want %q", unit, got, want)
		}
	}
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
	// No fake process has a cgroup: every app is named by its tree.
	checkGroups(t, tb, []wantGroup{
		{"agent:211:1000", "claude", "acme-api", "", Agent, []int{211, 212, 213}},
		{"agent:214:0", "codex", "", "", Agent, []int{214, 215}},
		{"agent:221:2000", "claude", "web-app", "fix/login", Agent, []int{221, 222, 223}},
		{"app:Chromium", "Chromium", "", "", App, []int{300, 301}},
		{"system", "System", "", "", System, []int{1, 2, 50, 90, 100}},
		{"other", "Everything else", "", "", Other, []int{200, 210, 220, 400}},
	})
	// ghostty and tinyapp folded.
	for _, g := range tb.Groups {
		if want := map[Kind]int{Other: 2}[g.Kind]; g.Folded != want {
			t.Errorf("%s: Folded = %d, want %d", g.Key, g.Folded, want)
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
