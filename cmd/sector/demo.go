package main

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/quanticstudios/sector/procs"
	"github.com/quanticstudios/sector/sample"
)

// demo makes up a busy machine for -demo: a 16-core, 2-die CPU, two NVMe
// drives and a handful of fictional agents and apps. Every value wanders
// around a base so the graphs move.
type demo struct {
	rnd   *rand.Rand
	start time.Time
	load  [32]float64 // per thread
	walks map[string]float64
	rxTot uint64
	txTot uint64
}

func newDemo() *demo {
	d := &demo{rnd: rand.New(rand.NewPCG(1, 2)), start: time.Now().Add(-(2*time.Hour + 30*time.Minute)), walks: map[string]float64{}}
	for i := range d.load {
		d.load[i] = 0.3
	}
	d.rxTot, d.txTot = 11<<30, 731<<20
	return d
}

// walk moves the value named k a step toward base, plus noise of size j,
// never under lo.
func (d *demo) walk(k string, base, j, lo float64) float64 {
	p, ok := d.walks[k]
	if !ok {
		p = base
	}
	p = math.Max(lo, p+(base-p)*0.3+(d.rnd.Float64()-0.5)*j)
	d.walks[k] = p
	return p
}

type demoGroup struct {
	key, name, agent, repo, branch string
	kind                           procs.Kind
	base                           float64 // cores
	rss                            uint64
	members                        []string
}

var demoGroups = []demoGroup{
	{"agent:acme-api", "claude", "claude", "acme-api", "", procs.Agent, 6.5, 3 << 30, []string{"vitest", "claude", "node", "esbuild"}},
	{"agent:web-app", "claude", "claude", "web-app", "fix/login", procs.Agent, 2.2, 1200 << 20, []string{"claude", "node", "next-server"}},
	{"app:chromium", "Chromium", "", "", "", procs.App, 1.1, 2600 << 20, []string{"chromium", "chromium", "chromium"}},
	{"agent:billing", "codex", "codex", "billing", "", procs.Agent, 0.6, 600 << 20, []string{"codex", "cargo", "rustc"}},
	{"app:slack", "Slack", "", "", "", procs.App, 0.15, 350 << 20, []string{"slack", "slack"}},
}

func (d *demo) next() (sample.Snapshot, procs.Table) {
	now := time.Now()
	s := sample.Snapshot{Time: now, Host: sample.Host{
		Name: "devbox", Model: "AMD Ryzen 9 9950X 16-Core Processor", Threads: 32,
		Uptime: now.Sub(d.start), Load: [3]float64{12.9, 11.6, 8.4},
	}}

	var t procs.Table
	busy := 0.0
	pid := 4000
	for _, g := range demoGroups {
		c := d.walk(g.key, g.base, g.base*0.25, 0.01)
		grp := procs.Group{Key: g.key, Kind: g.kind, Name: g.name, Agent: g.agent, Repo: g.repo, Branch: g.branch, Cores: c, RSS: g.rss}
		for i, m := range g.members {
			share := c / float64(len(g.members))
			if i == 0 {
				share = c - share*float64(len(g.members)-1)
			}
			grp.Procs = append(grp.Procs, procs.Proc{PID: pid, PPID: 1, Comm: m, State: 'S', Cores: share, RSS: g.rss / uint64(len(g.members)), Threads: 12})
			grp.Threads += 12
			pid++
		}
		busy += c
		t.Groups = append(t.Groups, grp)
	}
	rest := d.walk("rest", 1.6, 0.5, 0.2)
	busy += rest
	t.Groups = append(t.Groups, procs.Group{Key: "other", Kind: procs.Other, Name: "Everything else", Cores: rest, RSS: 15900 << 20, Threads: 7530, Folded: 31,
		Procs: []procs.Proc{{PID: 900, PPID: 1, Comm: "systemd", State: 'S', Cores: rest, RSS: 15900 << 20, Threads: 7530}}})
	t.Procs, t.Threads = 1071, 8400

	// Spread the busy cores over the threads: die 0 takes most of it.
	for i := range d.load {
		w := 1.0
		if i%16 >= 8 {
			w = 0.3
		}
		d.load[i] = math.Min(1, math.Max(0.02, d.load[i]+(busy/32*w*1.5-d.load[i])*0.3+(d.rnd.Float64()-0.5)*0.25))
	}
	s.CPU.Threads = d.load[:]
	for _, l := range d.load {
		s.CPU.Total += l / 32
	}
	for c := range 16 {
		s.CPU.Cores = append(s.CPU.Cores, sample.Core{ID: c, Die: c / 8, Threads: []int{c, c + 16}})
	}
	s.CPU.Dies = []sample.Die{{ID: 0, TempC: d.walk("t0", 72, 2, 40)}, {ID: 1, TempC: d.walk("t1", 52, 2, 40)}}
	s.CPU.TempC = d.walk("tctl", 74, 2, 40)
	s.CPU.MHz = d.walk("mhz", 5400, 150, 3000)
	s.CPU.PowerW = d.walk("w", 116, 8, 30)

	used := uint64(d.walk("used", 25.4, 0.5, 10) * (1 << 30))
	cached := uint64(d.walk("cache", 29.6, 0.4, 10) * (1 << 30))
	total := uint64(61850 << 20)
	s.Mem = sample.Mem{Total: total, Used: used, Cached: cached, Free: total - used - cached, Available: total - used,
		SwapTotal: 15 << 30, SwapUsed: 1800 << 20}

	s.Drives = []sample.Drive{
		{Name: "nvme0n1", Mounts: []string{"/", "/home", "/var/log", "/var/cache/pacman/pkg"}, Size: 929 << 30, Used: 481 << 30, Free: 448 << 30,
			ReadBps: d.walk("r0", 2.3, 1.6, 0.02) * 1e6, WriteBps: d.walk("w0", 21, 12, 0.02) * 1e6, Busy: d.walk("b0", 0.14, 0.06, 0.01),
			Wait: d.walk("q0", 0.0005, 0.0003, 0.0001), Queue: d.walk("n0", 0.8, 0.5, 0.05)},
		{Name: "nvme1n1", Mounts: []string{"/data", "/data/work", "/data/scratch"}, Size: 931 << 30, Used: 461 << 30, Free: 470 << 30,
			ReadBps: d.walk("r1", 1.0, 0.8, 0.02) * 1e6, WriteBps: d.walk("w1", 7, 5, 0.02) * 1e6, Busy: d.walk("b1", 0.04, 0.03, 0.01),
			Wait: d.walk("q1", 0.0003, 0.0002, 0.0001), Queue: d.walk("n1", 0.2, 0.1, 0.05)},
	}
	rx, tx := d.walk("rx", 2.05, 0.9, 0.02)*(1<<20), d.walk("tx", 0.53, 0.3, 0.02)*(1<<20)
	d.rxTot += uint64(rx)
	d.txTot += uint64(tx)
	s.Nets = []sample.Net{{Name: "enp16s0", Addr: "192.168.1.20", RxBps: rx, TxBps: tx, RxTotal: d.rxTot, TxTotal: d.txTot}}
	return s, t
}
