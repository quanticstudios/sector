# Changelog

Releases are `v0.1.0`, `v0.1.1`, ...: each release bumps the patch number
and becomes the latest release on GitHub. A new minor, such as `v0.2.0`,
starts only when you pass its version. A release is a git tag on `main`;
`scripts/install.sh` stamps the binary with `git describe`, and
`sector --version` prints it.

Changes land with their notes under `## Unreleased` at the top. To cut a
release from an up-to-date, clean `main`, run `sh scripts/release.sh`. It
renames `## Unreleased` to the next patch of the newest release, such as
`## v0.1.1` after `v0.1.0`, or to the version you pass, and opens a
`Release v0.1.N` pull request. The first release has no tag to count from,
so it takes its version: `sh scripts/release.sh 0.1.0`. Once the pull
request merges, `git pull --ff-only` and run `sh scripts/release.sh --tag`.
It tags the release commit on `main` and pushes the tag once, which starts
the release build. Either step takes `--dry-run`.

## v0.1.7

- The title bar switches each card on and off, as btop's box toggles do:
  CPU, Cores, Mem, Disks, Net and Procs. The other cards close up around
  a hidden one, and the last card shown stays on. The switches show from
  960 dp wide.

## v0.1.6

- Each drive says how long it keeps programs waiting instead of "busy
  100%", which an NVMe drive reads under any load: "waits 0.4 ms", or
  "slow" or "jammed" with the wait and how many requests are queued. The
  graph tints the stretch where the drive was slow (yellow) or jammed
  (red), and while a drive is slow the Disks card names the process
  moving the most bytes.
- Processes report their disk reads and writes per second (`DiskBps`),
  and drives their average wait and queue depth (`Wait`, `Queue`).
- A graph leaves a gap where no samples came, such as while pitwall's
  Monitor tab was hidden, instead of drawing a straight line across it.

## v0.1.5

- The graphs show the last 2 minutes instead of the last 60 seconds.

## v0.1.4

- A graph's top axis number and its unit stay in the gutter, the unit on
  the line below, so they no longer run into the plot's labels such as
  "cached".

## v0.1.3

- The Cores card numbers cores 0 up. On an Intel hybrid CPU such as the
  i9-13900 it read "cores 0 to 47", the kernel's core ids, for 24 cores;
  it now reads "cores 0 to 23, 8 with two threads".

## v0.1.2

- The memory, disk and network graphs keep only as much room on the right
  as their axis numbers need, like the CPU graph, instead of a wide empty
  strip.

## v0.1.1

- `go get github.com/quanticstudios/sector` works: a test fixture's folder name had a colon, which Go's module zips refuse.

## v0.1.0

- CPU and memory grouped by the coding agent, repo and app that use them, so eleven `claude` processes show up as the agents and worktrees they are.
- Cores laid out by CCD with each die's temperature, beside overall CPU in percent.
- Memory and swap, drives with their subvolumes folded into one row, and network traffic.
- A process table under the groups. sector never shows, keeps or logs a process's command line.
- `sector -demo` shows a made-up busy machine, for screenshots or a first look.
- `curl -fsSL https://raw.githubusercontent.com/quanticstudios/sector/main/scripts/get.sh | sh` installs the latest release on Linux amd64 or arm64, with a launcher entry when there is a desktop, and names any missing libraries. `scripts/install.sh` builds and installs from a checkout. `sector --version` prints the version.
- `procs.Group.PID` is an agent group's own process, and `docs/pitwall.md` explains how pitwall embeds sector to show a Monitor tab per machine.
- Refreshes every 2 seconds by default, draws a taller network graph on wide windows, and keeps the clock, uptime and other live numbers in fixed-width digits so they no longer shift the title bar's controls or header text.
- A host that embeds the view can name groups after its own tabs (`SetLabels`), learn which group row was clicked (`Clicked`), dim the graphs under "No data since 20:41" while a machine is out of reach (`SetStale`) and leave sector's mark out of the title bar (`view.NewWith` with `Options{Embedded: true}`). The host chip's clock and uptime now come from the sampled machine, in its own time zone, instead of the local clock.
