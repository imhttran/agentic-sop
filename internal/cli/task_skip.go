package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/git"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

const taskSkipUsage = "usage: sop task skip <task-id> --reason <text> --evidence <path> [--evidence <path>]..."

// runTaskSkip dispositions a task NOT_REQUIRED (`sop task skip`): an explicit, model-free
// operator action stating that prerequisite evidence proved the task's conditional work
// unnecessary. It runs no implementation, records no approval, and fails closed unless
// the task's dependencies are satisfied, no human gate is pending on it, and every cited
// evidence file exists in the repository. The reason and evidence hashes are kept in the
// task history and a not-required.json run artifact.
func runTaskSkip(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	id, reason, evidence, ok := parseTaskSkipArgs(args, stderr)
	if !ok {
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}
	defer st.Close()

	task, err := st.Get(id)
	if store.IsNotFound(err) {
		fmt.Fprintf(stderr, "task skip: task %s not found\n", id)
		return exitError
	}
	if err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}
	previous := task.Status

	// The evidence comes from upstream work, so the dependencies must be satisfied.
	if err := requireDependenciesSatisfied(st, task); err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}

	// A pending human gate is resolved only by approve/decline, never bypassed by a skip.
	if view, err := approvalService(st, dir).Approval(id); err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	} else if view.Applicable {
		fmt.Fprintf(stderr, "task skip: %s has a pending %s approval gate; resolve it with `sop approve` or `sop decline` first\n", id, view.Kind)
		return exitError
	}

	cited, err := hashEvidence(dir, evidence)
	if err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}

	if err := task.MarkNotRequired(reason); err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}

	rec := runpkg.NotRequired{
		Version:        runpkg.NotRequiredVersion,
		TaskID:         id,
		PreviousStatus: string(previous),
		Reason:         strings.TrimSpace(reason),
		Evidence:       cited,
		RecordedBy:     runpkg.RecordedByOperatorAction,
		RecordedAt:     time.Now().UTC(),
	}
	if head, err := git.New(dir).Head(context.Background()); err == nil {
		rec.RepositoryHead = head
	}

	// The record is written before the state change, so a persisted NOT_REQUIRED always
	// has its provenance; a failed state save removes the record again.
	runDir := runpkg.Dir(dir, id)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}
	if err := runpkg.At(runDir).WriteNotRequired(rec); err != nil {
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}
	if err := st.Save(task); err != nil {
		_ = os.Remove(filepath.Join(runDir, runpkg.NotRequiredFile))
		fmt.Fprintf(stderr, "task skip: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "task %s: %s -> %s\n", id, previous, task.Status)
	fmt.Fprintf(stdout, "  reason:   %s\n", rec.Reason)
	for _, e := range cited {
		fmt.Fprintf(stdout, "  evidence: %s (sha256 %s)\n", e.Path, e.SHA256[:12])
	}
	fmt.Fprintf(stdout, "  record:   %s\n", filepath.Join(runDir, runpkg.NotRequiredFile))
	return exitOK
}

// hashEvidence resolves each cited path inside the repository and pins it by content
// hash. A path outside the repository, a directory, or a missing file fails closed.
func hashEvidence(dir string, paths []string) ([]runpkg.NotRequiredEvidence, error) {
	out := make([]runpkg.NotRequiredEvidence, 0, len(paths))
	for _, p := range paths {
		if !safeRepoPath(p) {
			return nil, fmt.Errorf("evidence %q must be a repository-relative path", p)
		}
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			return nil, fmt.Errorf("evidence %q: %w", p, err)
		}
		sum := sha256.Sum256(data)
		out = append(out, runpkg.NotRequiredEvidence{Path: filepath.ToSlash(filepath.Clean(p)), SHA256: hex.EncodeToString(sum[:])})
	}
	return out, nil
}

// parseTaskSkipArgs parses `sop task skip <task-id> --reason <text> --evidence <path>...`.
// Both a reason and at least one evidence file are required, so a skip is never silent.
func parseTaskSkipArgs(args []string, stderr io.Writer) (id, reason string, evidence []string, ok bool) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--reason" || a == "--evidence":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, taskSkipUsage)
				return "", "", nil, false
			}
			if a == "--reason" {
				reason = args[i+1]
			} else {
				evidence = append(evidence, args[i+1])
			}
			i++
		case strings.HasPrefix(a, "--reason="):
			reason = strings.TrimPrefix(a, "--reason=")
		case strings.HasPrefix(a, "--evidence="):
			evidence = append(evidence, strings.TrimPrefix(a, "--evidence="))
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "task skip: unknown flag %s\n%s\n", a, taskSkipUsage)
			return "", "", nil, false
		case id == "":
			id = a
		default:
			fmt.Fprintln(stderr, taskSkipUsage)
			return "", "", nil, false
		}
	}
	switch {
	case id == "":
		fmt.Fprintln(stderr, taskSkipUsage)
	case strings.TrimSpace(reason) == "":
		fmt.Fprintf(stderr, "task skip: an explicit --reason is required\n%s\n", taskSkipUsage)
	case len(evidence) == 0:
		fmt.Fprintf(stderr, "task skip: at least one --evidence file is required\n%s\n", taskSkipUsage)
	default:
		return id, reason, evidence, true
	}
	return "", "", nil, false
}
