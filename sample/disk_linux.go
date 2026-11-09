package sample

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// blockIndex maps the kernel's block devices onto physical disks.
type blockIndex struct {
	disk  map[string]string             // disk or partition name to its physical disk
	split map[string]map[string]float64 // dm-N to the share of it on each physical disk
	dev   map[string]string             // "major:minor" to block device name
	dm    map[string]string             // device-mapper name ("cryptroot") to dm-N
}

// readBlockIndex walks /sys/block. A physical disk has a device link, which
// loop, ram, zram and dm devices lack; its partitions are subdirectories
// with a partition file. Device-mapper devices (LUKS, LVM) resolve through
// their slaves down to every partition or disk under them.
func readBlockIndex() blockIndex {
	idx := blockIndex{disk: map[string]string{}, split: map[string]map[string]float64{},
		dev: map[string]string{}, dm: map[string]string{}}
	root := filepath.Join(sysRoot, "block")
	ents, _ := os.ReadDir(root)
	var dms []string
	for _, e := range ents {
		name, dir := e.Name(), filepath.Join(root, e.Name())
		idx.dev[readStr(filepath.Join(dir, "dev"))] = name
		if dm := readStr(filepath.Join(dir, "dm/name")); dm != "" {
			idx.dm[dm] = name
			dms = append(dms, name)
			continue
		}
		if !exists(filepath.Join(dir, "device")) {
			continue
		}
		idx.disk[name] = name
		subs, _ := os.ReadDir(dir)
		for _, p := range subs {
			if exists(filepath.Join(dir, p.Name(), "partition")) {
				idx.disk[p.Name()] = name
				idx.dev[readStr(filepath.Join(dir, p.Name(), "dev"))] = p.Name()
			}
		}
	}
	for _, dm := range dms {
		w := map[string]float64{}
		idx.leafSizes(dm, 0, w)
		var total float64
		for _, v := range w {
			total += v
		}
		for d, v := range w {
			if total == 0 { // sizes unknown: split evenly
				w[d] = 1 / float64(len(w))
			} else {
				w[d] = v / total
			}
		}
		idx.split[dm] = w
	}
	return idx
}

// leafSizes follows a dm device's slaves down to partitions and disks and
// adds each one's size in sectors to w under its physical disk. The split
// goes by slave size, not by where LVM placed the volume's extents: that
// layout lives in the LVM metadata, which only root can read.
func (idx blockIndex) leafSizes(name string, depth int, w map[string]float64) {
	if d, ok := idx.disk[name]; ok {
		dir := filepath.Join(sysRoot, "block", d)
		if name != d {
			dir = filepath.Join(dir, name)
		}
		n, _ := readInt(filepath.Join(dir, "size"))
		w[d] += float64(n)
		return
	}
	if depth > 8 {
		return
	}
	slaves, _ := os.ReadDir(filepath.Join(sysRoot, "block", name, "slaves"))
	for _, s := range slaves {
		idx.leafSizes(s.Name(), depth+1, w)
	}
}

// on lists the physical disks a block device sits on, with the share of it
// on each.
func (idx blockIndex) on(dev string) map[string]float64 {
	if d, ok := idx.disk[dev]; ok {
		return map[string]float64{d: 1}
	}
	return idx.split[dev]
}

// device names the block device a mount comes from. btrfs reports an
// anonymous major:minor (0:N), so the mount source names it instead.
func (idx blockIndex) device(m mount) string {
	if n, ok := idx.dev[m.dev]; ok {
		return n
	}
	if n, ok := strings.CutPrefix(m.source, "/dev/mapper/"); ok {
		return idx.dm[n]
	}
	if !strings.HasPrefix(m.source, "/dev/") {
		return "" // proc, tmpfs, overlay and other pseudo filesystems
	}
	if p, err := filepath.EvalSymlinks(m.source); err == nil { // /dev/disk/by-uuid/...
		return filepath.Base(p)
	}
	return filepath.Base(m.source)
}

type mount struct{ dev, point, source string }

// parseMountinfo reads /proc/self/mountinfo: the major:minor, the mount
// point and the source after the "-" separator.
func parseMountinfo(s string) []mount {
	var out []mount
	for line := range strings.Lines(s) {
		f := strings.Fields(line)
		sep := slices.Index(f, "-")
		if sep < 6 || len(f) < sep+3 {
			continue
		}
		out = append(out, mount{dev: f[2], point: unescapeMount(f[4]), source: f[sep+2]})
	}
	return out
}

// unescapeMount undoes the kernel's octal escapes (\040 for a space).
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// fsShare is one filesystem on a disk: a mount point to measure it at and
// the share of it on that disk.
type fsShare struct {
	point string
	share float64
}

// foldMounts groups mount points by physical disk, sorted so "/" comes
// first. A volume over several disks (LVM) lists its points on each. fs
// lists each filesystem once per disk, so btrfs subvolumes and bind mounts
// of one filesystem are measured once.
func foldMounts(ms []mount, idx blockIndex) (points map[string][]string, fs map[string][]fsShare) {
	points, fs = map[string][]string{}, map[string][]fsShare{}
	counted := map[string]bool{}
	for _, m := range ms {
		first := !counted[m.dev]
		counted[m.dev] = true
		for disk, share := range idx.on(idx.device(m)) {
			points[disk] = append(points[disk], m.point)
			if first {
				fs[disk] = append(fs[disk], fsShare{m.point, share})
			}
		}
	}
	for d, p := range points {
		slices.Sort(p) // every point starts with "/", so "/" sorts first
		points[d] = slices.Compact(p)
	}
	return points, fs
}

// statfs measures the filesystem mounted at path. Tests replace it.
var statfs = func(path string) (size, used, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, 0, err
	}
	bs := uint64(st.Frsize)
	if bs == 0 {
		bs = uint64(st.Bsize)
	}
	return st.Blocks * bs, (st.Blocks - st.Bfree) * bs, st.Bavail * bs, nil
}

type ioCounters struct{ read, written, ticks uint64 } // sectors, sectors, ms

// parseDiskstats reads sectors read and written and the ms with I/O in
// flight per device from /proc/diskstats.
func parseDiskstats(s string) map[string]ioCounters {
	out := map[string]ioCounters{}
	for line := range strings.Lines(s) {
		f := strings.Fields(line)
		if len(f) < 13 {
			continue
		}
		var c ioCounters
		c.read, _ = strconv.ParseUint(f[5], 10, 64)
		c.written, _ = strconv.ParseUint(f[9], 10, 64)
		c.ticks, _ = strconv.ParseUint(f[12], 10, 64)
		out[f[2]] = c
	}
	return out
}

func (s *sampler) readDrives(dt float64) ([]Drive, error) {
	idx := readBlockIndex()
	mi, err1 := os.ReadFile(filepath.Join(procRoot, "self/mountinfo"))
	ds, err2 := os.ReadFile(filepath.Join(procRoot, "diskstats"))
	points, fs := foldMounts(parseMountinfo(string(mi)), idx)
	cur := parseDiskstats(string(ds))

	var drives []Drive
	for name, disk := range idx.disk {
		if name != disk {
			continue
		}
		d := Drive{
			Name:   name,
			Model:  readStr(filepath.Join(sysRoot, "block", name, "device/model")),
			Mounts: points[name],
		}
		for _, f := range fs[name] {
			if size, used, free, err := statfs(f.point); err == nil {
				part := func(n uint64) uint64 { return uint64(float64(n) * f.share) }
				d.Size, d.Used, d.Free = d.Size+part(size), d.Used+part(used), d.Free+part(free)
			}
		}
		c := cur[name]
		if p, ok := s.disks[name]; ok && dt > 0 {
			d.ReadBps = rate(p.read, c.read, dt) * 512
			d.WriteBps = rate(p.written, c.written, dt) * 512
			d.Busy = min(rate(p.ticks, c.ticks, dt)/1000, 1)
		}
		drives = append(drives, d)
	}
	slices.SortFunc(drives, func(a, b Drive) int { return strings.Compare(a.Name, b.Name) })
	s.disks = cur
	return drives, errors.Join(err1, err2)
}
