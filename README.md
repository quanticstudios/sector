<p align="center"><img src="assets/logo.svg" width="96" alt="sector"></p>

# sector

A system monitor for machines that run many coding agents. Where btop shows
eleven rows named `claude`, sector shows which agent, in which repo and
worktree, is using the CPU and memory, and keeps command lines off the
screen.

It is a native Gio window, drawn in pixels with pitwall's design language.
The dashboard is also a Go package (`view`) with no window of its own, so
pitwall can embed it and show several machines at once.

Status: early. Linux only for now.

## Run

```sh
mise install
GOFLAGS=-tags=novulkan go run ./cmd/sector
```

## Packages

| Package      | Owns                                                           |
| ------------ | -------------------------------------------------------------- |
| `sample`     | CPU per thread, cores and dies, temps, memory, drives, network |
| `procs`      | Processes, grouped by agent, repo and app                      |
| `theme`      | Colors, type and spacing scales, the Geist font                |
| `view`       | The dashboard as a Gio widget, for any host window             |
| `cmd/sector` | The standalone window                                          |

## Package power

sector reads CPU package power from the RAPL energy counter in
`/sys/class/powercap/intel-rapl:*/energy_uj`, which AMD CPUs expose too.
Since the PLATYPUS fix (CVE-2020-8694) the kernel makes that file readable
by root only, so sector shows no power figure unless it runs as root. On an
AMD Ryzen 9 9950X with a stock kernel, no hwmon driver (`amd_energy`,
`zenpower`) offers a readable CPU power value either.

To show power without root, a udev rule can make the counters world
readable. This re-opens the PLATYPUS side channel: any local user can then
watch energy use closely enough to leak secrets from other processes,
including keys. Only do it on a machine where every local user is you.

```
# /etc/udev/rules.d/90-powercap-read.rules
SUBSYSTEM=="powercap", RUN+="/bin/chmod o+r /sys%p/energy_uj"
```

Then `sudo udevadm control --reload && sudo udevadm trigger -s powercap`, or
reboot. Delete the file and reboot to undo it.

## License

MIT. Geist is under the SIL Open Font License (`theme/fonts/OFL.txt`).
