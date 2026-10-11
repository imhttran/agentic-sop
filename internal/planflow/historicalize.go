package planflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/run"
)

// Historicalization is the deterministic, model-free lifecycle transition that
// moves a plan from ACTIVE into its historical record. It reuses the archive
// storage the plan lifecycle already owns (.agent-sdlc/archive/<plan-id>/), so it
// introduces no second source of truth.
//
// It NEVER rewrites a task outcome to make a plan appear complete. A normal
// historicalization (disposition COMPLETE) is refused unless every task is
// satisfied, no human approval gate is pending, and every satisfied task has
// verification evidence. An explicit disposition (SUPERSEDED) is the operator's
// intentional disposal of unfinished work: it preserves the truth (an unfinished
// task stays unfinished, a BLOCKED task stays BLOCKED) rather than fabricating a
// completion.
//
// The transition is idempotent (a second invocation reports
// ALREADY_HISTORICALIZED and changes nothing) and guarded against stale state (a
// plan mutated between readiness and mutation is refused). Because the plan's
// active association (the task graph plus .agent-sdlc/plan.json) and the archive
// are separate stores, the transition is staged — archive first, then release —
// and a re-run deterministically completes a partially released plan rather than
// claiming it historicalized while lifecycle records remain active.
const (
	// OutcomeHistoricalized is returned when the invocation performed the
	// transition (or completed a staged one).
	OutcomeHistoricalized = "HISTORICALIZED"
	// OutcomeAlreadyHistoricalized is returned when the plan was already
	// historicalized; the operation is a deterministic no-op.
	OutcomeAlreadyHistoricalized = "ALREADY_HISTORICALIZED"

	// Historicalization lifecycle states reported by readiness.
	HistoricalizationStateActive         = "ACTIVE"
	HistoricalizationStateHistoricalized = "HISTORICALIZED"
	HistoricalizationStateNone           = "NONE"
	HistoricalizationStateInconsistent   = "INCONSISTENT"
)

// Historicalization errors. They are deterministic and model-free; a caller
// distinguishes them with errors.Is.
var (
	ErrNoActivePlan                = errors.New("historicalize: no active plan to historicalize")
	ErrHistoricalizationIneligible = errors.New("historicalize: the active plan is not eligible for historicalization")
	ErrStaleHistoricalizationState = errors.New("historicalize: the active plan changed since readiness was evaluated; re-run readiness")
	ErrPartialHistoricalization    = errors.New("historicalize: the plan was archived but its active association was not fully released; re-run historicalization to complete it")
)

// runsDirName mirrors internal/run's run directory. Approvals and run evidence
// live under .agent-sdlc/runs/<task-id>/; internal/run owns the artifact names,
// and this package reads them through run.At/run.Load/run.Dir.
const runsDirName = "runs"

// HistoricalizationReadiness is the deterministic pre-mutation evaluation of a
// plan: the plan identity, its current lifecycle state, task counts by state, and
// the discrete blockers that make it ineligible. It is a pure read; it mutates
// nothing and consults no model.
type HistoricalizationReadiness struct {
	PlanID      string `json:"plan_id"`
	Source      string `json:"source,omitempty"`
	State       string `json:"state"`
	Disposition string `json:"disposition"`
	TotalTasks  int    `json:"total_tasks"`
	// TaskCounts is the number of tasks in each persisted status.
	TaskCounts map[string]int `json:"task_counts"`
	// UnresolvedTasks are tasks that have not reached a satisfied terminal state,
	// rendered as "ID (STATUS)".
	UnresolvedTasks []string `json:"unresolved_tasks"`
	// UnresolvedApprovals are tasks parked at a pending human approval gate.
	UnresolvedApprovals []string `json:"unresolved_approvals"`
	// UnresolvedVerification are satisfied tasks with no recorded verification
	// evidence (a PASSED run or an external-completion record).
	UnresolvedVerification []string `json:"unresolved_verification"`
	// Eligible reports whether the disposition permits the transition now.
	Eligible bool `json:"eligible"`
	// Reason explains ineligibility (or a staged/complete state).
	Reason string `json:"reason,omitempty"`
	// Fingerprint is a stable hash of the plan's identity and task states, used to
	// detect a plan that changed between readiness and mutation.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Archived is the archive directory, relative to the project, when a terminal
	// record already exists.
	Archived string `json:"archived,omitempty"`
}

// HistoricalizeOptions configures Historicalize. Dir and Store are required.
type HistoricalizeOptions struct {
	Dir   string
	Store TaskStore
	// Plan optionally names the plan to historicalize, as a PLAN path (resolved
	// like any plan argument) or a plan id. When set and an active plan is
	// recorded, it must be that plan. When empty, the active plan is used.
	Plan string
	// Disposition selects the terminal disposition: "" or COMPLETE for a normal
	// historicalization of satisfied work, SUPERSEDED for an explicit disposal of
	// unfinished work. An unknown value is a usage error.
	Disposition string
	// ExpectedFingerprint, when set, is the fingerprint a prior readiness
	// evaluation returned. A mismatch refuses the mutation as stale.
	ExpectedFingerprint string
}

// HistoricalizeResult reports what Historicalize did. Readiness is the evaluation
// immediately before the transition (for a refusal) or after it (for a success),
// so the caller always has the lifecycle picture.
type HistoricalizeResult struct {
	Outcome   string
	Readiness HistoricalizationReadiness
	Archived  string
	Tasks     int
}

// EvaluateHistoricalization performs the deterministic readiness evaluation for a
// plan without mutating anything. It is the reusable domain operation behind the
// CLI's --check and the historicalize skill's preflight.
func EvaluateHistoricalization(dir string, store TaskStore, plan, disposition string) (HistoricalizationReadiness, error) {
	targetID, targetSource, err := historicalizationTarget(dir, plan)
	if err != nil {
		return HistoricalizationReadiness{}, err
	}
	return evaluateHistoricalization(dir, store, targetID, targetSource, disposition)
}

// evaluateHistoricalization is EvaluateHistoricalization with an already-resolved
// target plan id (used after a mutation to report the resulting state).
func evaluateHistoricalization(dir string, store TaskStore, targetID, targetSource, disposition string) (HistoricalizationReadiness, error) {
	if strings.TrimSpace(dir) == "" {
		return HistoricalizationReadiness{}, errors.New("historicalize: dir is required")
	}
	if store == nil {
		return HistoricalizationReadiness{}, errors.New("historicalize: task store is required")
	}
	disp, err := normalizeHistoricalizationDisposition(disposition)
	if err != nil {
		return HistoricalizationReadiness{}, err
	}

	meta := readMetadata(filepath.Join(dir, config.DirName, metaFileName))
	tasks, err := store.List()
	if err != nil {
		return HistoricalizationReadiness{}, err
	}

	state, id := historicalizationState(dir, meta, tasks, targetID)
	r := HistoricalizationReadiness{
		PlanID:      id,
		State:       state,
		Disposition: disp,
		TotalTasks:  len(tasks),
		TaskCounts:  historicalizationTaskCounts(tasks),
		Fingerprint: historicalizationFingerprint(meta, tasks),
	}
	r.Source = strings.TrimSpace(meta.Source)
	if r.Source == "" {
		r.Source = targetSource
	}
	r.UnresolvedTasks = historicalizationUnresolvedTasks(tasks)

	r.UnresolvedApprovals, err = historicalizationUnresolvedApprovals(dir, tasks)
	if err != nil {
		return HistoricalizationReadiness{}, err
	}
	r.UnresolvedVerification = historicalizationMissingVerification(dir, tasks)

	if id != "" {
		archive := filepath.Join(dir, config.DirName, archiveDirName, archiveID(id))
		if fileExists(filepath.Join(archive, lifecycleFile)) {
			r.Archived = relOf(dir, archive)
		}
	}

	// A named plan that is not the active plan is never historicalized: it would
	// archive the wrong identity. This is a refusal, not an eligibility nuance.
	if state == HistoricalizationStateActive {
		activeID := strings.TrimSpace(meta.PlanID)
		if targetID != "" && targetID != activeID {
			r.Eligible = false
			r.Reason = fmt.Sprintf("the named plan (%s) is not the active plan (%s)", targetID, activeID)
			return r, nil
		}
	}

	r.Eligible, r.Reason = historicalizationEligibility(state, disp, &r)
	return r, nil
}

// Historicalize performs the transition when the plan is eligible. On an
// ineligible plan it returns ErrHistoricalizationIneligible with the readiness
// populated and mutates nothing. On an already-historicalized plan it returns
// OutcomeAlreadyHistoricalized with no writes.
func Historicalize(opts HistoricalizeOptions) (HistoricalizeResult, error) {
	readiness, err := EvaluateHistoricalization(opts.Dir, opts.Store, opts.Plan, opts.Disposition)
	if err != nil {
		return HistoricalizeResult{}, err
	}
	res := HistoricalizeResult{Readiness: readiness}

	switch readiness.State {
	case HistoricalizationStateHistoricalized:
		res.Outcome = OutcomeAlreadyHistoricalized
		res.Archived = readiness.Archived
		return res, nil
	case HistoricalizationStateNone:
		return res, ErrNoActivePlan
	}

	if readiness.State == HistoricalizationStateActive {
		if opts.ExpectedFingerprint != "" && opts.ExpectedFingerprint != readiness.Fingerprint {
			return res, ErrStaleHistoricalizationState
		}
		if !readiness.Eligible {
			return res, fmt.Errorf("%w: %s", ErrHistoricalizationIneligible, readiness.Reason)
		}
	}

	metaPath := filepath.Join(opts.Dir, config.DirName, metaFileName)
	planPath := filepath.Join(opts.Dir, config.DirName, planFileName)

	// Re-read immediately before mutation, so a plan changed since readiness is
	// refused rather than overwritten by an older decision.
	meta := readMetadata(metaPath)
	tasks, err := opts.Store.List()
	if err != nil {
		return res, err
	}
	if readiness.State == HistoricalizationStateActive {
		if fp := historicalizationFingerprint(meta, tasks); fp != readiness.Fingerprint {
			return res, ErrStaleHistoricalizationState
		}
		if err := historicalizationStillEligible(opts.Dir, tasks, readiness.Disposition); err != nil {
			return res, err
		}
	}

	planID := strings.TrimSpace(meta.PlanID)
	if planID == "" {
		planID = readiness.PlanID
	}
	archiveRoot := filepath.Join(opts.Dir, config.DirName, archiveDirName, archiveID(planID))
	lifecyclePath := filepath.Join(archiveRoot, lifecycleFile)

	// Stage 1: archive. archivePlan rewrites a fixed set of files, so re-running it
	// over a partially written archive cannot duplicate records. It is skipped when
	// the terminal record already exists (a completed or staged transition), so a
	// re-run never rewrites the recorded disposition or timestamp.
	if !fileExists(lifecyclePath) {
		root, err := archivePlan(opts.Dir, meta, tasks, planPath, metaPath, readiness.Disposition)
		if err != nil {
			return res, err
		}
		archiveRoot = root
	}
	res.Archived = relOf(opts.Dir, archiveRoot)

	// Stage 2: release the active association. Each step is individually durable;
	// if one fails the error says the release is incomplete, and a re-run completes
	// it (the archive already exists, so archivePlan is skipped).
	if err := opts.Store.ClearTasks(); err != nil {
		return res, fmt.Errorf("%w: clear tasks: %v", ErrPartialHistoricalization, err)
	}
	if err := writeMetadata(metaPath, Metadata{}); err != nil {
		return res, fmt.Errorf("%w: release metadata: %v", ErrPartialHistoricalization, err)
	}
	if err := os.Remove(planPath); err != nil && !os.IsNotExist(err) {
		return res, fmt.Errorf("%w: remove machine plan: %v", ErrPartialHistoricalization, err)
	}

	// Verify the resulting lifecycle state before reporting success.
	if err := verifyHistoricalized(opts.Dir, opts.Store, planID, readiness.Disposition); err != nil {
		return res, err
	}

	res.Outcome = OutcomeHistoricalized
	res.Tasks = len(tasks)
	if final, err := evaluateHistoricalization(opts.Dir, opts.Store, planID, "", readiness.Disposition); err == nil {
		res.Readiness = final
	}
	return res, nil
}

// closureBlocker is the shared, fail-closed closure invariant used by both
// `plan complete` and `plan historicalize`. It reuses the exact same sources of
// truth the historicalize readiness predicate consults, so the two closure paths
// cannot disagree about what "ready to close" means (invariants I3/I4/I5):
//
//   - unresolved work: a task that has not reached a satisfied terminal state
//   - an unresolved approval: a PENDING approval gate bound to a task in the
//     active plan
//   - missing verification: a satisfied task with no recorded verification
//     evidence (a PASSED run, an external-completion record, or a NOT_REQUIRED
//     disposition)
//
// It returns "" when the plan is clean and fully satisfied, or a human-readable
// reason naming the offending tasks otherwise. It is fail-closed: a read error on
// the approval artifacts is surfaced as a blocker (the caller must not close a
// plan whose readiness could not be established), and any unknown or missing
// approval/verification state counts as not ready.
func closureBlocker(dir string, tasks []*domain.Task) string {
	if unresolved := historicalizationUnresolvedTasks(tasks); len(unresolved) > 0 {
		return "the active plan still has unresolved work: " + summarize(unresolved)
	}
	approvals, err := historicalizationUnresolvedApprovals(dir, tasks)
	if err != nil {
		return "could not determine approval readiness: " + err.Error()
	}
	if len(approvals) > 0 {
		return "plan has unresolved approval requests: " + summarize(approvals)
	}
	if missing := historicalizationMissingVerification(dir, tasks); len(missing) > 0 {
		return "plan has unresolved verification requirements: " + summarize(missing)
	}
	return ""
}

// historicalizationTarget resolves an optional plan argument to a plan id. A
// resolvable PLAN path yields its derived id and relative source. A value that
// looks like a path but does not resolve surfaces the resolution error, so an
// ambiguous or missing plan is never silently treated as an id. A bare token
// (no path separator, no .md suffix) is accepted as a plan id literal.
func historicalizationTarget(dir, arg string) (id, source string, err error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", "", nil
	}
	if resolved, rerr := ResolvePlanPath(dir, arg); rerr == nil {
		rel := relOf(dir, resolved)
		return planID(rel), rel, nil
	} else if looksLikePlanPath(arg) {
		return "", "", rerr
	}
	return arg, "", nil
}

// looksLikePlanPath reports whether an argument is meant as a filesystem path
// rather than a plan id literal.
func looksLikePlanPath(arg string) bool {
	return strings.ContainsAny(arg, "/\\") || strings.EqualFold(filepath.Ext(arg), ".md")
}

// historicalizationState classifies the plan's current lifecycle state. id is the
// plan id the evaluation is about: the recorded active plan id when one exists,
// otherwise the caller's target.
func historicalizationState(dir string, meta Metadata, tasks []*domain.Task, targetID string) (state, id string) {
	active := len(tasks) > 0 || strings.TrimSpace(meta.PlanID) != "" || strings.TrimSpace(meta.Source) != ""
	id = strings.TrimSpace(meta.PlanID)
	if id == "" {
		id = targetID
	}
	archived := id != "" && fileExists(filepath.Join(dir, config.DirName, archiveDirName, archiveID(id), lifecycleFile))
	switch {
	case active && archived:
		return HistoricalizationStateInconsistent, id
	case active:
		return HistoricalizationStateActive, id
	case archived:
		return HistoricalizationStateHistoricalized, id
	default:
		return HistoricalizationStateNone, id
	}
}

// historicalizationEligibility is the deterministic decision. It reuses the
// existing terminal vocabulary: a satisfied task is DONE/LOCAL_DONE/MERGED, and
// the only plan dispositions are COMPLETE and SUPERSEDED.
func historicalizationEligibility(state, disposition string, r *HistoricalizationReadiness) (bool, string) {
	switch state {
	case HistoricalizationStateHistoricalized:
		return true, "already historicalized"
	case HistoricalizationStateInconsistent:
		return true, "a staged historicalization was detected; historicalization will complete the release"
	case HistoricalizationStateNone:
		return false, "no active plan is recorded"
	}
	// State is ACTIVE. An explicit SUPERSEDED disposal permits unfinished work; a
	// normal COMPLETE historicalization requires fully satisfied, verified work and
	// no pending human gate.
	explicit := disposition == DispositionSuperseded
	if len(r.UnresolvedTasks) > 0 {
		if explicit {
			return true, ""
		}
		return false, "plan has unresolved work: " + summarize(r.UnresolvedTasks)
	}
	if len(r.UnresolvedApprovals) > 0 {
		if explicit {
			return true, ""
		}
		return false, "plan has unresolved approval requests: " + summarize(r.UnresolvedApprovals)
	}
	if len(r.UnresolvedVerification) > 0 {
		if explicit {
			return true, ""
		}
		return false, "plan has unresolved verification requirements: " + summarize(r.UnresolvedVerification)
	}
	return true, ""
}

// historicalizationStillEligible re-checks the eligibility blockers against the
// freshly read state, closing the window between readiness and mutation for
// blockers that a concurrent process could change without moving the task
// fingerprint (a new approval gate, or removed run evidence). It applies the
// same active-plan scoping as readiness, so no new blocker appears between
// readiness and mutation.
func historicalizationStillEligible(dir string, tasks []*domain.Task, disposition string) error {
	if disposition == DispositionSuperseded {
		return nil
	}
	if unresolved := historicalizationUnresolvedTasks(tasks); len(unresolved) > 0 {
		return fmt.Errorf("%w: unresolved work appeared: %s", ErrStaleHistoricalizationState, summarize(unresolved))
	}
	approvals, err := historicalizationUnresolvedApprovals(dir, tasks)
	if err != nil {
		return err
	}
	if len(approvals) > 0 {
		return fmt.Errorf("%w: an approval gate appeared: %s", ErrStaleHistoricalizationState, summarize(approvals))
	}
	if missing := historicalizationMissingVerification(dir, tasks); len(missing) > 0 {
		return fmt.Errorf("%w: verification evidence changed: %s", ErrStaleHistoricalizationState, summarize(missing))
	}
	return nil
}

// verifyHistoricalized asserts the post-conditions of the transition. A failure
// is a partial historicalization the caller must not report as success.
func verifyHistoricalized(dir string, store TaskStore, planID, disposition string) error {
	tasks, err := store.List()
	if err != nil {
		return err
	}
	if len(tasks) != 0 {
		return fmt.Errorf("%w: %d task(s) are still active", ErrPartialHistoricalization, len(tasks))
	}
	if _, _, ok := ActivePlanID(dir); ok {
		return fmt.Errorf("%w: the plan is still recorded ACTIVE", ErrPartialHistoricalization)
	}
	if fileExists(filepath.Join(dir, config.DirName, planFileName)) {
		return fmt.Errorf("%w: the machine plan still exists", ErrPartialHistoricalization)
	}
	life, err := readRecordedLifecycle(filepath.Join(dir, config.DirName, archiveDirName, archiveID(planID), lifecycleFile))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPartialHistoricalization, err)
	}
	if life.Disposition != disposition {
		return fmt.Errorf("%w: archived disposition %q, want %q", ErrPartialHistoricalization, life.Disposition, disposition)
	}
	return nil
}

// historicalizationTaskCounts counts tasks by persisted status, treating an empty
// status as PLANNED (the store's default for an unset status).
func historicalizationTaskCounts(tasks []*domain.Task) map[string]int {
	counts := make(map[string]int, len(tasks))
	for _, t := range tasks {
		status := string(t.Status)
		if strings.TrimSpace(status) == "" {
			status = string(domain.PLANNED)
		}
		counts[status]++
	}
	return counts
}

// historicalizationUnresolvedTasks lists tasks that have not reached a satisfied
// terminal state, as "ID (STATUS)". This is the deterministic definition of
// unresolved work: a PLANNED/READY/in-flight task, an unresolved BLOCKED task, or
// any task that is not DONE/LOCAL_DONE/MERGED.
func historicalizationUnresolvedTasks(tasks []*domain.Task) []string {
	var out []string
	for _, t := range tasks {
		if t.IsSatisfied() {
			continue
		}
		status := string(t.Status)
		if strings.TrimSpace(status) == "" {
			status = string(domain.PLANNED)
		}
		if t.IsBlocked() && strings.TrimSpace(string(t.BlockedReason)) != "" {
			status = status + "/" + string(t.BlockedReason)
		}
		out = append(out, fmt.Sprintf("%s (%s)", t.ID, status))
	}
	sort.Strings(out)
	return out
}

// historicalizationUnresolvedApprovals reads the persisted approval requests
// under .agent-sdlc/runs/ and returns the tasks parked at a pending gate that
// belong to the active plan. It reuses internal/run's artifact reader, so the
// approval format has one owner.
//
// Under the canonical contract (LC-001) an approval is *unresolved* while it is
// PENDING and its task is in the active plan, REGARDLESS of task satisfaction: a
// satisfied task can still carry an open gate that must be explicitly resolved or
// superseded (for example by external completion) before the plan is
// historicalized, and the historical evidence must never be silently discarded.
// Only a request whose task is absent from the active plan — a cross-plan or
// already-released task — is out of scope. The scan is a pure read: it never
// writes or removes an approval artifact.
func historicalizationUnresolvedApprovals(dir string, tasks []*domain.Task) ([]string, error) {
	root := filepath.Join(dir, config.DirName, runsDirName)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		req, ok := run.At(filepath.Join(root, entry.Name())).Approval()
		if !ok || req.Status != domain.ApprovalPending {
			continue
		}
		taskID := entry.Name()
		if strings.TrimSpace(req.TaskID) != "" {
			taskID = strings.TrimSpace(req.TaskID)
		}
		if !historicalizationApprovalInPlan(taskID, tasks) {
			continue
		}
		label := taskID
		if strings.TrimSpace(req.ID) != "" {
			label = fmt.Sprintf("%s (%s)", label, req.ID)
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out, nil
}

// historicalizationApprovalInPlan reports whether a pending approval bound to
// taskID belongs to the active plan. Per the canonical contract (LC-001) the
// blocker is scoping, not staleness: a PENDING request whose task is present in
// the active plan is a plan-level blocker REGARDLESS of task satisfaction, because
// a satisfied task can still carry an open gate that must be explicitly resolved
// or superseded (e.g. by external completion) before the plan is historicalized.
// Only a request whose task is absent from the active plan (a cross-plan or
// already-released task) is out of scope. It is a pure predicate over the task
// slice and the request's task id, and writes nothing.
func historicalizationApprovalInPlan(taskID string, tasks []*domain.Task) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	for _, t := range tasks {
		if t.ID == taskID {
			return true
		}
	}
	return false
}

// historicalizationMissingVerification reports satisfied tasks with no recorded
// verification evidence. A task declared ExecutionDone is the operator's explicit
// statement that its work already exists, so it needs no run evidence.
func historicalizationMissingVerification(dir string, tasks []*domain.Task) []string {
	var out []string
	for _, t := range tasks {
		if !t.IsSatisfied() || t.ExecutionMode == domain.ExecutionDone {
			continue
		}
		if !historicalizationHasVerification(dir, t.ID) {
			out = append(out, t.ID)
		}
	}
	sort.Strings(out)
	return out
}

// historicalizationHasVerification reports whether a satisfied task has
// verification evidence: a PASSED run stage, an external-completion record (the
// explicit, model-free completion path), or a NOT_REQUIRED disposition record.
func historicalizationHasVerification(dir, taskID string) bool {
	if stage, ok := run.Load(dir, taskID); ok && stage == run.Passed {
		return true
	}
	runDir := run.Dir(dir, taskID)
	return fileExists(filepath.Join(runDir, run.ExternalCompletionFile)) ||
		fileExists(filepath.Join(runDir, run.NotRequiredFile))
}

// historicalizationFingerprint is a stable hash of the plan identity and every
// task's state, attempt count, and update time. It detects a plan that changed
// between readiness and mutation.
func historicalizationFingerprint(meta Metadata, tasks []*domain.Task) string {
	h := sha256.New()
	fmt.Fprintf(h, "plan_id=%s\nsource=%s\nsha=%s\n", meta.PlanID, meta.Source, meta.SourceSHA256)
	type row struct {
		id, status, reason string
		attempt            int
		updated            int64
	}
	rows := make([]row, 0, len(tasks))
	for _, t := range tasks {
		rows = append(rows, row{
			id:      t.ID,
			status:  string(t.Status),
			reason:  string(t.BlockedReason),
			attempt: t.Attempt,
			updated: t.UpdatedAt.UTC().UnixNano(),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	for _, r := range rows {
		fmt.Fprintf(h, "%s|%s|%s|%d|%d\n", r.id, r.status, r.reason, r.attempt, r.updated)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// normalizeHistoricalizationDisposition maps the operator's disposition argument
// to a recorded disposition, rejecting anything the archive does not own.
func normalizeHistoricalizationDisposition(d string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(d)) {
	case "":
		return DispositionComplete, nil
	case DispositionComplete:
		return DispositionComplete, nil
	case DispositionSuperseded:
		return DispositionSuperseded, nil
	default:
		return "", fmt.Errorf("historicalize: unknown disposition %q (want %s or %s)", d, DispositionComplete, DispositionSuperseded)
	}
}

// archiveID normalizes a plan id for its archive directory, matching archivePlan.
func archiveID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "plan"
	}
	return id
}

// readRecordedLifecycle reads a plan's terminal disposition record.
func readRecordedLifecycle(path string) (Lifecycle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Lifecycle{}, err
	}
	var life Lifecycle
	if err := json.Unmarshal(data, &life); err != nil {
		return Lifecycle{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return life, nil
}

// summarize renders a blocker list for a reason string.
func summarize(items []string) string {
	return strings.Join(items, ", ")
}
