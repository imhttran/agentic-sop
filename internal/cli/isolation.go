package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/git"
)

// requirePlanIsolation refuses to start a new plan (no SOP state yet) on main in
// the repository's primary checkout, so plan work never lands directly on main.
// It names the worktree command to run instead. A resumed plan, another branch,
// a linked worktree, a detached HEAD, or a non-Git directory is left alone.
func requirePlanIsolation(dir, planArg string) error {
	if _, err := os.Stat(statePath(dir)); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	ctx := context.Background()
	g := git.New(dir)
	if branch, err := g.CurrentBranch(ctx); err != nil || branch != "main" {
		return nil
	}
	if linked, err := g.IsLinkedWorktree(ctx); err != nil || linked {
		return nil
	}
	slug := "plan"
	if planArg != "" {
		slug = strings.ToLower(strings.TrimSuffix(filepath.Base(planArg), filepath.Ext(planArg)))
	}
	return fmt.Errorf("a new plan must not run on main in the primary checkout; create a worktree from main and run there:\n"+
		"  git worktree add -b sop/%s ../%s-%s main", slug, filepath.Base(dir), slug)
}
