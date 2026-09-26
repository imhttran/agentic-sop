// Package git provides SOP's deterministic Git boundary: a small adapter over
// the installed git CLI. Workflow control decides when Git state may change;
// agents and models never call these operations directly.
package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Status describes whether the working tree has changes.
type Status string

const (
	Clean Status = "CLEAN"
	Dirty Status = "DIRTY"
)

var (
	// ErrDetachedHead is returned by CurrentBranch when HEAD is not on a branch.
	ErrDetachedHead = errors.New("git: detached HEAD")
	// ErrBranchExists is returned by CreateBranch when the branch already exists.
	ErrBranchExists = errors.New("git: branch already exists")
)

// Adapter performs deterministic Git operations in a working directory.
type Adapter struct {
	dir string
}

// New returns an Adapter that runs Git in dir.
func New(dir string) *Adapter {
	return &Adapter{dir: dir}
}

// ValidateRepository verifies that dir is a Git working tree. It does not
// initialize a repository.
func (a *Adapter) ValidateRepository(ctx context.Context) error {
	out, err := run(ctx, a.dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("validate repository: %w", err)
	}
	if strings.TrimSpace(out) != "true" {
		return errors.New("validate repository: not a git working tree")
	}
	return nil
}

// Status reports whether the working tree has changes. Untracked files count.
func (a *Adapter) Status(ctx context.Context) (Status, error) {
	out, err := run(ctx, a.dir, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("status: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		return Clean, nil
	}
	return Dirty, nil
}

// CurrentBranch returns the checked-out branch. A detached HEAD is reported as
// ErrDetachedHead rather than guessing a branch.
func (a *Adapter) CurrentBranch(ctx context.Context) (string, error) {
	out, err := run(ctx, a.dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("current branch: %w", err)
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		return "", ErrDetachedHead
	}
	return branch, nil
}

// UpdateIntegrationBranch fast-forwards the local integration branch from
// origin. It never creates a merge commit or force-updates history.
func (a *Adapter) UpdateIntegrationBranch(ctx context.Context, branch string) error {
	if err := a.validateBranch(ctx, branch); err != nil {
		return err
	}
	if err := a.requireClean(ctx); err != nil {
		return fmt.Errorf("update integration branch %s: %w", branch, err)
	}
	if _, err := run(ctx, a.dir, "fetch", "origin", branch); err != nil {
		return fmt.Errorf("update integration branch %s: %w", branch, err)
	}
	if _, err := run(ctx, a.dir, "checkout", branch); err != nil {
		return fmt.Errorf("update integration branch %s: %w", branch, err)
	}
	if _, err := run(ctx, a.dir, "merge", "--ff-only", "origin/"+branch); err != nil {
		return fmt.Errorf("update integration branch %s: %w", branch, err)
	}
	return nil
}

// BranchExists reports whether a local branch exists.
func (a *Adapter) BranchExists(ctx context.Context, branch string) (bool, error) {
	if err := a.validateBranch(ctx, branch); err != nil {
		return false, err
	}
	_, err := run(ctx, a.dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) && cmdErr.ExitCode == 1 {
		return false, nil
	}
	return false, fmt.Errorf("branch exists %s: %w", branch, err)
}

// CreateBranch creates branch from the current HEAD and switches to it. It
// refuses to reset an existing branch and refuses a dirty working tree.
func (a *Adapter) CreateBranch(ctx context.Context, branch string) error {
	if err := a.validateBranch(ctx, branch); err != nil {
		return err
	}
	if err := a.requireClean(ctx); err != nil {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}
	exists, err := a.BranchExists(ctx, branch)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("create branch %s: %w", branch, ErrBranchExists)
	}
	if _, err := run(ctx, a.dir, "checkout", "-b", branch); err != nil {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}
	current, err := a.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}
	if current != branch {
		return fmt.Errorf("create branch %s: ended on %s", branch, current)
	}
	return nil
}

// Checkout switches to an existing branch. It never forces and refuses a dirty
// working tree.
func (a *Adapter) Checkout(ctx context.Context, branch string) error {
	if err := a.validateBranch(ctx, branch); err != nil {
		return err
	}
	if err := a.requireClean(ctx); err != nil {
		return fmt.Errorf("checkout %s: %w", branch, err)
	}
	if _, err := run(ctx, a.dir, "checkout", branch); err != nil {
		return fmt.Errorf("checkout %s: %w", branch, err)
	}
	current, err := a.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("checkout %s: %w", branch, err)
	}
	if current != branch {
		return fmt.Errorf("checkout %s: ended on %s", branch, current)
	}
	return nil
}

// Diff returns the read-only working-tree diff.
func (a *Adapter) Diff(ctx context.Context) (string, error) {
	out, err := run(ctx, a.dir, "diff")
	if err != nil {
		return "", fmt.Errorf("diff: %w", err)
	}
	return out, nil
}

// Add stages paths for the next commit. With no paths it stages all changes
// (git add -A). Arguments are structured, never a shell; a leading "--" keeps a
// path from being read as an option.
func (a *Adapter) Add(ctx context.Context, paths ...string) error {
	args := []string{"add"}
	if len(paths) == 0 {
		args = append(args, "-A")
	} else {
		args = append(args, "--")
		args = append(args, paths...)
	}
	if _, err := run(ctx, a.dir, args...); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	return nil
}

// Commit creates a normal commit from staged changes. It returns an error when
// there is nothing to commit, never amends, and never bypasses hooks.
func (a *Adapter) Commit(ctx context.Context, message string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("commit: message must not be empty")
	}
	if _, err := run(ctx, a.dir, "commit", "-m", message); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// AddWorktree creates a linked worktree at path on a new branch, so parallel
// work can proceed in an isolated directory. It refuses an invalid branch name
// and never forces an existing worktree.
func (a *Adapter) AddWorktree(ctx context.Context, path, branch string) error {
	if err := a.validateBranch(ctx, branch); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("worktree path is empty")
	}
	if _, err := run(ctx, a.dir, "worktree", "add", "-b", branch, path); err != nil {
		return fmt.Errorf("add worktree %s: %w", branch, err)
	}
	return nil
}

// RemoveWorktree removes a linked worktree. It never forces removal of a
// worktree with uncommitted changes.
func (a *Adapter) RemoveWorktree(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("worktree path is empty")
	}
	if _, err := run(ctx, a.dir, "worktree", "remove", path); err != nil {
		return fmt.Errorf("remove worktree %s: %w", path, err)
	}
	return nil
}

func (a *Adapter) requireClean(ctx context.Context) error {
	status, err := a.Status(ctx)
	if err != nil {
		return err
	}
	if status != Clean {
		return errors.New("working tree is dirty; commit or stash changes first")
	}
	return nil
}

// TaskBranchName builds the deterministic task branch name task/<id>-<slug>
// from a task ID and title. It never uses a model. The result is a candidate:
// Git validates the name authoritatively when it is used in an operation.
func TaskBranchName(id, title string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("task id is empty")
	}
	slug := slugify(title)
	if slug == "" {
		return "", fmt.Errorf("cannot build branch name: title %q has no usable characters", title)
	}
	return "task/" + id + "-" + slug, nil
}

// slugify lowercases s and replaces every run of unsupported characters with a
// single dash, trimming leading and trailing dashes.
func slugify(s string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
			continue
		}
		pendingDash = true
	}
	return b.String()
}

// validateBranch asks Git whether name is an acceptable branch name. Git is the
// authority for ref syntax; SOP does not reimplement the rules.
func (a *Adapter) validateBranch(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("branch name is empty")
	}
	if _, err := run(ctx, a.dir, "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("invalid branch name %q: %w", name, err)
	}
	return nil
}
