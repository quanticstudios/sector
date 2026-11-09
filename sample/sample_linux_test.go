package sample

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// useTree points procRoot and sysRoot at dir/proc and dir/sys for one test.
func useTree(t *testing.T, dir string) {
	t.Helper()
	p, s := procRoot, sysRoot
	procRoot, sysRoot = filepath.Join(dir, "proc"), filepath.Join(dir, "sys")
	t.Cleanup(func() { procRoot, sysRoot = p, s })
}

// writeTree writes files (path to content) under a temp dir and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, c := range files {
		p = filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// stubFS replaces statfs and ifaceAddr for one test.
func stubFS(t *testing.T, sizes map[string][3]uint64) {
	t.Helper()
	sf, ia := statfs, ifaceAddr
	statfs = func(p string) (uint64, uint64, uint64, error) {
		s, ok := sizes[p]
		if !ok {
			t.Errorf("statfs(%q): not a filesystem to measure", p)
			return 0, 0, 0, os.ErrNotExist
		}
		return s[0], s[1], s[2], nil
	}
	ifaceAddr = func(name string) string { return "192.0.2.10" }
	t.Cleanup(func() { statfs, ifaceAddr = sf, ia })
}

var devboxSizes = map[string][3]uint64{
	"/":         {100, 40, 60},
	"/boot":     {10, 1, 9},
	"/srv/data": {500, 100, 400},
}

func TestMeminfo(t *testing.T) {
	const k = 1024
	tests := []struct {
		name, in string
		want     Mem
	}{
		{
			name: "available",
			in: "MemTotal: 32000 kB\nMemFree: 4000 kB\nMemAvailable: 20000 kB\nBuffers: 500 kB\n" +
				"Cached: 12000 kB\nSwapCached: 7 kB\nSReclaimable: 1000 kB\nSwapTotal: 8000 kB\nSwapFree: 6000 kB\n",
			want: Mem{Total: 32000 * k, Used: 12000 * k, Available: 20000 * k, Cached: 13000 * k, Free: 4000 * k,
				SwapTotal: 8000 * k, SwapUsed: 2000 * k},
		},
		{
			name: "no MemAvailable",
			in:   "MemTotal: 32000 kB\nMemFree: 4000 kB\nCached: 12000 kB\nSReclaimable: 1000 kB\n",
			want: Mem{Total: 32000 * k, Used: 15000 * k, Available: 17000 * k, Cached: 13000 * k, Free: 4000 * k},
		},
	}
	for _, tc := range tests {
		if got := parseMeminfo(tc.in); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestNets(t *testing.T) {
	useTree(t, "testdata/devbox")
	stubFS(t, nil)
	var s sampler
	s.nets = map[string]netCounters{"enp5s0": {rx: 500000, tx: 50000}}
	got, err := s.readNets(2)
	if err != nil {
		t.Fatal(err)
	}
	want := []Net{{Name: "enp5s0", Addr: "192.0.2.10", RxBps: 250000, TxBps: 100000, RxTotal: 1000000, TxTotal: 250000}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v (lo, docker0, veth and the down wlan0 drop out)", got, want)
	}
}

func TestSampleFixture(t *testing.T) {
	useTree(t, "testdata/devbox")
	stubFS(t, devboxSizes)
	snap, err := New().Sample()
	if err != nil {
		t.Fatal(err)
	}
	h := snap.Host
	if h.Name != "devbox" || h.Model != "Acme Zen 4-Core Processor" || h.Threads != 8 ||
		h.Uptime != 3725500*time.Millisecond || h.Load != [3]float64{1.25, 0.75, 0.5} {
		t.Errorf("host %+v", h)
	}
	c := snap.CPU
	if c.Total != 0 || !reflect.DeepEqual(c.Threads, make([]float64, 8)) {
		t.Errorf("first sample has rates: total %v threads %v", c.Total, c.Threads)
	}
	if want := []Die{{0, 58.25}, {1, 47.125}}; !reflect.DeepEqual(c.Dies, want) {
		t.Errorf("dies %+v, want %+v", c.Dies, want)
	}
	if c.MHz != 4000 || c.PowerW != 0 || len(c.Cores) != 4 {
		t.Errorf("MHz %v power %v cores %d", c.MHz, c.PowerW, len(c.Cores))
	}
	if snap.Mem.Total != 32000000*1024 || len(snap.Drives) != 2 || len(snap.Nets) != 1 {
		t.Errorf("mem %+v drives %d nets %d", snap.Mem, len(snap.Drives), len(snap.Nets))
	}
}

func TestLive(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the real machine")
	}
	s := New()
	if _, err := s.Sample(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	snap, err := s.Sample()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Host.Threads <= 0 || len(snap.CPU.Threads) != snap.Host.Threads {
		t.Errorf("threads %d, per-thread %d", snap.Host.Threads, len(snap.CPU.Threads))
	}
	if snap.Mem.Total == 0 || snap.Host.Uptime <= 0 || len(snap.CPU.Cores) == 0 || len(snap.CPU.Dies) == 0 {
		t.Errorf("empty totals: mem %d uptime %v cores %d dies %d",
			snap.Mem.Total, snap.Host.Uptime, len(snap.CPU.Cores), len(snap.CPU.Dies))
	}
	if snap.CPU.Total < 0 || snap.CPU.Total > 1 {
		t.Errorf("total %v outside 0 to 1", snap.CPU.Total)
	}
	for _, d := range snap.Drives {
		if d.Busy < 0 || d.Busy > 1 || d.Used > d.Size {
			t.Errorf("drive %s: busy %v used %d size %d", d.Name, d.Busy, d.Used, d.Size)
		}
	}
}
