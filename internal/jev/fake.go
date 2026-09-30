package jev

import (
	"context"
	"errors"

	"github.com/imhttran/agentic-sop/internal/review"
)

// FakeAnalyzer is a deterministic Analyzer test double. It performs no model or
// network access and no repository mutation: it returns a canned Result (or
// error), which is exactly the shape the real boundary allows.
//
// It exists so lifecycle tests can exercise SOP policy against JEV results
// without a live model, and can also be used to confirm interface conformance.
type FakeAnalyzer struct {
	// Result is returned by Analyze.
	Result Result
	// Err, when set, is returned instead of Result.
	Err error
	// Requests records each request, so tests can assert on the bounded context
	// JEV received.
	Requests []Request
}

var _ Analyzer = (*FakeAnalyzer)(nil)

// Analyze returns the configured result or error. It never mutates state.
func (f *FakeAnalyzer) Analyze(_ context.Context, req Request) (Result, error) {
	f.Requests = append(f.Requests, req)
	if f.Err != nil {
		return Result{}, f.Err
	}
	return f.Result, nil
}

// Early-decision scenario contract (P3-003).
//
// The PRD's early-decision lifecycle requires deterministic coverage of exactly
// these cases, all of which the fake can produce with no network, no OpenJEV,
// and no LLM:
//
//   - clear:               analysis found nothing worth reporting (PASS).
//   - ambiguous:           the task is underspecified; a QUALITY/INCOMPLETE
//                          evidence is returned rather than guessing.
//   - scope concern:       the change touches more than the task asked for;
//                          a scope finding is reported (FINDINGS).
//   - destructive concern: the change risks destructive behavior; a
//                          SECURITY finding is reported (FINDINGS).
//   - provider failure:    the analyzer could not run; Analyze returns an
//                          error and an empty Result. Never a finding, never a
//                          pass (mirrors the P3-013 distinction).
//   - malformed:           a value that fail-closed validation must reject
//                          rather than silently accept as PASS.
//
// Every scenario is deterministic and side-effect free: builders read no time,
// randomness, environment, files, or network, so repeated calls yield identical
// values and the fake stays race-safe. Each builder returns a *FakeAnalyzer so
// the same value can be driven through the Analyzer interface and its recorded
// Requests asserted on.

// Scenario names identify the required early-decision cases. They are stable,
// machine-readable identifiers used by tests and (later) lifecycle wiring.
type Scenario string

const (
	// ScenarioClear: the implementation looked fine; nothing to report.
	ScenarioClear Scenario = "clear"
	// ScenarioAmbiguous: the task was underspecified; analysis is INCOMPLETE.
	ScenarioAmbiguous Scenario = "ambiguous"
	// ScenarioScopeConcern: the change exceeded the task's scope.
	ScenarioScopeConcern Scenario = "scope-concern"
	// ScenarioDestructiveConcern: the change risks destructive behavior.
	ScenarioDestructiveConcern Scenario = "destructive-concern"
	// ScenarioProviderFailure: the analyzer failed to run (error, not a finding).
	ScenarioProviderFailure Scenario = "provider-failure"
	// ScenarioMalformed: a value fail-closed validation must reject.
	ScenarioMalformed Scenario = "malformed"
)

// Scenarios lists the required early-decision cases in a stable order. It is
// the closed set the fake is required to cover.
func Scenarios() []Scenario {
	return []Scenario{
		ScenarioClear,
		ScenarioAmbiguous,
		ScenarioScopeConcern,
		ScenarioDestructiveConcern,
		ScenarioProviderFailure,
		ScenarioMalformed,
	}
}

// ErrProviderFailure is the deterministic error surfaced by the provider-failure
// scenario. It is a distinct sentinel so tests can assert the failure is an
// error (never converted into a finding or a PASS).
var ErrProviderFailure = errors.New("jev: fake provider failure")

// NewClearFake returns a fake whose Analyze reports a clean (PASS) analysis with
// validated structured evidence for the clear case.
func NewClearFake() *FakeAnalyzer {
	return &FakeAnalyzer{Result: Result{
		Status:  StatusPass,
		Summary: "no concerns found",
		StructuredEvidence: &Evidence{
			Version:  EvidenceVersion,
			Purpose:  PurposeQuality,
			Severity: SeverityInfo,
			Status:   EvidencePass,
			Summary:  "no concerns found",
		},
	}}
}

// NewAmbiguousFake returns a fake for the ambiguous task case. The task is
// underspecified, so analysis completes INCOMPLETE rather than guessing: no
// finding is fabricated and the result is never a PASS.
func NewAmbiguousFake() *FakeAnalyzer {
	return &FakeAnalyzer{Result: Result{
		Status:  StatusIncomplete,
		Summary: "task is underspecified; unable to analyze",
		StructuredEvidence: &Evidence{
			Version:  EvidenceVersion,
			Purpose:  PurposeQuality,
			Severity: SeverityLow,
			Status:   EvidenceIncomplete,
			Items: []EvidenceItem{{
				Purpose:  PurposeQuality,
				Severity: SeverityLow,
				Category: "ambiguity",
				Detail:   "task acceptance criteria are ambiguous",
				Evidence: "criteria do not state a concrete expected outcome",
			}},
			Summary: "task is underspecified; unable to analyze",
		},
	}}
}

// NewScopeConcernFake returns a fake for the scope-concern case: the change
// touches more than the task asked for. It reports FINDINGS at MEDIUM severity.
func NewScopeConcernFake() *FakeAnalyzer {
	return &FakeAnalyzer{Result: Result{
		Status:   StatusFindings,
		Summary:  "change exceeds task scope",
		Findings: []Finding{{ID: "scope", Severity: review.Medium, Category: "scope", Message: "change touches files outside the task scope"}},
		StructuredEvidence: &Evidence{
			Version:  EvidenceVersion,
			Purpose:  PurposeCorrectness,
			Severity: SeverityMedium,
			Status:   EvidenceFindings,
			Items: []EvidenceItem{{
				Purpose:  PurposeCorrectness,
				Severity: SeverityMedium,
				Category: "scope",
				Detail:   "change touches files outside the task scope",
				Evidence: "repository context shows unrelated files modified",
			}},
			Summary: "change exceeds task scope",
		},
	}}
}

// NewDestructiveConcernFake returns a fake for the destructive-concern case: the
// change risks destructive behavior. It reports FINDINGS at HIGH severity under
// a security purpose.
func NewDestructiveConcernFake() *FakeAnalyzer {
	return &FakeAnalyzer{Result: Result{
		Status:   StatusFindings,
		Summary:  "change may perform a destructive operation",
		Findings: []Finding{{ID: "destructive", Severity: review.High, Category: "security", Message: "change may delete or overwrite data irreversibly"}},
		StructuredEvidence: &Evidence{
			Version:  EvidenceVersion,
			Purpose:  PurposeSecurity,
			Severity: SeverityHigh,
			Status:   EvidenceFindings,
			Items: []EvidenceItem{{
				Purpose:  PurposeSecurity,
				Severity: SeverityHigh,
				Category: "destructive",
				Detail:   "change may delete or overwrite data irreversibly",
				Evidence: "repository context shows an unconditional remove operation",
			}},
			Summary: "change may perform a destructive operation",
		},
	}}
}

// NewProviderFailureFake returns a fake for the provider-failure case. Analyze
// returns ErrProviderFailure and an empty Result, so the failure can never be
// interpreted as a finding or a PASS.
func NewProviderFailureFake() *FakeAnalyzer {
	return &FakeAnalyzer{Err: ErrProviderFailure}
}

// NewMalformedFake returns a fake for the malformed case: its Result carries an
// unknown evidence purpose, so fail-closed validation must reject it. It is
// built directly (bypassing marshalling) so callers can assert the rejection.
func NewMalformedFake() *FakeAnalyzer {
	return &FakeAnalyzer{Result: Result{
		Status:  StatusPass,
		Summary: "malformed",
		StructuredEvidence: &Evidence{
			Version:  EvidenceVersion,
			Purpose:  Purpose("UNKNOWN_PURPOSE"),
			Severity: SeverityInfo,
			Status:   EvidencePass,
		},
	}}
}
