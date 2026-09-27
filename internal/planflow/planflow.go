// Package planflow prepares a project for execution: it discovers the human
// planning source, keeps the machine plan (.agent-sdlc/plan.json) in step with
// it, and creates persisted tasks when none exist. It is deterministic except
// where the configured agent is genuinely needed (generating or normalizing a
// plan), and it is safe to run repeatedly.
//
// Source precedence:
//
//  1. an existing valid .agent-sdlc/plan.json (reused only when the human source
//     it was built from is unchanged)
//  2. docs/PLAN.md
//  3. PLAN.md
//  4. docs/PRD.md
//  5. PRD.md
//
// A human PLAN is preferred over the PRD: the PRD is only used to generate a plan
// when no PLAN exists. plan.json never silently overrides a newer human PLAN —
// the source path and a content fingerprint are recorded so a changed plan
// document triggers a rebuild.
package planflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/taskbuilder"
)

const (
	planFileName = "plan.json"
	metaFileName = "plan.meta.json"
)

// Source kinds reported by Prepare.
const (
	KindPlan     = "plan"
	KindPRD      = "prd"
	KindExisting = "existing"
)

// TaskStore is the persistence slice Prepare needs.
type TaskStore interface {
	List() ([]*domain.Task, error)
	SaveTasks(tasks []*domain.Task) error
}

// Options configures Prepare. Agent may be nil when no generation or
// normalization is required (for example when an up-to-date plan.json is reused).
type Options struct {
	Dir   string
	Agent agent.Agent
	Store TaskStore
}

// Result reports what Prepare did.
type Result struct {
	Source       string // relative path of the human source; "" when none is used
	SourceKind   string // KindPlan | KindPRD | KindExisting
	PlanRebuilt  bool
	TasksCreated int
}

// Metadata records plan.json's provenance so a changed human source is detected.
type Metadata struct {
	Source      string    `json:"source"`
	SourceKind  string    `json:"source_kind"`
	Fingerprint string    `json:"fingerprint"`
	GeneratedAt time.Time `json:"generated_at"`
}

// Prepare ensures the machine plan and persisted tasks exist. It is idempotent:
// with tasks already present it does nothing.
func Prepare(ctx context.Context, opts Options) (Result, error) {
	var res Result

	tasks, err := opts.Store.List()
	if err != nil {
		return res, err
	}
	if len(tasks) > 0 {
		res.SourceKind = KindExisting
		return res, nil
	}

	stateDir := filepath.Join(opts.Dir, config.DirName)
	planPath := filepath.Join(stateDir, planFileName)
	metaPath := filepath.Join(stateDir, metaFileName)

	docPath, docKind := discover(opts.Dir)
	meta := readMetadata(metaPath)

	var plan *planner.Plan
	switch {
	case docPath != "":
		data, err := os.ReadFile(docPath)
		if err != nil {
			return res, fmt.Errorf("planflow: read %s: %w", docPath, err)
		}
		rel, err := filepath.Rel(opts.Dir, docPath)
		if err != nil {
			rel = docPath
		}
		res.Source = rel
		res.SourceKind = docKind

		// Reuse the machine plan when it was built from this exact source.
		if meta.Source == rel && meta.Fingerprint == fingerprint(data) {
			if existing, ok := loadPlan(planPath); ok {
				plan = existing
			}
		}

		if plan == nil {
			built, err := buildPlan(ctx, opts.Agent, docKind, string(data))
			if err != nil {
				return res, err
			}
			if err := writePlan(planPath, built); err != nil {
				return res, err
			}
			_ = writeMetadata(metaPath, Metadata{
				Source:      rel,
				SourceKind:  docKind,
				Fingerprint: fingerprint(data),
				GeneratedAt: time.Now().UTC(),
			})
			plan = built
			res.PlanRebuilt = true
		}

	default:
		// No human source: fall back to an existing machine plan.
		existing, ok := loadPlan(planPath)
		if !ok {
			return res, errors.New("planflow: no docs/PLAN.md, PLAN.md, docs/PRD.md, PRD.md, or .agent-sdlc/plan.json found")
		}
		plan = existing
		res.SourceKind = KindExisting
	}

	created, err := taskbuilder.CreateTasksFromPlan(plan, opts.Store.SaveTasks)
	if err != nil {
		return res, err
	}
	res.TasksCreated = len(created)
	return res, nil
}

// discover returns the first human planning document by precedence: a PLAN is
// preferred over a PRD.
func discover(dir string) (path, kind string) {
	for _, candidate := range []string{filepath.Join("docs", "PLAN.md"), "PLAN.md"} {
		full := filepath.Join(dir, candidate)
		if fileExists(full) {
			return full, KindPlan
		}
	}
	for _, candidate := range []string{filepath.Join("docs", "PRD.md"), "PRD.md"} {
		full := filepath.Join(dir, candidate)
		if fileExists(full) {
			return full, KindPRD
		}
	}
	return "", ""
}

// buildPlan compiles a human PLAN.md or generates a plan from a PRD.
func buildPlan(ctx context.Context, a agent.Agent, kind, content string) (*planner.Plan, error) {
	if kind == KindPlan {
		return planner.New(a).Compile(ctx, content)
	}
	if a == nil {
		return nil, errors.New("planflow: no agent available to generate a plan from the PRD")
	}
	return planner.New(a).Generate(ctx, content)
}

// loadPlan reads and validates a machine plan; ok is false when it is missing or
// invalid.
func loadPlan(path string) (*planner.Plan, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var plan planner.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, false
	}
	if err := plan.Validate(); err != nil {
		return nil, false
	}
	return &plan, true
}

// writePlan writes the machine plan atomically.
func writePlan(path string, plan *planner.Plan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("planflow: encode plan: %w", err)
	}
	return atomicWrite(path, append(data, '\n'))
}

// writeMetadata records plan.json's provenance atomically.
func writeMetadata(path string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("planflow: encode metadata: %w", err)
	}
	return atomicWrite(path, append(data, '\n'))
}

// readMetadata reads provenance, returning the zero value when it is absent or
// unreadable (an unreadable metadata file is treated as "unknown source").
func readMetadata(path string) Metadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}
	}
	return meta
}

// fingerprint returns a stable content hash of a planning document.
func fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// atomicWrite writes content to path via a temporary file and rename, so a
// failure never leaves a truncated file.
func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
