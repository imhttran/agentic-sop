package jev

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Operation is a tool/capability operation JEV requests. Operations are
// classified by the JEV read-only policy before they run.
type Operation string

const (
	// Allowed read-only inspection operations.
	OpReadFile    Operation = "read_file"
	OpSearchFiles Operation = "search_files"
	OpListFiles   Operation = "list_files"
	OpInspectDiff Operation = "inspect_diff"
	OpInspectCtx  Operation = "inspect_context"

	// Denied operations: repository mutation and destructive Git.
	OpWriteFile  Operation = "write_file"
	OpDeleteFile Operation = "delete_file"
	OpGitReset   Operation = "git_reset"
	OpGitClean   Operation = "git_clean"
	OpCommit     Operation = "commit"
	OpPush       Operation = "push"
	OpMerge      Operation = "merge"
)

// allowedOperations is the JEV read-only allow-list. JEV operates as a
// read-only capability: it may inspect, never mutate.
var allowedOperations = map[Operation]bool{
	OpReadFile:    true,
	OpSearchFiles: true,
	OpListFiles:   true,
	OpInspectDiff: true,
	OpInspectCtx:  true,
}

// deniedOperations is the JEV deny-list: repository mutation and destructive
// Git operations. It is explicit so the denial is deterministic and documented.
var deniedOperations = map[Operation]bool{
	OpWriteFile:  true,
	OpDeleteFile: true,
	OpGitReset:   true,
	OpGitClean:   true,
	OpCommit:     true,
	OpPush:       true,
	OpMerge:      true,
}

// Policy is the JEV read-only capability policy. It is intentionally separate
// from the IMPLEMENT/FIX tool policy: granting JEV a capability never grants it
// mutation authority, and the IMPLEMENT/FIX policy is untouched.
//
// It reuses SOP's capability classification vocabulary (allow-list inspection,
// deny-list mutation) rather than introducing a parallel authorization
// framework.
type Policy struct{}

// DefaultPolicy returns the standard read-only JEV policy.
func DefaultPolicy() Policy { return Policy{} }

// Allows reports whether op is permitted by the JEV read-only policy. It returns
// a focused error naming the denied operation, so a denied request is rejected
// deterministically without side effects.
func (Policy) Allows(op Operation) error {
	if allowedOperations[op] {
		return nil
	}
	if deniedOperations[op] || op != "" {
		return fmt.Errorf("jev: operation %q denied: JEV is read-only and cannot mutate the repository or run destructive Git", op)
	}
	return fmt.Errorf("jev: operation %q denied: unknown operation for read-only JEV policy", op)
}

// ClassifyCommand applies the JEV deny-list to a structured command (program
// plus arguments) and rejects destructive Git and file mutation, reusing the
// same structured-argv discipline as SOP's command policy. It returns nil when
// the command is permitted for JEV.
func (Policy) ClassifyCommand(argv []string) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return fmt.Errorf("jev: empty command denied: JEV may only run read-only inspection")
	}
	switch filepath.Base(argv[0]) {
	case "git":
		return classifyGitForJEV(argv[1:])
	default:
		// JEV only runs recognized read-only inspection tooling; anything else
		// (including editors and shells) is denied.
		return fmt.Errorf("jev: command %q denied: JEV may only run read-only inspection", argv[0])
	}
}

// classifyGitForJEV permits only non-mutating git inspection subcommands.
func classifyGitForJEV(args []string) error {
	sub := gitSubcommand(args)
	switch sub {
	case "diff", "status", "log", "show", "rev-parse", "ls-files", "describe", "blame", "grep", "shortlog", "cat-file":
		return nil
	case "reset", "clean", "commit", "push", "merge":
		return fmt.Errorf("jev: git %s denied: JEV cannot mutate the repository or run destructive Git", sub)
	default:
		return fmt.Errorf("jev: git %q denied: JEV may only run read-only inspection", sub)
	}
}

// gitGlobalWithValue are git global options that consume the next argument.
var gitGlobalWithValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true,
	"--exec-path": true, "--namespace": true,
}

// gitSubcommand returns the first non-flag argument, skipping global options and
// the values they consume.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if gitGlobalWithValue[a] {
				i++
			}
			continue
		}
		return a
	}
	return ""
}
