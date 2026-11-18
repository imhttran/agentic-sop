package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/git"
)

// semgrepIgnoredExcludeLimit bounds how many derived exclusions are passed to
// Semgrep. The list is derived from the checkout, not configured, so it must not
// grow without an upper bound. Past the limit no exclusion is derived at all:
// the scanner's own skips then still count as gaps, which is the fail-closed
// outcome rather than a partial and confusing exclusion.
const semgrepIgnoredExcludeLimit = 512

// semgrepIgnoredScope is the out-of-scope content derived for one Semgrep scan.
//
// The repository already defines its source scope as "every tracked file plus
// every untracked file that is not ignored" (git.Adapter.SnapshotFiles), and
// Gitleaks scans exactly that set. Semgrep applies its own ignore files and
// reports a git-ignored skip as a coverage gap, so a checkout that happens to
// hold ignored build or runtime scratch could never reach COMPLETE while the
// same revision in a clean checkout does. Deriving the ignored set from Git and
// declaring it to Semgrep as exclusions makes the two agree without inventing a
// static exclusion list.
type semgrepIgnoredScope struct {
	// Excludes are the anchored, repository-relative patterns passed to Semgrep.
	Excludes []string
	// entries is the membership set of ignored paths, used to drop findings that
	// name out-of-scope content.
	entries map[string]bool
	// Dropped counts ignored paths that could not be declared safely. They stay
	// in scope, so a scanner skip of one of them remains a gap.
	Dropped int
	// Bounded is true when the ignored set exceeded semgrepIgnoredExcludeLimit.
	// No exclusion is derived in that case.
	Bounded bool
}

// contains reports whether path is ignored content: the exact path, or anything
// below an ignored directory.
func (s semgrepIgnoredScope) contains(path string) bool {
	for entry := path; entry != "" && entry != "."; entry = parentDir(entry) {
		if s.entries[entry] {
			return true
		}
	}
	return false
}

// deriveSemgrepIgnoredScope enumerates the paths Git ignores in the scan root.
//
// The derivation is path-based, never count-based: Semgrep's summary exposes
// counts without paths, so "there were N skips and M ignored files" cannot
// distinguish a skipped in-scope file from a skipped ignored one. Only an
// enumerated set can.
//
// A changed-files scope names its own targets explicitly, so nothing is derived
// there; a skip in that mode keeps counting as a gap.
//
// A directory that is not a checkout has no ignore database and no ignored set.
// A checkout whose Git state cannot be read fails closed.
func deriveSemgrepIgnoredScope(ctx context.Context, req Request) (semgrepIgnoredScope, error) {
	if strings.TrimSpace(req.WorkspaceRoot) == "" || req.Scope.Mode == "changed-files" {
		return semgrepIgnoredScope{}, nil
	}
	repo := git.New(req.WorkspaceRoot)
	ignored, err := repo.IgnoredPaths(ctx)
	if err != nil {
		return semgrepIgnoredScope{}, gitScopeFailure(ctx, req.WorkspaceRoot, err)
	}
	if len(ignored) == 0 {
		return semgrepIgnoredScope{}, nil
	}
	snapshot, err := repo.SnapshotFiles(ctx)
	if err != nil {
		return semgrepIgnoredScope{}, gitScopeFailure(ctx, req.WorkspaceRoot, err)
	}
	return planSemgrepIgnoredScope(ignored, snapshot)
}

// planSemgrepIgnoredScope turns enumerated ignored and in-scope paths into the
// exclusions to declare. It is separate from the Git calls so the admission
// rules can be exercised directly with adversarial input.
func planSemgrepIgnoredScope(ignored, snapshot []string) (semgrepIgnoredScope, error) {
	var scope semgrepIgnoredScope
	if len(ignored) == 0 {
		return scope, nil
	}
	if len(ignored) > semgrepIgnoredExcludeLimit {
		scope.Bounded = true
		return scope, nil
	}
	inScopeFiles, inScopeDirs, err := semgrepInScopeIndex(snapshot)
	if err != nil {
		return semgrepIgnoredScope{}, err
	}
	scope.entries = make(map[string]bool, len(ignored))
	seen := make(map[string]bool, len(ignored))
	for _, entry := range ignored {
		path, ok := safeSemgrepExcludePath(entry)
		if !ok {
			scope.Dropped++
			continue
		}
		// A path that Git reports as ignored must never overlap in-scope content.
		// The check is redundant with Git's own accounting, and it is what keeps a
		// tracked file in scope if the two ever disagree mid-scan.
		if inScopeFiles[path] || inScopeDirs[path] {
			scope.Dropped++
			continue
		}
		pattern := semgrepExcludePattern(path)
		if seen[pattern] {
			continue
		}
		seen[pattern] = true
		scope.Excludes = append(scope.Excludes, pattern)
		scope.entries[path] = true
	}
	sort.Strings(scope.Excludes)
	return scope, nil
}

// gitScopeFailure converts a Git scope-enumeration failure into what the scan
// must do. A directory without Git metadata has no snapshot and no ignore
// database, so it is simply unscoped. A checkout whose Git state cannot be read
// is an unknown scope: the scan must fail rather than walk ignored local files.
func gitScopeFailure(ctx context.Context, root string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if _, statErr := os.Lstat(filepath.Join(root, ".git")); statErr == nil {
		return err
	}
	return nil
}

// semgrepInScopeIndex builds exact-path and directory membership from the
// snapshot. Directory membership is precomputed so a check is a map lookup
// instead of a scan over every in-scope path.
func semgrepInScopeIndex(snapshot []string) (map[string]bool, map[string]bool, error) {
	files := make(map[string]bool, len(snapshot))
	dirs := make(map[string]bool, len(snapshot))
	for _, raw := range snapshot {
		path, reason := NormalizePath(raw)
		if path == "" {
			// An empty snapshot entry carries no path and no meaning.
			continue
		}
		if reason != "" {
			return nil, nil, fmt.Errorf("semgrep out-of-scope set: in-scope path rejected: %s", reason)
		}
		files[path] = true
		for dir := parentDir(path); dir != "" && dir != "."; dir = parentDir(dir) {
			dirs[dir] = true
		}
	}
	return files, dirs, nil
}

// safeSemgrepExcludePath accepts only a plain repository-relative path. Semgrep
// reads --exclude values as ignore patterns, so a value that carries pattern
// syntax would exclude content the derivation never named: a glob metacharacter
// would widen the match, and a leading "!" or ":" would be read as an ignore
// directive. Such a path is left in scope instead; its skips keep counting as
// gaps, which is the fail-closed outcome.
func safeSemgrepExcludePath(entry string) (string, bool) {
	trimmed := strings.TrimSuffix(entry, "/")
	path, reason := NormalizePath(trimmed)
	if path == "" || reason != "" {
		return "", false
	}
	if strings.ContainsAny(path, "*?[]{}!\\\n\r") {
		return "", false
	}
	if strings.HasPrefix(path, ":") {
		return "", false
	}
	return path, true
}

// semgrepExcludePattern anchors a derived path to the scan root. An unanchored
// ignore pattern matches its last segment at any depth, so excluding the file
// "sop" would also exclude "cmd/sop" and silently drop real source. A path that
// already contains a separator is root-relative on its own.
func semgrepExcludePattern(path string) string {
	if strings.Contains(path, "/") {
		return path
	}
	return "/" + path
}

// parentDir returns the repository-relative parent of path, or "" for a
// top-level entry.
func parentDir(path string) string {
	index := strings.LastIndex(path, "/")
	if index <= 0 {
		return ""
	}
	return path[:index]
}
