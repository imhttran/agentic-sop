package orchestration

// This file implements ORCH-007: the explicit INTEGRATE stage.
//
// STAGE MODEL
//
// ORCH-007 makes integration a first-class, traceable phase distinct from
// worker execution. A run traverses four explicit stages, in order:
//
//	StageWork -> StageResults -> StageIntegrate -> StageVerify
//
// WORK executes the worker assignments; RESULTS collects their claims;
// INTEGRATE combines those non-authoritative claims into one coherent view; and
// VERIFY runs the centralized, deterministic verification path
// (internal/validate + internal/quality + internal/review). The Stage type is a
// provider/model-neutral orchestration label only: it is NOT a lifecycle state
// and it never redefines a domain transition. No lifecycle transition is defined
// in this file; every state change remains delegated to internal/domain.
//
// NON-AUTHORITATIVE WORKER CLAIMS
//
// A worker's WorkResult is a claim, never success. A result that self-reports a
// PASS-like status (for example completed with evidence containing "PASS") is
// tagged NonAuthoritative at INTEGRATE. The authoritative verdict is produced
// ONLY by the centralized VERIFY path; orchestration forwards the claim and
// never maps it directly to a final PASS/FAIL. A worker claiming PASS cannot
// satisfy verification, and a worker claiming FAIL does not by itself block
// beyond the harness policy.
//
// BOUNDED INTEGRATION ATTEMPTS (ORCH-008)
//
// Every integration pass is accounted against OrchestrationBudget.
// MaxIntegrationAttempts via IntegrationAttempts. Once the bound is exhausted,
// further integration is deterministically refused with an explicit, typed
// *IntegrationRefusedError (an explicit finding, NEVER a transition).
//
// CONSISTENT EVIDENCE FOR PROGRESS
//
// FromReport carries each outcome's evidence (EvidenceStrings /
// WorkerOutcomeFindings) into the WorkerClaim's Findings field, and Integrate
// appends those same strings verbatim into IntegratedView.ConsolidatedFindings.
// Task and integration progress therefore compare the SAME evidence strings as
// worker progress, so a claim cannot be classified differently by the two paths.
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

// Stage names one explicit orchestration stage. It is a plain, provider-neutral
// label recording where a run is in the orchestrator -> workers -> integrator ->
// verification flow. It is deliberately distinct from domain.TaskStatus: a Stage
// describes orchestration execution phase, not workflow lifecycle state, and it
// defines no transition.
type Stage string

const (
	// StageWork is worker execution: the orchestrator dispatches assignments and
	// workers perform them through the provider-neutral agent boundary.
	StageWork Stage = "WORK"
	// StageResults is result collection: the workers' non-authoritative claims are
	// gathered (validated at the contract boundary only).
	StageResults Stage = "RESULTS"
	// StageIntegrate is integration: distinct from worker execution, it combines the
	// worker claims into one coherent integrated view. It holds no lifecycle
	// authority.
	StageIntegrate Stage = "INTEGRATE"
	// StageVerify is centralized, authoritative verification: the deterministic
	// validation/review/quality path proves the integrated result against the
	// caller-owned contract. Only this stage produces a final verdict.
	StageVerify Stage = "VERIFY"
)

// CanonicalStageOrder is the fixed traversal order of the ORCH-007 pipeline.
// Integration sits between RESULT collection and authoritative VERIFY; it is
// never skipped and never precedes WORK.
var CanonicalStageOrder = []Stage{StageWork, StageResults, StageIntegrate, StageVerify}

// StageIndex returns the zero-based position of a stage in CanonicalStageOrder,
// or -1 when the stage is not a canonical stage.
func StageIndex(s Stage) int {
	for i, candidate := range CanonicalStageOrder {
		if candidate == s {
			return i
		}
	}
	return -1
}

// KnownStage reports whether s is one of the canonical pipeline stages.
func KnownStage(s Stage) bool { return StageIndex(s) >= 0 }

// StagesBefore reports whether stage a precedes stage b in the canonical order.
// Unknown stages are never ordered before anything, so a caller cannot smuggle
// integration ahead of verification.
func StagesBefore(a, b Stage) bool {
	ia, ib := StageIndex(a), StageIndex(b)
	if ia < 0 || ib < 0 {
		return false
	}
	return ia < ib
}

// WorkerClaim is a worker's self-reported validation statement, tagged
// non-authoritative. It records what the worker said about its own output; the
// harness treats it as evidence to be checked, never as a verdict.
type WorkerClaim struct {
	// AssignmentID and TaskID correlate the claim with its assignment.
	AssignmentID string
	TaskID       string
	// Status is the worker-reported outcome status (a claim, not a verdict).
	Status string
	// SelfReportedValidation is the worker's own validation statement (for example
	// "PASS" or "FAIL"). It is preserved verbatim and never interpreted as a
	// verdict.
	SelfReportedValidation string
	// Findings is the worker's evidence text (its conclusions and evidence
	// references). It is the SAME set of strings carried into
	// IntegratedView.ConsolidatedFindings, so task/integration progress compares
	// identical evidence to worker progress.
	Findings []string
	// ChangedFiles names the repository-relative files the worker claimed to change.
	ChangedFiles []string
	// NonAuthoritative is always true: it is the explicit, machine-checkable mark
	// that this claim must not be treated as final verification. It exists so a
	// reader or an arch check can prove the seam is present.
	NonAuthoritative bool
}

// IntegratedView is the coherent view the INTEGRATE stage produces from the
// workers' claims. It is read-only with respect to domain state and grants no
// lifecycle authority: it is input to the authoritative VERIFY stage, never a
// verdict itself.
type IntegratedView struct {
	// TaskID is the integration target.
	TaskID string
	// Stages records the stages traversed to produce this view, in canonical order,
	// ending at StageIntegrate. It makes integration distinguishable from worker
	// execution in state.
	Stages []Stage
	// Claims are the non-authoritative worker claims consumed as inputs, in
	// deterministic order.
	Claims []WorkerClaim
	// ConsolidatedFindings is the merged, deterministic rendering of worker claims.
	// It contains each claim's evidence strings VERBATIM, so it is directly
	// comparable to the evidence the worker-progress path uses.
	ConsolidatedFindings []string
	// ChangedFiles is the deterministic union of every worker-claimed changed file.
	ChangedFiles []string
	// Progress is the integration-progress record for this pass, a non-authoritative
	// observation comparing the view's consolidated evidence against a baseline. It
	// grants no lifecycle authority.
	Progress ProgressRecord
	// NextStage is always StageVerify: integration hands off to centralized,
	// authoritative verification, never to a verdict.
	NextStage Stage
	// Authoritative is always false: an integrated view is never itself a verdict.
	Authoritative bool
}

// NonAuthoritativeValidation marks a worker-supplied validation string as a claim.
// It is the explicit seam that separates worker self-report from the
// authoritative VERIFY verdict.
func NonAuthoritativeValidation(claim string) WorkerClaim {
	return WorkerClaim{SelfReportedValidation: strings.TrimSpace(claim), NonAuthoritative: true}
}

// Integrate combines worker claims into one deterministic IntegratedView. It
// performs no I/O, consults no provider, mutates no lifecycle state, and defines
// no transition. Every claim it consumes is tagged NonAuthoritative; the view's
// NextStage is always StageVerify and its Authoritative flag is always false, so
// a worker claim can never be mapped directly to a final verdict.
//
// ConsolidatedFindings carries each claim's evidence strings (Findings) verbatim
// — the same strings the worker-progress path compares — plus a deterministic
// status/validation line, so the two paths cannot disagree.
func Integrate(taskID string, claims []WorkerClaim) IntegratedView {
	view := IntegrateAgainst(taskID, claims, NewProgressBaseline())
	return view
}

// IntegrateAgainst combines worker claims into one deterministic IntegratedView
// and also derives the integration-progress record for this pass against the
// supplied baseline. The classification compares the SAME consolidated evidence
// strings as the worker-progress path, so the two cannot disagree. The returned
// view is non-authoritative.
func IntegrateAgainst(taskID string, claims []WorkerClaim, baseline ProgressBaseline) IntegratedView {
	view := IntegratedView{
		TaskID:        taskID,
		Stages:        []Stage{StageWork, StageResults, StageIntegrate},
		NextStage:     StageVerify,
		Authoritative: false,
	}

	ordered := make([]WorkerClaim, len(claims))
	copy(ordered, claims)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].AssignmentID != ordered[j].AssignmentID {
			return ordered[i].AssignmentID < ordered[j].AssignmentID
		}
		return ordered[i].TaskID < ordered[j].TaskID
	})

	files := make(map[string]bool)
	for _, c := range ordered {
		// Defensive: even a caller-built claim that forgot the tag is recorded as
		// non-authoritative at the integration boundary.
		if !c.NonAuthoritative {
			c.NonAuthoritative = true
		}
		if c.Status != "" {
			view.ConsolidatedFindings = append(view.ConsolidatedFindings, c.AssignmentID+": "+c.Status)
		}
		if c.SelfReportedValidation != "" {
			view.ConsolidatedFindings = append(view.ConsolidatedFindings, c.AssignmentID+" reported "+c.SelfReportedValidation+" (non-authoritative)")
		}
		// Carry the worker's evidence strings verbatim so task/integration progress
		// compares the same evidence as worker progress.
		view.ConsolidatedFindings = append(view.ConsolidatedFindings, c.Findings...)
		for _, f := range c.ChangedFiles {
			if strings.TrimSpace(f) != "" {
				files[f] = true
			}
		}
		view.Claims = append(view.Claims, c)
	}

	for f := range files {
		view.ChangedFiles = append(view.ChangedFiles, f)
	}
	sort.Strings(view.ChangedFiles)

	if baseline.files == nil || baseline.findings == nil {
		baseline = NewProgressBaseline()
	}
	view.Progress = IntegrationProgress(view, baseline)
	return view
}

// FromReport integrates the outcomes of a coordinator execution report into an
// IntegratedView. Each outcome is converted to a non-authoritative claim. This is
// the coordinator-to-INTEGRATE wiring: it carries worker claims forward without
// granting any authority. The claim's Findings are the same strings
// WorkerOutcomeFindings derives for the worker-progress path, so worker, task,
// and integration progress all compare identical evidence.
func FromReport(report ExecutionReport) IntegratedView {
	claims := make([]WorkerClaim, 0, len(report.Outcomes))
	for _, o := range report.Outcomes {
		claim := WorkerClaim{
			AssignmentID:     o.AssignmentID,
			TaskID:           o.TaskID,
			Status:           string(o.Result.Status),
			Findings:         WorkerOutcomeFindings(o),
			ChangedFiles:     append([]string(nil), o.Result.ChangedFiles...),
			NonAuthoritative: true,
		}
		if o.Failure != FailureNone {
			claim.Status = string(o.Failure)
			claim.Findings = nil
		}
		claims = append(claims, claim)
	}
	return Integrate(firstTaskID(report), claims)
}

// firstTaskID returns the task id shared by the report outcomes, or "" when the
// report is empty. It never invents an id.
func firstTaskID(report ExecutionReport) string {
	for _, o := range report.Outcomes {
		if strings.TrimSpace(o.TaskID) != "" {
			return o.TaskID
		}
	}
	return ""
}

// BoundedIntegrate performs one integration pass under the orchestration
// envelope: it accounts the attempt against MaxIntegrationAttempts and refuses
// deterministically with an explicit typed *IntegrationRefusedError once the
// bound is exhausted. It never maps a worker claim to a verdict and grants no
// lifecycle authority; the IntegratedView it returns is always non-authoritative.
func BoundedIntegrate(attempts *IntegrationAttempts, taskID string, claims []WorkerClaim, baseline ProgressBaseline) (IntegratedView, error) {
	if err := attempts.Attempt(taskID); err != nil {
		return IntegratedView{}, err
	}
	return IntegrateAgainst(taskID, claims, baseline), nil
}
