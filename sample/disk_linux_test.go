package sample

import (
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
	wantFS := map[string][]string{
		"nvme0n1": {"/", "/boot"},
		"nvme1n1": {"/srv/data"},
	}
	if !reflect.DeepEqual(points, wantPoints) {
		t.Errorf("points %q, want %q", points, wantPoints)
	}
	if !reflect.DeepEqual(fs, wantFS) {
		t.Errorf("filesystems %q, want %q", fs, wantFS)
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
		if e := drives[1]; e.Size != 500 || e.ReadBps != 0 || e.Busy != 0 {
			t.Errorf("nvme1n1 %+v", e)
		}
	}
	s := sampler{}
	drives, _ := s.readDrives(0)
	if drives[0].ReadBps != 0 || drives[0].Busy != 0 {
		t.Errorf("first sample has rates: %+v", drives[0])
	}
}
