# Embedding sector in pitwall

This guide is for a coding agent working in the pitwall repository
(github.com/quanticstudios/pitwall). It explains how to add a Monitor tab per
machine: one for the desktop pitwall runs on, and one for each remote host
the window already reaches, such as `box` from `pitwall --host box` or a
`[[hosts]]` entry with `window = true`. Each machine's own pitwall daemon
samples that machine. The window draws every machine with sector's `view`
package.

Paths below are relative to the pitwall repository unless they start with a
sector package name. Every pitwall file, type and function named here
existed when this guide was written. Check each one again before you build
on it, and if one moved, follow the code, not this guide.

## What sector gives you

| Package                                   | Where pitwall uses it | What it does                                                                                                                                                                                                                                     |
| ----------------------------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `github.com/quanticstudios/sector/sample` | daemon                | `sample.New()` returns a `*Sampler`. Each `Sample()` returns a `Snapshot`: CPU per thread with core and die layout, temperatures, memory, drives, network. Rates cover the time since the previous call, so the first one reports zero rates.    |
| `github.com/quanticstudios/sector/procs`  | daemon                | `procs.NewScanner()` returns a `*Scanner`. Each `Scan()` returns a `Table`: processes grouped by coding agent, repo and app, busiest first, "Everything else" last.                                                                              |
| `github.com/quanticstudios/sector/view`   | GUI                   | `view.NewWith(th, view.Options{Embedded: true})` returns a `*View`, a Gio widget with no window of its own. The host calls `Push(snapshot, table)` for each reading and `Layout(gtx)` each frame. `Interval()` and `Paused()` report what the user picked in the view's title bar. `SetLabels`, `Clicked` and `SetStale` are described under "The tab". |
| `github.com/quanticstudios/sector/theme`  | GUI                   | `theme.Theme`, the palette and shaper the view draws with.                                                                                                                                                                                       |

`sample` and `procs` read Linux's `/proc` and `/sys`. On other systems
`Sample` and `Scan` return `errors.ErrUnsupported`. Neither type is safe for
concurrent use, and each keeps the previous reading to compute rates, so a
daemon keeps one `Sampler` and one `Scanner` and calls them from one
goroutine.

`sample.Snapshot` and `procs.Table` are plain exported structs that round-trip
through `encoding/gob` unchanged. sector tests that in
`sample/wire_linux_test.go` and `procs/wire_linux_test.go`. The only loss is
`Snapshot.Time`'s monotonic clock reading, which gob drops. They are the
payload pitwall sends over its socket.

`procs.Group.PID` is the agent process's pid for a group of `Kind`
`procs.Agent`, and 0 for every other group. It exists so the daemon can tell
which pane runs that agent. See "Agent groups and tabs".

## go.mod

1. Require a tagged sector release, never a branch or a pseudo-version:
   `mise exec -- go get github.com/quanticstudios/sector@v0.N.M`. Ask the
   user which tag to use. If sector has no tag yet, stop and ask for one.
2. Keep `replace gioui.org => ./third_party/gioui.org` in pitwall's `go.mod`.
   sector requires `gioui.org v0.10.3`, the release pitwall vendors (see
   `third_party/gioui.org/PATCH.md`), and a `replace` in the main module
   applies to every dependency, so sector's `view` builds against pitwall's
   patched copy. The patch adds `FileDropEvent` and changes Wayland
   presentation and key mapping. It leaves the `layout`, `widget`, `op`,
   `paint`, `clip`, `text` and `font` APIs the view uses unchanged.
3. Check API compatibility before you write any other code. sector was
   built against pitwall's vendored Gio with this command, run from a sector
   checkout, and it passed:

   ```sh
   cp go.mod /tmp/gio.mod && cp go.sum /tmp/gio.sum
   echo 'replace gioui.org => <pitwall checkout>/third_party/gioui.org' >> /tmp/gio.mod
   GOFLAGS=-tags=novulkan go build -modfile=/tmp/gio.mod ./...
   ```

   In pitwall, `mise exec -- go build ./...` after the `go get` is the same
   check. If it fails, report the failing symbol. Don't patch
   `third_party/gioui.org` to make sector build.
4. The two modules share `github.com/go-text/typesetting v0.3.5`. Go picks
   the higher version of every other shared module, which is pitwall's for
   `golang.org/x/image`, `golang.org/x/sys` and `godbus/dbus`.
5. sector is MIT. Add it to `THIRD_PARTY_NOTICES.md`, as CONTRIBUTING.md asks
   for a new dependency, and give the reason in the commit message. sector's
   `theme` also embeds Geist, which pitwall already bundles under the same
   OFL.

## Protocol

Read the long comment above `Version` in `internal/proto/proto.go` first.
It decides what kind of change this is, and it says this one is additive. A
new message type appended to `Messages`, which a peer that never sends it
can ignore, bumps `Level`, not `Version`. Bumping `Version` would make every
running daemon refuse new windows and force a daemon restart, which stops
every program in every pane. Don't bump `Version` for the monitor.

Add, in `internal/proto/proto.go`:

```go
// MonitorLevel added MonitorWatch, MonitorSample, NewSession.Monitor and
// model.Workspace.Monitor: a tab that shows its machine's CPU, memory,
// drives, network and processes, drawn with sector's view.
const MonitorLevel = 69

// MonitorWatch asks for MonitorSample every Interval while the Monitor tab
// WorkspaceID is on screen. Interval 0 stops this client's watch, for a
// paused view or a tab that left the screen.
type MonitorWatch struct {
	WorkspaceID string
	Interval    time.Duration
}

// MonitorSample is one reading of the daemon's machine. Err is set, and
// the reading empty, where sector cannot sample, such as on macOS. Tabs
// maps the Key of each procs.Agent group to the pane its agent runs in.
type MonitorSample struct {
	Snapshot sample.Snapshot
	Table    procs.Table
	Tabs     map[string]string
	Err      string
}
```

Then:

1. Set `Level = 69` and add a `Level 69 added ...` line on top of the list
   in the `Version` comment.
2. Append `MonitorWatch{}, MonitorSample{}` to `Messages`. `init` in
   `internal/proto/conn.go` registers them with gob.
3. Return `MonitorLevel` for both from `Since`. `backend.Send` in
   `cmd/pitwall/backend.go` then refuses to send `MonitorWatch` to an older
   daemon with `errOlderDaemon`, and `TestSinceWithinLevel` stays green.
4. Run `mise exec -- go test ./internal/proto -run TestWireFingerprint
   -update` and commit `internal/proto/testdata/wire.txt`.
   `wireLayout` in `internal/proto/fingerprint_test.go` walks into every
   struct a message reaches, so the file now lists `sample.Snapshot`,
   `procs.Table` and every struct inside them.

That last point is a standing rule. Every later sector upgrade that adds a
field to those structs makes `TestWireFingerprint` fail until you bump
`Level` and record it again. If a sector upgrade removes, renames or retypes
one of those fields, the test reports a breaking change. Don't take that
sector release. Ask the sector maintainers to restore the field. sector's
doc comments on `Snapshot` and `Table` promise to treat such a change as
breaking for embedders.

`MonitorSample` carries a whole `procs.Table`, members included. One reading
of a desktop with 1,097 processes in 35 groups, Snapshot and Table
together, encoded to 39 KB, far under `maxFrameSize` (64 MiB) in
`internal/proto/conn.go`. At one sample a second that is negligible on the
unix socket and fine over ssh.

## Daemon side

The daemon samples its own machine. A daemon on box samples box, never the
desktop. The window never reads `/proc` itself.

### One sampler per daemon

Add `internal/daemon/monitor.go`:

- A `monitor` struct on `Daemon`, guarded by its own mutex: the watching
  clients and each one's interval, as `map[*client]time.Duration`, and
  whether the loop runs.
- `MonitorWatch` handling in `serveConn` (`internal/daemon/daemon.go`), next
  to the `proto.View` case. Accept it from `hello.Kind == "gui"` clients
  whose `hello.Level >= proto.MonitorLevel` only. Interval 0 deletes the
  client. Any other value sets it, clamped to a floor of 500 ms.
- Remove the client from the watchers in the `defer` that already deletes
  it from `d.clients`, so a closed window stops counting.
- The loop starts when the first watcher arrives and returns when the last
  one leaves. It samples at the shortest interval any watcher asked for,
  and checks again after each sample, so a change applies within one tick.
  With no watchers, no goroutine runs and nothing reads `/proc`. Start it
  from the `Serve` context, `d.served`, so it ends with the daemon.
- Keep one `*sample.Sampler` and one `*procs.Scanner` for the daemon's
  life, not per run of the loop. The first reading after a pause then
  reports average rates over the pause, which is better than zeros.
- Sample outside `d.mu`. A scan reads every `/proc/<pid>` and takes
  milliseconds. Take `d.mu` only to copy what the tab labels need (see
  below), then build the message and queue it.
- Send each sample to every watcher. Add a slot for the latest sample on
  `client` next to `frames`, filled by `push` and drained by `writeLoop`,
  so a slow window gets the newest reading and never a backlog. Don't use
  `queue`: 1,024 queued samples (`maxQueued`) would disconnect the window.
- A sampler error goes into `MonitorSample.Err` once per watcher. Log it
  once a minute at most with a `logs.Limiter`, as `refusals` does.

Make the readers a daemon option so tests never read the real machine.
`Options` in `internal/daemon/daemon.go` exists for this: "New wires the
real packages; tests pass fakes to NewWith". For example:

```go
// Monitor reads the machine for Monitor tabs; nil means sector's sample
// and procs.
Monitor func() (sample.Snapshot, procs.Table, error)
```

`Defaults()` sets it from a `sample.Sampler` and a `procs.Scanner`.

### Agent groups and tabs

A pane's first process leads its own session, and every process started in
the pane shares that session id. `internal/daemon/detect.go` keeps the
session id per pane as `detected.sid` in `d.live.det`, read with
`procSession` from `internal/daemon/proc_linux.go` (field 6 of
`/proc/<pid>/stat`).

For each group with `Kind == procs.Agent`, read `procSession(g.PID)` and
look for a pane whose `d.live.det[pane].sid` matches. Put each match in
`MonitorSample.Tabs[g.Key] = paneID`. Copy the sids under `d.mu`, then read
`/proc` outside it. An agent started outside pitwall matches no pane and
gets no entry. `procSession` returns 0 on systems other than Linux and
macOS, so nothing matches there.

Send pane ids, not tab names. The window resolves them against its live
state on every frame: `model.Pane.WorkspaceID`, then that workspace's
`Label`. A renamed tab then shows its new name at once, and in a window of
several machines the pane id resolves through `multi`'s merged state
(`cmd/pitwall/multi.go`).

### Monitor tabs in the state

A Monitor tab is a workspace with no panes, like a chat tab.

- Add `Monitor bool` to `model.Workspace` (`internal/model/model.go`) and
  to `proto.NewSession`. Both are additive and belong to Level 69.
- `newSession` (`internal/daemon/sessions.go`) with `Monitor` set opens a
  tab with no pane. If the session already has a Monitor tab on this
  daemon, it focuses that tab instead, so each machine has at most one per
  session.
- `forLevel` (`internal/daemon/chats.go`) hides chat tabs from windows below
  `ChatLevel`. Extend it to hide Monitor tabs, and their ids in
  `Session.Order`, from windows below `MonitorLevel`. As written it returns
  early for any level at or above `ChatLevel`, so restructure that check.
- `Since(proto.NewSession{Monitor: true})` must return `MonitorLevel`, so
  an older daemon is never asked for a tab it cannot make.
- A Monitor tab survives a daemon restart like any tab. It holds no
  process, so a restore has nothing to relaunch.

## GUI side

### The tab

`layoutPanes` in `internal/ui/app/app.go` draws a chat tab with
`u.layoutChat(gtx, st, ws)` when `ws.Chat != ""`. Add the same branch for
`ws.Monitor` and put the drawing in a new `internal/ui/app/monitor.go`,
modeled on `internal/ui/app/chat.go`:

- An optional backend interface, as `Chatter` is:

  ```go
  // Monitorer is optionally implemented by a Backend: the latest
  // MonitorSample from the daemon that owns workspace ws.
  type Monitorer interface {
  	Monitor(ws string) (proto.MonitorSample, bool)
  }
  ```

  `backend` in `cmd/pitwall/backend.go` keeps the last `MonitorSample` in
  `recvLoop`, as it keeps `ChatDeltas`. `multi` in `cmd/pitwall/multi.go`
  answers from the backend of the host that owns `ws`, through `m.of(ws)`.
  Add both to the `var _ app.Chatter = ...` blocks.
- One `*view.View` per Monitor workspace id, kept on `ui` in a map like
  `panes`, made on first draw and dropped when the workspace is gone from
  the state. Each view keeps its own 120-sample history, so the desktop and
  box never mix.
- On each frame, if the backend's sample is newer than the last one pushed,
  call `v.Push(s.Snapshot, s.Table)`. Push each sample once, or the graphs
  repeat it.
- Call `v.Layout(gtx)` with `gtx.Constraints` set to exactly the tab's
  content rect, under an `op.Offset` to its corner. The view fills and
  clips that rect itself and paints its own background. It asks for a
  redraw each second with `op.InvalidateCmd`, which keeps its clock
  current.
- Without a `Monitorer`, or with an `Err`, draw a centered line with `para`,
  as `layoutChat` does: "Monitor needs Linux on box", or the error text.

Make each view with `view.NewWith(st, view.Options{Embedded: true})`.
`Embedded` leaves sector's mark and name out of the title bar, so the bar
starts with the host chip: the machine's name, CPU model, threads, memory,
its own clock and uptime. `view.New(th)` is the standalone window's and
draws the mark.

Call every `View` method from the window's goroutine, the one that calls
`Layout`. Only `Interval` and `Paused` are safe from another goroutine.

- `v.SetLabels(m)` names groups by `Group.Key`. A label replaces the
  group's "repo, agent" in the CPU legend and the hover tooltip, and its
  bold name in the process table, where the branch and the agent stay
  beside it. The filter matches labels too. Build `m` each frame from
  `MonitorSample.Tabs`: for each group key, the pane's `WorkspaceID`, then
  that workspace's `Label`. The view copies `m`, so pass a new map when a
  tab is renamed or an agent comes or goes. `SetLabels(nil)` clears them.
- `v.Clicked()` returns the `Group.Key` of the group row the user clicked
  in the process table since the last call, once per click. Call it after
  `Layout`. The click also opens or closes the row, as in sector's own
  window. When the key is in `Tabs`, focus that tab with
  `u.queueFocus(proto.FocusSession{WorkspaceID: ..., Pane: paneID})`
  (`internal/ui/app/app.go`). A click on any other row needs nothing.
- `v.SetStale(since)` dims every graph and writes "No data since 20:41"
  over the CPU graph, with `since` shown on the sampled machine's clock.
  The graphs already stand still without a `Push`, since they end at the
  last sample. The next `Push` clears it, and so does `SetStale(time.Time{})`.
  See "Remote machines" for when to call it.

### Interval and pause

The view's title bar has an interval control (1, 2 or 5 s) and a pause
button. It stores them in atomics and has no callback. After each `Layout`,
compare `v.Interval()` and `v.Paused()` with what you last sent for that
tab. On a change, send `proto.MonitorWatch{WorkspaceID: ws.ID, Interval:
iv}` through `u.send`, with 0 for paused. `multi.route` sends a message to
the owner of the first id among its fields, so `WorkspaceID` takes it to
the right daemon.

Also send:

- A watch when the tab comes on screen, and `Interval: 0` when it leaves.
  A tab that is not drawn costs its daemon nothing.
- The watch again whenever `Link.Epoch` changes (`internal/ui/app/link.go`).
  A new connection, after a daemon restart or a dropped ssh link, starts
  with no watchers. `attach` in `cmd/pitwall/backend.go` resets `b.view` for
  the same reason.

### Theme

Build sector's theme from pitwall's instead of calling `theme.Dark()`,
which loads its own fonts:

```go
st := &sectortheme.Theme{
	Shaper: th.Shaper, Font: th.UIFont, Mono: th.MonoFont,
	Bg: th.Bg, Surface: th.Surface, SurfaceSecondary: th.SurfaceSecondary,
	SurfaceElevated: th.SurfaceElevated, Border: th.Border, Fg: th.Fg,
	Muted: th.Muted, Primary: th.Primary, Red: th.Red, Yellow: th.Yellow,
	Green: th.Green, Blue: th.Blue, Purple: th.Purple,
	MemUsed: sectortheme.Hex("#a184f0"), MemCache: sectortheme.Hex("#4a3d78"),
}
st.BorderSubtle = sectortheme.Mix(st.Surface, st.Fg, 0.07)
st.TextQuiet = sectortheme.Mix(st.Bg, st.Muted, 0.85)
```

`th` is pitwall's `*theme.Theme` from `internal/ui/theme/theme.go`. Rebuild
the views when the user's theme changes. The view's type sizes are fixed in
sp and do not follow pitwall's `TextSize`.

### Opening a Monitor tab

Add an entry to the palette and to a machine line's menu: "Monitor this
machine" and "Monitor box". It sends `proto.NewSession{SessionID: ...,
Monitor: true}`, wrapped in `app.NewTabOn{Host: "box", Tab: ...}` for a
host, as the sidebar's + does. Hide the entry for a host whose `Link.Level`
is below `proto.MonitorLevel`. If it is clicked anyway, `backend.Send`
returns `errOlderDaemon`. Show "box runs an older pitwall; update it there
to monitor it" in the notice through `app.Refusal`.

### Keys

The view has a filter field (a Gio `widget.Editor`). Once clicked, it takes
typed keys. pitwall's own bindings must still win for the keys that switch
tabs, or the user is stuck in the field. A Monitor tab has no pane, so no
`proto.Input` may go anywhere while it is focused.

## Remote machines

- box's daemon samples box. The window receives box's samples over the
  same forwarded socket and ssh connection as box's state and frames
  (`sshHost` in `cmd/pitwall/host.go`). Nothing new is opened, and
  nothing about the desktop is sent to box.
- Version skew. The desktop and box each build their own pitwall, and so
  their own sector. Within one `proto.Version`, gob leaves a field the
  sender lacks at its zero value and skips one the receiver lacks. So a
  newer desktop draws an older box's sample with zeros in the new fields.
  A box below Level 69 never gets a `MonitorWatch`; the GUI refuses to
  send it, as above. A breaking change in sector's structs would show as a
  breaking wire change in `TestWireFingerprint`, which is why a sector
  release that removes a field must not be taken.
- Unreachable box. `multi` keeps a down host's last state on screen and
  retries with backoff (`docs/remote.md`). Keep box's view with its last
  sample. Note the local `time.Now()` each time a `MonitorSample` arrives,
  and call `v.SetStale(thatTime)` once the host's `Link.State` is no
  longer `app.LinkUp`, or no sample has come for three intervals while the
  view is not paused. The sidebar's machine line already says "offline,
  retry in 7s". On reconnect the `Epoch` changes, the window sends its
  watch again, and the first `Push` clears the stale state. The gap shows
  as a straight line in the graphs. Never block the desktop's tab on box.
- Clocks and time zones. The host chip shows the sampled machine's own
  clock and uptime. `sample` stamps `Snapshot.Time` with `time.Now()` in
  that machine's local zone. gob keeps the time's UTC offset, though not
  the zone's name, so box's sample arrives in box's offset. The view shows
  `Snapshot.Time` in that offset, run on by the desktop's clock since the
  `Push`, and adds the same time to `Host.Uptime`. A desktop in Berlin
  watching box in Tokyo shows Tokyo's time on box's tab. A clock that is
  off on box shows as off, which is what box's own `date` says. A daylight
  saving change on box shows from the first sample after it. A host that
  pushes a UTC time shows UTC. The graphs only compare samples from one
  machine, so skew between the machines does not bend them.
- Ask the user before any live check against box. It runs a daemon on a
  real server and loads agents' machines.

## Security

- sector never keeps, returns or logs a command line. `procs` reads at most
  4,096 bytes of a process's argv (`argsRead`), and only for an interpreter
  such as node or python, to name the agent script it runs. It drops the
  argv right after. A `procs.Proc` holds `Comm` (`/proc/<pid>/comm`, 15
  bytes at most) and no argument. pitwall's own `readArgs` in
  `internal/daemon/proc_linux.go` follows the same rule.
- Keep it that way across the wire. `MonitorSample` carries only what
  `Snapshot` and `Table` hold, plus pane ids. Don't add command lines,
  environment variables or cwds to it. When `wire.txt` changes in review,
  read every new field of `sample.*` and `procs.*`, and refuse one that
  could hold argv, environment or file contents.
- `Group.Repo` and `Group.Branch` are a folder's base name and a branch
  name. pitwall already sends both for its tabs (`model.Workspace.Branch`).
- Never log a `MonitorSample` or a `Table`. Log an error, not the reading.
- Leave Monitor tabs out of the web protocol (`internal/web`,
  `docs/web-protocol.md`) and the phone page (`internal/remote`) until
  someone decides what a paired phone may see. Process names and repos are
  more than the phone page shows today.

## Testing

- Build and test as CONTRIBUTING.md says, with `mise exec --` so
  `GOFLAGS=-tags=novulkan` applies. Test packages call `testenv.Main`
  from their `TestMain` (`internal/daemon/main_test.go`,
  `cmd/pitwall/main_test.go`), and a new test package must too. It
  (`internal/testenv/testenv.go`) points
  `XDG_RUNTIME_DIR`, `XDG_STATE_HOME` and `XDG_CONFIG_HOME` at a temporary
  folder and clears `PITWALL_SOCKET`. Never dial, start or restart the
  user's running daemon from a test or by hand.
- For a manual run, start a throwaway daemon and window with the same
  three variables pointing at a new temporary folder, so it gets its own
  socket and state.
- Daemon tests use the `Monitor` option, never the real machine. Check
  that nothing samples without a watcher, that the first watch starts the
  loop, that the shortest interval wins, that Interval 0 and a disconnect
  stop it, that a client below Level 69 never gets a `MonitorSample`, that
  a slow client gets the newest sample and is not disconnected, and that
  `Tabs` maps an agent group to its pane with a fake `procSession` (as
  `sessionOf` is swapped in `detect.go`'s tests).
- Proto: `TestWireFingerprint`, `TestWireVerdict` and `TestSinceWithinLevel`
  in `internal/proto/fingerprint_test.go`. A test with
  `testenv.FromFuture` (`internal/testenv/future.go`) shows an older peer
  survives the new messages.
- GUI: give the fake backend in `internal/ui/app/fake.go` a `Monitorer`
  with a few fixed samples, so `go run ./cmd/pwdemo-app` shows a Monitor
  tab. Run every window inside `agent-ws` (skill `agent-desktop`), never on
  the user's desktop, and run `agent-ws down <name>` when done. Gio repaints
  hover only after the second pointer move, so move twice before a hover
  screenshot. Attach screenshots of the desktop tab, a box tab, a box tab
  offline and an older box to the pull request.
- Live check against box: only with the user's go-ahead, and after the
  release that carries the change is installed on box.

## Pull requests, in order

Each one goes to `main` through its own pull request, from a branch off
current `main`, and passes the commands in CONTRIBUTING.md. A user-visible
change adds a line under `## Unreleased` in `CHANGELOG.md`. A change users
will not notice gets the `no-changelog` label from a maintainer instead.

1. **Protocol and sampler.** Add sector to `go.mod`, plus `MonitorLevel`,
   `MonitorWatch`, `MonitorSample`, `Since`, `wire.txt`,
   `internal/daemon/monitor.go`, the `Monitor` option, the client slot and
   `Tabs`. Nothing draws it yet, so ask for `no-changelog`. Done when the
   proto and daemon tests above pass, `go build ./...` passes with the
   vendored Gio, and a throwaway daemon with a test client prints samples
   at the asked interval and stops sampling when the client leaves.
2. **Monitor tabs in the state.** Add `model.Workspace.Monitor`,
   `NewSession.Monitor`, `newSession`'s open-or-focus, `forLevel`, and the
   sidebar row's icon and name ("Monitor"). Done when a daemon test opens
   one Monitor tab per session, a second request focuses it, a Level 68
   client never sees it, and it survives a daemon restart.
3. **The Monitor tab in the window.** `internal/ui/app/monitor.go`,
   `Monitorer` on `backend` and `multi`, the theme bridge, the watch on
   show, hide, interval, pause and `Epoch` change, `SetStale` while the
   host is down, the older-daemon notice, and the palette and menu entries. Add a CHANGELOG
   line and a section in `docs/usage.md`, with a line in `docs/remote.md`.
   Done when `pwdemo-app` shows the tab in `agent-ws`, a throwaway daemon's
   tab shows live data, switching away stops sampling (no sampler
   goroutine in the daemon's log or stack dump), and the screenshots are
   attached.
4. **Agent labels.** Needs a sector release with `SetLabels` and
   `Clicked`, v0.1.0 or later. Pass `Tabs`, resolved to tab labels, to
   `SetLabels`, and focus the tab `Clicked` names. Done when an agent started in a
   pane shows its tab's label in the process table and the CPU graph's
   legend, a click on that row focuses the tab, and an agent started
   outside pitwall keeps its repo and agent name.
5. **Remote check.** With the user's go-ahead, after a release is on box:
   open box's Monitor tab from the desktop. Compare its thread count and
   memory with `ssh box nproc` and `ssh box free -b`. Drop the link
   (`ssh -O exit` on the control socket) and watch "No data since" and the
   recovery. Report the result in the pull request of step 4 or in an issue.
