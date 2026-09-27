// Package planflow prepares a project for execution: it discovers the human
// planning source, keeps the machine plan (.agent-sdlc/plan.json) in step with
// it, and creates persisted tasks when none exist. It is deterministic except
// where the configured agent is genuinely needed (generating a plan from a PRD,
// or normalizing a document that is not recognizably a plan), and it is safe to
// run repeatedly.
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
// the source path and a content hash (source_sha256) are recorded so a changed
// plan document is detected. If the source changes after tasks already exist,
// reconciliation is unsafe and Prepare stops with an actionable NEEDS_HUMAN error
// rather than discarding task execution history.
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
	"strings"
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
	PlanRebuilt  bool   // the machine plan was (re)built this run
	PlanDoc      string // human plan document written when generating from a PRD
	TasksCreated int    // tasks created this run (0 when they already existed)
}

// Metadata records plan.json's provenance so a changed human source is detected.
type Metadata struct {
	Source       string    `json:"source"`
	SourceKind   string    `json:"source_kind"`
	SourceSHA256 string    `json:"source_sha256"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// Prepare ensures the project is ready to execute: it reconciles existing tasks,
// or discovers the source, builds the machine plan, validates it, and creates
// tasks. It is idempotent.
func Prepare(ctx context.Context, opts Options) (Result, error) {
	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)
	metaPath := filepath.Join(opts.Dir, config.DirName, metaFileName)

	tasks, err := opts.Store.List()
	if err != nil {
		return Result{}, err
	}
	if len(tasks) > 0 {
		return reconcileState(opts, planPath, metaPath)
	}

	docPath, docKind := discoverPlanningSource(opts.Dir)
	res := Result{
		Source:     relOf(opts.Dir, docPath),
		SourceKind: docKind,
	}
	if docPath == "" {
		res.SourceKind = KindExisting
	}

	plan, rebuilt, docWritten, err := ensurePlan(ctx, opts, docPath, docKind, planPath, metaPath)
	if err != nil {
		return res, err
	}
	res.PlanRebuilt = rebuilt
	res.PlanDoc = docWritten

	if err := validatePlan(res.Source, plan); err != nil {
		return res, err
	}

	created, err := ensureTasks(res.Source, plan, opts.Store)
	if err != nil {
		return res, err
	}
	res.TasksCreated = created
	return res, nil
}

// reconcileState handles the case where tasks already exist. It resumes when the
// human source still matches what the machine plan was built from, and otherwise
// stops with an actionable NEEDS_HUMAN error.
func reconcileState(opts Options, planPath, metaPath string) (Result, error) {
	res := Result{SourceKind: KindExisting}

	docPath, docKind := discoverPlanningSource(opts.Dir)
	if docPath == "" {
		return res, nil // no human source to reconcile against
	}

	data, err := os.ReadFile(docPath)
	if err != nil {
		return res, fmt.Errorf("planflow: read %s: %w", docPath, err)
	}
	rel := relOf(opts.Dir, docPath)
	res.Source = rel
	res.SourceKind = docKind

	meta := readMetadata(metaPath)
	// Only a recorded source that no longer matches proves a change; an
	// unrecorded source (for example after `sop plan` + `sop tasks`) is reused.
	if meta.Source != "" && (meta.Source != rel || meta.SourceSHA256 != fingerprint(data)) {
		return res, reconcileError(rel, meta)
	}
	return res, nil
}

// ensurePlan returns the machine plan, rebuilding it when the human source is new
// or has changed. It reports whether the plan was rebuilt and, when generating
// from a PRD, the relative path of a human plan document it wrote.
func ensurePlan(ctx context.Context, opts Options, docPath, docKind, planPath, metaPath string) (*planner.Plan, bool, string, error) {
	if docPath == "" {
		existing, ok := loadPlan(planPath)
		if !ok {
			return nil, false, "", errors.New("planflow: no docs/PLAN.md, PLAN.md, docs/PRD.md, PRD.md, or .agent-sdlc/plan.json found")
		}
		return existing, false, "", nil
	}

	data, err := os.ReadFile(docPath)
	if err != nil {
		return nil, false, "", fmt.Errorf("planflow: read %s: %w", docPath, err)
	}
	rel := relOf(opts.Dir, docPath)

	// Reuse the machine plan when it was built from this exact source.
	if meta := readMetadata(metaPath); meta.Source == rel && meta.SourceSHA256 == fingerprint(data) {
		if existing, ok := loadPlan(planPath); ok {
			return existing, false, "", nil
		}
	}

	plan, err := buildPlan(ctx, opts.Agent, docKind, string(data))
	if err != nil {
		return nil, false, "", planError(rel, err)
	}
	if err := plan.Validate(); err != nil {
		return nil, false, "", planError(rel, err)
	}
	if err := writePlan(planPath, plan); err != nil {
		return nil, false, "", err
	}
	_ = writeMetadata(metaPath, Metadata{
		Source:       rel,
		SourceKind:   docKind,
		SourceSHA256: fingerprint(data),
		GeneratedAt:  time.Now().UTC(),
	})

	docWritten := ""
	if docKind == KindPRD {
		docWritten = writeGeneratedPlanDoc(opts.Dir, docPath, plan)
	}
	return plan, true, docWritten, nil
}

// validatePlan re-checks the executable graph deterministically (ids, references,
// cycles, non-empty) and attaches the source path to any diagnostic.
func validatePlan(source string, plan *planner.Plan) error {
	if err := plan.Validate(); err != nil {
		return planError(source, err)
	}
	return nil
}

// ensureTasks creates the task graph when none exists yet.
func ensureTasks(source string, plan *planner.Plan, store TaskStore) (int, error) {
	tasks, err := taskbuilder.CreateTasksFromPlan(plan, store.SaveTasks)
	if err != nil {
		return 0, planError(source, err)
	}
	return len(tasks), nil
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

// discoverPlanningSource returns the first human planning document by precedence:
// a PLAN is preferred over a PRD.
func discoverPlanningSource(dir string) (path, kind string) {
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

// writeGeneratedPlanDoc writes the human-readable plan generated from a PRD to a
// sibling PLAN.md (docs/PLAN.md for docs/PRD.md, PLAN.md for PRD.md). It never
// overwrites an existing PLAN. It returns the relative path written, or "".
func writeGeneratedPlanDoc(dir, prdPath string, plan *planner.Plan) string {
	target := filepath.Join(filepath.Dir(prdPath), "PLAN.md")
	if fileExists(target) {
		return ""
	}
	if err := atomicWrite(target, []byte(plan.RenderMarkdown())); err != nil {
		return ""
	}
	return relOf(dir, target)
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

// planError formats a plan-problem diagnostic that names the source and the
// recovery command.
func planError(source string, err error) error {
	name := source
	if strings.TrimSpace(name) == "" {
		name = filepath.Join(config.DirName, planFileName)
	}
	detail := strings.TrimSpace(strings.TrimPrefix(err.Error(), "plan: "))
	return fmt.Errorf("PLAN validation failed\n\n%s\n%s\n\nFix %s and rerun:\n\n  sop run", name, detail, name)
}

// reconcileError explains an unsafe reconciliation.
func reconcileError(source string, meta Metadata) error {
	recorded := meta.Source
	if recorded == "" {
		recorded = "(not recorded)"
	}
	return fmt.Errorf("NEEDS_HUMAN: plan changed since the task graph was created\n\nSource:   %s\nRecorded: %s\n\nSOP will not silently rebuild the machine plan or discard task\nexecution history. Review the change, reconcile explicitly, then rerun:\n\n  sop run", source, recorded)
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
// unreadable (treated as "unknown source").
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

// relOf returns target relative to base when possible, else target. An empty
// target stays empty.
func relOf(base, target string) string {
	if target == "" {
		return ""
	}
	if rel, err := filepath.Rel(base, target); err == nil {
		return rel
	}
	return target
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
