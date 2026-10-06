package orchestration

// This file implements ORCH-008 progress semantics: distinguishing worker
// ACTIVITY from worker PROGRESS, task PROGRESS, and integration PROGRESS.
//
// ACTIVITY IS NOT PROGRESS
//
// A worker turn is ACTIVITY: the worker executed. It is not progress. Progress
// requires NEW EVIDENCE — a new governed changed file, or a new consolidated
// finding that was not already present. Repeated reasoning without new evidence,
// or two identical successive worker claims, classify as no-progress and are
// never treated as progress.
//
// FOUR PROGRESS KINDS
//
// A ProgressRecord explicitly labels which signal it represents:
//
//	ProgressWorkerActivity        — a worker turn executed (activity, never progress)
//	ProgressWorkerProgress        — a worker produced new evidence
//	ProgressTaskProgress          — the task's integrated view gained new evidence
//	ProgressIntegrationProgress   — an integration pass produced new evidence
//
// CONSISTENT EVIDENCE ACROSS THE FOUR KINDS
//
// Worker progress, task progress, and integration progress are all keyed on the
// SAME evidence: the changed files a claim carries, and the consolidated finding
// strings a claim carries. WorkerOutcomeFindings is the SINGLE derivation of a
// worker's evidence used by every progress kind: the worker-progress path
// (ProgressFromReport/findingsFor) and the integration path (FromReport, which
// carries WorkerOutcomeFindings into WorkerClaim.Findings and so into
// IntegratedView.ConsolidatedFindings). EvidenceStrings delegates to the same
// derivation, so there is exactly one accessor for a result's evidence and no
// second path can disagree.
//
// RELATIONSHIP TO AGENT-002 NO-PROGRESS / BLOCK
//
// ORCH-008-01 recorded a NEGATIVE FINDING: no separate AGENT-002-owned,
// exported progress/no-progress/BLOCK API was confirmed in this repository. The
// existing no-progress and BLOCK semantics live in internal/failure
// (failure.NoProgress, failure.Block) and internal/budget (StaleIterations);
// they are NOT redefined here. This file defines an ADDITIVE, orchestration-local
// ProgressRecord only. It defines NO BLOCK transition and NO no-progress semantic
// of its own: ClassNoProgress is a non-authoritative observation label, not a
// lifecycle state, and every state change remains delegated to internal/domain.
//
// LIFECYCLE AUTHORITY
//
// This file grants NO lifecycle authority. It defines no transition, approval,
// commit, budget-extension, or completion operation and holds no handle to
// workflow state, providers, models, transports, or filesystems.

import (
	"sort"
	"strings"
)

// ProgressKind labels which orchestration signal a ProgressRecord represents. It
// is a plain, provider-neutral label, never a lifecycle state.
type ProgressKind string

const (
	// ProgressWorkerActivity: a worker turn executed. Activity alone is never
	// progress.
	ProgressWorkerActivity ProgressKind = "WORKER_ACTIVITY"
	// ProgressWorkerProgress: a worker produced new evidence (a new changed file or
	// a new finding).
	ProgressWorkerProgress ProgressKind = "WORKER_PROGRESS"
	// ProgressTaskProgress: the task's integrated view gained new evidence.
	ProgressTaskProgress ProgressKind = "TASK_PROGRESS"
	// ProgressIntegrationProgress: an integration pass produced new evidence.
	ProgressIntegrationProgress ProgressKind = "INTEGRATION_PROGRESS"
)

// ProgressClass is the deterministic classification of a record's evidence
// delta. It is deliberately separate from failure.Disposition.Block: it is an
// observation, not a lifecycle action.
type ProgressClass string

const (
	// ClassProgress: new evidence appeared, so the record is progress.
	ClassProgress ProgressClass = "PROGRESS"
	// ClassNoProgress: no new evidence appeared across successive records, so the
	// record is activity without progress. It is NOT the harness no-progress stop
	// (failure.NoProgress) and defines no BLOCK transition.
	ClassNoProgress ProgressClass = "NO_PROGRESS"
)

// ProgressRecord is an additive, non-authoritative observation about
// orchestration progress. It records worker activity separately from worker,
// task, and integration progress, and carries the evidence delta that justified
// the classification. It exposes no transition, approval, commit, or
// budget-extension field or method.
type ProgressRecord struct {
	// Kind labels which of the four signals the record represents.
	Kind ProgressKind
	// AssignmentID and TaskID correlate the record with its assignment, when
	// applicable.
	AssignmentID string
	TaskID       string
	// Active is true when the record describes an executed turn (activity). It is
	// an observation about execution, not about progress.
	Active bool
	// Class is the deterministic classification: PROGRESS or NO_PROGRESS.
	Class ProgressClass
	// NewChangedFiles names the repository-relative files that are new relative to
	// the compared baseline and justified a PROGRESS classification.
	NewChangedFiles []string
	// NewFindings names the consolidated findings that are new relative to the
	// compared baseline and justified a PROGRESS classification.
	NewFindings []string
	// Reason is the human-readable justification.
	Reason string
}

// Progress reports whether the record represents actual progress (new
// evidence), as opposed to mere activity.
func (r ProgressRecord) Progress() bool { return r.Class == ClassProgress }

// ProgressInput is the evidence a single worker outcome contributes to progress
// classification. ChangedFiles and Findings are the outcome's evidence;
// Activity reports whether a turn executed.
type ProgressInput struct {
	AssignmentID string
	TaskID       string
	Activity     bool
	ChangedFiles []string
	Findings     []string
}

// ClassifyProgress classifies one input against a baseline (the evidence already
// observed). A record is PROGRESS only when it contributes evidence — a new
// changed file or a new finding — that is absent from the baseline. Repeated
// reasoning with no new evidence, or two identical successive claims, classify as
// NO_PROGRESS regardless of Activity. It is pure and deterministic.
func ClassifyProgress(in ProgressInput, baseline ProgressBaseline) ProgressRecord {
	rec := ProgressRecord{
		Kind:         ProgressWorkerActivity,
		AssignmentID: in.AssignmentID,
		TaskID:       in.TaskID,
		Active:       in.Activity,
		Class:        ClassNoProgress,
	}

	newFiles := baseline.newFiles(in.ChangedFiles)
	newFindings := baseline.newFindings(in.Findings)
	rec.NewChangedFiles = newFiles
	rec.NewFindings = newFindings

	if len(newFiles) == 0 && len(newFindings) == 0 {
		rec.Reason = "worker activity produced no new evidence; repeated reasoning without new evidence is not progress"
		return rec
	}

	rec.Kind = ProgressWorkerProgress
	rec.Class = ClassProgress
	rec.Reason = "worker produced new evidence"
	return rec
}

// ProgressBaseline accumulates the evidence already observed, so successive
// records can be compared for new evidence. It is deterministic and exposes no
// lifecycle operation.
type ProgressBaseline struct {
	files    map[string]bool
	findings map[string]bool
}

// NewProgressBaseline builds an empty baseline.
func NewProgressBaseline() ProgressBaseline {
	return ProgressBaseline{files: map[string]bool{}, findings: map[string]bool{}}
}

// Include returns a baseline that also contains the given evidence, leaving the
// receiver unchanged.
func (b ProgressBaseline) Include(files, findings []string) ProgressBaseline {
	out := NewProgressBaseline()
	for f := range b.files {
		out.files[f] = true
	}
	for f := range b.findings {
		out.findings[f] = true
	}
	for _, f := range files {
		if s := strings.TrimSpace(f); s != "" {
			out.files[s] = true
		}
	}
	for _, f := range findings {
		if s := strings.TrimSpace(f); s != "" {
			out.findings[s] = true
		}
	}
	return out
}

// newFiles returns the deduplicated, sorted changed files not present in the
// baseline.
func (b ProgressBaseline) newFiles(files []string) []string {
	return b.missing(files, b.files)
}

// newFindings returns the deduplicated, sorted findings not present in the
// baseline.
func (b ProgressBaseline) newFindings(findings []string) []string {
	return b.missing(findings, b.findings)
}

func (b ProgressBaseline) missing(values []string, known map[string]bool) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" || known[s] || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// TaskProgress derives a TASK_PROGRESS record from an IntegratedView's
// consolidated evidence against a baseline. New consolidated findings or new
// changed files are task progress; an unchanged view is not. The evidence it
// compares (IntegratedView.ChangedFiles and IntegratedView.ConsolidatedFindings)
// is derived from the same claim strings as the worker-progress path, so the two
// paths cannot disagree about a single claim.
func TaskProgress(view IntegratedView, baseline ProgressBaseline) ProgressRecord {
	files := baseline.newFiles(view.ChangedFiles)
	findings := baseline.newFindings(view.ConsolidatedFindings)
	rec := ProgressRecord{
		Kind:            ProgressTaskProgress,
		TaskID:          view.TaskID,
		NewChangedFiles: files,
		NewFindings:     findings,
		Class:           ClassNoProgress,
	}
	if len(files) == 0 && len(findings) == 0 {
		rec.Reason = "the integrated view added no new evidence; task progress is unchanged"
		return rec
	}
	rec.Class = ClassProgress
	rec.Reason = "the integrated view added new evidence"
	return rec
}

// IntegrationProgress derives an INTEGRATION_PROGRESS record from one integration
// pass. An integration pass that consolidates new evidence is progress; a pass
// over identical claims is not.
func IntegrationProgress(view IntegratedView, baseline ProgressBaseline) ProgressRecord {
	rec := TaskProgress(view, baseline)
	rec.Kind = ProgressIntegrationProgress
	if rec.Class == ClassProgress {
		rec.Reason = "the integration pass consolidated new evidence"
	} else {
		rec.Reason = "the integration pass consolidated no new evidence"
	}
	return rec
}

// ProgressFromReport derives deterministic worker progress records from an
// execution report, in caller order, comparing each outcome's evidence against a
// baseline that accumulates earlier outcomes. It records activity separately
// from progress and grants no lifecycle authority.
//
// The evidence it compares is WorkerOutcomeFindings(o) — the same strings that
// FromReport carries into IntegratedView.ConsolidatedFindings — so a worker whose
// claim is classified here cannot be classified differently by TaskProgress or
// IntegrationProgress.
func ProgressFromReport(report ExecutionReport, baseline ProgressBaseline) []ProgressRecord {
	if baseline.files == nil || baseline.findings == nil {
		baseline = NewProgressBaseline()
	}
	records := make([]ProgressRecord, 0, len(report.Outcomes))
	for _, o := range report.Outcomes {
		result := o.Result
		if o.Failure != FailureNone {
			result = WorkResult{}
		}
		in := ProgressInput{
			AssignmentID: o.AssignmentID,
			TaskID:       o.TaskID,
			Activity:     true,
			ChangedFiles: result.ChangedFiles,
			Findings:     EvidenceStrings(result),
		}
		rec := ClassifyProgress(in, baseline)
		records = append(records, rec)
		baseline = baseline.Include(in.ChangedFiles, in.Findings)
	}
	return records
}

// WorkerOutcomeFindings renders a WorkResult's evidence into the stable strings
// used for progress comparison across BOTH the worker-progress path and the
// integration path. It is the deterministic union of the result's Findings and
// Evidence, deduplicated and sorted, so a claim that populates either field (or
// both) is compared on the same strings by every progress kind. It performs no
// I/O and grants no authority.
func WorkerOutcomeFindings(o AssignmentOutcome) []string {
	if o.Failure != FailureNone {
		return nil
	}
	return EvidenceStrings(o.Result)
}

// EvidenceStrings is the SINGLE derivation of a WorkResult's evidence for
// progress comparison. It is the deterministic, deduplicated, sorted union of
// the result's Findings and Evidence, so every progress kind (worker, task,
// integration) compares identical evidence and cannot disagree about a claim. It
// performs no I/O and grants no authority.
func EvidenceStrings(r WorkResult) []string {
	seen := make(map[string]bool, len(r.Findings)+len(r.Evidence))
	out := make([]string, 0, len(r.Findings)+len(r.Evidence))
	for _, f := range r.Findings {
		if s := strings.TrimSpace(f); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, e := range r.Evidence {
		if s := strings.TrimSpace(e); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
