package ollamaagent

import (
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Invocation mutation evidence.
//
// Mutation evidence is the primary execution signal used to ground a mutating
// capability's outcome: it records whether *this* invocation performed a
// successful controlled mutation verified by before/after repository state.
// It is invocation-scoped (created by, and carried through, a single
// Complete/Execute call — never stored on the
// Harness), so one invocation cannot contaminate the next.
//
// Only a successful, verified content/type/mode change sets it. A no-op, failed
// execution (a refused path, a protected file, an IO error), a denied tool call (capability policy or
// the state-database guard), and a non-mutating tool (a read, a search, a git
// inspection) never do.
type mutationEvidence struct {
	observed bool
	// tools records which controlled mutations were observed, in order. It is
	// diagnostic only; the observed boolean is the signal.
	tools []string
	// paths records the repository paths a successful controlled mutation changed,
	// first-seen and deduplicated. Unlike observed it is not truncated to the first
	// mutation: it is authoritative task change evidence, so an unrelated
	// pre-existing dirty file the invocation never touched is never listed and can
	// never be attributed to the task.
	paths []string
}

// record marks that a successful controlled mutation was independently verified.
// It is idempotent: the first successful mutation is the evidence; later ones are
// recorded for diagnostics only.
func (m *mutationEvidence) record(tool string) {
	if m.observed {
		return
	}
	m.observed = true
	m.tools = append(m.tools, tool)
}

// maxMutationPaths bounds the recorded mutation paths, so a model that writes
// thousands of files cannot grow the in-memory evidence or the reported set
// without limit. It matches the run directory's task change-set bound.
const maxMutationPaths = 64

// recordPath records a successful controlled mutation's repository path. It is
// first-seen ordered, deduplicated, and bounded, so a multi-file change reports
// each file once and a runaway writer cannot grow the evidence indefinitely, and
// it accepts every mutation (not only the first).
func (m *mutationEvidence) recordPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" || len(m.paths) >= maxMutationPaths {
		return
	}
	for _, seen := range m.paths {
		if seen == path {
			return
		}
	}
	m.paths = append(m.paths, path)
}

// mutationPaths returns a copy of the recorded mutation paths, so a caller cannot
// mutate the accumulator's state through the returned slice.
func (m *mutationEvidence) mutationPaths() []string {
	if m == nil || len(m.paths) == 0 {
		return nil
	}
	return append([]string(nil), m.paths...)
}

// controlledMutation reports whether name (with args) is mutation-capable and
// requires before/after verification: a file tool or an in-place formatting
// command. Classification alone is never mutation evidence. It returns false
// for any other tool, so a read, a search, an inspection, or an unrelated command
// is never evidence.
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
	switch filepath.Base(argv[0]) {
	case "gofmt":
		// Only write mode can change repository content. Listing/printing
		// formatting differences supplies no mutation evidence.
		for _, arg := range argv[1:] {
			if arg == "-w" || arg == "-w=true" {
				return true
			}
		}
	}
	return false
}

// mutationPath returns the repository path a mutating tool was asked to change,
// and whether the tool names one. A run_command cannot name a path reliably, so it
// contributes none; only the file-mutating tools do. It reads only the tool's own
// path argument.
func mutationPath(name string, args map[string]any) (string, bool) {
	switch name {
	case toolharness.ToolWriteFile, toolharness.ToolCreateFile,
		toolharness.ToolDeleteFile, toolharness.ToolRestoreFile:
		if path, ok := args["path"].(string); ok && strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path), true
		}
	}
	return "", false
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

// inspectionIdentity identifies an operation and its canonical repository target.
// Search queries distinguish inspections of the same scope; result contents are
// never retained in progress state or diagnostics.
type inspectionIdentity struct {
	tool, path, query string
}

// discoveryIdentity accepts only successful, informative repository inspections.
// The controlled tool has already enforced repository access; canonicalizing the
// successful target prevents ./, absolute paths, and symlinks earning extra credit.
func discoveryIdentity(root, name string, args map[string]any, result string, err error) (inspectionIdentity, bool) {
	if err != nil || strings.TrimSpace(result) == "" {
		return inspectionIdentity{}, false
	}
	switch name {
	case toolharness.ToolReadFile:
	case toolharness.ToolListFiles:
		if result == "(empty)" {
			return inspectionIdentity{}, false
		}
	case toolharness.ToolSearchFiles:
		if result == "(no matches)" {
			return inspectionIdentity{}, false
		}
	default:
		return inspectionIdentity{}, false
	}
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return inspectionIdentity{}, false
	}
	query := ""
	if name == toolharness.ToolSearchFiles {
		query, _ = args["pattern"].(string)
		query = strings.TrimSpace(query)
	}
	return inspectionIdentity{tool: name, path: path, query: query}, true
}
