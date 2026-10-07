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
// Once tasks exist, the plan that owns them is authoritative while it still has
// unresolved work: with no plan named, a run reconciles against that plan's
// recorded source rather than discovering another PLAN file. A named plan is
// always authoritative, and a completed active plan may hand off.
package planflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/bootstrap"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/taskbuilder"
)

const (
	planFileName = "plan.json"
	metaFileName = "plan.meta.json"
	// archiveDirName holds preserved records of completed plans, under DirName.
	archiveDirName = "archive"
	// archivedTasksFile is the task-record snapshot written into a plan archive.
	archivedTasksFile = "tasks.json"
	// lifecycleFile records a plan's terminal disposition inside its archive.
	lifecycleFile = "lifecycle.json"
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

// ErrUnsupportedReconcileSource is the stable diagnostic a reconciliation returns
// when the requested source differs from the recorded one and is not recognizably a
// structured plan. Reconciliation is deliberately deterministic and never calls a
// model to synthesize definitions: structured compilation and comparison are
// deterministic, while arbitrary model prose is not something SOP will infer as
// equivalent. An operator whose source is free-form must restate it in an explicit
// structured plan. The error text is a fixed string so repeated Inspect/Reconcile
// calls report byte-identical diagnostics, and it is returned before any store or
// file mutation, so the authoritative state is untouched. It is exported so a
// read-only consumer (for example a continuation classifier) can distinguish an
// uncompilable source from an operational failure without matching on its text.
var ErrUnsupportedReconcileSource = errors.New("NEEDS_HUMAN: the requested plan source changed but is not a recognizable structured plan\n\nSOP reconciles a changed plan deterministically by compiling its structured Markdown (\"## Project\", \"## Summary\", and one \"## <id> — <title>\" section per stage, or a \"## Tasks\" section with \"### <id> — <title>\" stages). Structured compilation and comparison are deterministic; arbitrary model prose is not, so SOP will not call a model to synthesize or normalize definitions just for a reconciliation.\n\nRewrite the requested source explicitly in the canonical rendered layout: each stage starts with its \"## <id> — <title>\" heading, then the objective prose comes directly after the heading, followed by the optional structured sub-sections (Dependencies, Requires, Deliverables, Acceptance Criteria, Execution). Do not put the objective under an \"Objective\" subheading — that is not a recognized field. Then rerun the reconciliation. No tasks, attempts, approvals or provenance were changed")

// TaskStore is the persistence slice Prepare needs.
type TaskStore interface {
	List() ([]*domain.Task, error)
	SaveTasks(tasks []*domain.Task) error
	// ClearTasks removes every task from the active graph. It is used only
	// during completed-plan handoff, after the previous records are archived.
	ClearTasks() error
}

// Options configures Prepare. PlanSource, when set, is the resolved absolute path
// of an explicitly selected PLAN and is authoritative for the execution. Agent
// may be nil when no generation or normalization is required. OnRepair, when set,
// is notified of each plan-repair attempt so the caller can surface the recovery.
type Options struct {
	Dir        string
	PlanSource string
	Agent      agent.Agent
	Store      TaskStore
	OnRepair   planner.RepairFunc
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
	// ReconciledTasks records the executed task IDs whose definitions a human
	// explicitly approved during reconciliation, so the approval is durable
	// provenance rather than an ephemeral one-line report. AutoReconciled records
	// the tasks an autonomy policy reconciled automatically, with the two definition
	// hashes and why, so a silent refresh stays auditable.
	ReconciledTasks []string         `json:"reconciled_tasks,omitempty"`
	AutoReconciled  []AutoReconciled `json:"auto_reconciled,omitempty"`
}

// Prepare ensures the project is ready to execute: it reconciles existing tasks,
// or resolves the source, builds the machine plan, validates it, and creates
// tasks. It is idempotent. When existing tasks belong to a completed plan and a
// different plan is requested, it hands off: the completed plan is preserved and
// the requested plan initializes in the same invocation.
func Prepare(ctx context.Context, opts Options) (Result, error) {
	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)
	metaPath := filepath.Join(opts.Dir, config.DirName, metaFileName)

	tasks, err := opts.Store.List()
	if err != nil {
		return Result{}, err
	}
	if len(tasks) > 0 {
		return reconcileState(ctx, opts, planPath, metaPath, tasks)
	}
	return initialize(ctx, opts, planPath, metaPath)
}

// initialize resolves the requested source, builds and validates the machine
// plan, and creates the task graph. It is the path taken when no tasks exist,
// including immediately after a completed-plan handoff has released the
// previous graph.
func initialize(ctx context.Context, opts Options, planPath, metaPath string) (Result, error) {
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
// requested plan matches what the task graph was built from; it hands off when
// the active plan is complete and a different plan is requested; and otherwise
// stops with an actionable NEEDS_HUMAN error rather than mixing plans or
// discarding history.
func reconcileState(ctx context.Context, opts Options, planPath, metaPath string, tasks []*domain.Task) (Result, error) {
	res := Result{SourceKind: KindExisting}
	meta := readMetadata(metaPath)

	docPath, docKind := requestedSource(opts)

	// A plain `sop run` (no plan named) whose persisted active plan still has
	// unresolved work owns the run: reconcile against that plan's own source
	// instead of discovering an unrelated PLAN.md and reporting a spurious
	// "different plan is active". An explicitly named plan stays authoritative,
	// and a completed active plan keeps the discovery behavior that permits safe
	// handoff.
	if strings.TrimSpace(opts.PlanSource) == "" && meta.Source != "" && !domain.AllSatisfied(tasks) {
		docPath = filepath.Join(opts.Dir, meta.Source)
		docKind = meta.SourceKind
		if docKind == "" {
			docKind = KindPlan
		}
		if !fileExists(docPath) {
			// The active plan's source is gone; resume the persisted graph rather
			// than guessing at another plan.
			res.Source = meta.Source
			res.SourceKind = docKind
			res.PlanID = meta.PlanID
			return res, nil
		}
	}

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

	switch {
	case meta.Source == "":
		// No recorded provenance (for example after `sop plan` + `sop tasks`):
		// reuse rather than block.
		return res, nil
	case meta.Source == rel && meta.SourceSHA256 == fingerprint(data):
		return res, nil // same plan, same content
	case meta.Source == rel:
		return res, planChangedError(rel)
	case !domain.AllSatisfied(tasks):
		// A different plan is requested while the active plan still has
		// unresolved work: keep the safety gate.
		return res, differentPlanError(rel, meta.Source, meta.PlanID)
	default:
		// The active plan is complete: release it and initialize the requested
		// plan in the same invocation.
		return handOff(ctx, opts, meta, tasks, data, docPath, docKind, rel, planPath, metaPath)
	}
}

// handOff releases a completed active plan so an explicitly requested different
// plan can initialize in the same invocation. The requested plan is built and
// validated before anything is mutated, so a plan that cannot initialize leaves
// the completed plan's record intact.
func handOff(ctx context.Context, opts Options, meta Metadata, tasks []*domain.Task, data []byte, docPath, docKind, rel, planPath, metaPath string) (Result, error) {
	res := Result{Source: rel, SourceKind: docKind, PlanID: planID(rel), PlanRebuilt: true}

	plan, err := buildPlan(ctx, opts.Agent, opts.OnRepair, docKind, string(data))
	if err != nil {
		if errors.Is(err, errNoAgent) {
			return res, err
		}
		return res, planError(rel, err)
	}
	if err := plan.Validate(); err != nil {
		return res, planError(rel, err)
	}

	// The requested plan is viable: preserve the completed plan, then release
	// its active association so the new graph can be created without mixing.
	if _, err := archivePlan(opts.Dir, meta, tasks, planPath, metaPath, DispositionComplete); err != nil {
		return res, err
	}
	if err := opts.Store.ClearTasks(); err != nil {
		return res, err
	}

	docWritten, err := persistPlan(opts, plan, docPath, docKind, data, planPath, metaPath)
	if err != nil {
		return res, err
	}
	res.PlanDoc = docWritten

	created, err := ensureTasks(rel, plan, opts.Store)
	if err != nil {
		return res, err
	}
	res.TasksCreated = created
	return res, nil
}

// Plan archive dispositions. A plan reaches its archive either COMPLETE (its work
// finished and a new plan took over) or SUPERSEDED (the operator intentionally
// replaced it while work remained). Recording the disposition keeps a historical
// plan unambiguous evidence: it is never runnable work, and a supersession is never
// mistaken for a completion or an approval.
const (
	DispositionComplete   = "COMPLETE"
	DispositionSuperseded = "SUPERSEDED"
)

// ActivePlanID reads the recorded active plan's id and source, or ("", false) when
// none is recorded. It is the one canonical active-plan reference: SOP records it
// beside the machine plan, so lifecycle state never depends on scanning Markdown.
func ActivePlanID(dir string) (id, source string, ok bool) {
	meta := readMetadata(filepath.Join(dir, config.DirName, metaFileName))
	id = strings.TrimSpace(meta.PlanID)
	source = strings.TrimSpace(meta.Source)
	if id == "" && source == "" {
		return "", "", false
	}
	return id, source, true
}

// Lifecycle records a plan's terminal disposition, written into its archive beside
// the task-record snapshot and provenance.
type Lifecycle struct {
	PlanID      string    `json:"plan_id"`
	Source      string    `json:"source,omitempty"`
	Disposition string    `json:"disposition"`
	RecordedAt  time.Time `json:"recorded_at"`
}

// archivePlan preserves a plan's task records, provenance, and terminal
// disposition under DirName/archive/<plan-id>/ before its active association is
// released, so the previous plan's history stays readable and its lifecycle state
// stays unambiguous. It returns the archive directory it wrote.
func archivePlan(dir string, meta Metadata, tasks []*domain.Task, planPath, metaPath, disposition string) (string, error) {
	id := strings.TrimSpace(meta.PlanID)
	if id == "" {
		id = "plan"
	}
	root := filepath.Join(dir, config.DirName, archiveDirName, id)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}

	for _, file := range []struct{ src, dst string }{
		{planPath, filepath.Join(root, planFileName)},
		{metaPath, filepath.Join(root, metaFileName)},
	} {
		data, err := os.ReadFile(file.src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := atomicWrite(file.dst, data); err != nil {
			return "", err
		}
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return "", fmt.Errorf("planflow: encode archived tasks: %w", err)
	}
	if err := atomicWrite(filepath.Join(root, archivedTasksFile), append(data, '\n')); err != nil {
		return "", err
	}

	life := Lifecycle{
		PlanID:      meta.PlanID,
		Source:      meta.Source,
		Disposition: disposition,
		RecordedAt:  time.Now().UTC(),
	}
	lifeDoc, err := json.MarshalIndent(life, "", "  ")
	if err != nil {
		return "", fmt.Errorf("planflow: encode plan lifecycle: %w", err)
	}
	if err := atomicWrite(filepath.Join(root, lifecycleFile), append(lifeDoc, '\n')); err != nil {
		return "", err
	}
	return root, nil
}

// --- Explicit reconciliation ---------------------------------------------

// GraphStore is the persistence Reconcile needs: the active task graph and an
// atomic operation to apply a reconciled set.
type GraphStore interface {
	List() ([]*domain.Task, error)
	ReplaceGraph(tasks []*domain.Task, remove []string) error
}

// ReconcileOptions configures Reconcile. PlanSource is the absolute path of the
// requested PLAN document and is required. Agent is not used by the
// reconciliation compile path: a changed source is compiled deterministically by
// the structured Markdown parser, and a changed source that is not recognizably
// structured fails with a stable diagnostic instead of being normalized by a
// model. Initial free-form planning (Prepare/handOff from a PRD) still uses it.
type ReconcileOptions struct {
	Dir        string
	PlanSource string
	Agent      agent.Agent
	Store      GraphStore
	OnRepair   planner.RepairFunc
	// AcceptChanged names executed tasks whose changed definition the human has
	// explicitly approved for replacement. Their history and lifecycle state are
	// preserved; only the definition is refreshed. An ID that does not name an
	// executed task whose definition changed is rejected.
	AcceptChanged []string
	// AutoAcceptExecuted, when non-nil, is consulted for each executed task whose
	// definition changed and that no explicit approval covers. It returns the
	// autonomy decision for the change; when the decision is ActionAutoReconcile the
	// definition is refreshed (preserving history) and the decision is recorded as
	// provenance. A nil callback keeps the explicit-approval behavior, so a caller
	// that does not opt in is unaffected.
	AutoAcceptExecuted func(ExecutedChange) autonomy.Decision
}

// ExecutedChange describes an executed task whose definition changed, offered to
// ReconcileOptions.AutoAcceptExecuted. Equivalent is true when only descriptive
// text changed and the executable semantics (acceptance criteria, execution mode,
// dependencies) are unchanged. Before and After are stable hashes of the two
// definitions, for provenance.

type ExecutedChange struct {
	TaskID     string
	Equivalent bool
	Before     string
	After      string
}

// AutoReconciled records one automatic reconciliation: the provenance a silent
// replacement must leave behind (the two definition hashes, the classification,
// the risk, and the reason). Validation/review are required afterward because the
// task's definition changed.

type AutoReconciled struct {
	TaskID         string `json:"task_id"`
	Before         string `json:"before"`
	After          string `json:"after"`
	Classification string `json:"classification"`
	Risk           string `json:"risk"`
	Reason         string `json:"reason"`
}

// ReconcileResult reports what an explicit reconciliation did. The slices hold
// task IDs.
type ReconcileResult struct {
	Source      string // relative path of the requested plan
	PlanID      string
	PlanChanged bool // the requested plan differs from the recorded machine plan
	Unchanged   []string
	Updated     []string
	Added       []string
	Removed     []string

	// ChangedExecuted lists executed tasks whose definition changed and that the
	// requested plan would replace only with an explicit approval. The applying path
	// stops on these; the read-only listing (Inspect) reports them instead, so a
	// client can name every changed task before anything is mutated.
	ChangedExecuted []string
	// ChangedExecutedEquivalent and ChangedExecutedMaterial partition ChangedExecuted by
	// whether the change is semantics-preserving (only descriptive text differs: the
	// acceptance criteria, execution mode, and dependencies are unchanged) or material
	// (a real executable-semantics change). They are populated only by the read-only
	// Inspect path, so a client can tell "refresh the definition, meaning preserved"
	// apart from "a human must review the new meaning" without re-deriving the
	// comparison. They are nil on the applying path, which stops on the first
	// unapproved change rather than partitioning it.
	ChangedExecutedEquivalent []string
	ChangedExecutedMaterial   []string
	// RemovedExecuted lists executed tasks the requested plan drops. They cannot be
	// approved with AcceptChanged: a removed executed task must be kept or completed
	// first, so the listing reports them rather than offering an approval.
	RemovedExecuted []string
	// Accepted names executed tasks whose changed definition the human approved
	// with --accept-changed and that were replaced by the requested definition.
	Accepted []string
	// AutoReconciled records the tasks reconciled automatically under the autonomy
	// policy, with their provenance. Their history is preserved.
	AutoReconciled []AutoReconciled
}

// reconcilePlan is the in-memory outcome of diffing a requested plan against the
// active graph: the tasks to upsert, the IDs to remove, and the classification
// used for reporting.
type reconcilePlan struct {
	unchanged               []string
	updated                 []string
	added                   []string
	removed                 []string
	accepted                []string
	autoReconciled          []AutoReconciled
	changedExecuted         []string
	changedExecutedEquiv    []string
	changedExecutedMaterial []string
	removedExecuted         []string
	upserts                 []*domain.Task
}

// reconcileGraphMode selects how a diff disposes of a task that needs an explicit
// human decision. The two modes share one diff, so a read-only listing can never
// disagree with the reconciliation it previews.
type reconcileGraphMode int

const (
	// reconcileApply mutates: an unapproved changed executed task, an executed task
	// the plan removes, or an approval that names neither, is an error.
	reconcileApply reconcileGraphMode = iota
	// reconcileInspect only reports: those categories are returned in the result
	// instead of stopping the call, and nothing is mutated.
	reconcileInspect
)

// Reconcile applies an intentional PLAN change to the persisted active plan. It
// compiles and validates the requested plan deterministically, diffs its
// executable task definitions against the active graph, and updates only tasks
// that have never executed. A task with execution history whose definition
// changed, or that the requested plan removes, stops with NEEDS_HUMAN: SOP never
// silently overwrites a task's definition or discards its history. The one
// exception is a task named in AcceptChanged, which the human has explicitly
// approved: its definition is refreshed while its lifecycle state, attempts, and
// history are preserved.
//
// It never deletes or recreates state.db and never clears the graph wholesale.
// The reconciled graph is persisted first; the machine plan and its provenance
// are written afterwards. A validation or graph-persistence failure therefore
// leaves the previous active graph and its metadata authoritative. The reverse
// window, where the graph commits but a later file write fails, cannot be closed
// without a shared journal across SQLite and the files. The graph-leading order
// is deliberate: a stale metadata file only makes the next `sop run` re-prompt
// for a reconciliation, whereas writing the files first could let that guard pass
// while the graph still held the old definitions. A retry once the file write
// succeeds completes idempotently.
func Reconcile(ctx context.Context, opts ReconcileOptions) (ReconcileResult, error) {
	res, diff, plan, data, err := reconcileDiff(ctx, opts, reconcileApply)
	if err != nil {
		return res, err
	}

	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)
	metaPath := filepath.Join(opts.Dir, config.DirName, metaFileName)

	if len(diff.upserts) > 0 || len(diff.removed) > 0 {
		if err := opts.Store.ReplaceGraph(diff.upserts, diff.removed); err != nil {
			return res, err
		}
	}

	// Provenance is refreshed only after the reconciled graph is persisted, so a
	// failed persistence leaves the previous active graph and metadata
	// authoritative. Any previously recorded reconciliation is carried forward so
	// the human-approval record survives a later no-op rewrite of this file.
	if err := writePlan(planPath, plan); err != nil {
		return res, err
	}
	// Read the previous provenance once, so the human-approval and automatic-
	// reconciliation records are both carried forward.
	prev := readMetadata(metaPath)
	if err := writeMetadata(metaPath, Metadata{
		Source:          res.Source,
		SourceKind:      KindPlan,
		SourceSHA256:    fingerprint(data),
		PlanID:          res.PlanID,
		GeneratedAt:     time.Now().UTC(),
		ReconciledTasks: mergeIDs(prev.ReconciledTasks, diff.accepted),
		AutoReconciled:  mergeAutoReconciled(prev.AutoReconciled, diff.autoReconciled),
	}); err != nil {
		return res, err
	}

	return res, nil
}

// Inspect computes what Reconcile would do without changing anything. It compiles
// and validates the requested plan, diffs it against the active graph, and reports
// every category - including the tasks that would stop a reconciliation for an
// explicit human decision (ChangedExecuted) or that cannot be reconciled at all
// (RemovedExecuted). It writes no graph, no machine plan, and no metadata, so it is
// the read-only listing a client uses to show a human what a reconciliation would
// change before anything is applied.
func Inspect(ctx context.Context, opts ReconcileOptions) (ReconcileResult, error) {
	res, _, _, _, err := reconcileDiff(ctx, opts, reconcileInspect)
	return res, err
}

// reconcileDiff is the shared first half of a reconciliation: it compiles and
// validates the requested plan, diffs it against the active graph, and returns the
// classification, the in-memory graph change, the compiled plan, and the source
// bytes. mode decides only how a task needing an explicit human decision is
// disposed of (an error when applying, a report when inspecting), so the listing
// and the reconciliation are one diff and can never disagree.
//
// Compilation on this path is deliberately deterministic: the requested plan is
// compiled by the structured Markdown parser/Planner with NO agent, so a model is
// never called to synthesize or normalize definitions just for a reconciliation. A
// changed source that is not recognizably structured fails with
// ErrUnsupportedReconcileSource before any mutation. Initial free-form planning
// (Prepare/handOff) still uses the configured model normalizer; only reconciliation
// is restrictive. It honors ctx.Err() before reading or mutating anything, so a
// canceled reconciliation reports the cancellation and changes no state.
func reconcileDiff(ctx context.Context, opts ReconcileOptions, mode reconcileGraphMode) (ReconcileResult, reconcilePlan, *planner.Plan, []byte, error) {
	res := ReconcileResult{}
	if err := ctx.Err(); err != nil {
		return res, reconcilePlan{}, nil, nil, err
	}
	if strings.TrimSpace(opts.PlanSource) == "" {
		return res, reconcilePlan{}, nil, nil, errors.New("reconcile: a plan path is required")
	}

	data, err := os.ReadFile(opts.PlanSource)
	if err != nil {
		return res, reconcilePlan{}, nil, nil, fmt.Errorf("reconcile: read %s: %w", opts.PlanSource, err)
	}
	rel := relOf(opts.Dir, opts.PlanSource)
	res.Source = rel
	res.PlanID = planID(rel)

	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)

	// The comparison baseline is the recorded machine plan, which the active task
	// graph mirrors. It must exist: an explicit reconciliation only makes sense
	// against an active plan.
	recorded, ok := loadPersistedPlan(planPath)
	if !ok {
		return res, reconcilePlan{}, nil, nil, fmt.Errorf("NEEDS_HUMAN: no persisted plan to reconcile against\n\n%s is missing or unreadable. Run `sop run` to establish the active plan first", filepath.Join(config.DirName, planFileName))
	}

	// An unchanged active source is a deterministic no-op. SOP must not regenerate
	// the machine plan merely to compare it with itself: plan generation is not
	// guaranteed byte-stable, so re-deriving an unchanged plan can report a spurious
	// plan_changed. Reconciliation exists to apply a source CHANGE, so when the
	// requested source is the active plan's recorded source and its content hash is
	// unchanged, reuse the recorded plan and report nothing to change.
	// --accept-changed is a human decision about a CHANGED definition, so an approval
	// request always takes the full path (which validates it); the no-op shortcut
	// applies only when no approval is requested.
	meta := readMetadata(filepath.Join(opts.Dir, config.DirName, metaFileName))
	if len(opts.AcceptChanged) == 0 && meta.Source == rel && meta.SourceSHA256 == fingerprint(data) {
		active, aerr := opts.Store.List()
		if aerr != nil {
			return res, reconcilePlan{}, nil, nil, aerr
		}
		unchanged := make([]string, 0, len(active))
		for _, t := range active {
			unchanged = append(unchanged, t.ID)
		}
		sort.Strings(unchanged)
		res.PlanChanged = false
		res.Unchanged = unchanged
		return res, reconcilePlan{unchanged: unchanged}, recorded, data, nil
	}

	plan, err := compileReconcileSource(string(data))
	if err != nil {
		return res, reconcilePlan{}, nil, nil, planError(rel, err)
	}
	res.PlanChanged = !plansEquivalent(recorded, plan)

	desired, err := desiredTasks(plan)
	if err != nil {
		return res, reconcilePlan{}, nil, nil, planError(rel, err)
	}

	active, err := opts.Store.List()
	if err != nil {
		return res, reconcilePlan{}, nil, nil, err
	}

	// A listing is independent of any approval: it reports what would need one.
	accept := opts.AcceptChanged
	if mode == reconcileInspect {
		accept = nil
	}
	diff, err := reconcileGraph(rel, active, desired, acceptSet(accept), opts.AutoAcceptExecuted, mode)
	if err != nil {
		return res, reconcilePlan{}, nil, nil, err
	}

	res.Unchanged = diff.unchanged
	res.Updated = diff.updated
	res.Added = diff.added
	res.Removed = diff.removed
	res.Accepted = diff.accepted
	res.AutoReconciled = diff.autoReconciled
	res.ChangedExecuted = diff.changedExecuted
	res.ChangedExecutedEquivalent = diff.changedExecutedEquiv
	res.ChangedExecutedMaterial = diff.changedExecutedMaterial
	res.RemovedExecuted = diff.removedExecuted
	return res, diff, plan, data, nil
}

// compileReconcileSource compiles a changed reconciliation source deterministically:
// it uses the structured Markdown parser/Planner with NO agent, so no model is ever
// called to synthesize or normalize definitions for a reconciliation. A document the
// parser recognizes as a plan is authoritative and is validated in place; then the
// same missing-capability ownership gate the planner applies (acceptPlan) runs here,
// so a MISSING/PARTIAL required capability with neither an owner nor a resolution
// stops as NEEDS_HUMAN instead of proceeding through reconciliation. A document that
// is not recognizably structured returns the stable, actionable
// ErrUnsupportedReconcileSource, leaving all authoritative state untouched.
//
// Parse and validation failures are returned raw (no source prefix here); the caller
// wraps them with planError so the diagnostic names the requested source Markdown
// rather than SOP-owned plan.json. The caller must have already handled the
// unchanged-source no-op and the persisted-plan baseline; that no-op path never
// reaches here.
func compileReconcileSource(content string) (*planner.Plan, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("plan document is empty")
	}
	parsed, err := planner.PlanFromMarkdown(content)
	if err != nil {
		return nil, err
	}
	if parsed == nil || len(parsed.Stages) == 0 {
		return nil, ErrUnsupportedReconcileSource
	}
	if err := parsed.Validate(); err != nil {
		return nil, err
	}
	// The ownership gate: a non-EXISTS capability some stage requires, with neither
	// an owner nor a resolution, is a genuine human decision. It must be enforced on
	// the reconciliation path exactly as planner.acceptPlan enforces it on the
	// initial build path, so a missing capability cannot slip through reconciliation.
	if _, needsHuman := parsed.CapabilityGaps(); len(needsHuman) > 0 {
		return nil, &planner.CapabilityGapError{Capabilities: needsHuman}
	}
	return parsed, nil
}

// desiredTasks builds the executable task definitions for a plan, wiring in the
// environment bootstrap dependency and validating the resulting graph. It is the
// same deterministic construction used when tasks are first created.
func desiredTasks(plan *planner.Plan) ([]*domain.Task, error) {
	tasks, err := taskbuilder.Build(plan)
	if err != nil {
		return nil, err
	}
	if err := bootstrap.Apply(plan, tasks); err != nil {
		return nil, err
	}
	if err := taskbuilder.ValidateDAG(tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// reconcileGraph diffs the requested task definitions against the active graph.
// A task that already matches is left untouched. A changed task with execution
// history stops with NEEDS_HUMAN unless its ID is in accept (a human approval) or
// autoAccept authorizes the refresh; in either case only its definition is
// refreshed and its history is preserved. A removed task with execution history
// always stops with NEEDS_HUMAN. An accepted ID that does not name an executed
// task whose definition changed is rejected, as is any changed executed task left
// unapproved. The reconciled graph is validated as a whole before being returned,
// so a change that would leave a dangling dependency is rejected up front rather
// than mid-mutation.
//
// mode decides the disposition of the categories that need a human: reconcileApply
// stops the call (an unapproved changed executed task, an executed task the plan
// removes, or an approval naming neither is an error), while reconcileInspect
// reports them in the result and mutates nothing.
func reconcileGraph(source string, active, desired []*domain.Task, accept map[string]bool, autoAccept func(ExecutedChange) autonomy.Decision, mode reconcileGraphMode) (reconcilePlan, error) {
	var out reconcilePlan

	// kept holds executed tasks whose current definition survives because an
	// explicit human decision is outstanding (reconcileInspect). It keeps the
	// graph validation honest: the task still exists, so a dependency on it is not
	// dangling. It is always empty when applying, because those paths stop instead.
	var kept []*domain.Task

	activeTasks := taskIndex(active)
	desiredByID := taskIndex(desired)
	desiredIDs := make(map[string]bool, len(desired))
	for _, d := range desired {
		desiredIDs[d.ID] = true
	}

	// A changed task that already executed is replaced only when its ID is an
	// explicit approval or the autonomy policy authorizes it; otherwise it is
	// reported after every accepted ID has been checked, so an unrelated approval
	// is rejected first.
	reconciledExecuted := make(map[string]bool) // replaced (explicitly or automatically)
	changedExecuted := make(map[string]bool)    // still need an explicit approval
	for _, d := range desired {
		task, ok := activeTasks[d.ID]
		if !ok {
			out.upserts = append(out.upserts, d)
			out.added = append(out.added, d.ID)
			continue
		}
		if sameTaskDefinition(task, d) {
			out.unchanged = append(out.unchanged, d.ID)
			continue
		}
		if hasExecution(task) {
			if accept[d.ID] {
				out.upserts = append(out.upserts, redefineTask(task, d))
				out.accepted = append(out.accepted, d.ID)
				reconciledExecuted[d.ID] = true
				continue
			}
			// A changed executed task is a candidate for automatic reconciliation when
			// the autonomy policy authorizes it; otherwise it needs an explicit approval.
			if rec, ok := autoReconcile(task, d, autoAccept); ok {
				out.upserts = append(out.upserts, redefineTask(task, d))
				out.autoReconciled = append(out.autoReconciled, rec)
				reconciledExecuted[d.ID] = true
				continue
			}
			if mode == reconcileInspect {
				// A listing reports the changed executed task instead of stopping, and
				// keeps its current definition in the graph it validates. It is classified
				// as semantics-preserving (only descriptive text changed) or material (a
				// real executable-semantics change) so a read-only consumer can decide
				// whether a refresh preserves meaning.
				out.changedExecuted = append(out.changedExecuted, d.ID)
				if equivalentExecutedChange(task, d) {
					out.changedExecutedEquiv = append(out.changedExecutedEquiv, d.ID)
				} else {
					out.changedExecutedMaterial = append(out.changedExecutedMaterial, d.ID)
				}
				kept = append(kept, task)
				continue
			}
			changedExecuted[d.ID] = true
			continue
		}
		out.upserts = append(out.upserts, redefineTask(task, d))
		out.updated = append(out.updated, d.ID)
	}

	// Every accepted ID must name an executed task whose definition changed in the
	// requested plan (whether the human approved it or the policy reconciled it), so
	// one approval can never silently cover an unrelated task.
	for _, id := range sortedSet(accept) {
		if reconciledExecuted[id] {
			continue
		}
		return reconcilePlan{}, acceptChangedError(id, activeTasks, desiredByID)
	}

	// Any changed executed task left unapproved needs its own explicit approval:
	// one approval never covers another task.
	if unapproved := missingAccept(changedExecuted, accept); len(unapproved) > 0 {
		cur := unapproved[0]
		return reconcilePlan{}, changedExecutedError(activeTasks[cur], desiredByID[cur], unapproved[1:])
	}

	for _, id := range sortedIDs(activeTasks) {
		if desiredIDs[id] {
			continue
		}
		task := activeTasks[id]
		if hasExecution(task) {
			if mode == reconcileInspect {
				// A removed executed task cannot be approved with --accept-changed: SOP
				// never discards history. The listing reports it; the task stays in the
				// graph it validates.
				out.removedExecuted = append(out.removedExecuted, id)
				kept = append(kept, task)
				continue
			}
			return reconcilePlan{}, removedExecutedError(task)
		}
		out.removed = append(out.removed, id)
	}

	// Validate the reconciled graph before any mutation, catching a surviving task
	// whose dependency the requested plan drops.
	final := make([]*domain.Task, 0, len(out.unchanged)+len(kept)+len(out.upserts))
	for _, id := range out.unchanged {
		final = append(final, activeTasks[id])
	}
	final = append(final, kept...)
	final = append(final, out.upserts...)
	if err := taskbuilder.ValidateDAG(final); err != nil {
		return reconcilePlan{}, planError(source, err)
	}

	return out, nil
}

// redefineTask returns cur carrying d's definition while preserving everything
// that records the task's identity and history (its ID, lifecycle state,
// attempts, budget, and creation time). It backs both the silent update of a
// task that has never executed and the explicit, human-approved update of an
// executed task, so neither loses history or changes state.
func redefineTask(cur, d *domain.Task) *domain.Task {
	updated := *cur
	updated.Title = d.Title
	updated.Objective = d.Objective
	updated.AcceptanceCriteria = d.AcceptanceCriteria
	updated.ExecutionMode = d.ExecutionMode
	updated.DependencyIDs = append([]string{}, d.DependencyIDs...)
	updated.UpdatedAt = time.Now().UTC()
	return &updated
}

// hasExecution reports whether authoritative lifecycle evidence shows the task's
// execution began. Only a task that has never executed may have its definition
// replaced or be removed. It defers to domain.Task.Executed so the classification
// is defined once: a READY task that has never run is NOT executed, so a READY task
// with a changed definition is a safe update and a removed READY task needs no
// approval.
func hasExecution(t *domain.Task) bool {
	return t.Executed()
}

// autoReconcile consults the autonomy callback for a changed executed task. It
// reports a provenance record and true only when the callback authorizes an
// automatic reconciliation. A nil callback authorizes nothing, so a caller that
// does not opt in keeps the explicit-approval behavior.
func autoReconcile(cur, desired *domain.Task, autoAccept func(ExecutedChange) autonomy.Decision) (AutoReconciled, bool) {
	if autoAccept == nil {
		return AutoReconciled{}, false
	}
	change := ExecutedChange{
		TaskID:     cur.ID,
		Equivalent: equivalentExecutedChange(cur, desired),
		Before:     definitionHash(cur),
		After:      definitionHash(desired),
	}
	decision := autoAccept(change)
	if decision.Action != autonomy.ActionAutoReconcile {
		return AutoReconciled{}, false
	}
	classification := autonomy.PlanChangeExecutedMaterial
	if change.Equivalent {
		classification = autonomy.PlanChangeExecutedEquivalent
	}
	return AutoReconciled{
		TaskID:         change.TaskID,
		Before:         change.Before,
		After:          change.After,
		Classification: string(classification),
		Risk:           string(decision.Risk),
		Reason:         decision.Reason,
	}, true
}

// definitionHash is a stable hash of the fields a reconciliation replaces, so the
// provenance records exactly which two definitions were swapped.
func definitionHash(t *domain.Task) string {
	sum := sha256.New()
	for _, part := range []string{t.ID, t.Title, t.Objective, t.AcceptanceCriteria, string(t.ExecutionMode)} {
		_, _ = io.WriteString(sum, part)
		sum.Write([]byte{0})
	}
	deps := append([]string(nil), t.DependencyIDs...)
	sort.Strings(deps)
	_, _ = io.WriteString(sum, strings.Join(deps, ","))
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// mergeAutoReconciled folds the current reconciliation's automatic records into
// the previously recorded ones, keeping the latest record per task and the
// first-seen order, so repeated reconciliations accumulate provenance instead of
// discarding earlier history.
func mergeAutoReconciled(prev, cur []AutoReconciled) []AutoReconciled {
	if len(prev) == 0 && len(cur) == 0 {
		return nil
	}
	latest := make(map[string]AutoReconciled, len(prev)+len(cur))
	order := make([]string, 0, len(prev)+len(cur))
	for _, r := range append(append([]AutoReconciled(nil), prev...), cur...) {
		if _, seen := latest[r.TaskID]; !seen {
			order = append(order, r.TaskID)
		}
		latest[r.TaskID] = r
	}
	out := make([]AutoReconciled, 0, len(order))
	for _, id := range order {
		out = append(out, latest[id])
	}
	return out
}

// taskIndex keys tasks by id.
func taskIndex(tasks []*domain.Task) map[string]*domain.Task {
	index := make(map[string]*domain.Task, len(tasks))
	for _, task := range tasks {
		index[task.ID] = task
	}
	return index
}

// sortedIDs returns the ids in index, sorted for deterministic reporting.
func sortedIDs(index map[string]*domain.Task) []string {
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// loadPersistedPlan reads the recorded machine plan. A missing, unreadable, or
// empty (no project, no stages) file counts as absent.
func loadPersistedPlan(path string) (*planner.Plan, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var plan planner.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, false
	}
	if strings.TrimSpace(plan.Project) == "" || len(plan.Stages) == 0 {
		return nil, false
	}
	return &plan, true
}

// changedExecutedError explains that a task with execution history changed and
// has not been explicitly approved. others names any further changed executed
// tasks that also require their own approval.
func changedExecutedError(cur, desired *domain.Task, others []string) error {
	msg := fmt.Sprintf("NEEDS_HUMAN: task %s already has execution history and its definition changed\n\n%s\n\nSOP will not silently replace the definition of an executed task.\nReview the change, then approve it explicitly with `sop reconcile <PLAN.md> --accept-changed %s`, or complete %s first", cur.ID, describeTaskDifference(cur, desired), cur.ID, cur.ID)
	if len(others) > 0 {
		msg += fmt.Sprintf("\n\nOther changed executed tasks also need their own explicit approval: %s", strings.Join(others, ", "))
	}
	return errors.New(msg)
}

// acceptChangedError explains that an --accept-changed ID does not name an
// executed task whose definition changed, so approving it would be unrelated to
// the change being reconciled.
func acceptChangedError(id string, active, desired map[string]*domain.Task) error {
	switch {
	case active[id] == nil && desired[id] == nil:
		return fmt.Errorf("reconcile: --accept-changed %s does not name a task in the active or requested plan", id)
	case active[id] == nil:
		return fmt.Errorf("reconcile: --accept-changed %s names a task that is not in the active plan; only an executed task whose definition changed can be approved", id)
	case desired[id] == nil:
		return fmt.Errorf("reconcile: --accept-changed %s names a task the requested plan removes; removing an executed task cannot be approved with --accept-changed", id)
	default:
		return fmt.Errorf("reconcile: --accept-changed %s does not name an executed task whose definition changed", id)
	}
}

// acceptSet returns the approved IDs as a set, ignoring blanks so a stray
// separator does not become a task name.
func acceptSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	return set
}

// missingAccept returns, sorted, the changed-executed IDs that accept does not
// cover.
func missingAccept(changedExecuted, accept map[string]bool) []string {
	var missing []string
	for id := range changedExecuted {
		if !accept[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	return missing
}

// sortedSet returns the keys of a string set in sorted order.
func sortedSet(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// mergeIDs returns the sorted, de-duplicated union of prior and added, so a
// recorded human-approval survives a later no-op rewrite of the metadata.
func mergeIDs(prior, added []string) []string {
	seen := make(map[string]bool, len(prior)+len(added))
	for _, id := range prior {
		seen[id] = true
	}
	for _, id := range added {
		seen[id] = true
	}
	return sortedSet(seen)
}

// removedExecutedError explains that a task with execution history was dropped.
func removedExecutedError(t *domain.Task) error {
	return fmt.Errorf("NEEDS_HUMAN: task %s has execution history but is absent from the requested plan\n\nSOP will not discard an executed task's history by removing it.\nComplete or retire %s before reconciling", t.ID, t.ID)
}

// describeTaskDifference lists the fields that differ between a task's persisted
// definition and the requested one, so the human sees exactly what changed.
func describeTaskDifference(cur, desired *domain.Task) string {
	var lines []string
	if canonicalProse(cur.Title) != canonicalProse(desired.Title) {
		lines = append(lines, fmt.Sprintf("  title: %q -> %q", cur.Title, desired.Title))
	}
	if canonicalProse(cur.Objective) != canonicalProse(desired.Objective) {
		lines = append(lines, "  objective changed")
	}
	if !sameAcceptanceCriteria(splitCriteria(cur.AcceptanceCriteria), splitCriteria(desired.AcceptanceCriteria)) {
		lines = append(lines, "  acceptance criteria changed")
	}
	if !sameExecutionMode(cur.ExecutionMode, desired.ExecutionMode) {
		lines = append(lines, fmt.Sprintf("  execution mode: %q -> %q", cur.ExecutionMode, desired.ExecutionMode))
	}
	if !sameStringSet(cur.DependencyIDs, desired.DependencyIDs) {
		lines = append(lines, fmt.Sprintf("  dependencies: [%s] -> [%s]", strings.Join(cur.DependencyIDs, ", "), strings.Join(desired.DependencyIDs, ", ")))
	}
	if len(lines) == 0 {
		lines = append(lines, "  definition changed")
	}
	return strings.Join(lines, "\n")
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

	plan, err := buildPlan(ctx, opts.Agent, opts.OnRepair, docKind, string(data))
	if err != nil {
		if errors.Is(err, errNoAgent) {
			return nil, false, "", err
		}
		return nil, false, "", planError(rel, err)
	}
	if err := plan.Validate(); err != nil {
		return nil, false, "", planError(rel, err)
	}
	docWritten, err := persistPlan(opts, plan, docPath, docKind, data, planPath, metaPath)
	if err != nil {
		return nil, false, "", err
	}
	return plan, true, docWritten, nil
}

// persistPlan writes the machine plan, its provenance, and (when generating from
// a PRD) the human-readable plan document. It is shared by the initial build and
// by completed-plan handoff.
func persistPlan(opts Options, plan *planner.Plan, docPath, docKind string, data []byte, planPath, metaPath string) (string, error) {
	rel := relOf(opts.Dir, docPath)
	if err := writePlan(planPath, plan); err != nil {
		return "", err
	}
	_ = writeMetadata(metaPath, Metadata{
		Source:       rel,
		SourceKind:   docKind,
		SourceSHA256: fingerprint(data),
		PlanID:       planID(rel),
		GeneratedAt:  time.Now().UTC(),
	})
	if docKind == KindPRD {
		return writeGeneratedPlanDoc(opts.Dir, docPath, plan), nil
	}
	return "", nil
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

// buildPlan compiles a human PLAN.md or generates a plan from a PRD. The
// repair observer is attached to the planner so generated plans that fail
// deterministic validation are corrected rather than failing the run.
func buildPlan(ctx context.Context, a agent.Agent, onRepair planner.RepairFunc, kind, content string) (*planner.Plan, error) {
	p := planner.New(a)
	if onRepair != nil {
		p.OnRepair(onRepair)
	}
	if kind == KindPlan {
		plan, err := p.Compile(ctx, content)
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
	return p.Generate(ctx, content)
}

// --- Explicit supersession --------------------------------------------------

// SupersedeResult reports what Supersede did.
type SupersedeResult struct {
	SupersededSource string // relative path of the plan that was active; "" when none
	SupersededPlanID string // plan id of the superseded plan; "" when none
	Source           string // relative path of the newly active plan
	PlanID           string // plan id of the newly active plan
	Archived         string // archive directory written for the superseded plan; "" when none
	TasksCreated     int    // tasks created for the newly active plan
}

// Supersede explicitly replaces the active plan with a validated new plan, even
// when the active plan still has unresolved work. It is the operator's intentional
// "abandon / replace" transition, distinct from completing a plan (handoff) and
// from the automatic refusal to switch away from unfinished work.
//
// The previous plan is archived as SUPERSEDED with its task records, run evidence,
// and approvals preserved verbatim; the new plan is installed ACTIVE with its first
// task READY. It fabricates nothing: an unfinished task stays unfinished, a FAILED
// task stays FAILED, and a NEEDS_HUMAN record is historical evidence, never an
// approval. It never executes a task and never runs the lifecycle for one.
func Supersede(ctx context.Context, opts Options) (SupersedeResult, error) {
	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)
	metaPath := filepath.Join(opts.Dir, config.DirName, metaFileName)

	docPath, docKind := requestedSource(opts)
	if strings.TrimSpace(docPath) == "" {
		return SupersedeResult{}, errors.New("supersede: a plan path is required")
	}
	data, err := os.ReadFile(docPath)
	if err != nil {
		return SupersedeResult{}, fmt.Errorf("planflow: read %s: %w", docPath, err)
	}
	rel := relOf(opts.Dir, docPath)
	res := SupersedeResult{Source: rel, PlanID: planID(rel)}

	// Build and validate the requested plan BEFORE any mutation, so a plan that
	// cannot initialize leaves the active plan and its evidence intact.
	plan, err := buildPlan(ctx, opts.Agent, opts.OnRepair, docKind, string(data))
	if err != nil {
		if errors.Is(err, errNoAgent) {
			return res, err
		}
		return res, planError(rel, err)
	}
	if err := plan.Validate(); err != nil {
		return res, planError(rel, err)
	}

	// Preserve the currently active plan (if any) as SUPERSEDED, then release its
	// graph so the new plan can be created without mixing the two.
	meta := readMetadata(metaPath)
	active, err := opts.Store.List()
	if err != nil {
		return res, err
	}
	if len(active) > 0 || strings.TrimSpace(meta.Source) != "" || strings.TrimSpace(meta.PlanID) != "" {
		root, aerr := archivePlan(opts.Dir, meta, active, planPath, metaPath, DispositionSuperseded)
		if aerr != nil {
			return res, aerr
		}
		res.SupersededSource = meta.Source
		res.SupersededPlanID = meta.PlanID
		res.Archived = relOf(opts.Dir, root)
		if err := opts.Store.ClearTasks(); err != nil {
			return res, err
		}
	}

	// Install the new plan ACTIVE with its first task READY. No task is executed and
	// no model run is recorded.
	if _, err := persistPlan(opts, plan, docPath, docKind, data, planPath, metaPath); err != nil {
		return res, err
	}
	created, err := ensureTasks(rel, plan, opts.Store)
	if err != nil {
		return res, err
	}
	res.TasksCreated = created
	return res, nil
}

// CompleteResult reports what Complete did.
type CompleteResult struct {
	PlanID   string // stable identity of the completed plan
	Source   string // relative path of the plan's human source, if known
	Archived string // archive directory written, relative to the project
	Tasks    int    // task records preserved as history
}

// Complete archives the active plan as COMPLETE and releases its active association,
// without installing a successor. It is the operator's terminal "the plan's work is
// done" transition: distinct from handoff (which requires a successor plan to take
// over) and from supersede (which abandons unfinished work).
//
// It fails closed: it refuses unless the active plan's work is fully satisfied, so a
// plan with any unresolved task cannot be completed. It fabricates nothing — the
// archived records are the task snapshot verbatim — and it never executes a task or
// runs a model.
func Complete(opts Options) (CompleteResult, error) {
	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)
	metaPath := filepath.Join(opts.Dir, config.DirName, metaFileName)
	meta := readMetadata(metaPath)

	tasks, err := opts.Store.List()
	if err != nil {
		return CompleteResult{}, err
	}
	if len(tasks) == 0 && strings.TrimSpace(meta.Source) == "" && strings.TrimSpace(meta.PlanID) == "" {
		return CompleteResult{}, errors.New("complete: no active plan to complete")
	}
	if len(tasks) > 0 && !domain.AllSatisfied(tasks) {
		return CompleteResult{}, errors.New("complete: the active plan still has unresolved work; refusing to complete it")
	}

	root, err := archivePlan(opts.Dir, meta, tasks, planPath, metaPath, DispositionComplete)
	if err != nil {
		return CompleteResult{}, err
	}
	if err := opts.Store.ClearTasks(); err != nil {
		return CompleteResult{}, err
	}
	// Release the active association and remove the machine plan, so the completed
	// plan is no longer ACTIVE and cannot be re-activated by `sop tasks` reading a
	// stale plan.json.
	if err := writeMetadata(metaPath, Metadata{}); err != nil {
		return CompleteResult{}, err
	}
	if err := os.Remove(planPath); err != nil && !os.IsNotExist(err) {
		return CompleteResult{}, err
	}

	return CompleteResult{
		PlanID:   meta.PlanID,
		Source:   meta.Source,
		Archived: relOf(opts.Dir, root),
		Tasks:    len(tasks),
	}, nil
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

// ReportsDir is where SOP writes generated, human-readable artifacts, kept out of
// the project root. Change-detection excludes it (SOP's own output is not the
// task's change).
const ReportsDir = "docs/reports"

// writeGeneratedPlanDoc writes the human-readable plan generated from a PRD to
// docs/reports/<plan-id>.md and returns the relative path, or "". It never writes
// to the project root or beside the PRD.
func writeGeneratedPlanDoc(dir, prdPath string, plan *planner.Plan) string {
	id := planID(relOf(dir, prdPath))
	if id == "" {
		id = "plan"
	}
	target := filepath.Join(dir, ReportsDir, id+".md")
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
// recovery command. A capability-gap ambiguity is a genuine human decision, not a
// structural defect, so it is passed through unchanged rather than reframed as
// "fix the plan and rerun".
func planError(source string, err error) error {
	if planner.IsNeedsHuman(err) {
		return err
	}
	name := source
	if strings.TrimSpace(name) == "" {
		name = filepath.Join(config.DirName, planFileName)
	}
	detail := strings.TrimSpace(strings.TrimPrefix(err.Error(), "plan: "))
	return fmt.Errorf("Plan validation failed:\n\n  %s\n\nNo work was executed.\n\nFix %s and rerun:\n\n  sop run %s", detail, name, name)
}

// planChangedError explains that the recorded plan source changed.
func planChangedError(source string) error {
	return fmt.Errorf("NEEDS_HUMAN: plan changed since the task graph was created\n\nSource: %s\n\nSOP will not silently rebuild the machine plan or discard task\nexecution history. Review the change, reconcile it explicitly, then rerun:\n\n  sop reconcile %s\n  sop run %s", source, source, source)
}

// differentPlanError explains that a different plan is already active and the
// active plan still has unresolved work, so automatic handoff is refused.
func differentPlanError(requested, active, activeID string) error {
	if activeID == "" {
		activeID = "(unknown)"
	}
	return fmt.Errorf("NEEDS_HUMAN: a different plan is already active\n\nActive:    %s\nRequested: %s\n\nThe existing tasks belong to %s (%s), which still has unresolved\nwork. SOP will not mix two plans in one task graph, and it will not switch\naway from unfinished work. Complete or reconcile the active plan, then\nrerun:\n\n  sop run %s", active, requested, active, activeID, requested)
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
