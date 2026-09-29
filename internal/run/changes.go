package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Task-scoped implementation-change evidence (JEV task-evidence assembly).
//
// A task may be implemented across several bounded invocations (CONTINUE, retry,
// or fix cycles), and its work may be committed between them. The run directory
// is keyed by the task id and survives every invocation, so it is the durable
// home for evidence that must outlive a single invocation. This artifact records
// the repository paths the task changed, so a later validation/review/JEV
// invocation reviews the task's accumulated implementation rather than only the
// last invocation's — possibly empty — working-tree change.
//
// It is read-only diagnostic evidence: SOP never reads it back to drive a task
// state transition, and it never replaces state.json or the state database. It
// holds repository paths only — never file contents, prompts, or secrets.

// changedFilesName is the artifact recording the repository paths a task changed
// across all of its invocations.
const changedFilesName = "changed-files.json"

// maxTaskChangedFiles bounds the accumulated task change set so a long task
// cannot grow the evidence without limit.
const maxTaskChangedFiles = 64

// RecordChangedFiles merges paths into the task's accumulated change set and
// persists it in the run directory. The run directory is keyed by task id and
// survives every invocation, so a later invocation (for example the no-change
// final invocation of a task whose earlier work was committed) still knows which
// files the task changed.
//
// It is idempotent and order-preserving: first-seen order is kept, duplicates and
// blanks are ignored, and the set is bounded, so recording the same paths again
// never changes the result and never grows the artifact. It performs no state
// transition and is never read back to drive a decision; a write failure is
// returned so the caller can treat it as best-effort.
func (r *Run) RecordChangedFiles(paths []string) error {
	merged := mergeChangedFiles(r.ChangedFiles(), paths)
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(r.dir, changedFilesName), data, 0o644)
}

// ChangedFiles returns the task's accumulated change set, or nil when nothing has
// been recorded. It never creates or modifies the artifact.
func (r *Run) ChangedFiles() []string {
	data, err := os.ReadFile(filepath.Join(r.dir, changedFilesName))
	if err != nil {
		return nil
	}
	var files []string
	if err := json.Unmarshal(data, &files); err != nil {
		return nil
	}
	return mergeChangedFiles(nil, files)
}

// mergeChangedFiles returns existing followed by the new paths, first-seen order
// preserved, blanks and duplicates dropped, and the result bounded. It is the
// single place the accumulator's ordering and bound are defined, so recording and
// reading agree.
func mergeChangedFiles(existing, added []string) []string {
	seen := make(map[string]bool, len(existing)+len(added))
	out := make([]string, 0, len(existing)+len(added))
	appendPath := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] || len(out) >= maxTaskChangedFiles {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range existing {
		appendPath(p)
	}
	for _, p := range added {
		appendPath(p)
	}
	return out
}
