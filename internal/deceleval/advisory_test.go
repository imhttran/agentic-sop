package deceleval

import "testing"

// TestBuildPairedMetrics proves the A/B aggregation: a useful escalation, an
// unavailable fallback (no change), and provider usage counts.
func TestBuildPairedMetrics(t *testing.T) {
	c := &Corpus{Scenarios: []Scenario{
		{ID: "a", Category: "security_sensitive", Expected: Expected{Action: NeedsHuman}},
		{ID: "b", Category: "benign_impl", Expected: Expected{Action: AutoFix}},
	}}
	baseline := BaselineResult{CorpusSHA256: "x", Records: []BaselineRecord{
		{ScenarioID: "a", Category: "security_sensitive", Expected: "NEEDS_HUMAN", Action: "BLOCK"},
		{ScenarioID: "b", Category: "benign_impl", Expected: "AUTO_FIX", Action: "AUTO_FIX"},
	}}
	adv := []AdvisoryRecord{
		{ScenarioID: "a", Category: "security_sensitive", Expected: "NEEDS_HUMAN", BaselineAction: "BLOCK", FinalAction: "NEEDS_HUMAN", Consulted: true, ProviderUsed: "clef", LatencyMS: 120},
		{ScenarioID: "b", Category: "benign_impl", Expected: "AUTO_FIX", BaselineAction: "AUTO_FIX", FinalAction: "AUTO_FIX", Consulted: true, OutcomeClass: "unavailable", ProviderUsed: "unavailable", LatencyMS: 50},
	}
	res := BuildPaired(c, baseline, adv)
	m := res.Metrics
	if m.Paired != 2 {
		t.Fatalf("paired = %d, want 2", m.Paired)
	}
	if m.DeterministicCorrect != 1 || m.AdvisoryCorrect != 2 {
		t.Errorf("correct det=%d adv=%d, want 1/2", m.DeterministicCorrect, m.AdvisoryCorrect)
	}
	if m.UnsafeMissedDeterministic != 1 || m.UnsafeMissedAdvisory != 0 {
		t.Errorf("unsafe missed det=%d adv=%d, want 1/0", m.UnsafeMissedDeterministic, m.UnsafeMissedAdvisory)
	}
	if m.UsefulEscalations != 1 || m.FalseEscalations != 0 || m.FinalActionsChanged != 1 {
		t.Errorf("useful=%d false=%d changed=%d, want 1/0/1", m.UsefulEscalations, m.FalseEscalations, m.FinalActionsChanged)
	}
	if m.ProviderUsage["clef"] != 1 || m.ProviderUsage["unavailable"] != 1 {
		t.Errorf("provider usage = %v", m.ProviderUsage)
	}
	if res.ByCategory["security_sensitive"].UsefulEscalations != 1 {
		t.Errorf("by-category = %+v", res.ByCategory)
	}
}

// TestBuildPairedDeterministic proves the paired aggregation is deterministic.
func TestBuildPairedDeterministic(t *testing.T) {
	c := &Corpus{Scenarios: []Scenario{{ID: "a", Category: "benign_impl", Expected: Expected{Action: AutoFix}}}}
	baseline := BaselineResult{Records: []BaselineRecord{{ScenarioID: "a", Category: "benign_impl", Expected: "AUTO_FIX", Action: "AUTO_FIX"}}}
	adv := []AdvisoryRecord{{ScenarioID: "a", Category: "benign_impl", Expected: "AUTO_FIX", BaselineAction: "AUTO_FIX", FinalAction: "AUTO_FIX", ProviderUsed: "clef"}}
	a, _ := MarshalPaired(BuildPaired(c, baseline, adv))
	b, _ := MarshalPaired(BuildPaired(c, baseline, adv))
	if Hash(a) != Hash(b) {
		t.Error("paired aggregation is not deterministic")
	}
}
