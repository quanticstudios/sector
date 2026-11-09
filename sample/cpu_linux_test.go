package sample

import (
	"reflect"
	"testing"
)

func TestStatShares(t *testing.T) {
	// cpu0 spends half the interval busy (user and steal), cpu1 only idles
	// and waits on I/O. The guest column (9th) is inside user already and
	// must not count twice.
	prev := "cpu  100 0 0 900 0 0 0 0 0 0\ncpu0 50 0 0 450 0 0 0 0 0 0\ncpu1 50 0 0 450 0 0 0 0 0 0\nintr 5\n"
	cur := "cpu  150 0 0 1000 100 0 0 0 999 0\ncpu0 90 0 0 500 0 0 0 10 999 0\ncpu1 50 0 0 500 50 0 0 0 0 0\nintr 9\n"
	pa, pp := parseStat(prev)
	ca, cp := parseStat(cur)
	if len(cp) != 2 {
		t.Fatalf("got %d CPUs, want 2", len(cp))
	}
	for _, tc := range []struct {
		name      string
		prev, cur cpuTimes
		want      float64
	}{
		{"total", pa, ca, 0.2},
		{"cpu0", pp[0], cp[0], 0.5},
		{"cpu1", pp[1], cp[1], 0},
		{"no time passed", cp[0], cp[0], 0},
	} {
		if got := busyShare(tc.prev, tc.cur); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTopology(t *testing.T) {
	cpu := func(n, core, die, l3 string) map[string]string {
		d := "sys/devices/system/cpu/cpu" + n + "/"
		return map[string]string{
			d + "topology/core_id": core, d + "topology/die_id": die,
			d + "topology/physical_package_id": "0", d + "cache/index3/id": l3,
		}
	}
	merge := func(ms ...map[string]string) string {
		all := map[string]string{}
		for _, m := range ms {
			for k, v := range m {
				all[k] = v
			}
		}
		return writeTree(t, all)
	}
	tests := []struct {
		name  string
		dir   string
		cores []Core
		dies  int
	}{
		{
			// Zen: die_id is 0 everywhere, each CCD has its own L3; CPU n
			// and n+4 are siblings.
			name: "L3 splits dies",
			dir:  "testdata/devbox",
			cores: []Core{
				{ID: 0, Die: 0, Threads: []int{0, 4}}, {ID: 1, Die: 0, Threads: []int{1, 5}},
				{ID: 2, Die: 1, Threads: []int{2, 6}}, {ID: 3, Die: 1, Threads: []int{3, 7}},
			},
			dies: 2,
		},
		{
			name: "die_id splits dies",
			dir:  merge(cpu("0", "0", "0", "0"), cpu("1", "4", "1", "0"), cpu("10", "0", "0", "0")),
			cores: []Core{
				{ID: 0, Die: 0, Threads: []int{0, 10}}, {ID: 4, Die: 1, Threads: []int{1}},
			},
			dies: 2,
		},
		{
			name:  "one die",
			dir:   merge(cpu("0", "0", "0", "0"), cpu("1", "1", "0", "0")),
			cores: []Core{{ID: 0, Die: 0, Threads: []int{0}}, {ID: 1, Die: 0, Threads: []int{1}}},
			dies:  1,
		},
	}
	for _, tc := range tests {
		useTree(t, tc.dir)
		cores, dies, pkgs := readTopology()
		if !reflect.DeepEqual(cores, tc.cores) || len(dies) != tc.dies || len(pkgs) != tc.dies {
			t.Errorf("%s: cores %+v dies %+v, want %+v and %d dies", tc.name, cores, dies, tc.cores, tc.dies)
		}
	}
}

func TestTemps(t *testing.T) {
	const h = "sys/class/hwmon/"
	tests := []struct {
		name   string
		files  map[string]string
		diePkg []int
		want   []float64
	}{
		{
			name: "Tccd in label order, not file order",
			files: map[string]string{
				h + "hwmon0/name": "nvme", h + "hwmon0/temp1_label": "Composite", h + "hwmon0/temp1_input": "40000",
				h + "hwmon1/name": "k10temp", h + "hwmon1/temp1_label": "Tctl", h + "hwmon1/temp1_input": "70000",
				h + "hwmon1/temp3_label": "Tccd2", h + "hwmon1/temp3_input": "45500",
				h + "hwmon1/temp4_label": "Tccd1", h + "hwmon1/temp4_input": "52000",
			},
			diePkg: []int{0, 0},
			want:   []float64{52, 45.5},
		},
		{
			name: "Tctl for every die",
			files: map[string]string{
				h + "hwmon0/name": "k10temp", h + "hwmon0/temp1_label": "Tctl", h + "hwmon0/temp1_input": "61250",
			},
			diePkg: []int{0, 0},
			want:   []float64{61.25, 61.25},
		},
		{
			name: "Tdie over Tctl",
			files: map[string]string{
				h + "hwmon0/name": "k10temp", h + "hwmon0/temp1_label": "Tctl", h + "hwmon0/temp1_input": "80000",
				h + "hwmon0/temp2_label": "Tdie", h + "hwmon0/temp2_input": "53000",
			},
			diePkg: []int{0},
			want:   []float64{53},
		},
		{
			name: "coretemp per package",
			files: map[string]string{
				h + "hwmon2/name": "coretemp", h + "hwmon2/temp1_label": "Package id 1", h + "hwmon2/temp1_input": "44000",
				h + "hwmon2/temp2_label": "Package id 0", h + "hwmon2/temp2_input": "39000",
				h + "hwmon2/temp3_label": "Core 0", h + "hwmon2/temp3_input": "90000",
			},
			diePkg: []int{0, 1},
			want:   []float64{39, 44},
		},
		{name: "no sensors", files: map[string]string{"sys/x": ""}, diePkg: []int{0}, want: []float64{0}},
	}
	for _, tc := range tests {
		useTree(t, writeTree(t, tc.files))
		dies := make([]Die, len(tc.diePkg))
		readTemps(dies, tc.diePkg)
		var got []float64
		for _, d := range dies {
			got = append(got, d.TempC)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPower(t *testing.T) {
	const z = "sys/class/powercap/intel-rapl:0/"
	var s sampler
	step := func(uj string, dt float64) float64 {
		useTree(t, writeTree(t, map[string]string{
			z + "energy_uj": uj, z + "max_energy_range_uj": "10000000",
			"sys/class/powercap/intel-rapl:0:0/energy_uj": "123",
		}))
		return s.readPower(dt)
	}
	for i, tc := range []struct {
		uj   string
		dt   float64
		want float64
	}{
		{"1000000", 0, 0}, // first reading
		{"7000000", 2, 3}, // 6 J over 2 s
		{"1000000", 2, 2}, // wrapped: 3 J to the top, then 1 J
		{"", 1, 0},        // root-only counter
		{"5000000", 1, 0}, // first reading again after a gap
	} {
		if got := step(tc.uj, tc.dt); got != tc.want {
			t.Errorf("step %d: got %v W, want %v", i, got, tc.want)
		}
	}
}
