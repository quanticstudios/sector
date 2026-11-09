package view

import (
	"testing"

	"github.com/quanticstudios/sector/sample"
)

func TestDiskTop(t *testing.T) {
	h := []entry{
		{drives: map[string][2]float64{"nvme0n1": {2e6, 21e6}, "nvme1n1": {1e6, 1e6}}},
		{drives: map[string][2]float64{"nvme0n1": {30e6, 5e6}}},
	}
	// 30 MB/s read with 10% headroom is 33, so the ceiling is 64.
	if got := diskTop(h, "nvme0n1"); got != 64 {
		t.Errorf("busy drive: %v, want 64", got)
	}
	if got := diskTop(h, "nvme1n1"); got != 4 {
		t.Errorf("quiet drive: %v, want the floor of 4", got)
	}
	if got := diskTop(nil, "sda"); got != 4 {
		t.Errorf("no history: %v, want 4", got)
	}
}

func TestMainNet(t *testing.T) {
	if _, ok := mainNet(nil); ok {
		t.Fatal("no interfaces should report none")
	}
	n, _ := mainNet([]sample.Net{{Name: "wlan0", RxTotal: 50}, {Name: "enp16s0", RxTotal: 10, TxTotal: 90}, {Name: "tun0", TxTotal: 40}})
	if n.Name != "enp16s0" {
		t.Fatalf("picked %s", n.Name)
	}
}
