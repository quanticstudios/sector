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
