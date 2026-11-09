package sample

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// procRoot and sysRoot are where /proc and /sys live; tests point them at a
// fake tree.
var procRoot, sysRoot = "/proc", "/sys"

type sampler struct {
	last   time.Time
	cpu    []cpuTimes // per logical CPU, nil before the first Sample
	all    cpuTimes
	energy map[string]uint64 // RAPL microjoules per zone
	disks  map[string]ioCounters
	nets   map[string]netCounters
}

func (s *Sampler) sample() (Snapshot, error) {
	now := time.Now()
	var dt float64 // seconds since the previous Sample, 0 on the first
	if !s.last.IsZero() {
		dt = now.Sub(s.last).Seconds()
	}
	s.last = now

	snap := Snapshot{Time: now}
	var errs []error
	cpu, err := s.readCPU(dt)
	errs = append(errs, err)
	snap.CPU = cpu
	snap.Host = readHost(len(cpu.Threads))

	mi, err := os.ReadFile(filepath.Join(procRoot, "meminfo"))
	errs = append(errs, err)
	snap.Mem = parseMeminfo(string(mi))

	snap.Drives, err = s.readDrives(dt)
	errs = append(errs, err)
	snap.Nets, err = s.readNets(dt)
	errs = append(errs, err)
	return snap, errors.Join(errs...)
}

func readHost(threads int) Host {
	h := Host{Threads: threads, Name: readStr(filepath.Join(procRoot, "sys/kernel/hostname"))}
	if h.Name == "" {
		h.Name, _ = os.Hostname()
	}
	for line := range strings.Lines(readStr(filepath.Join(procRoot, "cpuinfo"))) {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "model name" {
			h.Model = strings.TrimSpace(v)
			break
		}
	}
	if f := strings.Fields(readStr(filepath.Join(procRoot, "uptime"))); len(f) > 0 {
		sec, _ := strconv.ParseFloat(f[0], 64)
		h.Uptime = time.Duration(sec * float64(time.Second))
	}
	for i, f := range strings.Fields(readStr(filepath.Join(procRoot, "loadavg"))) {
		if i == 3 {
			break
		}
		h.Load[i], _ = strconv.ParseFloat(f, 64)
	}
	return h
}

// parseMeminfo reads /proc/meminfo. Used is MemTotal minus MemAvailable, as
// btop and free(1) report it; Cached adds the reclaimable slab to the page
// cache, as free(1) does.
func parseMeminfo(s string) Mem {
	kv := map[string]uint64{}
	for line := range strings.Lines(s) {
		k, v, ok := strings.Cut(line, ":")
		if f := strings.Fields(v); ok && len(f) > 0 {
			n, _ := strconv.ParseUint(f[0], 10, 64)
			kv[k] = n * 1024
		}
	}
	m := Mem{
		Total:     kv["MemTotal"],
		Free:      kv["MemFree"],
		Cached:    kv["Cached"] + kv["SReclaimable"],
		SwapTotal: kv["SwapTotal"],
	}
	avail, ok := kv["MemAvailable"]
	if !ok { // kernels before 3.14
		avail = m.Free + m.Cached
	}
	m.Available = avail
	m.Used = m.Total - min(avail, m.Total)
	m.SwapUsed = m.SwapTotal - min(kv["SwapFree"], m.SwapTotal)
	return m
}

type netCounters struct{ rx, tx uint64 }

// parseNetDev reads /proc/net/dev into received and sent bytes per interface.
func parseNetDev(s string) map[string]netCounters {
	out := map[string]netCounters{}
	for line := range strings.Lines(s) {
		name, rest, ok := strings.Cut(line, ":")
		f := strings.Fields(rest)
		if !ok || len(f) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(f[0], 10, 64)
		tx, _ := strconv.ParseUint(f[8], 10, 64)
		out[strings.TrimSpace(name)] = netCounters{rx, tx}
	}
	return out
}

// physicalUp reports whether an interface is up and backed by hardware.
// Loopback, bridges, veth, docker and tunnels have no device link.
func physicalUp(name string) bool {
	dir := filepath.Join(sysRoot, "class/net", name)
	return readStr(filepath.Join(dir, "operstate")) == "up" && exists(filepath.Join(dir, "device"))
}

// ifaceAddr returns an interface's first IPv4 address, else its first IPv6
// one. Tests replace it, since it asks the kernel and not the fake tree.
var ifaceAddr = func(name string) string {
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, _ := ifi.Addrs()
	v6 := ""
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if n.IP.To4() != nil {
			return n.IP.String()
		}
		if v6 == "" {
			v6 = n.IP.String()
		}
	}
	return v6
}

func (s *sampler) readNets(dt float64) ([]Net, error) {
	b, err := os.ReadFile(filepath.Join(procRoot, "net/dev"))
	cur := parseNetDev(string(b))
	var nets []Net
	for name, c := range cur {
		if !physicalUp(name) {
			continue
		}
		n := Net{Name: name, Addr: ifaceAddr(name), RxTotal: c.rx, TxTotal: c.tx}
		if p, ok := s.nets[name]; ok {
			n.RxBps, n.TxBps = rate(p.rx, c.rx, dt), rate(p.tx, c.tx, dt)
		}
		nets = append(nets, n)
	}
	slices.SortFunc(nets, func(a, b Net) int { return strings.Compare(a.Name, b.Name) })
	s.nets = cur
	return nets, err
}

// rate is the per-second change of a counter, 0 when it went backwards
// (reset or wrap) or no time passed.
func rate(prev, cur uint64, dt float64) float64 {
	if dt <= 0 || cur < prev {
		return 0
	}
	return float64(cur-prev) / dt
}

func readStr(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

func readInt(path string) (int, bool) {
	n, err := strconv.Atoi(readStr(path))
	return n, err == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
