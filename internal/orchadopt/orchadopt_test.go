package orchadopt

// ORCH-012 adoption-gate tests. They are deterministic and model-free: the gate
// runs scripted agents only, and no provider or network is touched.

import (
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/orchestration"
)

// TestCorpusIsGenuinelyParallelizable proves each case decomposes into more than one
// independent unit (so the comparison is meaningful), and all units are non-mutating.
func TestCorpusIsGenuinelyParallelizable(t *testing.T) {
	cases := Corpus()
	if len(cases) == 0 {
		t.Fatal("empty corpus")
	}
	for _, c := range cases {
		if len(c.Units) < 2 {
			t.Errorf("case %s has %d unit(s); a parallelizable case needs >= 2", c.Name, len(c.Units))
		}
		for _, u := range c.Units {
			if agent.IsRepositoryMutation(u.Capability) {
				t.Errorf("case %s unit %s mutates the repository; the gate corpus is analysis-only", c.Name, u.TaskID)
			}
		}
	}
}

// TestEvaluateMeasuresRequiredDimensions proves the gate measures the plan's
// dimensions and does not invent the ones it cannot observe.
func TestEvaluateMeasuresRequiredDimensions(t *testing.T) {
	rep := Evaluate(Corpus())
	if rep.Cases != len(Corpus()) {
		t.Errorf("cases = %d, want %d", rep.Cases, len(Corpus()))
	}
	if rep.Baseline.TaskTotal != rep.Cases || rep.Candidate.TaskTotal != rep.Cases {
		t.Errorf("task totals = %d/%d, want %d", rep.Baseline.TaskTotal, rep.Candidate.TaskTotal, rep.Cases)
	}
	if rep.Candidate.WorkerCount <= rep.Baseline.WorkerCount {
		t.Errorf("candidate worker count %d should exceed baseline %d (one worker per unit)", rep.Candidate.WorkerCount, rep.Baseline.WorkerCount)
	}
	if rep.Candidate.ToolCalls <= rep.Baseline.ToolCalls {
		t.Errorf("candidate tool calls %d should exceed baseline %d", rep.Candidate.ToolCalls, rep.Baseline.ToolCalls)
	}
	if rep.Candidate.ContextBytes <= 0 || rep.Candidate.ContextItems <= 0 {
		t.Errorf("candidate context not measured: bytes=%d items=%d", rep.Candidate.ContextBytes, rep.Candidate.ContextItems)
	}
	if len(rep.NotMeasured) == 0 {
		t.Error("the gate must name the dimensions it cannot measure (e.g. wall-clock time)")
	}
}

// TestEvaluateRejectsWithoutBenefit proves that a candidate which does more work with
// no deterministic benefit is rejected, so default execution remains single-agent.
func TestEvaluateRejectsWithoutBenefit(t *testing.T) {
	rep := Evaluate(Corpus())
	if rep.Decision != Reject {
		t.Errorf("decision = %s, want %s (multi-agent has no deterministic benefit)", rep.Decision, Reject)
	}
	if rep.Reason == "" {
		t.Error("rejection must carry a reason")
	}
}

// TestEvaluateIsDeterministic proves identical cases yield identical reports.
func TestEvaluateIsDeterministic(t *testing.T) {
	r1 := Evaluate(Corpus())
	r2 := Evaluate(Corpus())
	if !reflect.DeepEqual(r1, r2) {
		t.Error("gate is not deterministic")
	}
}

// TestDecisionRuleAdoptsCheaperCandidate proves the rule is evidence-driven, not
// hard-coded to reject: a candidate that is at least as successful and no more costly
// is adopted.
func TestDecisionRuleAdoptsCheaperCandidate(t *testing.T) {
	baseline := Metrics{TaskTotal: 3, TaskSuccess: 3, VerificationSuccess: 3, WorkerCount: 3, ToolCalls: 3}
	candidate := Metrics{TaskTotal: 3, TaskSuccess: 3, VerificationSuccess: 3, WorkerCount: 3, ToolCalls: 3}
	if got, _ := decide(baseline, candidate); got != Adopt {
		t.Errorf("equal candidate = %s, want %s", got, Adopt)
	}
}

// TestDecisionRuleRejectsRegression proves a candidate that regresses success is
// rejected.
func TestDecisionRuleRejectsRegression(t *testing.T) {
	baseline := Metrics{TaskTotal: 3, TaskSuccess: 3, VerificationSuccess: 3, WorkerCount: 3, ToolCalls: 3}
	candidate := Metrics{TaskTotal: 3, TaskSuccess: 2, VerificationSuccess: 2, WorkerCount: 3, ToolCalls: 3}
	if got, _ := decide(baseline, candidate); got != Reject {
		t.Errorf("regressing candidate = %s, want %s", got, Reject)
	}
}

// TestEmptyCorpusIsSafe proves the gate handles an empty corpus without panicking.
func TestEmptyCorpusIsSafe(t *testing.T) {
	rep := Evaluate(nil)
	if rep.Cases != 0 {
		t.Errorf("cases = %d, want 0", rep.Cases)
	}
	_ = orchestration.StageVerify
}
