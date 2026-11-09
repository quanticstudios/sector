# sector

A native (Gio, pixel-drawn) system monitor that groups processes by the
coding agent, repo or app they belong to. It ships standalone and as an
embeddable view for pitwall (github.com/quanticstudios/pitwall).

## Layout

| Package      | Owns                                                           |
| ------------ | -------------------------------------------------------------- |
| `sample`     | System counters from /proc and /sys, rates between samples     |
| `procs`      | Process scan, CPU rates, grouping by agent, repo, app          |
| `theme`      | aide-dark colors, scales, Geist                                |
| `view`       | The dashboard widget: `New`, `Push`, `Layout`                  |
| `cmd/sector` | The window and the sampling loop (orchestrator only)           |

Exported signatures are the contract other packages build against. Implement
them as written; if one is wrong, say so in your report instead of changing it.

## Rules

- Go 1.27.1 via `mise.toml`. Build with `GOFLAGS=-tags=novulkan`.
  `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass before
  every commit.
- Stdlib first. A new dependency needs a reason in the commit message.
- One runnable test per non-trivial piece of logic, stdlib `testing` only.
  /proc and /sys readers take a root path so tests run on a fake tree.
- sector never keeps, shows or logs a process's command line. It may read
  the first argv entries only to name an agent script, and drops them at once.
- In a shell, never list processes with their arguments and never dump the
  environment: the shell carries secrets. `pgrep -c -f <pattern>` is fine.
- GUI windows only through `agent-ws` (skill `agent-desktop`), never on the
  user's desktop. `agent-ws down <name>` when done.
- Commits are authored `Quantic Studios <info@quanticstudios.io>`, no
  trailers. Fictional names in fixtures (`acme-api`, `web-app`, user `dev`).
- Every change reaches `main` through a pull request.
