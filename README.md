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

## License

MIT. Geist is under the SIL Open Font License (`theme/fonts/OFL.txt`).
