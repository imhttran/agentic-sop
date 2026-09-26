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
func New(projectDir, id string) (*Run, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("run: id is required")
	}
	dir := filepath.Join(projectDir, config.DirName, runsDirName, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("run: create %s: %w", dir, err)
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
