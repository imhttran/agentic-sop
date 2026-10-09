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
	Evidence    string
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
		cr := CriterionResult{ID: c.ID, Required: c.Required, Boundary: c.Boundary, Description: c.Description}
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
