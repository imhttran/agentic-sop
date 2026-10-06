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
