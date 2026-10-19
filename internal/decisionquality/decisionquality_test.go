package decisionquality

import (
	"path/filepath"
	"strings"
	"testing"
)

const evalDir = "../../evals/decision-quality"

func TestFrozenCorpusSatisfiesContract(t *testing.T) {
	c, err := LoadCorpus(filepath.Join(evalDir, "corpus-heldout.json"))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(c.Cases) != 30 {
		t.Fatalf("cases = %d, want 30", len(c.Cases))
	}
	counts := c.Counts()
	for _, class := range []string{ClassPlanning, ClassRecovery, ClassNegative} {
		if counts[class] != 10 {
			t.Errorf("%s = %d, want 10", class, counts[class])
		}
	}
}

func TestFreezeSidecarsMatchFrozenInputs(t *testing.T) {
	for _, name := range []string{"PROTOCOL.sha256", "CORPUS.sha256"} {
		if err := VerifyFreeze(evalDir, filepath.Join(evalDir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestRubricRequiresEveryAnswer(t *testing.T) {
	r, err := LoadRubric(filepath.Join(evalDir, "RUBRIC.json"))
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]bool{}
	for _, c := range append(append([]string{}, r.RequiredChecks...), r.SafetyChecks...) {
		all[c] = true
	}
	if ok, failed := r.Score(all); !ok {
		t.Fatalf("all checks true failed: %v", failed)
	}
	delete(all, "no_post_decision_leak")
	if ok, failed := r.Score(all); ok || len(failed) != 1 || failed[0] != "no_post_decision_leak" {
		t.Fatalf("missing answer = %v, %v; a missing measurement must not pass", ok, failed)
	}
}

func TestPairedRunClustersRepeatsByCase(t *testing.T) {
	mk := func(arm string, c1, c2 bool) RunRecord {
		r := RunRecord{Arm: arm}
		for rep := 1; rep <= 3; rep++ {
			r.Results = append(r.Results,
				CaseResult{Arm: arm, CaseID: "c1", Repeat: rep, Correct: c1},
				CaseResult{Arm: arm, CaseID: "c2", Repeat: rep, Correct: c2},
			)
		}
		return r
	}
	a := mk("A", false, true)
	b := mk("B", true, true)
	p := PairedRun(a, b, 42)
	if p.CaseCount != 2 {
		t.Fatalf("paired cases = %d, want 2 (repeats are not independent)", p.CaseCount)
	}
	if p.MeanDiffPP != 50 {
		t.Fatalf("mean diff = %.2f, want 50", p.MeanDiffPP)
	}
	if p.LowerPP > p.UpperPP {
		t.Fatalf("interval inverted: %v..%v", p.LowerPP, p.UpperPP)
	}
	// Determinism: the same seed yields the same interval.
	if q := PairedRun(a, b, 42); q.LowerPP != p.LowerPP || q.UpperPP != p.UpperPP {
		t.Fatalf("bootstrap is not deterministic")
	}
}

func TestInsufficientEvidenceIsExplicit(t *testing.T) {
	c := Corpus{Cases: []Case{{ID: "c1"}, {ID: "c2"}}}
	b := RunRecord{Arm: "B", Results: []CaseResult{{CaseID: "c1", Correct: true}, {CaseID: "c2", NotEvaluable: true}}}
	if got := InsufficientEvidence(c, RunRecord{Arm: "A"}, b); got == "" {
		t.Fatal("a not-evaluable case must be reported as insufficient evidence")
	}
}

func TestReportRecordsMissingEvidence(t *testing.T) {
	in := ReportInput{
		Corpus:       Corpus{Cases: []Case{{ID: "c1", Class: ClassPlanning}}},
		Rules:        FrozenRules(),
		A:            ArmSummary{Arm: "A"},
		B:            ArmSummary{Arm: "B"},
		Paired:       PairedResult{Base: "A", Arm: "B"},
		Gate:         Gate{},
		Decision:     "retain_disabled_defaults",
		Insufficient: "arm B evaluated 0 of 1 cases",
		NotEvaluated: []string{"C", "P"},
	}
	out := RenderReport(in)
	for _, want := range []string{"Insufficient evidence", "Not evaluated: C, P", "no go/no-go conclusion"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}
