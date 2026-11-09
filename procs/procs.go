// Package procs lists processes and groups them by the coding agent, repo
// or app they belong to, so a reader sees "which agent is eating the
// machine" instead of a wall of pids.
//
// It never keeps, returns or logs a command line: arguments carry secrets.
package procs

// Proc is one process.
type Proc struct {
	PID, PPID int
	Comm      string  // /proc/<pid>/comm
	State     byte    // 'R', 'S', 'D', 'Z' ...
	Cores     float64 // threads' worth of CPU since the previous Scan: 1.5 is one and a half threads busy
	RSS       uint64  // resident bytes
	Threads   int
}

// Kind is what a group is built around.
type Kind uint8

const (
	Other  Kind = iota // "Everything else": what fits no agent or app
	Agent              // one coding agent session and everything under it
	App                // a desktop app's process tree (Chromium, Slack)
	System             // kernel threads and system services
)

// Group is a set of processes that belong to one thing.
type Group struct {
	Key     string // stable across scans: same agent session, same app, same key
	Kind    Kind
	Name    string // "claude", "Chromium", "Everything else"
	Agent   string // the agent's name for Kind Agent: "claude", "codex"
	Repo    string // the repo folder an agent runs in, "" when unknown
	Branch  string // the worktree branch, "" outside a linked worktree
	Cores   float64
	RSS     uint64
	Threads int    // threads in the group
	Procs   []Proc // members, busiest first
}

// Table is one scan, grouped.
type Table struct {
	Groups  []Group // busiest first; Other always last
	Procs   int     // processes seen
	Threads int     // threads seen
}

// Scanner keeps each process's previous CPU time, so a Scan reports rates.
// It is not safe for concurrent use.
type Scanner struct {
	scanner // per-OS state
}

// NewScanner returns a Scanner; its first Scan reports zero CPU.
func NewScanner() *Scanner { return &Scanner{} }

// Scan reads every process once and groups them.
func (s *Scanner) Scan() (Table, error) { return s.scan() }
