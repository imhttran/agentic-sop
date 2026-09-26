package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runCommit commits the current changes with a message derived from a task file.
// It performs no push. When the human gate is enabled it requires an explicit
// --yes, so a commit is never silent.
func runCommit(args []string, stdout, stderr io.Writer, d deps) int {
	path, approve, ok := parseGateArgs("commit", args, stderr)
	if !ok {
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "commit: %v\n", err)
		return exitError
	}
	if cfg.Human.RequiresApprovalBeforeCommit() && !approve {
		fmt.Fprintln(stderr, "commit: human approval required; re-run with --yes")
		return exitError
	}

	message := "sop: apply changes"
	if path != "" {
		spec, err := loadTaskFile(dir, path)
		if err != nil {
			fmt.Fprintf(stderr, "commit: %v\n", err)
			return exitError
		}
		message = commitMessage(spec)
	}

	if err := d.commit(context.Background(), dir, message); err != nil {
		fmt.Fprintf(stderr, "commit: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "committed: %s\n", message)
	return exitOK
}

// commitMessage derives a commit subject from a task file.
func commitMessage(spec *taskfile.Spec) string {
	switch {
	case spec.ID != "" && spec.Title != "":
		return fmt.Sprintf("task(%s): %s", spec.ID, spec.Title)
	case spec.Title != "":
		return spec.Title
	default:
		return "sop: apply changes"
	}
}

// parseGateArgs parses "[TASK.md] [--yes]" for the commit and pr commands.
func parseGateArgs(cmd string, args []string, stderr io.Writer) (path string, approve bool, ok bool) {
	for _, a := range args {
		switch {
		case a == "--yes" || a == "-y":
			approve = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "unknown flag %s\nusage: sop %s [TASK.md] [--yes]\n", a, cmd)
			return "", false, false
		case path == "":
			path = a
		default:
			fmt.Fprintf(stderr, "usage: sop %s [TASK.md] [--yes]\n", cmd)
			return "", false, false
		}
	}
	return path, approve, true
}
