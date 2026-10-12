package prompt

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
)

// TestCompileCarriesAcceptanceCriterionOnce proves the RBV-011 dedup guarantee: the
// compiler carries an acceptance criterion exactly once (in the task section, which
// the request carries as its own field) and never duplicates it into the evidence
// sections, and compilation is deterministic for identical input.
func TestCompileCarriesAcceptanceCriterionOnce(t *testing.T) {
	const criterion = "the widget starts within 5 seconds"
	in := Input{
		Capability:         agent.Implement,
		Class:              Medium,
		Task:               "Implement the widget.\n\nAcceptance criteria:\n- " + criterion,
		Context:            sopctx.FromInputs(sopctx.Inputs{TaskID: "T1", Plan: "# Plan\n\n## T1 — Widget\n\nDo the work."}, sopctx.DefaultLimits()),
		OutputRequirements: "Summarize the changes.",
	}
	c := Compile(in)

	if n := strings.Count(c.Render(), criterion); n != 1 {
		t.Errorf("acceptance criterion appears %d times in the compiled prompt, want exactly once", n)
	}
	if strings.Contains(c.Evidence(), criterion) {
		t.Errorf("the evidence section duplicates the acceptance criterion:\n%s", c.Evidence())
	}
	if again := Compile(in); again.Render() != c.Render() || again.Digest != c.Digest {
		t.Error("Compile is not deterministic for identical input")
	}
}
