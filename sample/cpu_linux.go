package sample

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type cpuTimes struct{ busy, total uint64 }

// parseStat reads the cpu lines of /proc/stat: the all-CPU line and one per
// logical CPU, indexed by CPU number. Busy is every field but idle and
// iowait; guest time is already inside user and nice, so it is left out.
func parseStat(s string) (all cpuTimes, per []cpuTimes) {
	for line := range strings.Lines(s) {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		var t cpuTimes
		for i, v := range f[1:min(len(f), 9)] {
			n, _ := strconv.ParseUint(v, 10, 64)
			t.total += n
			if i != 3 && i != 4 {
				t.busy += n
			}
		}
		if f[0] == "cpu" {
			all = t
			continue
		}
		n, err := strconv.Atoi(f[0][3:])
		if err != nil {
			continue
		}
		for len(per) <= n {
			per = append(per, cpuTimes{})
		}
		per[n] = t
	}
	return all, per
}

// busyShare is the busy part of the time between two readings, 0 to 1.
func busyShare(prev, cur cpuTimes) float64 {
	if cur.total <= prev.total || cur.busy < prev.busy {
		return 0
	}
	return min(float64(cur.busy-prev.busy)/float64(cur.total-prev.total), 1)
}

func (s *sampler) readCPU(dt float64) (CPU, error) {
	b, err := os.ReadFile(filepath.Join(procRoot, "stat"))
	all, per := parseStat(string(b))
	c := CPU{Threads: make([]float64, len(per))}
	if s.cpu != nil {
		c.Total = busyShare(s.all, all)
		for i := range per {
			if i < len(s.cpu) {
				c.Threads[i] = busyShare(s.cpu[i], per[i])
			}
		}
	}
	s.all, s.cpu = all, per

	var diePkg []int
	c.Cores, c.Dies, diePkg = readTopology()
	readTemps(c.Dies, diePkg)
	c.MHz = readMHz()
	c.PowerW = s.readPower(dt)
	return c, err
}

// readTopology groups logical CPUs into cores and cores into dies. A die is
// what die_id names when it splits a package; Zen CPUs often report die_id 0
// for every CCD, so when die_id does not split but the L3 id does, each L3
// is a die. diePkg holds each die's physical package.
func readTopology() (cores []Core, dies []Die, diePkg []int) {
	type cpuInfo struct{ cpu, pkg, die, core, l3 int }
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "devices/system/cpu/cpu[0-9]*"))
	var cpus []cpuInfo
	byDie, byL3 := map[[2]int]bool{}, map[[2]int]bool{}
	for _, d := range dirs {
		n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(d), "cpu"))
		core, ok := readInt(filepath.Join(d, "topology/core_id"))
		if err != nil || !ok { // offline CPUs have no topology
			continue
		}
		c := cpuInfo{cpu: n, core: core, l3: -1}
		c.pkg, _ = readInt(filepath.Join(d, "topology/physical_package_id"))
		c.die, _ = readInt(filepath.Join(d, "topology/die_id"))
		if v, ok := readInt(filepath.Join(d, "cache/index3/id")); ok {
			c.l3 = v
		}
		cpus = append(cpus, c)
		byDie[[2]int{c.pkg, c.die}] = true
		byL3[[2]int{c.pkg, c.l3}] = true
	}
	useL3 := len(byL3) > len(byDie)
	dieKey := func(c cpuInfo) [2]int {
		if useL3 {
			return [2]int{c.pkg, c.l3}
		}
		return [2]int{c.pkg, c.die}
	}

	keys := []([2]int){}
	for _, c := range cpus {
		keys = append(keys, dieKey(c))
	}
	slices.SortFunc(keys, cmpPair)
	keys = slices.Compact(keys)
	for i, k := range keys {
		dies = append(dies, Die{ID: i})
		diePkg = append(diePkg, k[0])
	}

	byCore := map[[2]int]int{} // die index and core id to index in cores
	for _, c := range cpus {
		die, _ := slices.BinarySearchFunc(keys, dieKey(c), cmpPair)
		k := [2]int{die, c.core}
		i, ok := byCore[k]
		if !ok {
			i = len(cores)
			byCore[k] = i
			cores = append(cores, Core{ID: c.core, Die: die})
		}
		cores[i].Threads = append(cores[i].Threads, c.cpu)
	}
	for i := range cores {
		slices.Sort(cores[i].Threads)
	}
	slices.SortFunc(cores, func(a, b Core) int {
		return cmp.Or(cmp.Compare(a.Die, b.Die), cmp.Compare(a.ID, b.ID))
	})
	return cores, dies, diePkg
}

func cmpPair(a, b [2]int) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) }

// readTemps fills each die's temperature from hwmon. AMD k10temp reports
// Tccd1, Tccd2... which map to dies in order, one k10temp per package in
// hwmon number order; dies without one get Tdie, else Tctl. Intel coretemp
// reports "Package id N" for package N.
func readTemps(dies []Die, diePkg []int) {
	type ccd struct {
		dev, n int // hwmon position, Tccd number
		c      float64
	}
	var ccds []ccd
	var tdie, tctl float64
	pkg := map[int]float64{}
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "class/hwmon/hwmon*"))
	num := func(d string) int {
		n, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(d), "hwmon"))
		return n
	}
	slices.SortFunc(dirs, func(a, b string) int { return cmp.Compare(num(a), num(b)) })
	for i, d := range dirs {
		if name := readStr(filepath.Join(d, "name")); name != "k10temp" && name != "coretemp" {
			continue
		}
		labels, _ := filepath.Glob(filepath.Join(d, "temp*_label"))
		for _, l := range labels {
			milli, ok := readInt(strings.TrimSuffix(l, "_label") + "_input")
			if !ok {
				continue
			}
			c := float64(milli) / 1000
			switch label := readStr(l); {
			case strings.HasPrefix(label, "Tccd"):
				n, _ := strconv.Atoi(label[4:])
				ccds = append(ccds, ccd{i, n, c})
			case label == "Tdie":
				tdie = c
			case label == "Tctl":
				tctl = c
			case strings.HasPrefix(label, "Package id "):
				n, _ := strconv.Atoi(label[len("Package id "):])
				pkg[n] = c
			}
		}
	}
	slices.SortFunc(ccds, func(a, b ccd) int { return cmp.Or(cmp.Compare(a.dev, b.dev), cmp.Compare(a.n, b.n)) })
	fallback := cmp.Or(tdie, tctl)
	for i := range dies {
		t, ok := pkg[diePkg[i]]
		switch {
		case i < len(ccds):
			dies[i].TempC = ccds[i].c
		case ok:
			dies[i].TempC = t
		default:
			dies[i].TempC = fallback
		}
	}
}

// readMHz is the mean current clock over the CPUs that report one.
func readMHz() float64 {
	files, _ := filepath.Glob(filepath.Join(sysRoot, "devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq"))
	var sum, n float64
	for _, f := range files {
		if khz, ok := readInt(f); ok {
			sum += float64(khz)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n / 1000
}

// readPower sums package power over the top-level RAPL zones from the
// energy counter's change. The counter is often root-only; then it is 0.
func (s *sampler) readPower(dt float64) float64 {
	zones, _ := filepath.Glob(filepath.Join(sysRoot, "class/powercap/intel-rapl:[0-9]*"))
	cur := map[string]uint64{}
	var uj float64
	for _, z := range zones {
		if strings.Count(filepath.Base(z), ":") != 1 { // subzones are inside the package
			continue
		}
		e, ok := readInt(filepath.Join(z, "energy_uj"))
		if !ok {
			continue
		}
		cur[filepath.Base(z)] = uint64(e)
		prev, seen := s.energy[filepath.Base(z)]
		switch {
		case !seen:
		case uint64(e) >= prev:
			uj += float64(uint64(e) - prev)
		default: // the counter wrapped
			if top, ok := readInt(filepath.Join(z, "max_energy_range_uj")); ok {
				uj += float64(uint64(top) - prev + uint64(e))
			}
		}
	}
	s.energy = cur
	if dt <= 0 {
		return 0
	}
	return uj / 1e6 / dt
}
