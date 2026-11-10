package sample

import (
	"math"
	"os"
	"reflect"
	"testing"
)

func TestFoldMounts(t *testing.T) {
	// nvme0n1 holds /boot and a LUKS partition with btrfs subvolumes; nvme1n1
	// holds a plain btrfs filesystem with two subvolumes, one with a space.
	useTree(t, "testdata/devbox")
	mi, err := os.ReadFile("testdata/devbox/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	points, fs := foldMounts(parseMountinfo(string(mi)), readBlockIndex())
	wantPoints := map[string][]string{
		"nvme0n1": {"/", "/boot", "/home", "/var/log"},
		"nvme1n1": {"/home/dev/my projects", "/srv/data"},
	}
	wantFS := map[string][]fsShare{
		"nvme0n1": {{"/", 1}, {"/boot", 1}},
		"nvme1n1": {{"/srv/data", 1}},
	}
	if !reflect.DeepEqual(points, wantPoints) {
		t.Errorf("points %q, want %q", points, wantPoints)
	}
	if !reflect.DeepEqual(fs, wantFS) {
		t.Errorf("filesystems %v, want %v", fs, wantFS)
	}
}

func TestDriveRates(t *testing.T) {
	useTree(t, "testdata/devbox")
	stubFS(t, devboxSizes)
	prev := parseDiskstats(" 259 0 nvme0n1 50 0 1000 0 100 0 2000 0 0 100 0\n 259 3 nvme1n1 50 0 1000 0 20 0 400 0 0 100 0\n")
	tests := []struct {
		dt         float64
		rd, wr, bz float64
	}{
		{dt: 2, rd: 1000 * 512 / 2, wr: 2000 * 512 / 2, bz: 0.1},
		{dt: 0.05, rd: 1000 * 512 / 0.05, wr: 2000 * 512 / 0.05, bz: 1}, // busy clamps at 1
	}
	for _, tc := range tests {
		s := sampler{disks: prev}
		drives, err := s.readDrives(tc.dt)
		if err != nil {
			t.Fatal(err)
		}
		if len(drives) != 2 || drives[0].Name != "nvme0n1" || drives[1].Name != "nvme1n1" {
			t.Fatalf("drives %+v, want nvme0n1 and nvme1n1 only", drives)
		}
		d := drives[0]
		if d.ReadBps != tc.rd || d.WriteBps != tc.wr || d.Busy != tc.bz {
			t.Errorf("dt %v: read %v write %v busy %v, want %v %v %v", tc.dt, d.ReadBps, d.WriteBps, d.Busy, tc.rd, tc.wr, tc.bz)
		}
		if d.Model != "Acme NVMe 2TB" || d.Size != 110 || d.Used != 41 || d.Free != 69 {
			t.Errorf("nvme0n1 %+v", d)
		}
		if e := drives[1]; e.Size != 500 || e.ReadBps != 0 || e.Busy != 0 || e.Wait != 0 {
			t.Errorf("nvme1n1 %+v", e)
		}
		// 150 requests finished in 130 ms between them; 130 ms of requests
		// in flight over the interval.
		if want := 130.0 / 150 / 1000; math.Abs(d.Wait-want) > 1e-12 || math.Abs(d.Queue-0.13/tc.dt) > 1e-12 {
			t.Errorf("dt %v: wait %v queue %v, want %v %v", tc.dt, d.Wait, d.Queue, want, 0.13/tc.dt)
		}
	}
	// nvme1n1 had I/O in flight the whole 50 ms and finished nothing.
	stuck := sampler{disks: parseDiskstats(" 259 3 nvme1n1 50 0 1000 20 20 0 400 10 0 50 0\n")}
	if drives, _ := stuck.readDrives(0.05); drives[1].Wait != 0.05 {
		t.Errorf("stuck drive waits %v, want the interval", drives[1].Wait)
	}
	s := sampler{}
	drives, _ := s.readDrives(0)
	if drives[0].ReadBps != 0 || drives[0].Busy != 0 {
		t.Errorf("first sample has rates: %+v", drives[0])
	}
}

func TestLVMOverTwoDisks(t *testing.T) {
	// vg-data spans sda1 and sda2 (100 sectors each) and sdb1 (200), so
	// half of it is on each disk. It is mounted twice; each disk lists both
	// points and counts the filesystem once.
	const b = "sys/block/"
	useTree(t, writeTree(t, map[string]string{
		b + "sda/device": "", b + "sda/dev": "8:0",
		b + "sda/sda1/partition": "1", b + "sda/sda1/dev": "8:1", b + "sda/sda1/size": "100",
		b + "sda/sda2/partition": "2", b + "sda/sda2/dev": "8:2", b + "sda/sda2/size": "100",
		b + "sdb/device": "", b + "sdb/dev": "8:16",
		b + "sdb/sdb1/partition": "1", b + "sdb/sdb1/dev": "8:17", b + "sdb/sdb1/size": "200",
		b + "dm-0/dev": "254:0", b + "dm-0/dm/name": "vg-data",
		b + "dm-0/slaves/sda1": "", b + "dm-0/slaves/sda2": "", b + "dm-0/slaves/sdb1": "",
		"proc/diskstats": "",
		"proc/self/mountinfo": "40 1 254:0 / /srv/vol rw - ext4 /dev/mapper/vg-data rw\n" +
			"41 40 254:0 /sub /srv/vol/sub rw - ext4 /dev/mapper/vg-data rw\n",
	}))
	stubFS(t, map[string][3]uint64{"/srv/vol": {400, 100, 300}})
	var s sampler
	drives, err := s.readDrives(0)
	if err != nil {
		t.Fatal(err)
	}
	mounts := []string{"/srv/vol", "/srv/vol/sub"}
	want := []Drive{
		{Name: "sda", Mounts: mounts, Size: 200, Used: 50, Free: 150},
		{Name: "sdb", Mounts: mounts, Size: 200, Used: 50, Free: 150},
	}
	if !reflect.DeepEqual(drives, want) {
		t.Errorf("got %+v, want %+v", drives, want)
	}
}
