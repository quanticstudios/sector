package view

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var binUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

// unitOf is the binary unit for n bytes: the largest one n is at least 1 of.
func unitOf(n float64) (div float64, name string) {
	i := 0
	for div = 1; n >= 1024*div && i < len(binUnits)-1; i++ {
		div *= 1024
	}
	return div, binUnits[i]
}

// num3 is v with about three significant digits: 3.90, 25.4, 481.
func num3(v float64) string {
	switch {
	case v < 9.995:
		return strconv.FormatFloat(v, 'f', 2, 64)
	case v < 99.95:
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	return strconv.FormatFloat(v, 'f', 0, 64)
}

// inUnit is n bytes in the unit div, plain for whole bytes.
func inUnit(n, div float64) string {
	if div == 1 {
		return strconv.FormatFloat(n, 'f', 0, 64)
	}
	return num3(n / div)
}

// bytesStr is n bytes as "25.4 GiB".
func bytesStr(n float64) string {
	div, u := unitOf(n)
	return inUnit(n, div) + " " + u
}

// rateStr is a byte rate as "2.05 MiB/s".
func rateStr(bps float64) string { return bytesStr(bps) + "/s" }

// ceilStr is a power-of-two ceiling in bytes, no decimals: "4 MiB/s".
func ceilStr(n float64) string {
	div, u := unitOf(n)
	return strconv.FormatFloat(n/div, 'f', -1, 64) + " " + u + "/s"
}

// mbStr is a disk rate in decimal MB/s: one decimal under 10, none above.
func mbStr(bps float64) string {
	mb := bps / 1e6
	if mb < 9.95 {
		return strconv.FormatFloat(mb, 'f', 1, 64)
	}
	return strconv.FormatFloat(mb, 'f', 0, 64)
}

// bitsStr is a byte rate as bits: "536 Mbps".
func bitsStr(bps float64) string {
	b := bps * 8
	switch {
	case b >= 1e9:
		return num3(b/1e9) + " Gbps"
	case b >= 1e6:
		return num3(b/1e6) + " Mbps"
	}
	return num3(b/1e3) + " Kbps"
}

// ceilPow2 is the smallest of start, 2*start, 4*start... at or above v.
func ceilPow2(v, start float64) float64 {
	top := start
	for top < v && !math.IsInf(v, 0) {
		top *= 2
	}
	return top
}

// commas writes n with thousands separators: 1,071.
func commas(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + commas(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// uptimeStr is h:mm:ss, with days in front after the first day.
func uptimeStr(d time.Duration) string {
	s := int(d / time.Second)
	hms := fmt.Sprintf("%d:%02d:%02d", s/3600%24, s/60%60, s%60)
	if days := s / 86400; days > 0 {
		return fmt.Sprintf("%dd %s", days, hms)
	}
	return hms
}

// shortModel trims a CPU model string to what a person calls it:
// "AMD Ryzen 9 9950X 16-Core Processor" becomes "Ryzen 9 9950X".
func shortModel(m string) string {
	for _, cut := range []string{" @ ", " w/ ", " with "} {
		if i := strings.Index(m, cut); i >= 0 {
			m = m[:i]
		}
	}
	for _, tm := range []string{"(R)", "(TM)", "(tm)", "(r)"} {
		m = strings.ReplaceAll(m, tm, "")
	}
	var keep []string
	for _, w := range strings.Fields(m) {
		switch {
		case w == "AMD" || w == "Intel" || w == "Processor" || w == "CPU",
			strings.HasSuffix(w, "-Core"):
			continue
		}
		keep = append(keep, w)
	}
	return strings.Join(keep, " ")
}

// mountNames names a drive's mount points by their last folder, "/" as
// root, three at most and a count of the rest.
func mountNames(mounts []string) string {
	var names []string
	for _, m := range mounts {
		n := m[strings.LastIndexByte(m, '/')+1:]
		if n == "" {
			n = "root"
		}
		names = append(names, n)
	}
	if len(names) > 4 {
		names = append(names[:3], fmt.Sprintf("%d more", len(names)-3))
	}
	return strings.Join(names, ", ")
}
