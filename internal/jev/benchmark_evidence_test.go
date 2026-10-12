package jev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// This file holds the DETERMINISTIC JEV-208 benchmark evidence *validator* and
// its unit tests. It contains no live-evidence acceptance test, so an ordinary
// `go test ./...` stays green before any benchmark artifact exists.
//
// Contract note (narrow refinement, awaiting review): a *measured* configuration
// reports only the metrics that are genuinely measurable at its sample size. At
// sample_size == 1 only single-observation metrics are allowed (no p50/p95 or
// rates, which would be invented from one sample); at sample_size >= 2 the
// multi-sample metrics are required.

const benchmarkEvidencePath = "docs/reports/evidence/JEV-WAVE2-BENCHMARK.json"

var requiredBenchmarkConfigs = []string{"deterministic", "jev-assisted", "julia", "nimble"}

// singleSampleMetrics may be reported with sample_size >= 1.
var singleSampleMetrics = map[string]bool{"latency_ms": true, "latency_us": true, "inference_calls": true}

// requiredMultiSampleMetrics must be present when sample_size >= 2. Latency is in
// microseconds so the in-process deterministic baseline is reported with real
// precision (its latency is sub-millisecond).
var requiredMultiSampleMetrics = []string{
	"correct_rate",
	"unsafe_continuation_rate",
	"unnecessary_escalation_rate",
	"schema_validity",
	"repeatability",
	"latency_p50_us",
	"latency_p95_us",
}

type benchmarkEvidence struct {
	Version        int               `json:"version"`
	Command        string            `json:"command"`
	Argv           []string          `json:"argv"`
	Revision       string            `json:"revision"`
	Inputs         []benchmarkInput  `json:"inputs"`
	Configurations []benchmarkConfig `json:"configurations"`
}

type benchmarkInput struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type benchmarkConfig struct {
	Name           string             `json:"name"`
	Classification string             `json:"classification"`
	SampleSize     int                `json:"sample_size,omitempty"`
	Provider       string             `json:"provider,omitempty"`
	Model          string             `json:"model,omitempty"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	ArtifactPath   string             `json:"artifact_path,omitempty"`
	Reason         string             `json:"reason,omitempty"`
}

var benchmarkSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

func decodeBenchmarkEvidence(data []byte) (benchmarkEvidence, error) {
	var ev benchmarkEvidence
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return benchmarkEvidence{}, err
	}
	return ev, nil
}

// validateBenchmarkEvidence is the pure validator. artifactExists reports whether
// a referenced artifact resolves; synthetic tests inject a stub.
func validateBenchmarkEvidence(ev benchmarkEvidence, artifactExists func(string) bool) error {
	if ev.Version != 1 {
		return fmt.Errorf("benchmark version = %d, want 1", ev.Version)
	}
	if strings.TrimSpace(ev.Command) == "" {
		return errors.New("benchmark records no command")
	}
	if len(ev.Argv) == 0 {
		return errors.New("benchmark records no argv")
	}
	if !benchmarkSHARe.MatchString(ev.Revision) {
		return fmt.Errorf("revision %q is not a 40-hex commit", ev.Revision)
	}
	if len(ev.Inputs) == 0 {
		return errors.New("benchmark records no inputs")
	}
	inputIDs := make(map[string]bool, len(ev.Inputs))
	for _, in := range ev.Inputs {
		if strings.TrimSpace(in.ID) == "" {
			return errors.New("benchmark input has an empty id")
		}
		if inputIDs[in.ID] {
			return fmt.Errorf("duplicate benchmark input id %q", in.ID)
		}
		inputIDs[in.ID] = true
	}
	if len(ev.Configurations) == 0 {
		return errors.New("benchmark records no configurations")
	}
	seen := make(map[string]bool, len(ev.Configurations))
	for _, c := range ev.Configurations {
		if err := validateBenchmarkConfig(c, artifactExists); err != nil {
			return fmt.Errorf("configuration %q: %w", c.Name, err)
		}
		seen[c.Name] = true
	}
	for _, want := range requiredBenchmarkConfigs {
		if !seen[want] {
			return fmt.Errorf("required configuration %q is not present", want)
		}
	}
	return nil
}

func validateBenchmarkConfig(c benchmarkConfig, artifactExists func(string) bool) error {
	switch c.Classification {
	case "measured":
		if c.SampleSize < 1 {
			return errors.New("measured configuration must declare sample_size >= 1")
		}
		if len(c.Metrics) == 0 {
			return errors.New("measured configuration has no numeric metrics")
		}
		for k, v := range c.Metrics {
			if v != v { // NaN
				return fmt.Errorf("metric %q is NaN", k)
			}
		}
		if c.SampleSize == 1 {
			// Only single-observation metrics; anything else would be invented.
			for k := range c.Metrics {
				if !singleSampleMetrics[k] {
					return fmt.Errorf("sample_size 1 must not report multi-sample metric %q", k)
				}
			}
		} else {
			for _, m := range requiredMultiSampleMetrics {
				if _, ok := c.Metrics[m]; !ok {
					return fmt.Errorf("measured configuration (sample_size %d) is missing metric %q", c.SampleSize, m)
				}
			}
		}
		if strings.TrimSpace(c.ArtifactPath) == "" || !artifactExists(c.ArtifactPath) {
			return fmt.Errorf("measured configuration references missing execution evidence %q", c.ArtifactPath)
		}
	case "simulated":
		if strings.TrimSpace(c.Reason) == "" {
			return errors.New("simulated configuration must be identified (reason is required)")
		}
	case "untested":
		if strings.TrimSpace(c.Reason) == "" {
			return errors.New("untested configuration must state a reason")
		}
		if len(c.Metrics) > 0 {
			return errors.New("untested configuration must not carry invented metrics")
		}
		if c.SampleSize != 0 {
			return errors.New("untested configuration must not declare a sample_size")
		}
	default:
		return fmt.Errorf("classification %q is not measured/simulated/untested", c.Classification)
	}
	return nil
}

// --- Synthetic validator tests (deterministic; never benchmark evidence) ---

func multiMetrics() map[string]float64 {
	m := map[string]float64{"latency_us": 12.5, "inference_calls": 0}
	for i, k := range requiredMultiSampleMetrics {
		m[k] = float64(i) / 10
	}
	return m
}

func syntheticBenchmarkEvidence() benchmarkEvidence {
	return benchmarkEvidence{
		Version:  1,
		Command:  "go test",
		Argv:     []string{"go", "test"},
		Revision: strings.Repeat("a", 40),
		Inputs:   []benchmarkInput{{ID: "scenario-1", Description: "shared scenario 1"}},
		Configurations: []benchmarkConfig{
			{Name: "deterministic", Classification: "measured", SampleSize: 50, Metrics: multiMetrics(), ArtifactPath: "artifacts/deterministic"},
			{Name: "jev-assisted", Classification: "measured", SampleSize: 1, Metrics: map[string]float64{"latency_ms": 15873, "inference_calls": 23}, ArtifactPath: "artifacts/jev"},
			{Name: "julia", Classification: "untested", Reason: "requires JEV-209"},
			{Name: "nimble", Classification: "untested", Reason: "requires JEV-209"},
		},
	}
}

func TestBenchmarkEvidenceValidatorAcceptsWellFormedSynthetic(t *testing.T) {
	if err := validateBenchmarkEvidence(syntheticBenchmarkEvidence(), func(string) bool { return true }); err != nil {
		t.Fatalf("well-formed synthetic benchmark rejected: %v", err)
	}
}

func TestBenchmarkEvidenceValidatorRejectsMissingConfig(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Configurations = ev.Configurations[:len(ev.Configurations)-1]
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for a missing configuration")
	}
}

func TestBenchmarkEvidenceValidatorRejectsMeasuredMissingMultiMetric(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	delete(ev.Configurations[0].Metrics, "correct_rate")
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for a multi-sample configuration missing a metric")
	}
}

func TestBenchmarkEvidenceValidatorRejectsSampleSize1WithRates(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Configurations[1].Metrics["correct_rate"] = 1
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: sample_size 1 must not report invented rate metrics")
	}
}

func TestBenchmarkEvidenceValidatorRejectsSampleSize1WithPercentile(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Configurations[1].Metrics["latency_p95_us"] = 5
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: sample_size 1 must not report a fabricated percentile")
	}
}

func TestBenchmarkEvidenceValidatorRejectsZeroSample(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Configurations[0].SampleSize = 0
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: a measured configuration must declare sample_size >= 1")
	}
}

func TestBenchmarkEvidenceValidatorRejectsMismatchedSampleCount(t *testing.T) {
	// A configuration claiming a multi-sample metric with sample_size 1 is a
	// mismatched sample-count claim and must be rejected (no invented p50/p95).
	ev := syntheticBenchmarkEvidence()
	c := &ev.Configurations[1]
	c.SampleSize = 1
	c.Metrics = map[string]float64{"latency_us": 1, "latency_p50_us": 1}
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: multi-sample metric at sample_size 1")
	}
}

func TestBenchmarkEvidenceValidatorRejectsMeasuredWithoutArtifact(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	if err := validateBenchmarkEvidence(ev, func(string) bool { return false }); err == nil {
		t.Fatal("expected rejection for measured evidence with an unresolved artifact")
	}
}

func TestBenchmarkEvidenceValidatorRejectsUntestedWithMetrics(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Configurations[2].Metrics = map[string]float64{"correct_rate": 1}
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for an untested configuration carrying metrics")
	}
}

func TestBenchmarkEvidenceValidatorRejectsUnknownClassification(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Configurations[1].Classification = "maybe"
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for an unknown classification")
	}
}

func TestBenchmarkEvidenceValidatorRejectsDuplicateInputs(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Inputs = append(ev.Inputs, ev.Inputs[0])
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for duplicate input ids")
	}
}

func TestBenchmarkEvidenceValidatorRejectsUnsignedRevision(t *testing.T) {
	ev := syntheticBenchmarkEvidence()
	ev.Revision = "abc"
	if err := validateBenchmarkEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for a non-40-hex revision")
	}
}

func TestBenchmarkEvidenceDecoderRejectsMalformedJSON(t *testing.T) {
	if _, err := decodeBenchmarkEvidence([]byte("{not json")); err == nil {
		t.Fatal("expected rejection of malformed JSON")
	}
}

func TestBenchmarkEvidenceDecoderRejectsUnknownField(t *testing.T) {
	if _, err := decodeBenchmarkEvidence([]byte(`{"version":1,"surprise":true}`)); err == nil {
		t.Fatal("expected rejection of an unknown field")
	}
}

func TestBenchmarkEvidenceDecoderRejectsNonNumericMetric(t *testing.T) {
	doc := `{"version":1,"command":"go test","argv":["go"],"revision":"` + strings.Repeat("a", 40) + `","inputs":[{"id":"s","description":"d"}],"configurations":[{"name":"deterministic","classification":"measured","sample_size":50,"metrics":{"correct_rate":"high"},"artifact_path":"a"}]}`
	if _, err := decodeBenchmarkEvidence([]byte(doc)); err == nil {
		t.Fatal("expected rejection of a non-numeric metric")
	}
}

func TestBenchmarkEvidenceCredentialsDetected(t *testing.T) {
	if len(credentialMarkersIn([]byte(`{"note":"api_key=abc"}`))) == 0 {
		t.Fatal("expected credential markers to be detected")
	}
	if len(credentialMarkersIn([]byte(`{"note":"clean"}`))) != 0 {
		t.Fatal("expected no markers in clean data")
	}
}
