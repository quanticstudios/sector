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
	disk map[string]string // disk, partition or dm-N name to its physical disk
	dev  map[string]string // "major:minor" to block device name
	dm   map[string]string // device-mapper name ("cryptroot") to dm-N
}

// readBlockIndex walks /sys/block. A physical disk has a device link, which
// loop, ram, zram and dm devices lack; its partitions are subdirectories
// with a partition file. Device-mapper devices (LUKS, LVM) resolve through
// their slaves down to a partition or disk.
func readBlockIndex() blockIndex {
	idx := blockIndex{disk: map[string]string{}, dev: map[string]string{}, dm: map[string]string{}}
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
		if d := idx.slaveDisk(dm, 0); d != "" {
			idx.disk[dm] = d
		}
	}
	return idx
}

// slaveDisk follows a dm device's slaves to the first physical disk.
func (idx blockIndex) slaveDisk(name string, depth int) string {
	if d, ok := idx.disk[name]; ok || depth > 8 {
		return d
	}
	slaves, _ := os.ReadDir(filepath.Join(sysRoot, "block", name, "slaves"))
	for _, s := range slaves {
		if d := idx.slaveDisk(s.Name(), depth+1); d != "" {
			return d
		}
	}
	return ""
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

// foldMounts groups mount points by physical disk, sorted so "/" comes
// first. fs lists one mount point per filesystem on each disk, so btrfs
// subvolumes of one filesystem are measured once.
func foldMounts(ms []mount, idx blockIndex) (points, fs map[string][]string) {
	points, fs = map[string][]string{}, map[string][]string{}
	counted := map[string]bool{}
	for _, m := range ms {
		disk := idx.disk[idx.device(m)]
		if disk == "" {
			continue
		}
		points[disk] = append(points[disk], m.point)
		if !counted[m.dev] {
			counted[m.dev] = true
			fs[disk] = append(fs[disk], m.point)
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
		for _, p := range fs[name] {
			if size, used, free, err := statfs(p); err == nil {
				d.Size, d.Used, d.Free = d.Size+size, d.Used+used, d.Free+free
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
