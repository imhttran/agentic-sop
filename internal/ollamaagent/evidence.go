package ollamaagent

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Invocation mutation evidence.
//
// Mutation evidence is the primary execution signal used to ground a mutating
// capability's outcome: it records whether *this* invocation performed a
// successful controlled mutation. It is deliberately invocation-scoped (created
// by, and carried through, a single Complete/Execute call — never stored on the
// Harness), so one invocation cannot contaminate the next.
//
// Only a successful controlled mutation sets it. A failed execution (a refused
// path, a protected file, an IO error), a denied tool call (capability policy or
// the state-database guard), and a non-mutating tool (a read, a search, a git
// inspection) never do.
type mutationEvidence struct {
	observed bool
	// tools records which controlled mutations were observed, in order. It is
	// diagnostic only; the observed boolean is the signal.
	tools []string
}

// record marks that a successful controlled mutation was observed. It is
// idempotent: the first successful mutation is the evidence; later ones are
// recorded for diagnostics only.
func (m *mutationEvidence) record(tool string) {
	if m.observed {
		return
	}
	m.observed = true
	m.tools = append(m.tools, tool)
}

// controlledMutation reports whether a successful call to name (with args) is a
// controlled mutation: a file-mutating tool, or a formatting/mutating command
// run through run_command. It returns false for any other tool, so a read, a
// search, an inspection, or an unrelated command is never evidence.
func controlledMutation(name string, args map[string]any) bool {
	switch name {
	case toolharness.ToolWriteFile, toolharness.ToolCreateFile,
		toolharness.ToolDeleteFile, toolharness.ToolRestoreFile:
		return true
	case toolharness.ToolRunCommand:
		command, _ := args["command"].(string)
		return commandMutates(command)
	}
	return false
}

// commandMutates reports whether a run_command invocation is a controlled
// mutation: a formatting/mutating command that changes files in place. The
// command policy already refuses destructive commands, so anything that reaches
// execution here and matches is a safe, formatting-style write.
func commandMutates(command string) bool {
	argv, err := toolharness.SplitCommand(command)
	if err != nil || len(argv) == 0 {
		return false
	}
	switch strings.TrimSpace(argv[0]) {
	case "gofmt":
		// `gofmt -w`/`-l` both format; treat a successful gofmt run as a
		// formatting mutation of the invocation's work.
		return true
	}
	return false
}

// checkpointPath returns the repository path a read/inspect tool was asked to
// inspect, and whether it is worth recording in the continuation checkpoint. It
// reads only the tool's own path argument; a search with no path is not a file
// and contributes nothing.
func checkpointPath(name string, args map[string]any) (string, bool) {
	switch name {
	case toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles:
		if path, ok := args["path"].(string); ok && strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path), true
		}
	}
	return "", false
}
