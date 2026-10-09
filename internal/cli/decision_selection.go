package cli

import (
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/decision"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// This file is the recording seam for SOP's deterministic engineering-approach
// selection (internal/decision.Select). A caller supplies the candidate approaches
// it has; SOP records the alternatives, the selected approach, and the rationale in
// the run artifacts and renders a concise choice/reason summary. It introduces no
// decision framework: the selection is the pure decision.Select result, and the
// artifacts reuse the existing run-artifact plumbing.

// approachSelection evaluates candidate approaches deterministically; it returns nil
// when no alternatives were supplied, so a caller with nothing to choose is a no-op.
func approachSelection(alts []decision.Alternative) *decision.Selection {
	if len(alts) == 0 {
		return nil
	}
	sel := decision.Select(alts)
	return &sel
}

// recordApproachSelection records the alternatives, the selected approach, and the
// rationale as a run artifact, and returns the selection.
func recordApproachSelection(rn *runpkg.Run, alts []decision.Alternative) *decision.Selection {
	sel := approachSelection(alts)
	if sel == nil {
		return nil
	}
	writeRunJSON(rn, "decision-selection.json", sel)
	return sel
}

// writeDecisionSummary renders the concise decision summary: the choice and its
// reason. Validation and status remain the run's own summary lines.
func writeDecisionSummary(w io.Writer, sel *decision.Selection) {
	if sel == nil {
		return
	}
	fmt.Fprintf(w, "decision: %s\n", sel.Choice)
	if sel.Reason != "" {
		fmt.Fprintf(w, "  reason: %s\n", sel.Reason)
	}
}
