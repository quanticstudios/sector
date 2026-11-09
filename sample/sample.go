// Package sample reads system-wide counters: CPU per thread with core and
// die layout, temperatures, memory, drives and network. Linux reads /proc
// and /sys; other systems get ErrUnsupported from Sample.
package sample

import "time"

// Snapshot is one reading of the whole machine. Rates cover the time since
// the previous Sample; the first Sample reports zero rates.
type Snapshot struct {
	Time   time.Time
	Host   Host
	CPU    CPU
	Mem    Mem
	Drives []Drive // physical drives, sorted by name
	Nets   []Net   // interfaces that are up, loopback and virtual ones left out
}

// Host names the machine and its load.
type Host struct {
	Name    string        // hostname
	Model   string        // CPU model, "AMD Ryzen 9 9950X 16-Core Processor"
	Threads int           // logical CPUs
	Uptime  time.Duration // since boot
	Load    [3]float64    // 1, 5 and 15 minute load average
}

// CPU is processor use since the previous Sample.
type CPU struct {
	Total   float64   // busy share of all threads, 0 to 1
	Threads []float64 // busy share per logical CPU, indexed by CPU number, 0 to 1
	Cores   []Core    // physical cores in die then core order
	Dies    []Die     // CCDs; one per package when the CPU reports no dies
	MHz     float64   // mean current clock, 0 when unknown
	PowerW  float64   // package power in watts from RAPL, 0 when unknown
}

// Core is one physical core and the logical CPUs (SMT siblings) on it.
type Core struct {
	ID      int   // core number within its package
	Die     int   // index into CPU.Dies
	Threads []int // logical CPU numbers, lowest first
}

// Die is one CCD (or package) with its temperature.
type Die struct {
	ID    int
	TempC float64 // 0 when unknown
}

// Mem is memory in bytes, named as free(1) and btop name them.
type Mem struct {
	Total, Used, Available, Cached, Free uint64
	SwapTotal, SwapUsed                  uint64
}

// Drive is one physical block device and the filesystems mounted from it.
type Drive struct {
	Name   string   // kernel name, "nvme0n1"
	Model  string   // /sys/block/<name>/device/model, may be empty
	Mounts []string // mount points from this drive, btrfs subvolumes folded in, "/" first
	// Size, Used and Free cover the filesystems on the drive, each counted
	// once however many subvolumes mount it. A volume over several drives
	// (LVM) lists its mounts on each and counts there by that drive's share
	// of the volume's underlying partition sizes.
	Size, Used, Free  uint64
	ReadBps, WriteBps float64 // bytes per second
	Busy              float64 // share of the interval with I/O in flight, 0 to 1
}

// Net is one network interface.
type Net struct {
	Name             string // "enp16s0"
	Addr             string // first IPv4 address, else the first IPv6 one, else ""
	RxBps, TxBps     float64
	RxTotal, TxTotal uint64 // bytes since boot
}

// Sampler keeps the previous counters, so each Sample reports rates.
// It is not safe for concurrent use.
type Sampler struct {
	sampler // per-OS state
}

// New returns a Sampler; its first Sample reports zero rates.
func New() *Sampler { return &Sampler{} }

// Sample reads every counter once.
func (s *Sampler) Sample() (Snapshot, error) { return s.sample() }
