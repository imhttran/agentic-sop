// Package planflow prepares a project for execution: it discovers or resolves the
// human planning source, keeps the machine plan (.agent-sdlc/plan.json) in step
// with it, and creates persisted tasks when none exist. It is deterministic except
// where the configured agent is genuinely needed (generating a plan from a PRD,
// or normalizing a document that is not recognizably a plan), and it is safe to
// run repeatedly.
//
// A project may hold several PLAN files (for example docs/PLAN.md,
// docs/PLAN-Hardening.md, docs/PLAN-MCP.md). Each has its own identity — source
// path, content hash, and plan id — recorded beside the machine plan, so SOP
// never confuses one plan's tasks with another's. When no plan is named, source
// precedence is:
//
//  1. an existing valid .agent-sdlc/plan.json (reused only when its source is unchanged)
//  2. docs/PLAN.md
//  3. PLAN.md
//  4. docs/PRD.md
//  5. PRD.md
//
// When a plan is named explicitly it is authoritative for that execution.
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

// errNoAgent is returned when building a plan requires the agent but none is
// configured. It is actionable on its own and is not wrapped as a validation
// failure.
var errNoAgent = errors.New("no agent configured: set SOP_AGENT_COMMAND (needed to generate a plan from a PRD, or to normalize a PLAN.md that is not recognizable)")

// TaskStore is the persistence slice Prepare needs.
type TaskStore interface {
	List() ([]*domain.Task, error)
	SaveTasks(tasks []*domain.Task) error
}

// Options configures Prepare. PlanSource, when set, is the resolved absolute path
// of an explicitly selected PLAN and is authoritative for the execution. Agent
// may be nil when no generation or normalization is required.
type Options struct {
	Dir        string
	PlanSource string
	Agent      agent.Agent
	Store      TaskStore
}

// Result reports what Prepare did.
type Result struct {
	Source       string // relative path of the human source; "" when none is used
	SourceKind   string // KindPlan | KindPRD | KindExisting
	PlanID       string // stable identity of the source plan
	PlanRebuilt  bool   // the machine plan was (re)built this run
	PlanDoc      string // human plan document written when generating from a PRD
	TasksCreated int    // tasks created this run (0 when they already existed)
}

// Metadata records plan.json's provenance so a changed or different human source
// is detected without relying on modification time.
type Metadata struct {
	Source       string    `json:"source"`
	SourceKind   string    `json:"source_kind"`
	SourceSHA256 string    `json:"source_sha256"`
	PlanID       string    `json:"plan_id"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// Prepare ensures the project is ready to execute: it reconciles existing tasks,
// or resolves the source, builds the machine plan, validates it, and creates
// tasks. It is idempotent.
func Prepare(ctx context.Context, opts Options) (Result, error) {
	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)
	metaPath := filepath.Join(opts.Dir, config.DirName, metaFileName)

	tasks, err := opts.Store.List()
	if err != nil {
		return Result{}, err
	}
	if len(tasks) > 0 {
		return reconcileState(opts, metaPath)
	}

	docPath, docKind := requestedSource(opts)
	rel := relOf(opts.Dir, docPath)
	res := Result{Source: rel, SourceKind: docKind, PlanID: planID(rel)}
	if docPath == "" {
		res.SourceKind = KindExisting
		res.PlanID = ""
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

// requestedSource returns the plan to execute: the explicit source when one was
// given, otherwise the discovered source by precedence.
func requestedSource(opts Options) (path, kind string) {
	if strings.TrimSpace(opts.PlanSource) != "" {
		return opts.PlanSource, KindPlan
	}
	return discoverPlanningSource(opts.Dir)
}

// reconcileState handles the case where tasks already exist. It resumes when the
// requested plan matches what the task graph was built from, and otherwise stops
// with an actionable NEEDS_HUMAN error rather than mixing plans or discarding
// history.
func reconcileState(opts Options, metaPath string) (Result, error) {
	res := Result{SourceKind: KindExisting}

	docPath, docKind := requestedSource(opts)
	if docPath == "" {
		return res, nil // no plan to reconcile against
	}

	data, err := os.ReadFile(docPath)
	if err != nil {
		return res, fmt.Errorf("planflow: read %s: %w", docPath, err)
	}
	rel := relOf(opts.Dir, docPath)
	res.Source = rel
	res.SourceKind = docKind
	res.PlanID = planID(rel)

	meta := readMetadata(metaPath)
	switch {
	case meta.Source == "":
		// No recorded provenance (for example after `sop plan` + `sop tasks`):
		// reuse rather than block.
		return res, nil
	case meta.Source == rel && meta.SourceSHA256 == fingerprint(data):
		return res, nil // same plan, same content
	case meta.Source == rel:
		return res, planChangedError(rel)
	default:
		return res, differentPlanError(rel, meta.Source, meta.PlanID)
	}
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

	// Reuse the machine plan only when it was built from this exact source.
	if meta := readMetadata(metaPath); meta.Source == rel && meta.SourceSHA256 == fingerprint(data) {
		if existing, ok := loadPlan(planPath); ok {
			return existing, false, "", nil
		}
	}

	plan, err := buildPlan(ctx, opts.Agent, docKind, string(data))
	if err != nil {
		if errors.Is(err, errNoAgent) {
			return nil, false, "", err
		}
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
		PlanID:       planID(rel),
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
		plan, err := planner.New(a).Compile(ctx, content)
		if err != nil {
			if a == nil && strings.Contains(err.Error(), "no agent is available") {
				return nil, errNoAgent
			}
			return nil, err
		}
		return plan, nil
	}
	if a == nil {
		return nil, errNoAgent
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

// ResolvePlanPath resolves a user-supplied plan path relative to the project
// root, additionally checking docs/ when the bare name is not at the root. When
// both locations exist the choice is ambiguous and an actionable error is
// returned rather than a silent pick.
func ResolvePlanPath(dir, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", errors.New("plan path is empty")
	}
	if filepath.IsAbs(arg) {
		if !fileExists(arg) {
			return "", fmt.Errorf("PLAN not found: %s", arg)
		}
		return arg, nil
	}

	root := filepath.Join(dir, filepath.Clean(arg))
	alt := filepath.Join(dir, "docs", filepath.Base(arg))

	hasRoot := fileExists(root)
	hasAlt := alt != root && fileExists(alt)

	switch {
	case hasRoot && hasAlt:
		return "", fmt.Errorf("multiple plans match %s:\n\n  %s\n  %s\n\nSpecify the plan explicitly:\n\n  sop run %s",
			arg, displayPath(dir, root), displayPath(dir, alt), displayPath(dir, alt))
	case hasRoot:
		return root, nil
	case hasAlt:
		return alt, nil
	default:
		looked := []string{displayPath(dir, root)}
		if alt != root {
			looked = append(looked, displayPath(dir, alt))
		}
		return "", fmt.Errorf("PLAN not found: %s\n\nLooked for:\n  %s", arg, strings.Join(looked, "\n  "))
	}
}

// reportsDir is where SOP writes generated, human-readable artifacts, kept out of
// the project root.
const reportsDir = "docs/reports"

// writeGeneratedPlanDoc writes the human-readable plan generated from a PRD to
// docs/reports/<plan-id>.md and returns the relative path, or "". It never writes
// to the project root or beside the PRD.
func writeGeneratedPlanDoc(dir, prdPath string, plan *planner.Plan) string {
	id := planID(relOf(dir, prdPath))
	if id == "" {
		id = "plan"
	}
	target := filepath.Join(dir, reportsDir, id+".md")
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
	return fmt.Errorf("Plan validation failed:\n\n  %s\n\nNo work was executed.\n\nFix %s and rerun:\n\n  sop run %s", detail, name, name)
}

// planChangedError explains that the recorded plan source changed.
func planChangedError(source string) error {
	return fmt.Errorf("NEEDS_HUMAN: plan changed since the task graph was created\n\nSource: %s\n\nSOP will not silently rebuild the machine plan or discard task\nexecution history. Review the change, reconcile explicitly, then rerun:\n\n  sop run %s", source, source)
}

// differentPlanError explains that a different plan is already active.
func differentPlanError(requested, active, activeID string) error {
	if activeID == "" {
		activeID = "(unknown)"
	}
	return fmt.Errorf("NEEDS_HUMAN: a different plan is already active\n\nActive:    %s\nRequested: %s\n\nThe existing tasks belong to %s (%s). SOP will not mix two\nplans in one task graph. Finish or reconcile the active plan (for example by\nremoving .agent-sdlc/state.db to start fresh), then rerun:\n\n  sop run %s", active, requested, active, activeID, requested)
}

// planID derives a stable identity from a plan's relative path: the file name
// without its extension, lowercased and dash-separated (docs/PLAN-Hardening.md →
// plan-hardening).
func planID(rel string) string {
	if strings.TrimSpace(rel) == "" {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(base) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
			continue
		}
		pendingDash = true
	}
	return b.String()
}

// displayPath renders a path for a message: relative to the project root with a
// leading "./".
func displayPath(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		rel = path
	}
	return "./" + rel
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
