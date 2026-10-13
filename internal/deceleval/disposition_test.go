package deceleval

import "testing"

// TestPostureOf pins the action -> execution-posture classification and the
// withhold predicate.
func TestPostureOf(t *testing.T) {
	cases := map[string]ExecutionPosture{
		"AUTO_FIX":    PostureProceed,
		"CONTINUE":    PostureProceed,
		"RETRY":       PostureProceed,
		"REPLAN":      PostureProceed,
		"NEEDS_HUMAN": PostureHuman,
		"BLOCK":       PostureTerminal,
		"":            PostureUnknown,
		"WAT":         PostureUnknown,
	}
	for a, want := range cases {
		if got := PostureOf(a); got != want {
			t.Errorf("PostureOf(%q) = %q, want %q", a, got, want)
		}
	}
	for _, a := range []string{"NEEDS_HUMAN", "BLOCK"} {
		if !WithholdsExecution(a) {
			t.Errorf("WithholdsExecution(%q) = false, want true", a)
		}
	}
	for _, a := range []string{"AUTO_FIX", "CONTINUE", "RETRY", "REPLAN"} {
		if WithholdsExecution(a) {
			t.Errorf("WithholdsExecution(%q) = true, want false", a)
		}
	}
}

// TestAnalyzeExecutionPostureSeparation proves the four mismatch buckets:
// less-conservative (unsafe), more-conservative, both-withhold, and
// posture-equivalent — with matches excluded.
func TestAnalyzeExecutionPostureSeparation(t *testing.T) {
	b := BaselineResult{Records: []BaselineRecord{
		{ScenarioID: "a", Expected: "NEEDS_HUMAN", Action: "BLOCK"},              // both withhold, different posture
		{ScenarioID: "b", Expected: "BLOCK", Action: "CONTINUE"},                 // baseline proceeds while label withholds
		{ScenarioID: "c", Expected: "AUTO_FIX", Action: "BLOCK"},                 // baseline withholds while label proceeds
		{ScenarioID: "d", Expected: "AUTO_FIX", Action: "CONTINUE"},              // both proceed automatically
		{ScenarioID: "e", Expected: "AUTO_FIX", Action: "AUTO_FIX"},              // exact match -> excluded
		{ScenarioID: "f", Expected: "RETRY", Action: "BLOCK", FixtureError: "x"}, // fixture error -> excluded
	}}
	a := AnalyzeExecutionPosture(b)
	if a.Total != 4 || a.LessConservative != 1 || a.MoreConservative != 1 || a.BothWithhold != 1 || a.PostureEquivalent != 1 {
		t.Fatalf("buckets = total %d less %d more %d both %d equiv %d, want 4/1/1/1/1",
			a.Total, a.LessConservative, a.MoreConservative, a.BothWithhold, a.PostureEquivalent)
	}
	if len(a.Rows) != 4 || a.Rows[0].ScenarioID != "a" || a.Rows[3].ScenarioID != "d" {
		t.Errorf("rows not sorted/complete: %+v", a.Rows)
	}
}

// TestAnalyzeExecutionPostureFrozen proves the analysis on the frozen baseline is
// deterministic and matches the recorded corpus: 14 exact mismatches reduce to 0
// unsafe continuations, 1 more-conservative, 1 both-withhold (dme-real-close-004),
// and 12 posture-equivalent.
func TestAnalyzeExecutionPostureFrozen(t *testing.T) {
	corpusPath, freezePath := corpusPaths()
	if err := VerifyFreeze(corpusPath, freezePath); err != nil {
		t.Fatalf("corpus freeze: %v", err)
	}
	c, err := Load(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := FreezeOf(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	a := AnalyzeExecutionPosture(RunBaseline(c, sha))
	if a.Total != 14 || a.LessConservative != 0 || a.MoreConservative != 1 || a.BothWithhold != 1 || a.PostureEquivalent != 12 {
		t.Fatalf("frozen buckets = total %d less %d more %d both %d equiv %d, want 14/0/1/1/12",
			a.Total, a.LessConservative, a.MoreConservative, a.BothWithhold, a.PostureEquivalent)
	}
	var close004 *PostureMismatch
	for i := range a.Rows {
		if a.Rows[i].ScenarioID == "dme-real-close-004" {
			close004 = &a.Rows[i]
		}
	}
	if close004 == nil || close004.Baseline != "BLOCK" || close004.Expected != "NEEDS_HUMAN" ||
		close004.BaselinePosture != PostureTerminal || close004.ExpectedPosture != PostureHuman {
		t.Errorf("dme-real-close-004 posture = %+v, want BLOCK/TERMINAL vs NEEDS_HUMAN/HUMAN", close004)
	}
}
