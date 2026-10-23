package hipcompare

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// changedPaths lists repository-relative paths that differ between commit and the
// worktree's working tree, including staged-only changes, unstaged changes,
// renames, deletions, and untracked files. It diffs the work tree against the
// fixed starting commit so a committed, staged, unstaged, or new change is all
// seen. Paths are read with `-z` so Git never quotes an unusual path.
func changedPaths(ctx context.Context, worktree, commit string) ([]string, error) {
	if strings.TrimSpace(worktree) == "" {
		return nil, errors.New("changed paths: worktree is empty")
	}
	// `git diff -z --name-only <commit>` compares the starting commit to the
	// working tree, covering committed, staged, and unstaged changes to tracked
	// files. `--` keeps a path from being read as an option.
	out, err := gitOutput(ctx, worktree, "diff", "-z", "--name-only", commit, "--")
	if err != nil {
		return nil, err
	}
	paths := splitNul(out)
	// Untracked files: `git diff` does not list them, so they are read from status
	// with `-z` and included, exactly the way an untracked new file counts as a
	// change. Renames are listed with both their old and new paths, so both are
	// checked.
	statusOut, err := gitOutput(ctx, worktree, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	paths = append(paths, statusPaths(statusOut)...)
	sort.Strings(paths)
	return dedupe(paths), nil
}

// statusPaths extracts every changed path from `git status --porcelain -z`
// output. A rename entry is `R  new\x00old`, so both paths are returned; a
// deletion or staged-only entry `D  path` is returned as `path`.
func statusPaths(s string) []string {
	fields := splitNul(s)
	var out []string
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		xy := entry[:2]
		p := entry[3:]
		if p != "" {
			out = append(out, p)
		}
		// A rename or copy is followed by the original path as a separate field.
		if strings.ContainsAny(xy, "RC") && i+1 < len(fields) {
			i++
			if orig := fields[i]; orig != "" {
				out = append(out, orig)
			}
		}
	}
	return out
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func splitNul(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\x00") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
