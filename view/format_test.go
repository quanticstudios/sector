package view

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{bytesStr(25.4 * (1 << 30)), "25.4 GiB"},
		{bytesStr(3.9 * (1 << 30)), "3.90 GiB"},
		{bytesStr(481 * (1 << 30)), "481 GiB"},
		{bytesStr(512), "512 B"},
		{rateStr(546 * 1024), "546 KiB/s"},
		{ceilStr(4 << 20), "4 MiB/s"},
		{mbStr(2.34e6), "2.3"},
		{mbStr(28.4e6), "28"},
		{bitsStr(67e6), "536 Mbps"},
		{commas(1071), "1,071"},
		{commas(7530000), "7,530,000"},
		{uptimeStr(2*time.Hour + 30*time.Minute + 55*time.Second), "2:30:55"},
		{uptimeStr(50 * time.Hour), "2d 2:00:00"},
		{shortModel("AMD Ryzen 9 9950X 16-Core Processor"), "Ryzen 9 9950X"},
		{shortModel("Intel(R) Core(TM) i7-12700K"), "Core i7-12700K"},
		{shortModel("AMD Ryzen 7 7840U w/ Radeon 780M Graphics"), "Ryzen 7 7840U"},
		{mountNames([]string{"/", "/home", "/var/log"}), "root, home, log"},
		{mountNames([]string{"/a", "/b", "/c", "/d", "/e"}), "a, b, c, 2 more"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestCeilPow2(t *testing.T) {
	for _, c := range []struct{ v, want float64 }{{0, 4}, {4, 4}, {4.1, 8}, {30, 32}, {33, 64}} {
		if got := ceilPow2(c.v, 4); got != c.want {
			t.Errorf("ceilPow2(%v) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestWaitStr(t *testing.T) {
	for sec, want := range map[float64]string{0.0004: "0.4 ms", 0.0061: "6.1 ms", 0.016: "16 ms", 0.25: "250 ms", 1.24: "1.2 s"} {
		if got := waitStr(sec); got != want {
			t.Errorf("waitStr(%v) = %q, want %q", sec, got, want)
		}
	}
}
