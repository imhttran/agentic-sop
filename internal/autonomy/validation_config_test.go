package autonomy

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/failure"
)

// TestValidationNotConfiguredIsTerminalNotHuman proves a missing required validation
// set is a terminal configuration block at every autonomy level: there is no human
// decision to approve or decline, and it must never be auto-continued.
func TestValidationNotConfiguredIsTerminalNotHuman(t *testing.T) {
	c := failure.Classification{
		Kind:        failure.ValidationNotConfigured,
		Disposition: failure.Block,
		Confidence:  failure.High,
		Reason:      "no validation configured",
	}
	for _, level := range []Level{Low, Balanced, High} {
		d := Decide(c, PolicyFor(level))
		if d.Action != ActionTerminal {
			t.Fatalf("%s: action = %s, want TERMINAL (operator intervention)", level, d.Action)
		}
		if d.RequiresHuman {
			t.Fatalf("%s: a configuration block must not require a human approval", level)
		}
		if d.Action == ActionAutoContinue || d.Action == ActionAutoFix {
			t.Fatalf("%s: a missing validation set must not be auto-continued or auto-fixed", level)
		}
	}
}
