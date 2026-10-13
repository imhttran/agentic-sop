// Package outcome verifies an objective's explicit acceptance criteria against the
// workspace, independently of any model self-report. It is the narrow, deterministic
// outcome-level verifier for the AUTONOMY milestone: a criterion's Check is a
// caller-supplied deterministic predicate (typically a file-content assertion, or an
// exit-code check composed from internal/validate). A model's PASS/LOCAL_DONE is never
// an input, so a fabricated claim cannot establish VERIFIED.
//
// It composes existing deterministic gates; it is a verifier, not an orchestrator. It
// starts no work and transitions no SOP state.
package outcome

import "fmt"

// Status is the outcome-level verdict.
type Status string

const (
	// Verified: every required criterion was independently met.
	Verified Status = "VERIFIED"
	// Partial: safe work was done, but a named required criterion is unmet or
	// unverifiable.
	Partial Status = "PARTIAL"
	// Hold: a protected/authority boundary was crossed, so the outcome cannot be
	// claimed regardless of the other criteria.
	Hold Status = "HOLD"
)

// Criterion is one explicit acceptance criterion for an objective. The criterion is
// caller-owned: it is never taken from a model response.
type Criterion struct {
	// ID identifies the criterion (stable, caller-owned).
	ID string
	// Required marks a criterion whose non-satisfaction prevents VERIFIED.
	Required bool
	// Boundary marks a safety/authority criterion (for example "a protected file is
	// unchanged"). A boundary criterion that is NOT met yields HOLD.
	Boundary bool
	// Description is the human-readable criterion text.
	Description string
	// Command is the deterministic check command whose exit status backs this
	// criterion (empty when the criterion has no command). It is recorded in the
	// outcome artifact as per-criterion evidence.
	Command string
	// Check is the deterministic predicate: a nil error means met. A nil Check is
	// unverifiable and counts as unmet; a model claim cannot satisfy it.
	Check func() error
}

// CriterionResult records how one criterion was evaluated.
type CriterionResult struct {
	ID          string
	Required    bool
	Boundary    bool
	Met         bool
	Description string
	Command     string
	Evidence    string
	// Argv is the exact argument vector that executed for this criterion's check
	// (empty when the criterion ran no external command).
	Argv []string
	// ExitCode is the process exit code observed for the executed check (0 when the
	// check did not run an external command, or ran and succeeded).
	ExitCode int
	// Output is a bounded excerpt of the executed check's combined stdout/stderr
	// (empty when the check produced no output or ran no external command).
	Output string
}

// Result is the deterministic outcome verdict.
type Result struct {
	Status   Status
	Criteria []CriterionResult
	Reasons  []string
}

// Verify evaluates each criterion deterministically and derives the status:
//
//   - any Boundary criterion unmet       -> HOLD (an authority boundary was crossed)
//   - else any Required criterion unmet,
//     or no Required criterion declared  -> PARTIAL
//   - else                               -> VERIFIED
//
// It never consults a model, a confidence score, or a recommendation.
func Verify(criteria []Criterion) Result {
	res := Result{Criteria: make([]CriterionResult, 0, len(criteria))}
	boundaryViolated := false
	requiredUnmet := false
	requiredTotal := 0

	for _, c := range criteria {
		cr := CriterionResult{ID: c.ID, Required: c.Required, Boundary: c.Boundary, Description: c.Description, Command: c.Command}
		switch {
		case c.Check == nil:
			cr.Met = false
			cr.Evidence = "unverifiable: no deterministic check supplied"
		default:
			if err := c.Check(); err != nil {
				cr.Met = false
				cr.Evidence = err.Error()
			} else {
				cr.Met = true
				cr.Evidence = "met"
			}
		}
		res.Criteria = append(res.Criteria, cr)

		if c.Boundary && !cr.Met {
			boundaryViolated = true
		}
		if c.Required {
			requiredTotal++
			if !cr.Met {
				requiredUnmet = true
			}
		}
	}

	switch {
	case boundaryViolated:
		res.Status = Hold
		for _, cr := range res.Criteria {
			if cr.Boundary && !cr.Met {
				res.Reasons = append(res.Reasons, fmt.Sprintf("boundary criterion %s not satisfied: %s", cr.ID, cr.Evidence))
			}
		}
	case requiredTotal == 0:
		res.Status = Partial
		res.Reasons = append(res.Reasons, "the objective declares no required criterion; nothing is verified")
	case requiredUnmet:
		res.Status = Partial
		for _, cr := range res.Criteria {
			if cr.Required && !cr.Met {
				res.Reasons = append(res.Reasons, fmt.Sprintf("required criterion %s not met: %s", cr.ID, cr.Evidence))
			}
		}
	default:
		res.Status = Verified
		res.Reasons = append(res.Reasons, "every required criterion was independently met")
	}
	return res
}

// PropagateEvidence copies the per-check execution evidence recorded by an executor
// onto the matching criterion result, so a criterion backed by an executed check
// carries the exact argv, exit code, and bounded output excerpt. It is a no-op for
// criteria that ran no external command (an empty argv).
func (r *Result) PropagateEvidence(id string, argv []string, exitCode int, output string) {
	for i := range r.Criteria {
		if r.Criteria[i].ID != id {
			continue
		}
		r.Criteria[i].Argv = append([]string(nil), argv...)
		r.Criteria[i].ExitCode = exitCode
		r.Criteria[i].Output = output
		return
	}
}

// HasUnmetRequired reports whether the verdict carries at least one required
// criterion that was not met. It is the classification the lifecycle gate reads: a
// PARTIAL from an unmet REQUIRED deliverable or REQUIRED executable check is exactly a
// result whose status is Partial and whose unmet criteria include at least one
// Required one. A PARTIAL from informational (non-required) criteria only, and a HOLD,
// both return false here and keep their existing lifecycle mapping.
func (r Result) HasUnmetRequired() bool {
	for _, cr := range r.Criteria {
		if cr.Required && !cr.Met {
			return true
		}
	}
	return false
}

// Lifecycle is the local lifecycle disposition the verdict resolves to. LOCAL_DONE is
// the terminal transition for a locally completed task; NEEDS_HUMAN is the existing
// human boundary. The lifecycle is expressed with the pinned terminology
// (LOCAL_DONE / PARTIAL / HOLD) and is returned by ResolveLifecycle.
type Lifecycle string

const (
	// LocalDone: the local lifecycle completed and reached its terminal transition.
	LocalDone Lifecycle = "LOCAL_DONE"
	// NeedsHuman: the lifecycle stopped at the human boundary instead of completing.
	NeedsHuman Lifecycle = "NEEDS_HUMAN"
)

// Waiver is a designated approver's explicit, criterion-scoped record authorizing a
// required-criterion failure to be waived so a legitimate LOCAL_DONE is explicit. A
// waiver is never implied: it must name the approver, the criteria it covers, a
// reason, and a timestamp.
type Waiver struct {
	// Approver identifies the designated approver who granted the waiver (attribution).
	// An empty Approver is not a designated approver and the waiver is invalid.
	Approver string
	// Criteria names the required criteria this waiver covers (criterion-scoped). It
	// must name every unmet required criterion the waiver intends to cover.
	Criteria []string
	// Reason is the approver's recorded justification. A waiver without a reason is
	// invalid: it is never silently applied.
	Reason string
	// RecordedAt is the approver's timestamp for the grant.
	RecordedAt string
}

// ErrNoDesignatedApprover is returned when a waiver cannot be validated because the
// repository provides no designated-approver identity source. It is a distinct,
// explicit error: the unsupported path never silently accepts a waiver.
var ErrNoDesignatedApprover = fmt.Errorf("outcome: no designated-approver identity source is available; a waiver cannot be validated")

// ValidateWaiver reports whether w is a valid waiver covering every unmet required
// criterion of the result r. A waiver is valid only when:
//
//   - the result is a PARTIAL with at least one unmet required criterion
//     (informational-only PARTIAL and HOLD are never waivable here);
//   - the designated approver is identified (a non-empty approver);
//   - the waiver carries a reason and a recording timestamp;
//   - the waiver names, criterion-scoped, every unmet required criterion of r.
//
// It fails closed: any unmet required criterion the waiver does not name makes the
// waiver invalid, so a partial waiver can never silently waive the rest. A HOLD (an
// authority boundary) is never waivable.
func ValidateWaiver(r Result, w Waiver) error {
	if r.Status == Hold {
		return fmt.Errorf("outcome: a HOLD (authority boundary crossed) is not waivable")
	}
	if !r.HasUnmetRequired() {
		return fmt.Errorf("outcome: no unmet required criterion to waive")
	}
	if w.Approver == "" {
		return fmt.Errorf("outcome: waiver requires a designated approver; %w", ErrNoDesignatedApprover)
	}
	if w.Reason == "" {
		return fmt.Errorf("outcome: waiver requires a recorded reason")
	}
	if w.RecordedAt == "" {
		return fmt.Errorf("outcome: waiver requires a recording timestamp")
	}
	covered := make(map[string]bool, len(w.Criteria))
	for _, id := range w.Criteria {
		covered[id] = true
	}
	for _, cr := range r.Criteria {
		if cr.Required && !cr.Met && !covered[cr.ID] {
			return fmt.Errorf("outcome: waiver does not cover required criterion %s; a waiver must name every unmet required criterion", cr.ID)
		}
	}
	return nil
}

// ResolveLifecycle maps an outcome verdict to the local lifecycle disposition.
//
// With the operator-enabled verdict gate OFF (the default), the mapping is the
// existing, unchanged behavior: every verdict the lifecycle reaches here yields
// LOCAL_DONE — the verdict is reported only and never changes the transition. This is
// the byte-identical flag-off path.
//
// With the gate ON, the pinned lifecycle semantics apply:
//
//   - VERIFIED                              -> LOCAL_DONE
//   - PARTIAL with an unmet REQUIRED
//     deliverable/check, no valid waiver    -> NEEDS_HUMAN (does not yield LOCAL_DONE)
//   - PARTIAL with an unmet REQUIRED
//     deliverable/check, valid waiver       -> LOCAL_DONE
//   - PARTIAL from informational-only
//     criteria (no unmet required)          -> LOCAL_DONE
//   - HOLD                                  -> NEEDS_HUMAN
//
// The waiver is consulted only for the PARTIAL-with-unmet-required case; it can never
// turn a HOLD into LOCAL_DONE. It never consults a model or a confidence score.
func ResolveLifecycle(gateEnabled bool, r Result, waiver *Waiver) Lifecycle {
	if !gateEnabled {
		return LocalDone
	}
	switch r.Status {
	case Verified:
		return LocalDone
	case Hold:
		return NeedsHuman
	case Partial:
		if !r.HasUnmetRequired() {
			// Informational-only PARTIAL: informational criteria do not gate.
			return LocalDone
		}
		if waiver != nil {
			if err := ValidateWaiver(r, *waiver); err == nil {
				return LocalDone
			}
		}
		return NeedsHuman
	default:
		return LocalDone
	}
}
