package git

import (
	"context"
	"errors"
	"strings"
)

// Head returns the current HEAD commit hash.
func (a *Adapter) Head(ctx context.Context) (string, error) {
	out, err := run(ctx, a.dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Tree returns the current HEAD tree object id. Together with Head it identifies
// the committed content a scan observed.
func (a *Adapter) Tree(ctx context.Context) (string, error) {
	out, err := run(ctx, a.dir, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ChangedFiles returns repository-relative tracked or untracked paths that differ
// from HEAD. It is read-only and used to scope optimization evidence; it never
// stages, commits, or cleans anything. Paths under .agent-sdlc are excluded by
// the caller, not here.
func (a *Adapter) ChangedFiles(ctx context.Context) ([]string, error) {
	out, err := run(ctx, a.dir, "diff", "--name-only", "--no-ext-diff", "HEAD")
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var files []string
	for _, line := range strings.Split(out, "\n") {
		path := strings.TrimSpace(line)
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	untracked, err := run(ctx, a.dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(untracked, "\n") {
		path := strings.TrimSpace(line)
		if path != "" && !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	return files, nil
}

// IsAncestor reports whether ancestor is contained by ref (ref equals ancestor or is a
// descendant of it). The exit code 1 that `git merge-base --is-ancestor` returns for
// "not an ancestor" is a valid answer, not an error.
func (a *Adapter) IsAncestor(ctx context.Context, ancestor, ref string) (bool, error) {
	if _, err := run(ctx, a.dir, "merge-base", "--is-ancestor", ancestor, ref); err != nil {
		var ce *CommandError
		if errors.As(err, &ce) && ce.ExitCode == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
