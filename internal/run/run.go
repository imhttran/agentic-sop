// Package run persists a single harness run: its artifacts and stage under
// .agent-sdlc/runs/<id>/. A run is the durable record left behind, so a run that
// terminates for any reason stays inspectable afterwards.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/durablefs"
	"github.com/imhttran/agentic-sop/internal/projectlock"
	"github.com/imhttran/agentic-sop/internal/redact"
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
	// WaitingForPlanner: the run is paused on a planner question. It is a distinct
	// pause, never a human gate, and the task resumes from the saved stage when the
	// answer is applied.
	WaitingForPlanner Stage = "WAITING_FOR_PLANNER"
	Passed            Stage = "PASSED"
	Failed            Stage = "FAILED"
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

// Open returns a handle to the task's run directory for provenance writes that
// must not start a new run. Unlike New it performs no destructive setup: it never
// clears attempts/ and never rewrites state.json, so an existing run's evidence
// (its stage, its attempt records, its approval artifacts) is preserved byte for
// byte. The directory is created only when it does not already exist, so a task
// that never ran still gets a place for its artifact.
func Open(projectDir, id string) (*Run, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("run: id is required")
	}
	dir := filepath.Join(projectDir, config.DirName, runsDirName, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("run: create %s: %w", dir, err)
	}
	return &Run{dir: dir}, nil
}

// SetStageOnExisting updates a run's stage WITHOUT the destructive setup New does: it
// preserves an existing state.json (its id and created_at) and its attempts evidence,
// and creates the state at the given stage when none exists. It is the durable
// checkpoint mutation the planner pause/continue uses.
func SetStageOnExisting(projectDir, id string, stage Stage) error {
	dir := filepath.Join(projectDir, config.DirName, runsDirName, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("run: create %s: %w", dir, err)
	}
	now := time.Now().UTC()
	st := State{ID: id, Stage: stage, CreatedAt: now, UpdatedAt: now}
	if data, err := os.ReadFile(filepath.Join(dir, stateFileName)); err == nil {
		var cur State
		if json.Unmarshal(data, &cur) == nil && strings.TrimSpace(cur.ID) != "" {
			st = cur
			st.Stage = stage
			st.UpdatedAt = now
		}
	}
	r := &Run{dir: dir, state: st}
	return r.save()
}

// Resume opens a run's EXISTING state WITHOUT New's destructive reset: it preserves
// state.json (its id and created_at) and the attempts/ evidence, so a parked run is
// resumed in the same run epoch rather than restarted. A run with no state is
// initialized at CREATED.
func Resume(projectDir, id string) (*Run, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("run: id is required")
	}
	dir := filepath.Join(projectDir, config.DirName, runsDirName, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("run: create %s: %w", dir, err)
	}
	r := &Run{dir: dir}
	if data, err := os.ReadFile(filepath.Join(dir, stateFileName)); err == nil {
		var st State
		if json.Unmarshal(data, &st) == nil && strings.TrimSpace(st.ID) != "" {
			r.state = st
		}
	}
	if strings.TrimSpace(r.state.ID) == "" {
		now := time.Now().UTC()
		r.state = State{ID: id, Stage: Created, CreatedAt: now, UpdatedAt: now}
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

// Write stores an artifact file inside the run directory. Artifacts can carry model
// output and diff content, so they are owner-only: a file already present keeps its
// old mode under WriteFile, hence the explicit chmod.
// WriteReply writes a model reply. Credential shapes and known secret values are
// redacted first, so a known-format secret the model repeats does not persist. It
// does not catch unknown formats (see PROPOSAL-LLM02-MODEL-OUTPUT-ARTIFACTS.md).
// Evidence such as diff.patch keeps using Write, verbatim.
func (r *Run) WriteReply(name, content string) error {
	return r.Write(name, redact.ModelInput(content, os.Environ()))
}

func (r *Run) Write(name, content string) error {
	path := filepath.Join(r.dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
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

// ConsumeContinuation atomically reads, checks, and increments the persisted
// continuation count under the project lock. The read and the write are one
// critical section, so two concurrent runs cannot both spend the same
// continuation and no caller can overspend the configured maximum.
func (r *Run) ConsumeContinuation(max int) (n int, allowed bool, err error) {
	if max <= 0 {
		return 0, false, fmt.Errorf("run: continuation max must be positive")
	}
	lock, err := projectlock.AcquireWait(context.Background(), r.projectRoot(), 0)
	if err != nil {
		return 0, false, err
	}
	defer lock.Release()

	n = r.ReadContinuations()
	if n >= max {
		return n, false, nil
	}
	if err := r.RecordContinuations(n + 1); err != nil {
		return n, false, err
	}
	return n + 1, true, nil
}

// projectRoot returns the project directory that owns this run directory. A run
// directory is always <project>/.agent-sdlc/runs/<id>, so three parents reach
// the project root without storing a second copy of it on every handle.
func (r *Run) projectRoot() string {
	return filepath.Dir(filepath.Dir(filepath.Dir(r.dir)))
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

// PersistDurable writes the run's current state durably: a temp file + fsync + atomic
// rename + directory fsync. Unlike save it guarantees the checkpoint is on disk before it
// returns, so a caller that must not record a dependent fact (the planner continuation's
// success) until the checkpoint is durable can require it.
func (r *Run) PersistDurable() error {
	data, err := json.MarshalIndent(r.state, "", "  ")
	if err != nil {
		return fmt.Errorf("run: encode state: %w", err)
	}
	return durablefs.WriteFile(filepath.Join(r.dir, stateFileName), append(data, '\n'))
}
