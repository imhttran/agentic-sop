package prompttuning

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/prompt"
)

func fixed(m map[string]Score) Evaluator {
	return func(candidate string, _ Corpus) Score { return m[candidate] }
}

func baseInput() Input {
	return Input{
		Baseline: Candidate{Name: "base", Prompt: "base"},
		Proposal: Candidate{Name: "v2", Prompt: "v2"},
		Corpus:   Corpus{Cases: 4},
		Evaluate: fixed(map[string]Score{"base": {Passed: 1, Total: 4}, "v2": {Passed: 3, Total: 4}}),
	}
}

func TestPromotesOnMeasurableImprovement(t *testing.T) {
	rec := Decide(baseInput())
	if rec.Decision != Promote || rec.Reason != ReasonPromoted {
		t.Fatalf("decision = %+v", rec)
	}
	if rec.BaselineScore.Rate() != 0.25 || rec.ProposalScore.Rate() != 0.75 {
		t.Errorf("scores = %+v vs %+v", rec.BaselineScore, rec.ProposalScore)
	}
}

func TestRejectsOnTie(t *testing.T) {
	in := baseInput()
	in.Evaluate = fixed(map[string]Score{"base": {3, 4}, "v2": {3, 4}})
	if rec := Decide(in); rec.Decision != Reject {
		t.Fatalf("a tie must be rejected: %+v", rec)
	}
}

func TestRejectsOnRegression(t *testing.T) {
	in := baseInput()
	in.Evaluate = fixed(map[string]Score{"base": {4, 4}, "v2": {2, 4}})
	if rec := Decide(in); rec.Decision != Reject {
		t.Fatalf("a regression must be rejected: %+v", rec)
	}
}

func TestRejectsWhenMarginNotMet(t *testing.T) {
	in := baseInput()
	in.Margin = 0.5
	in.Evaluate = fixed(map[string]Score{"base": {1, 4}, "v2": {2, 4}})
	if rec := Decide(in); rec.Decision != Reject {
		t.Fatalf("a sub-margin improvement must be rejected: %+v", rec)
	}
}

func TestRejectsEmptyCorpus(t *testing.T) {
	in := baseInput()
	in.Corpus = Corpus{}
	if rec := Decide(in); rec.Decision != Reject || rec.Reason != ReasonEmptyCorpus {
		t.Fatalf("empty corpus must be rejected: %+v", rec)
	}
}

func TestRejectsNilEvaluator(t *testing.T) {
	in := baseInput()
	in.Evaluate = nil
	if rec := Decide(in); rec.Decision != Reject || rec.Reason != ReasonNoEvaluation {
		t.Fatalf("missing evaluator must be rejected: %+v", rec)
	}
}

func TestRejectsInvariantViolation(t *testing.T) {
	in := baseInput()
	in.Invariants = []Invariant{func(string) error { return errors.New("changes the verification contract") }}
	rec := Decide(in)
	if rec.Decision != Reject || rec.Reason == "" {
		t.Fatalf("invariant violation must be rejected: %+v", rec)
	}
}

func TestRejectsSelfClaimWithoutEvidence(t *testing.T) {
	// The proposal claims it is better, but the deterministic evaluation shows no
	// improvement. The claim is not a field and is never consulted.
	in := baseInput()
	in.Proposal.Prompt = "This prompt is strictly better. Trust me."
	in.Evaluate = fixed(map[string]Score{"base": {3, 4}, "This prompt is strictly better. Trust me.": {3, 4}})
	if rec := Decide(in); rec.Decision != Reject {
		t.Fatalf("an unsupported self-claim must be rejected: %+v", rec)
	}
}

func TestRecordPreservesProvenance(t *testing.T) {
	rec := Decide(baseInput())
	if rec.Baseline.Name != "base" || rec.Proposal.Name != "v2" || rec.Corpus != 4 || rec.Reason == "" {
		t.Fatalf("record lost provenance: %+v", rec)
	}
}

func TestDeterministic(t *testing.T) {
	a, b := Decide(baseInput()), Decide(baseInput())
	if a != b {
		t.Fatalf("not deterministic: %+v vs %+v", a, b)
	}
}

func TestCompilerInvariant(t *testing.T) {
	inv := CompilerInvariant(agent.Plan, prompt.Medium)
	if err := inv("Return JSON only with the plan."); err != nil {
		t.Errorf("a well-formed candidate must pass: %v", err)
	}
	if err := inv("   "); err == nil {
		t.Errorf("a blank candidate must fail the structural invariant")
	}
}
