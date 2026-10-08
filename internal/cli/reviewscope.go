package cli

import (
	"errors"
	"sort"
	"strings"
)

// ErrReviewScopeUnestablished is returned by the review-scope seam when the
// task's own change set cannot be established. The seam fails closed on this
// error: it never falls back to reviewing the whole working tree.
var ErrReviewScopeUnestablished = errors.New("review scope cannot be established: no task-owned change is attributable")

// reviewScope is the path set the review input is restricted to: the task's
// invocation-scoped changed files (recorded by recordTaskChanges from
// invocationChanges, surfaced by rn.ChangedFiles()) unioned with the declared
// task deliverables. Nothing outside this set is ever reviewed, so unrelated
// pre-existing working-tree modifications cannot leak into the review.
type reviewScope struct {
	paths []string
	set   map[string]bool
}

// newReviewScope builds the scoped path set from the task's accumulated change
// evidence and its declared deliverables. Both inputs are normalized and
// de-duplicated deterministically. An empty result is a valid scope value; the
// caller decides whether an empty scope means "nothing to review" or a
// fail-closed condition.
func newReviewScope(changed, deliverables []string, reports ...string) reviewScope {
	set := make(map[string]bool)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || isSOPPath(p, reports...) {
			return
		}
		set[p] = true
	}
	for _, p := range changed {
		add(p)
	}
	for _, p := range deliverables {
		add(p)
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return reviewScope{paths: paths, set: set}
}

// empty reports whether the scope names no paths at all.
func (s reviewScope) empty() bool { return len(s.paths) == 0 }

// contains reports whether path is in the scope. Comparison is on the trimmed
// form so incidental whitespace never hides a membership.
func (s reviewScope) contains(path string) bool {
	return s.set[strings.TrimSpace(path)]
}

// pathsText renders the scope as a newline-joined, deterministic list for the
// review request header. It is empty for an empty scope.
func (s reviewScope) pathsText() string {
	return strings.Join(s.paths, "\n")
}

// scopedReviewDiff builds the review input for one task from the task's own
// change set: the invocation-scoped changed files (rn.ChangedFiles(), recorded
// by recordTaskChanges from invocationChanges) unioned with the declared task
// deliverables. The tracked and untracked portions of the diff are both
// filtered to that same set, so unrelated pre-existing working-tree
// modifications are excluded while a task-owned untracked deliverable is
// retained (the supplied diff already carries untracked files as synthetic
// additions, so no additional Git call is made).
//
// It fails closed: when the task required a change (changeRequired) and the
// diff names a repository file but the task's change set cannot be established
// (the tree is dirty but no task-owned change is attributable) it returns
// ErrReviewScopeUnestablished instead of the whole-tree diff. A legitimate
// no-change completion (changeRequired false, no attributable change) has
// nothing for the task to review, so it yields an empty review input rather
// than reviewing the whole tree or failing closed. An opaque diff that names no
// repository file cannot leak an unrelated modification and passes through
// unchanged. It is provider- and model-neutral and adds no dependency beyond the
// existing facilities.
func scopedReviewDiff(scope reviewScope, diff string, changeRequired bool) (string, error) {
	if scope.empty() {
		if !changeRequired {
			// The task did not require a change and produced no attributable
			// change: there is nothing for this task to review. Yield an empty
			// review input rather than a whole-tree diff (a legitimate no-change
			// completion), never failing closed for a task that owned no change.
			return "", nil
		}
		if len(changedFiles(diff)) > 0 {
			// The task required a change and the tree is dirty with a real
			// repository file, but nothing is attributable to this task: fail
			// closed rather than reviewing the whole working tree.
			return "", ErrReviewScopeUnestablished
		}
		// The diff names no repository file (an opaque or empty caller diff):
		// there is nothing to attribute and nothing that could leak an
		// unrelated modification, so it passes through unchanged.
		return diff, nil
	}
	return filterDiffToScope(diff, scope), nil
}

// filterDiffToScope returns the subset of a unified diff that touches only
// in-scope paths. It splits the diff into per-file chunks (each beginning at a
// `diff --git` line) and keeps a chunk only when every path it names is in
// scope. A chunk that names any out-of-scope path is dropped whole, so a rename
// from an out-of-scope source to an in-scope destination cannot smuggle the
// unrelated source into review.
func filterDiffToScope(diff string, scope reviewScope) string {
	if strings.TrimSpace(diff) == "" {
		return ""
	}
	var b strings.Builder
	var chunk []string
	flush := func() {
		if len(chunk) == 0 {
			return
		}
		keep := true
		seen := false
		for _, l := range chunk {
			p, ok := diffChunkPath(l)
			if !ok {
				continue
			}
			seen = true
			if !scope.contains(p) {
				keep = false
			}
		}
		if seen && keep {
			for _, l := range chunk {
				b.WriteString(l)
				b.WriteByte('\n')
			}
		}
		chunk = chunk[:0]
	}
	for _, l := range strings.Split(diff, "\n") {
		if strings.HasPrefix(l, "diff --git ") {
			flush()
		}
		chunk = append(chunk, l)
	}
	flush()
	return strings.TrimRight(b.String(), "\n")
}

// diffChunkPath extracts the repository-relative path named by a unified-diff
// header line. It recognizes both the `--- a/...` / `+++ b/...` file headers
// (including the `/dev/null` side of an addition or deletion) and git's
// `rename from` / `rename to` lines, so a renamed file is matched on both its
// sides. It reports ok=false for any other line, and for a header side naming
// no real path (/dev/null or empty), so a new or deleted file contributes only
// its real side.
func diffChunkPath(line string) (string, bool) {
	var path string
	switch {
	case strings.HasPrefix(line, "--- a/"):
		path = strings.TrimPrefix(line, "--- a/")
	case strings.HasPrefix(line, "+++ b/"):
		path = strings.TrimPrefix(line, "+++ b/")
	case strings.HasPrefix(line, "rename from "):
		path = strings.TrimPrefix(line, "rename from ")
	case strings.HasPrefix(line, "rename to "):
		path = strings.TrimPrefix(line, "rename to ")
	default:
		return "", false
	}
	path = diffHeaderPath(path)
	if path == "" || path == "/dev/null" {
		return "", false
	}
	return path, true
}
