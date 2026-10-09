package quality

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/review"
)

func acceptancePolicy() config.Quality {
	return config.Quality{MaxFixCycles: 3, FailOn: []string{"critical", "high"}}
}

// (1) All criteria verified → completion eligible.
func TestAcceptanceSatisfiedPasses(t *testing.T) {
	r := Evaluate(acceptancePolicy(), Input{
		BuildPassed: true, TestPassed: true,
		Acceptance: &Acceptance{Enforced: true, Satisfied: true},
	})
	if r.Decision != Pass {
		t.Fatalf("decision = %s (%v), want PASS", r.Decision, r.Reasons)
	}
}

// (2)/(3) NOT_MET or UNAVAILABLE (both surfaced as !Satisfied) → completion blocked.
func TestAcceptanceUnsatisfiedBlocksCompletion(t *testing.T) {
	r := Evaluate(acceptancePolicy(), Input{
		BuildPassed: true, TestPassed: true,
		Acceptance: &Acceptance{Enforced: true, Satisfied: false, Reason: "c=NOT_MET"},
	})
	if r.Decision != Fail {
		t.Fatalf("decision = %s, want FAIL", r.Decision)
	}
}

// (4) Missing/malformed bindings and missing evidence surface as a blocking reason.
func TestAcceptanceMissingEvidenceBlocks(t *testing.T) {
	r := Evaluate(acceptancePolicy(), Input{
		BuildPassed: true, TestPassed: true,
		Acceptance: &Acceptance{Enforced: true, Satisfied: false},
	})
	if r.Decision != Fail {
		t.Fatalf("decision = %s, want FAIL", r.Decision)
	}
}

// (13) Tasks without acceptance criteria / enforcement disabled → unchanged.
func TestAcceptanceNilOrDisabledUnchanged(t *testing.T) {
	for _, a := range []*Acceptance{nil, {Enforced: false, Satisfied: false}} {
		r := Evaluate(acceptancePolicy(), Input{BuildPassed: true, TestPassed: true, Acceptance: a})
		if r.Decision != Pass {
			t.Fatalf("acceptance %+v: decision = %s, want PASS (unchanged)", a, r.Decision)
		}
	}
}

// (11) FIX budget exhaustion with an acceptance failure → existing human escalation.
func TestAcceptanceExhaustionNeedsHuman(t *testing.T) {
	r := Evaluate(acceptancePolicy(), Input{
		BuildPassed: true, TestPassed: true, FixCycles: 3,
		Acceptance: &Acceptance{Enforced: true, Satisfied: false, Reason: "c=UNAVAILABLE"},
	})
	if r.Decision != NeedsHuman {
		t.Fatalf("decision = %s, want NEEDS_HUMAN", r.Decision)
	}
}

// (10) Human approval still takes precedence.
func TestAcceptanceDoesNotOverrideHumanBoundary(t *testing.T) {
	r := Evaluate(acceptancePolicy(), Input{
		BuildPassed: true, TestPassed: true, HumanRequired: true,
		Acceptance: &Acceptance{Enforced: true, Satisfied: false},
	})
	if r.Decision != NeedsHuman {
		t.Fatalf("decision = %s, want NEEDS_HUMAN", r.Decision)
	}
}

// (9) Severity-based quality failure still blocks.
func TestAcceptancePreservesSeverityChecks(t *testing.T) {
	r := Evaluate(acceptancePolicy(), Input{
		BuildPassed: true, TestPassed: true,
		Unresolved: []review.Finding{{Severity: review.Severity("HIGH"), Title: "x"}},
		Acceptance: &Acceptance{Enforced: true, Satisfied: true},
	})
	if r.Decision != Fail {
		t.Fatalf("decision = %s, want FAIL from the HIGH finding", r.Decision)
	}
}
