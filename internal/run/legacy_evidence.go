package run

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/activity"
)

// Legacy task-evidence bootstrap.
//
// A task implemented before task-scoped changed-file persistence existed has no
// changed-files.json. Its repository changes are still recorded, faithfully and
// task-scoped, in the run's persisted activity stream: every repository mutation
// the agent performed is a CHANGE event whose detail is the path it changed. That
// stream is the authoritative legacy source — it attributes a path to this task
// because the task's own agent changed it, never because the path merely happens
// to be dirty.
//
// The bootstrap is deliberately lazy and task-scoped (it reads one task's run
// directory when JEV is about to run), non-destructive (it only reads the stream
// and writes the ordinary changed-files artifact), and conservative: with no
// reliable mutation evidence it attributes nothing rather than guessing. It never
// consults the current working tree, so an unrelated dirty file is never adopted.

// ActivityArtifactName is the run artifact holding the task's activity stream. It
// is defined here, beside the other run-artifact names, so the producer (the CLI
// activity sink) and the reader (the legacy bootstrap) cannot diverge.
const ActivityArtifactName = "activity.jsonl"

// maxScannerLine bounds one activity line; the stream holds short summaries, so
// this is generous headroom rather than a real limit.
const maxScannerLine = 1 << 20

// ActivityChangePaths recovers the repository paths this task's agent changed,
// read from the run's persisted activity stream. It returns the first-seen,
// deduplicated, bounded set of CHANGE-stage event details — the paths of the
// mutations the task actually performed — or nil when the stream is absent or
// carries no such evidence.
//
// It is read-only: it never mutates the stream, the repository, or any other run
// artifact. Malformed lines are skipped, and an event naming another task is
// ignored, so a shared or hand-edited stream cannot leak a foreign attribution.
func (r *Run) ActivityChangePaths() []string {
	f, err := os.Open(filepath.Join(r.dir, ActivityArtifactName))
	if err != nil {
		return nil
	}
	defer f.Close()

	seen := make(map[string]bool)
	var paths []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), maxScannerLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e activity.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if !isChangeEvent(e, r.state.ID) {
			continue
		}
		path := strings.TrimSpace(e.Detail)
		if path == "" || seen[path] {
			continue
		}
		if len(paths) >= maxActivityChangePaths {
			break
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

// isChangeEvent reports whether e is a repository-mutation event belonging to this
// run. The CHANGE stage is the activity vocabulary's marker for mutating activity,
// and its detail is the changed path; DISCOVER/VALIDATE/… events are ignored. An
// event tagged with a different task id is not this task's evidence.
func isChangeEvent(e activity.Event, runID string) bool {
	if e.Stage != activity.StageChange {
		return false
	}
	return e.TaskID == "" || e.TaskID == runID
}

// maxActivityChangePaths bounds the recovered set, matching the task change-set
// bound so a legacy stream can never produce a larger attribution than a live run.
const maxActivityChangePaths = maxTaskChangedFiles
