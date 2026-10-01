// Package run persists a single harness run: its artifacts and stage under
// .agent-sdlc/runs/<id>/. A run is the durable record left behind, so a run that
// terminates for any reason stays inspectable afterwards.
package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// runsDirName is the subdirectory of the project state dir holding runs.
const runsDirName = "runs"

// stateFileName is the persisted run state file inside a run directory.
const stateFileName = "state.json"

// Stage is the deterministic lifecycle stage of a run.
type Stage string

const (
	Created         Stage = "CREATED"
	Planning        Stage = "PLANNING"
	Implementing    Stage = "IMPLEMENTING"
	Validating      Stage = "VALIDATING"
	Reviewing       Stage = "REVIEWING"
	Fixing          Stage = "FIXING"
	WaitingForHuman Stage = "WAITING_FOR_HUMAN"
	Passed          Stage = "PASSED"
	Failed          Stage = "FAILED"
)

// State is the persisted, inspectable run state.
type State struct {
	ID        string    `json:"id"`
	Stage     Stage     `json:"stage"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Run is a handle to one run directory.
type Run struct {
	dir   string
	state State
}

// New creates (or reopens) the run directory for id under the project's state
// dir and records the initial CREATED state.
//
// A new run starts a fresh set of per-run diagnostic artifacts: the execution-
// attempt records under attempts/ describe the attempts THIS run made, so the
// directory is cleared here. Durable budgets are deliberately NOT reset —
// attempt.txt and continuations.txt carry across runs so a task's retry and
// continuation budget survives — and state.db remains the source of task state.
func New(projectDir, id string) (*Run, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("run: id is required")
	}
	dir := filepath.Join(projectDir, config.DirName, runsDirName, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("run: create %s: %w", dir, err)
	}
	if err := os.RemoveAll(filepath.Join(dir, attemptsDirName)); err != nil {
		return nil, fmt.Errorf("run: reset attempts in %s: %w", dir, err)
	}

	now := time.Now().UTC()
	r := &Run{dir: dir, state: State{ID: id, Stage: Created, CreatedAt: now, UpdatedAt: now}}
	if err := r.save(); err != nil {
		return nil, err
	}
	return r, nil
}

// Dir returns the run directory.
func (r *Run) Dir() string { return r.dir }

// State returns a copy of the current run state.
func (r *Run) State() State { return r.state }

// SetStage advances the run's stage and persists state.
func (r *Run) SetStage(stage Stage) error {
	r.state.Stage = stage
	r.state.UpdatedAt = time.Now().UTC()
	return r.save()
}

// Write stores an artifact file inside the run directory.
func (r *Run) Write(name, content string) error {
	return os.WriteFile(filepath.Join(r.dir, name), []byte(content), 0o644)
}

// attemptFileName stores the last attempt's outcome signature. Unlike state.json
// it is not reset by New, so it survives across runs of the same task.
const attemptFileName = "attempt.txt"

// ReadAttempt returns the outcome signature recorded by the previous attempt, if
// any.
func (r *Run) ReadAttempt() (string, bool) {
	data, err := os.ReadFile(filepath.Join(r.dir, attemptFileName))
	if err != nil {
		return "", false
	}
	sig := strings.TrimSpace(string(data))
	return sig, sig != ""
}

// RecordAttempt stores this attempt's outcome signature for the next run.
func (r *Run) RecordAttempt(signature string) error {
	return os.WriteFile(filepath.Join(r.dir, attemptFileName), []byte(signature+"\n"), 0o644)
}

// continuationFileName stores how many bounded continuations a task has consumed.
// Like attempt.txt it is not reset by New, so the continuation budget survives
// across invocations. It is deliberately separate from attempt.txt: a RETRY
// repeats a failed attempt against max_attempts, while a CONTINUE resumes useful
// but unfinished work against max_continuations.
const continuationFileName = "continuations.txt"

// ReadContinuations returns the number of continuations recorded so far. A
// missing or malformed file reads as zero.
func (r *Run) ReadContinuations() int {
	data, err := os.ReadFile(filepath.Join(r.dir, continuationFileName))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// RecordContinuations stores the continuation count for the next run.
func (r *Run) RecordContinuations(n int) error {
	return os.WriteFile(filepath.Join(r.dir, continuationFileName), []byte(strconv.Itoa(n)+"\n"), 0o644)
}

// Load reads a run's persisted stage without creating or modifying anything.
// Unlike New it is read-only, so a caller can inspect an interrupted run before
// deciding whether to resume it.
func Load(projectDir, id string) (Stage, bool) {
	data, err := os.ReadFile(filepath.Join(projectDir, config.DirName, runsDirName, id, stateFileName))
	if err != nil {
		return "", false
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil || state.Stage == "" {
		return "", false
	}
	return state.Stage, true
}

// save writes state.json atomically enough for inspection: the payload is built
// first, then written.
func (r *Run) save() error {
	data, err := json.MarshalIndent(r.state, "", "  ")
	if err != nil {
		return fmt.Errorf("run: encode state: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(r.dir, stateFileName), data, 0o644)
}
