package retrievalgate

import "testing"

// TestEvaluateIsDeterministic proves the gate is pure: identical inputs yield an identical
// report, including the decision.
func TestEvaluateIsDeterministic(t *testing.T) {
	a := Evaluate(Corpus(), Candidates(), 5)
	b := Evaluate(Corpus(), Candidates(), 5)
	if a.String() != b.String() || a.Decision != b.Decision {
		t.Errorf("gate is not deterministic:\n a=%s\n b=%s", a.String(), b.String())
	}
	if a.K != 5 || a.Cases != len(Corpus()) {
		t.Errorf("report shape = %+v", a)
	}
}

// TestGatePasses proves structural + BM25 retrieval beats the unranked baseline on the
// representative corpus with no recall regression.
func TestGatePasses(t *testing.T) {
	rep := Evaluate(Corpus(), Candidates(), 5)
	t.Logf("\n%s", rep.String())
	if rep.Decision != Pass {
		t.Fatalf("RETRIEVAL_GATE = %s, want PASS: %s", rep.Decision, rep.Reason)
	}
	if rep.Candidate.Precision < rep.Baseline.Precision || rep.Candidate.MRR < rep.Baseline.MRR {
		t.Errorf("candidate must not regress: base=%+v cand=%+v", rep.Baseline, rep.Candidate)
	}
	if len(rep.NotMeasured) == 0 {
		t.Error("the gate must name the signals it does not measure")
	}
}

func TestEvaluateEmptyCorpusFails(t *testing.T) {
	if rep := Evaluate(nil, Candidates(), 5); rep.Decision != Fail {
		t.Errorf("an empty corpus must FAIL, got %s", rep.Decision)
	}
}
