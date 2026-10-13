package planner

import "fmt"

// Warning is a non-fatal authoring diagnostic surfaced during plan compilation
// and migration. Unlike a validation error, a warning never blocks a plan: an
// existing plan authored before a field existed must keep loading. The caller
// reports warnings to the operator and continues.
type Warning struct {
	// StageID names the stage the warning is about. Empty for a plan-level
	// warning.
	StageID string
	// Message is the human-readable diagnostic. It always names the offending
	// stage so a warning is actionable on its own.
	Message string
}

// String renders a warning as "stage <id>: <message>" (or just the message for a
// plan-level warning).
func (w Warning) String() string {
	if w.StageID == "" {
		return w.Message
	}
	return fmt.Sprintf("stage %s: %s", w.StageID, w.Message)
}

// Lint returns the non-fatal authoring warnings for the plan. It is deliberately
// separate from Validate: a warning reports an authoring gap (a stage that
// declares deliverables but no machine-checkable acceptance checks) so migration
// can flag it without rejecting the plan. Decision: a missing check is a warning,
// never a hard failure.
//
// Today the only warning is a stage that declares deliverables but no acceptance
// checks: the free-text AcceptanceCriteria describe the criterion, but without an
// AcceptanceChecks command whose exit status 0 proves it, the deliverable cannot be
// machine-checked. The warning names each offending stage.
func (p *Plan) Lint() []Warning {
	if p == nil {
		return nil
	}
	var warnings []Warning
	for _, stage := range p.Stages {
		if len(stage.Deliverables) == 0 {
			continue
		}
		if len(stage.AcceptanceChecks) > 0 {
			continue
		}
		warnings = append(warnings, Warning{
			StageID: stage.ID,
			Message: "declares deliverables but no acceptance checks; add an '### Acceptance Checks' section with a command whose exit status 0 proves each deliverable",
		})
	}
	return warnings
}

// LintWarnings returns the non-fatal authoring warnings for a loaded plan. It is
// the migration entry point: a plan migrated from the pre-AcceptanceChecks format
// is flagged here rather than rejected. The plan is not modified and no error is
// ever returned, so a check-less plan stays loadable and valid.
func LintWarnings(plan *Plan) []Warning {
	return plan.Lint()
}
