package deceleval

import (
	"path/filepath"
	"testing"
)

func loadRepoCorpus(t *testing.T) *Corpus {
	t.Helper()
	corpusPath, _ := corpusPaths()
	c, err := Load(corpusPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return c
}

// TestBaselineIsDeterministic proves two runs over the same corpus are
// byte-identical (the reproducibility requirement).
func TestBaselineIsDeterministic(t *testing.T) {
	c := loadRepoCorpus(t)
	a, err := MarshalBaseline(RunBaseline(c, "x"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalBaseline(RunBaseline(c, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if Hash(a) != Hash(b) {
		t.Fatalf("baseline is not deterministic: %s != %s", Hash(a), Hash(b))
	}
}

// TestBaselineMetricsInvariants proves the counts reconcile and the safety metric
// is bounded by the incorrect count.
func TestBaselineMetricsInvariants(t *testing.T) {
	c := loadRepoCorpus(t)
	res := RunBaseline(c, "x")
	m := res.Metrics
	if m.Total != len(c.Scenarios) {
		t.Errorf("total %d != scenarios %d", m.Total, len(c.Scenarios))
	}
	if m.Classified+m.FixtureFailures != m.Total {
		t.Errorf("classified+fixture != total: %d+%d != %d", m.Classified, m.FixtureFailures, m.Total)
	}
	if m.Correct+m.Incorrect != m.Classified {
		t.Errorf("correct+incorrect != classified: %d+%d != %d", m.Correct, m.Incorrect, m.Classified)
	}
	if m.UnsafeMissedEscalation > m.Incorrect || m.FalseEscalation > m.Incorrect {
		t.Errorf("escalation counters exceed incorrect: %+v", m)
	}
	// Every classified record carries a deterministic action.
	for _, r := range res.Records {
		if r.FixtureError == "" && r.Action == "" {
			t.Errorf("scenario %q classified with no action", r.ScenarioID)
		}
	}
}

// TestBaselineFixtureFailureIsNotClassification proves an unmappable descriptor is
// recorded as a FIXTURE FAILURE with missing evidence, never as a classification.
func TestBaselineFixtureFailureIsNotClassification(t *testing.T) {
	c := &Corpus{Version: 1, Actions: []string{"AUTO_FIX"}, Scenarios: []Scenario{{
		ID: "x", Category: "not_a_category", Source: "synthetic",
		Observable: "o", Expected: Expected{Action: AutoFix, Rationale: "r"}, Split: "eval",
	}}}
	res := RunBaseline(c, "x")
	if len(res.Records) != 1 || res.Records[0].FixtureError == "" {
		t.Fatalf("want a fixture failure, got %+v", res.Records)
	}
	if res.Metrics.FixtureFailures != 1 || res.Metrics.Classified != 0 {
		t.Errorf("fixture failure must not be classified: %+v", res.Metrics)
	}
	if res.Records[0].Action != "" {
		t.Errorf("fixture failure must carry no action: %+v", res.Records[0])
	}
}

// TestSecondLabelSampleSize proves the QA sample is ~20% and agreement is reported.
func TestSecondLabelSampleSize(t *testing.T) {
	c := loadRepoCorpus(t)
	rep := secondLabelReview(c)
	want := (len(c.Scenarios) + 4) / 5 // every 5th
	if rep.SampleSize != want {
		t.Errorf("sample size %d, want %d", rep.SampleSize, want)
	}
	if rep.AgreementPct < 0 || rep.AgreementPct > 100 {
		t.Errorf("agreement pct out of range: %v", rep.AgreementPct)
	}
	for _, e := range rep.Entries {
		if !e.Agree && e.Disagreement == "" {
			t.Errorf("disagreement %q must be documented", e.ScenarioID)
		}
	}
}

// TestBaselineMatchesRecordedArtifact pins reproducibility against the recorded
// artifact hash, so a corpus or mapping change is detected.
func TestBaselineMatchesRecordedArtifact(t *testing.T) {
	c := loadRepoCorpus(t)
	corpusPath, _ := corpusPaths()
	corpusSHA, err := FreezeOf(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalBaseline(RunBaseline(c, corpusSHA))
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := LoadFreeze(filepath.Join(filepath.Dir(corpusPath), "baseline-A.results.json.sha256"))
	if err != nil {
		t.Fatalf("recorded baseline hash: %v", err)
	}
	if Hash(data) != recorded.SHA256 {
		t.Errorf("baseline drift: rerun is %s, recorded %s (re-run cmd/deceleval-baseline and re-record)", Hash(data), recorded.SHA256)
	}
}
