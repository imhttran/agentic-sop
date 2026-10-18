package outcome

import (
	"fmt"
	"strings"
)

// This file is the criterion-evidence contract (SOP-003): every required acceptance
// criterion must be explicitly mapped to an executable acceptance check or to manual
// evidence, and the mapped evidence must be present and passing for the objective to
// be VERIFIED. Validation (the build/test/lint quality gate) is recorded separately
// from acceptance and never satisfies acceptance on its own.

// EvidenceKind names how a criterion is evidenced.
type EvidenceKind string

const (
	// EvidenceCheck: the criterion is met by an executed, passing acceptance check.
	EvidenceCheck EvidenceKind = "check"
	// EvidenceManual: the criterion is met by an explicit, traceable accept decision.
	EvidenceManual EvidenceKind = "manual"
)

// Decision values for an AcceptanceDecision.
const (
	DecisionAccept = "accept"
	DecisionReject = "reject"
)

// CriterionMapping binds one acceptance criterion to its evidence. It is
// operator-owned: it is never taken from a model response.
type CriterionMapping struct {
	CriterionID string       `json:"criterion_id"`
	Description string       `json:"description,omitempty"`
	Required    bool         `json:"required"`
	Kind        EvidenceKind `json:"kind"`
	// CheckRef names the CheckResult (by Ref) that evidences a check criterion.
	CheckRef string `json:"check_ref,omitempty"`
	// ManualEvidence describes the evidence a human reviews for a manual criterion.
	ManualEvidence string `json:"manual_evidence,omitempty"`
	// Attempt is the task attempt this mapping was derived for. A manual decision
	// is bound to it, so a decision recorded for an earlier attempt cannot satisfy
	// a later one. Zero means the attempt was not recorded (legacy callers).
	Attempt int `json:"attempt,omitempty"`
}

// CheckResult is the recorded execution of one acceptance check.
type CheckResult struct {
	Ref      string
	Command  string
	Executed bool
	Passed   bool
	Evidence string
	Argv     []string
	ExitCode int
	Output   string
	// WorkDir is the directory the check ran in; LogArtifact is the run-relative
	// path of a failed check's redacted full-output log.
	WorkDir     string
	LogArtifact string
}

// AcceptanceDecision is an explicit, attributed decision on a manual-evidence
// criterion, recorded as append-only audit evidence. It is bound to the exact
// criterion text, the manual-evidence description, and the task attempt it was
// recorded for, so a decision cannot silently carry over to changed criterion
// content or a later attempt.
type AcceptanceDecision struct {
	Task        string `json:"task"`
	CriterionID string `json:"criterion_id"`
	Evidence    string `json:"evidence"`
	Decision    string `json:"decision"`
	Approver    string `json:"approver"`
	RecordedAt  string `json:"recorded_at"`
	// CriterionDescription and ManualEvidence bind the decision to the mapping it
	// was reviewed against. Both must match the current mapping exactly.
	CriterionDescription string `json:"criterion_description,omitempty"`
	ManualEvidence       string `json:"manual_evidence,omitempty"`
	// Attempt binds the decision to the task attempt whose outcome was reviewed.
	// Zero means the attempt was not recorded (legacy callers).
	Attempt int `json:"attempt,omitempty"`
}

// ValidationResult is the quality-gate verdict, recorded apart from acceptance.
type ValidationResult struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons,omitempty"`
}

// VerifyAcceptance derives the verdict from mapped acceptance evidence.
//
// criteria holds the objective's criteria. A criterion with a non-nil Check (a
// deliverable presence check, an authorization boundary) is evaluated as Verify does.
// A criterion with a nil Check is an acceptance criterion: it is met only through its
// mapping — an executed, passing check, or an accepting decision for manual evidence.
// An unmapped, incompletely mapped, failed, unexecuted, or undecided required
// criterion is unmet and yields PARTIAL with a named reason. Boundary (HOLD)
// semantics are unchanged. Failing validation prevents VERIFIED, and passing
// validation never satisfies an acceptance criterion.
//
// With no mappings at all (a legacy plan) the result is PARTIAL — never VERIFIED —
// with a Compatibility message naming the unmapped criteria and how to map them.
func VerifyAcceptance(validation ValidationResult, criteria []Criterion, mappings []CriterionMapping, checks []CheckResult, decisions []AcceptanceDecision) Result {
	byCriterion := make(map[string]CriterionMapping, len(mappings))
	for _, m := range mappings {
		byCriterion[m.CriterionID] = m
	}
	byRef := make(map[string]CheckResult, len(checks))
	for _, c := range checks {
		byRef[c.Ref] = c
	}

	resolved := make([]Criterion, len(criteria))
	var unmapped []string
	for i, c := range criteria {
		resolved[i] = c
		if c.Check != nil {
			continue
		}
		m, ok := byCriterion[c.ID]
		if !ok {
			unmapped = append(unmapped, c.ID)
			resolved[i].Check = func() error {
				return fmt.Errorf("unmapped: no acceptance check or manual evidence is mapped to this criterion")
			}
			continue
		}
		resolved[i].Check = mappingCheck(m, byRef, decisions)
		if m.Kind == EvidenceCheck {
			resolved[i].Command = byRef[m.CheckRef].Command
		}
	}

	res := Verify(resolved)
	for _, c := range criteria {
		if m, ok := byCriterion[c.ID]; ok && c.Check == nil && m.Kind == EvidenceCheck {
			if run, ok := byRef[m.CheckRef]; ok {
				res.PropagateEvidence(c.ID, run)
			}
		}
	}
	v := validation
	res.Validation = &v

	if res.Status == Hold {
		return res
	}
	if len(mappings) == 0 {
		res.Compatibility = LegacyCompatibility(unmapped)
		if res.Status == Verified {
			res.Status = Partial
			res.Reasons = []string{res.Compatibility}
		}
		return res
	}
	if !validation.Passed {
		if res.Status == Verified {
			res.Status = Partial
			res.Reasons = nil
		}
		res.Reasons = append(res.Reasons, "validation did not pass: "+strings.Join(validation.Reasons, "; "))
	}
	return res
}

// LegacyCompatibility is the actionable result for a plan that declares no
// acceptance mapping: it names the unmapped criteria and how to map them.
func LegacyCompatibility(unmapped []string) string {
	return fmt.Sprintf("legacy plan: no acceptance mapping is declared, so criteria [%s] are unmapped and cannot be verified; add an '### Acceptance Mapping' section mapping each criterion to an acceptance check ('- criterion N: check M') or to manual evidence ('- criterion N: manual: <evidence>')", strings.Join(unmapped, ", "))
}

// mappingCheck returns the deterministic predicate a mapping resolves to.
func mappingCheck(m CriterionMapping, checks map[string]CheckResult, decisions []AcceptanceDecision) func() error {
	switch m.Kind {
	case EvidenceCheck:
		return func() error {
			if m.CheckRef == "" {
				return fmt.Errorf("incompletely mapped: check mapping names no acceptance check")
			}
			run, ok := checks[m.CheckRef]
			switch {
			case !ok || !run.Executed:
				return fmt.Errorf("mapped check %s was not executed", m.CheckRef)
			case !run.Passed:
				return fmt.Errorf("mapped check %s failed: %s", m.CheckRef, run.Evidence)
			}
			return nil
		}
	case EvidenceManual:
		return func() error {
			if strings.TrimSpace(m.ManualEvidence) == "" {
				return fmt.Errorf("incompletely mapped: manual mapping names no evidence")
			}
			d, ok := latestBoundDecision(m, decisions)
			switch {
			case !ok:
				return fmt.Errorf("manual evidence %q has no recorded acceptance decision bound to this criterion and attempt", m.ManualEvidence)
			case d.Decision != DecisionAccept:
				return fmt.Errorf("manual evidence %q was not accepted (decision %q by %s)", m.ManualEvidence, d.Decision, d.Approver)
			case strings.TrimSpace(d.Approver) == "" || strings.TrimSpace(d.Evidence) == "" || d.RecordedAt == "":
				return fmt.Errorf("manual acceptance decision is not traceable: it must name the approver, evidence and timestamp")
			}
			return nil
		}
	}
	return func() error { return fmt.Errorf("incompletely mapped: unknown evidence kind %q", m.Kind) }
}

// latestDecision returns the most recent decision recorded for a criterion, so a
// later rejection overrides an earlier acceptance.
func latestDecision(criterionID string, decisions []AcceptanceDecision) (AcceptanceDecision, bool) {
	for i := len(decisions) - 1; i >= 0; i-- {
		if decisions[i].CriterionID == criterionID {
			return decisions[i], true
		}
	}
	return AcceptanceDecision{}, false
}

// latestBoundDecision returns the most recent decision that is bound to this exact
// mapping. A decision for an earlier description, manual-evidence text, or attempt
// is ignored (it cannot satisfy the current criterion). A decision recorded
// without binding fields is legacy and is not treated as traceable.
func latestBoundDecision(m CriterionMapping, decisions []AcceptanceDecision) (AcceptanceDecision, bool) {
	for i := len(decisions) - 1; i >= 0; i-- {
		d := decisions[i]
		if d.CriterionID != m.CriterionID {
			continue
		}
		if !decisionBoundTo(d, m) {
			continue
		}
		return d, true
	}
	return AcceptanceDecision{}, false
}

// decisionBoundTo reports whether d names the exact criterion content, manual
// evidence, and attempt of m. The attempt is compared only when the mapping
// carries one, so a legacy mapping without attempt information stays usable.
func decisionBoundTo(d AcceptanceDecision, m CriterionMapping) bool {
	if strings.TrimSpace(d.CriterionDescription) == "" ||
		strings.TrimSpace(d.ManualEvidence) == "" ||
		d.CriterionDescription != m.Description ||
		d.ManualEvidence != m.ManualEvidence {
		return false
	}
	if m.Attempt != 0 && d.Attempt != m.Attempt {
		return false
	}
	return true
}

// BlocksAdvancement reports whether a mapped-plan verdict must stop downstream work:
// any acceptance-evidence result that is not VERIFIED. A result from the legacy Verify
// path or a legacy compatibility result returns false and keeps the existing
// verdict-gate behavior.
func (r Result) BlocksAdvancement() bool {
	return r.Validation != nil && r.Compatibility == "" && r.Status != Verified
}

// ValidateAcceptanceDecision fails closed unless d is an explicit, traceable decision
// by a designated approver on a criterion mapped to manual evidence, and is bound
// to that mapping's exact criterion content, manual evidence, and attempt.
func ValidateAcceptanceDecision(d AcceptanceDecision, mappings []CriterionMapping, designated []string) error {
	switch {
	case d.Task == "":
		return fmt.Errorf("outcome: an acceptance decision must name its task")
	case d.Decision != DecisionAccept && d.Decision != DecisionReject:
		return fmt.Errorf("outcome: acceptance decision must be %q or %q, got %q", DecisionAccept, DecisionReject, d.Decision)
	case !IsDesignatedApprover(d.Approver, designated):
		return fmt.Errorf("outcome: %q is not a designated approver; %w", d.Approver, ErrNoDesignatedApprover)
	case strings.TrimSpace(d.Evidence) == "":
		return fmt.Errorf("outcome: an acceptance decision requires evidence")
	case d.RecordedAt == "":
		return fmt.Errorf("outcome: an acceptance decision requires a recording timestamp")
	}
	for _, m := range mappings {
		if m.CriterionID == d.CriterionID && m.Kind == EvidenceManual {
			if !decisionBoundTo(d, m) {
				return fmt.Errorf("outcome: criterion %q decision is not bound to the current criterion text, manual evidence, and attempt", d.CriterionID)
			}
			return nil
		}
	}
	return fmt.Errorf("outcome: criterion %q is not mapped to manual evidence", d.CriterionID)
}
