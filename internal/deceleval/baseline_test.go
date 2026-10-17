package deceleval

import (
	"bytes"
	"os"
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

// TestBaselineIsDeterministic proves two runs over the same corpus agree through
// the canonical (latency-free) projection. Measured latency is excluded, so a
// loaded machine cannot make a reproducible run look nondeterministic.
func TestBaselineIsDeterministic(t *testing.T) {
	c := loadRepoCorpus(t)
	a, err := CanonicalHash(RunBaseline(c, "x"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalHash(RunBaseline(c, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("baseline is not deterministic: %s != %s", a, b)
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

// TestRecordedBaselineArtifactIsIntact proves the committed artifact is exactly
// the bytes its recorded checksum pins. Artifact integrity is checked on the raw
// bytes, separately from the semantic comparison below, so corruption is caught
// even when the decoded decisions happen to look plausible.
func TestRecordedBaselineArtifactIsIntact(t *testing.T) {
	recorded, sum := loadRecordedBaseline(t)
	if got := Hash(recorded); got != sum {
		t.Errorf("recorded baseline artifact is corrupt: sha256 is %s, recorded %s", got, sum)
	}
}

// TestBaselineTamperIsDetected proves a changed recorded artifact still fails its
// integrity check. A single flipped decision is enough to break the checksum,
// because the check is over the artifact's exact bytes.
func TestBaselineTamperIsDetected(t *testing.T) {
	recorded, sum := loadRecordedBaseline(t)
	res, err := UnmarshalBaseline(recorded)
	if err != nil {
		t.Fatalf("decode recorded baseline: %v", err)
	}
	if len(res.Records) == 0 {
		t.Fatal("recorded baseline has no records")
	}
	// Flip one decision, keeping the artifact otherwise well-formed.
	res.Records[0].Action = flippedAction(res.Records[0].Action)
	tampered, err := MarshalBaseline(res)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(tampered, recorded) {
		t.Fatal("tampering did not change the artifact")
	}
	if Hash(tampered) == sum {
		t.Error("a changed recorded artifact still matches its checksum")
	}
}

// TestBaselineMatchesRecordedArtifact pins reproducibility against the recorded
// artifact's decisions, never its timing: the fresh run and the recorded artifact
// are compared through the same canonical projection, so a corpus, mapping, or
// classifier change is detected while a slower or faster machine is not.
func TestBaselineMatchesRecordedArtifact(t *testing.T) {
	c := loadRepoCorpus(t)
	corpusPath, _ := corpusPaths()
	corpusSHA, err := FreezeOf(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	recorded, _ := loadRecordedBaseline(t)
	recordedRes, err := UnmarshalBaseline(recorded)
	if err != nil {
		t.Fatalf("decode recorded baseline: %v", err)
	}
	fresh, err := CanonicalHash(RunBaseline(c, corpusSHA))
	if err != nil {
		t.Fatal(err)
	}
	have, err := CanonicalHash(recordedRes)
	if err != nil {
		t.Fatal(err)
	}
	if fresh != have {
		t.Errorf("baseline drift: rerun is %s, recorded %s (re-run cmd/deceleval-baseline and re-record)", fresh, have)
	}
}

// loadRecordedBaseline reads the committed baseline artifact and its recorded
// checksum.
func loadRecordedBaseline(t *testing.T) (artifact []byte, sha256 string) {
	t.Helper()
	corpusPath, _ := corpusPaths()
	dir := filepath.Dir(corpusPath)
	artifact, err := os.ReadFile(filepath.Join(dir, "baseline-A.results.json"))
	if err != nil {
		t.Fatalf("read recorded baseline: %v", err)
	}
	sum, err := LoadFreeze(filepath.Join(dir, "baseline-A.results.json.sha256"))
	if err != nil {
		t.Fatalf("recorded baseline hash: %v", err)
	}
	return artifact, sum.SHA256
}

// flippedAction returns a governed action different from a.
func flippedAction(a string) string {
	if a == "NEEDS_HUMAN" {
		return "AUTO_FIX"
	}
	return "NEEDS_HUMAN"
}
