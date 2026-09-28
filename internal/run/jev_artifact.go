package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// JEV run artifact persistence (JEV010).
//
// JEV results are persisted alongside a run's other diagnostics under
// .agent-sdlc/runs/<TASK>/, following the same conventions as the existing run
// artifacts (a JSON file written into the run directory, best-effort, never read
// back to drive a decision).
//
// JEV artifacts are diagnostic evidence, not workflow state. They are never
// loaded by the lifecycle, never feed a gate, and never replace state.json or the
// state database. Persisting them performs no state transition and no migration.

// jevArtifactFileName is the artifact holding the latest JEV result for a run.
const jevArtifactFileName = "jev.json"

// jevHistoryFileName is the append-only artifact preserving every prior JEV
// result for the same task, so a re-run or a later fix cycle never destroys
// earlier evidence.
const jevHistoryFileName = "jev-history.jsonl"

// WriteJEVArtifact records a JEV diagnostic artifact in the run directory.
//
// The latest result is written to jev.json so it is available after the run, and
// every result is appended to jev-history.jsonl so repeated runs or fix attempts
// preserve earlier evidence instead of clobbering it. It writes plain data only:
// it never reads an artifact back to drive a decision, never touches state.json
// or state.db, and never deletes or migrates existing persistence.
func (r *Run) WriteJEVArtifact(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("run: encode jev artifact: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(r.dir, jevArtifactFileName), data, 0o644); err != nil {
		return fmt.Errorf("run: write %s: %w", jevArtifactFileName, err)
	}

	// Append-only history: one compact JSON object per result, so the order of
	// results is preserved and no prior line is ever rewritten.
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("run: encode jev history: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(r.dir, jevHistoryFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("run: append %s: %w", jevHistoryFileName, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("run: append %s: %w", jevHistoryFileName, err)
	}
	return nil
}
