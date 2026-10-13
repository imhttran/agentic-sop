package deceleval

import (
	"encoding/json"
	"testing"
)

// TestRecoveryRecommendationAction pins the recovery-choice -> disposition map.
func TestRecoveryRecommendationAction(t *testing.T) {
	cases := map[string]string{
		RecoveryAutoFix:  "AUTO_FIX",
		RecoveryContinue: "CONTINUE",
		RecoveryHuman:    "NEEDS_HUMAN",
		"":               "",
		"HIGH":           "",
		"low":            "",
	}
	for choice, want := range cases {
		if got := RecoveryRecommendationAction(choice); got != want {
			t.Errorf("RecoveryRecommendationAction(%q) = %q, want %q", choice, got, want)
		}
	}
}

// TestRecoveryChoiceValid proves only the closed set is accepted.
func TestRecoveryChoiceValid(t *testing.T) {
	for _, ok := range RecoveryChoices {
		if !RecoveryChoiceValid(ok) {
			t.Errorf("RecoveryChoiceValid(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "HIGH", "human", "AUTO"} {
		if RecoveryChoiceValid(bad) {
			t.Errorf("RecoveryChoiceValid(%q) = true, want false", bad)
		}
	}
}

// TestMarshalRecoveryRequest proves the wire shape is the adapter's decision
// contract: a state plus one choice question keyed by the recovery kind.
func TestMarshalRecoveryRequest(t *testing.T) {
	data, err := MarshalRecoveryRequest(Scenario{Observable: "a benign failure"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		State     string `json:"state"`
		Questions []struct {
			ID       string   `json:"id"`
			Type     string   `json:"type"`
			Criteria string   `json:"criteria"`
			Choices  []string `json:"choices"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.State != "a benign failure" {
		t.Errorf("state = %q", decoded.State)
	}
	if len(decoded.Questions) != 1 {
		t.Fatalf("questions = %d, want 1", len(decoded.Questions))
	}
	q := decoded.Questions[0]
	if q.ID != RecoveryKind || q.Type != "choice" || q.Criteria == "" {
		t.Errorf("question = %+v", q)
	}
	if len(q.Choices) != len(RecoveryChoices) {
		t.Errorf("choices = %v, want %v", q.Choices, RecoveryChoices)
	}
}

// TestParseRecoveryResultPrimary proves a primary-served result is read with the
// provider and model, and no fallback reason.
func TestParseRecoveryResultPrimary(t *testing.T) {
	ans, err := ParseRecoveryResult([]byte(`{"provider":"clef","model":"clef-4bit","answers":{"failure-recovery":{"type":"choice","choice":"AUTO_FIX","confidence":0.75}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if ans.Choice != RecoveryAutoFix || ans.ServedProvider != "clef" || ans.Model != "clef-4bit" {
		t.Errorf("answer = %+v", ans)
	}
	if ans.Confidence == nil || *ans.Confidence != 0.75 {
		t.Errorf("confidence = %v, want 0.75", ans.Confidence)
	}
	if ans.FallbackReason != "" {
		t.Errorf("fallback reason = %q, want empty", ans.FallbackReason)
	}
}

// TestParseRecoveryResultFallback proves that when the backup serves, the
// serving provider is read from the fallback diagnostics (the composite masks
// its provider name to the primary profile).
func TestParseRecoveryResultFallback(t *testing.T) {
	raw := `{"provider":"clef","model":"bespokelabs/Bespoke-Nimble-9B","answers":{"failure-recovery":{"type":"choice","choice":"HUMAN"}},"diagnostics":{"served_provider":"nimble","fallback":"primary unavailable"}}`
	ans, err := ParseRecoveryResult([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if ans.ServedProvider != "nimble" {
		t.Errorf("served provider = %q, want nimble", ans.ServedProvider)
	}
	if ans.FallbackReason == "" {
		t.Error("fallback reason is empty, want a recorded reason")
	}
	if ans.Confidence != nil {
		t.Errorf("confidence = %v, want nil when omitted", ans.Confidence)
	}
}

// TestParseRecoveryResultRejects proves malformed results are errors, never
// silently accepted as a decision.
func TestParseRecoveryResultRejects(t *testing.T) {
	bad := map[string]string{
		"bad json":         `{`,
		"no answers":       `{"provider":"clef"}`,
		"missing question": `{"provider":"clef","answers":{"other":{"type":"choice","choice":"HUMAN"}}}`,
		"unknown choice":   `{"provider":"clef","answers":{"failure-recovery":{"type":"choice","choice":"HIGH"}}}`,
		"empty choice":     `{"provider":"clef","answers":{"failure-recovery":{"type":"choice","choice":""}}}`,
	}
	for name, raw := range bad {
		if _, err := ParseRecoveryResult([]byte(raw)); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}

// TestBuildRecoveryAddAttention proves the clamp: only a HUMAN recommendation
// changes the action, so the advisory can only ADD attention.
func TestBuildRecoveryAddAttention(t *testing.T) {
	baseline := BaselineResult{Records: []BaselineRecord{
		{ScenarioID: "a", Category: "security_sensitive", Expected: "NEEDS_HUMAN", Action: "BLOCK"},
		{ScenarioID: "b", Category: "benign_impl", Expected: "AUTO_FIX", Action: "AUTO_FIX"},
		{ScenarioID: "c", Category: "test_regression", Expected: "AUTO_FIX", Action: "AUTO_FIX"},
	}}
	obs := []RecoveryObservation{
		{ScenarioID: "a", Split: "tuning", Expected: "NEEDS_HUMAN", Choice: RecoveryHuman, Recommendation: "NEEDS_HUMAN", Consulted: true, ServedProvider: "clef"},
		{ScenarioID: "b", Split: "tuning", Expected: "AUTO_FIX", Choice: RecoveryAutoFix, Recommendation: "AUTO_FIX", Consulted: true, ServedProvider: "clef"},
		{ScenarioID: "c", Split: "tuning", Expected: "AUTO_FIX", Choice: RecoveryContinue, Recommendation: "CONTINUE", Consulted: true, ServedProvider: "clef"},
	}
	res := BuildRecovery(&Corpus{}, baseline, obs)
	got := map[string]string{}
	for _, r := range res.Records {
		got[r.ScenarioID] = r.FinalAction
	}
	want := map[string]string{"a": "NEEDS_HUMAN", "b": "AUTO_FIX", "c": "AUTO_FIX"}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("scenario %s final action = %q, want %q", id, got[id], w)
		}
	}
	m := res.BySplit["tuning"]
	if m.UsefulEscalations != 1 || m.FalseEscalations != 0 || m.UnsafeMissed != 0 {
		t.Errorf("useful=%d false=%d unsafe=%d, want 1/0/0", m.UsefulEscalations, m.FalseEscalations, m.UnsafeMissed)
	}
}

// TestBuildRecoveryMetrics proves counts, provider usage, choice distribution,
// and latency-over-consulted.
func TestBuildRecoveryMetrics(t *testing.T) {
	baseline := BaselineResult{
		CorpusSHA256: "corpus-sha",
		Records: []BaselineRecord{
			{ScenarioID: "s1", Category: "benign_impl", Expected: "AUTO_FIX", Action: "AUTO_FIX"},
			{ScenarioID: "s2", Category: "security_sensitive", Expected: "NEEDS_HUMAN", Action: "BLOCK"},
			{ScenarioID: "s3", Category: "missing_validation", Expected: "BLOCK", Action: "BLOCK"},
		},
	}
	obs := []RecoveryObservation{
		{ScenarioID: "s1", Split: "tuning", Expected: "AUTO_FIX", Choice: RecoveryAutoFix, Recommendation: "AUTO_FIX", Consulted: true, ServedProvider: "clef", LatencyMS: 100},
		{ScenarioID: "s2", Split: "tuning", Expected: "NEEDS_HUMAN", Choice: RecoveryHuman, Recommendation: "NEEDS_HUMAN", Consulted: true, ServedProvider: "clef", LatencyMS: 200},
		{ScenarioID: "s3", Split: "tuning", Expected: "BLOCK", Choice: RecoveryContinue, Recommendation: "CONTINUE", Consulted: true, ServedProvider: "nimble", LatencyMS: 300},
	}
	res := BuildRecovery(&Corpus{}, baseline, obs)
	if res.CorpusSHA256 != "corpus-sha" {
		t.Errorf("corpus sha = %q", res.CorpusSHA256)
	}
	m := res.BySplit["tuning"]
	if m.N != 3 || m.Consulted != 3 {
		t.Errorf("n=%d consulted=%d, want 3/3", m.N, m.Consulted)
	}
	if m.BaselineCorrect != 2 || m.RecommendCorrect != 2 || m.FinalCorrect != 3 {
		t.Errorf("baseline=%d recommend=%d final=%d, want 2/2/3", m.BaselineCorrect, m.RecommendCorrect, m.FinalCorrect)
	}
	if m.UsefulEscalations != 1 || m.FalseEscalations != 0 {
		t.Errorf("useful=%d false=%d, want 1/0", m.UsefulEscalations, m.FalseEscalations)
	}
	if m.ByChoice[RecoveryAutoFix] != 1 || m.ByChoice[RecoveryHuman] != 1 || m.ByChoice[RecoveryContinue] != 1 {
		t.Errorf("by_choice = %v", m.ByChoice)
	}
	if m.ProviderUsage["clef"] != 2 || m.ProviderUsage["nimble"] != 1 {
		t.Errorf("provider_usage = %v", m.ProviderUsage)
	}
	if m.LatencyMedianMS != 200 || m.LatencyP95MS != 300 {
		t.Errorf("latency median=%v p95=%v, want 200/300", m.LatencyMedianMS, m.LatencyP95MS)
	}
	// Operational question ("needs human?") agrees on all three (none expects HUMAN
	// while rec/AUTO_FIX or rec/CONTINUE disagree), so operational agreement = 3.
	if m.OperationalAgree != 3 {
		t.Errorf("operational_agree = %d, want 3", m.OperationalAgree)
	}
}

// TestBuildRecoveryUnavailable proves an unavailable/malformed consultation is an
// outcome: it counts against "unavailable", adds no choice, and keeps the
// baseline action.
func TestBuildRecoveryUnavailable(t *testing.T) {
	baseline := BaselineResult{Records: []BaselineRecord{
		{ScenarioID: "s1", Category: "provider_fault", Expected: "RETRY", Action: "RETRY"},
	}}
	obs := []RecoveryObservation{
		{ScenarioID: "s1", Split: "eval", Expected: "RETRY", Consulted: false, OutcomeClass: "unavailable", Detail: "provider unavailable"},
	}
	res := BuildRecovery(&Corpus{}, baseline, obs)
	m := res.BySplit["eval"]
	if m.Consulted != 0 || m.NoRecommendation != 1 {
		t.Errorf("consulted=%d no_recommendation=%d, want 0/1", m.Consulted, m.NoRecommendation)
	}
	if m.ProviderUsage["unavailable"] != 1 {
		t.Errorf("provider_usage = %v, want unavailable:1", m.ProviderUsage)
	}
	if res.Records[0].FinalAction != "RETRY" {
		t.Errorf("final action = %q, want RETRY (baseline preserved)", res.Records[0].FinalAction)
	}
}

// TestCanonicalToNativeChoice pins the inverse of the eval choice map.
func TestCanonicalToNativeChoice(t *testing.T) {
	cases := map[string]string{"LOW": RecoveryAutoFix, "MEDIUM": RecoveryContinue, "HUMAN": RecoveryHuman, "": "", "HIGH": ""}
	for canonical, want := range cases {
		if got := CanonicalToNativeChoice(canonical); got != want {
			t.Errorf("CanonicalToNativeChoice(%q) = %q, want %q", canonical, got, want)
		}
	}
}

// TestRecoveryPromptStable proves the frozen prompt anchor is well-formed and
// stable.
func TestRecoveryPromptStable(t *testing.T) {
	p := RecoveryPromptSpec()
	if p.Kind != RecoveryKind || len(p.Choices) != 3 {
		t.Errorf("prompt = %+v", p)
	}
	if p.ChoiceMap["HUMAN"] != "HUMAN" || p.ChoiceMap["AUTO_FIX"] == p.ChoiceMap["HUMAN"] {
		t.Errorf("choice map = %v, want HUMAN adverse and AUTO_FIX benign", p.ChoiceMap)
	}
	a, b := RecoveryPromptSHA256(), RecoveryPromptSHA256()
	if a == "" || a != b {
		t.Errorf("prompt sha not stable: %q vs %q", a, b)
	}
}

// TestMarshalRecoveryDeterministic proves the artifact is stable regardless of
// input order.
func TestMarshalRecoveryDeterministic(t *testing.T) {
	baseline := BaselineResult{Records: []BaselineRecord{
		{ScenarioID: "a", Expected: "AUTO_FIX", Action: "AUTO_FIX"},
		{ScenarioID: "b", Expected: "NEEDS_HUMAN", Action: "BLOCK"},
	}}
	obs := []RecoveryObservation{
		{ScenarioID: "b", Split: "tuning", Expected: "NEEDS_HUMAN", Choice: RecoveryHuman, Recommendation: "NEEDS_HUMAN", Consulted: true, ServedProvider: "clef"},
		{ScenarioID: "a", Split: "tuning", Expected: "AUTO_FIX", Choice: RecoveryAutoFix, Recommendation: "AUTO_FIX", Consulted: true, ServedProvider: "clef"},
	}
	reversed := []RecoveryObservation{obs[1], obs[0]}
	x, err := MarshalRecovery(BuildRecovery(&Corpus{}, baseline, obs))
	if err != nil {
		t.Fatal(err)
	}
	y, err := MarshalRecovery(BuildRecovery(&Corpus{}, baseline, reversed))
	if err != nil {
		t.Fatal(err)
	}
	if Hash(x) != Hash(y) {
		t.Error("recovery artifact is not deterministic under input reordering")
	}
}
