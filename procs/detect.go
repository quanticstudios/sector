package procs

import (
	"path/filepath"
	"strings"
)

// Adapted from pitwall: internal/agent/detect.go

// programs names each agent by the program it runs as.
var programs = map[string]string{
	"claude":       "claude",
	"codex":        "codex",
	"pi":           "pi",
	"omp":          "omp",
	"gemini":       "gemini",
	"opencode":     "opencode",
	"cursor-agent": "cursor",
	"amp":          "amp",
	"aider":        "aider",
}

// packages names the agents whose npm package an interpreter runs a
// script from, by a directory on the script's path.
var packages = map[string]string{
	"/@anthropic-ai/claude-code/": "claude",
	"/@openai/codex/":             "codex",
	"/@google/gemini-cli/":        "gemini",
	"/opencode-ai/":               "opencode",
	"/cursor-agent/":              "cursor",
	"/@sourcegraph/amp/":          "amp",
	"/@ampcode/cli/":              "amp",
}

// identify names the agent a process is from its comm and the path of its
// executable, or returns "".
func identify(comm, exe string) string {
	for _, s := range []string{comm, strings.TrimSuffix(filepath.Base(exe), ".exe")} {
		if p := programs[s]; p != "" {
			return p
		}
	}
	return ""
}

// interpreter reports whether a process runs a script that script can
// name the agent by: node, bun, deno or python.
func interpreter(comm, exe string) bool {
	for _, s := range []string{comm, filepath.Base(exe)} {
		switch s {
		case "node", "nodejs", "bun", "deno":
			return true
		}
		if strings.HasPrefix(s, "python") {
			return true
		}
	}
	return false
}

// scriptArgs is how many arguments script reads past argv[0], to step
// over an interpreter's options.
const scriptArgs = 8

// script names the agent an interpreter runs from the start of its argv:
// argv[0] when the program set its title, else the first argument that is
// no option. Only the script's path is matched; the arguments after it are
// never looked at.
func script(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	if p := scriptAgent(argv[0]); p != "" {
		return p
	}
	for i := 1; i < len(argv) && i <= scriptArgs; i++ {
		switch a := argv[i]; {
		case a == "-m" && i+1 < len(argv):
			return programs[argv[i+1]]
		case a == "run" && i == 1, strings.HasPrefix(a, "-"):
		default:
			return scriptAgent(a)
		}
	}
	return ""
}

// scriptAgent names the agent a script path runs: by its base name, less
// a script extension, or by the npm package it lies in.
func scriptAgent(path string) string {
	base := filepath.Base(path)
	for _, ext := range []string{".js", ".mjs", ".cjs", ".ts", ".py"} {
		base = strings.TrimSuffix(base, ext)
	}
	if p := programs[base]; p != "" {
		return p
	}
	path = filepath.ToSlash(path)
	for dir, p := range packages {
		if strings.Contains(path, dir) {
			return p
		}
	}
	return ""
}
