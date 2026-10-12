package decision

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// This file measures the DETERMINISTIC decision baseline offline (no LLM
// inference) against stable fixtures, and — when JEV208_EMIT is set — emits the
// JEV-208 benchmark evidence artifact. It never invents metrics: every value is
// computed from actual executions of decision.Route.

// benchFixture is one deterministic scenario: an input decision and the target
// policy must route it to.
type benchFixture struct {
	ID             string
	Kind           string // continue | unsafe | escalation | fail_closed | human
	Request        Request
	Decision       Decision
	ExpectedTarget Target
}

var benchmarkThresholds = Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}

func benchmarkFixtures() []benchFixture {
	return []benchFixture{
		{ID: "correct-continuation", Kind: "continue",
			Request:        Request{UseCase: "change-risk", Subject: "small documentation fix", Signals: map[string]float64{"files": 1, "criteria": 1}},
			Decision:       Decision{Choice: Low, Confidence: 0.95},
			ExpectedTarget: SmallModel},
		{ID: "unsafe-continuation", Kind: "unsafe",
			Request:        Request{UseCase: "change-risk", Subject: "database migration touching auth", Signals: map[string]float64{"files": 10}},
			Decision:       Decision{Choice: High, Confidence: 0.9},
			ExpectedTarget: HumanTarget},
		{ID: "unnecessary-escalation", Kind: "escalation",
			Request:        Request{UseCase: "change-risk", Subject: "typo fix", Signals: map[string]float64{"files": 1}},
			Decision:       Decision{Choice: Low, Confidence: 0.99},
			ExpectedTarget: SmallModel},
		{ID: "fail-closed", Kind: "fail_closed",
			Request:        Request{UseCase: "change-risk"},
			Decision:       Decision{Choice: Choice("invented"), Confidence: 0.9},
			ExpectedTarget: HumanTarget},
		{ID: "human-approval", Kind: "human",
			Request:        Request{UseCase: "change-risk"},
			Decision:       Decision{Choice: Human, Confidence: 0.9},
			ExpectedTarget: HumanTarget},
	}
}

type detMetrics struct {
	CorrectRate               float64
	UnsafeContinuationRate    float64
	UnnecessaryEscalationRate float64
	SchemaValidity            float64
	Repeatability             float64
	LatencyP50US              float64
	LatencyP95US              float64
	LatencyUS                 float64
	InferenceCalls            int
	SampleSize                int
	TotalDecisions            int
}

// measureDeterministic runs the fixtures `repeats` times through decision.Route
// and computes the deterministic metrics.
func measureDeterministic(repeats int) (detMetrics, error) {
	fixtures := benchmarkFixtures()

	// Baseline targets and validation (one pass) for correctness/repeatability.
	first := make(map[string]Target, len(fixtures))
	valid := 0
	validTotal := 0
	for _, f := range fixtures {
		first[f.ID] = Route(benchmarkThresholds, f.Decision)
		if f.Kind != "fail_closed" {
			validTotal++
			if Validate(f.Decision) == nil {
				valid++
			}
		}
		if first[f.ID] != f.ExpectedTarget {
			return detMetrics{}, fmt.Errorf("fixture %q routed to %s, want %s", f.ID, first[f.ID], f.ExpectedTarget)
		}
	}

	var latencies []float64 // nanoseconds, from the monotonic clock
	correct, unsafe, unnecessary, repeatsOK, total := 0, 0, 0, 0, 0
	for r := 0; r < repeats; r++ {
		for _, f := range fixtures {
			start := time.Now()
			got := Route(benchmarkThresholds, f.Decision)
			latencies = append(latencies, float64(time.Since(start).Nanoseconds()))
			total++
			if got == f.ExpectedTarget {
				correct++
			}
			if got == first[f.ID] {
				repeatsOK++
			}
			if f.ExpectedTarget == HumanTarget && got != HumanTarget {
				unsafe++
			}
			if f.ExpectedTarget != HumanTarget && got == HumanTarget {
				unnecessary++
			}
		}
	}

	if total == 0 {
		return detMetrics{}, fmt.Errorf("no executions")
	}
	sort.Float64s(latencies)
	var sum float64
	for _, l := range latencies {
		sum += l
	}
	metrics := detMetrics{
		CorrectRate:               float64(correct) / float64(total),
		UnsafeContinuationRate:    float64(unsafe) / float64(total),
		UnnecessaryEscalationRate: float64(unnecessary) / float64(total),
		SchemaValidity:            float64(valid) / float64(validTotal),
		Repeatability:             float64(repeatsOK) / float64(total),
		LatencyP50US:              percentile(latencies, 0.50) / 1000.0,
		LatencyP95US:              percentile(latencies, 0.95) / 1000.0,
		LatencyUS:                 (sum / float64(total)) / 1000.0,
		InferenceCalls:            0,
		SampleSize:                repeats,
		TotalDecisions:            total,
	}
	return metrics, nil
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func TestDeterministicBenchmarkFixtures(t *testing.T) {
	m, err := measureDeterministic(50)
	if err != nil {
		t.Fatalf("deterministic baseline: %v", err)
	}
	if m.UnsafeContinuationRate != 0 {
		t.Fatalf("unsafe continuation rate = %v, want 0 (policy must withhold)", m.UnsafeContinuationRate)
	}
	if m.UnnecessaryEscalationRate != 0 {
		t.Fatalf("unnecessary escalation rate = %v, want 0", m.UnnecessaryEscalationRate)
	}
	if m.CorrectRate != 1 {
		t.Fatalf("correct rate = %v, want 1", m.CorrectRate)
	}
	if m.Repeatability != 1 {
		t.Fatalf("repeatability = %v, want 1 (deterministic)", m.Repeatability)
	}
	if m.InferenceCalls != 0 {
		t.Fatalf("deterministic baseline must make no inference calls, got %d", m.InferenceCalls)
	}
	if m.TotalDecisions != 250 {
		t.Fatalf("total decisions = %d, want 250 (50 repetitions x 5 fixtures)", m.TotalDecisions)
	}
	if os.Getenv("JEV208_EMIT") != "" {
		if err := emitBenchmarkEvidence(os.Getenv("JEV208_EMIT"), m); err != nil {
			t.Fatalf("emit benchmark evidence: %v", err)
		}
		t.Logf("emitted benchmark evidence to %s", os.Getenv("JEV208_EMIT"))
	}
}

// --- evidence emission (schema mirrors internal/jev benchmark_evidence_test.go) ---

type emitEvidence struct {
	Version        int          `json:"version"`
	Command        string       `json:"command"`
	Argv           []string     `json:"argv"`
	Revision       string       `json:"revision"`
	Inputs         []emitInput  `json:"inputs"`
	Configurations []emitConfig `json:"configurations"`
}

type emitInput struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type emitConfig struct {
	Name           string             `json:"name"`
	Classification string             `json:"classification"`
	SampleSize     int                `json:"sample_size,omitempty"`
	Provider       string             `json:"provider,omitempty"`
	Model          string             `json:"model,omitempty"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	ArtifactPath   string             `json:"artifact_path,omitempty"`
	Reason         string             `json:"reason,omitempty"`
}

func emitBenchmarkEvidence(relPath string, det detMetrics) error {
	root := repoRootT()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	inputs := []emitInput{}
	for _, f := range benchmarkFixtures() {
		inputs = append(inputs, emitInput{ID: f.ID, Description: f.Kind})
	}

	// The deterministic baseline's own execution artifact, written beside the
	// evidence file (repo-root-relative). Never reuse another configuration's evidence.
	artifactRel := filepath.ToSlash(filepath.Join(filepath.Dir(relPath), "artifacts", "deterministic-baseline"))
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(artifactRel)), 0o755); err != nil {
		return err
	}
	result := map[string]any{
		"command":         "go test -count=1 -run '^TestDeterministicBenchmarkFixtures$' ./internal/decision/",
		"sample_size":     det.SampleSize,
		"total_decisions": det.TotalDecisions,
		"clock":           "time.Now/time.Since (monotonic); latency reported in microseconds",
		"fixtures":        benchmarkFixtures(),
		"metrics": map[string]float64{
			"correct_rate": det.CorrectRate, "unsafe_continuation_rate": det.UnsafeContinuationRate,
			"unnecessary_escalation_rate": det.UnnecessaryEscalationRate, "schema_validity": det.SchemaValidity,
			"repeatability": det.Repeatability, "latency_p50_us": det.LatencyP50US,
			"latency_p95_us": det.LatencyP95US, "latency_us": det.LatencyUS, "inference_calls": float64(det.InferenceCalls),
		},
	}
	rj, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(artifactRel), "result.json"), append(rj, '\n'), 0o644); err != nil {
		return err
	}

	// deterministic: measured from actual executions above.
	deterministic := emitConfig{
		Name: "deterministic", Classification: "measured", SampleSize: det.SampleSize,
		Provider: "in-process", Model: "decision.Route",
		Metrics: map[string]float64{
			"correct_rate":                det.CorrectRate,
			"unsafe_continuation_rate":    det.UnsafeContinuationRate,
			"unnecessary_escalation_rate": det.UnnecessaryEscalationRate,
			"schema_validity":             det.SchemaValidity,
			"repeatability":               det.Repeatability,
			"latency_p50_us":              det.LatencyP50US,
			"latency_p95_us":              det.LatencyP95US,
			"latency_us":                  det.LatencyUS,
			"inference_calls":             float64(det.InferenceCalls),
		},
		ArtifactPath: artifactRel,
	}

	// jev-assisted: ONE measured observation from the committed JEV-207 evidence.
	jev := emitConfig{Name: "jev-assisted", Classification: "measured", SampleSize: 1,
		Provider: "ollama", Model: "deepseek-v4.1-flash:cloud", Reason: "single observation (N=1) from JEV-207 run DOGFOOD-001; p50/p95 and rates are not measurable at N=1 and are not reported",
		ArtifactPath: "docs/reports/evidence/artifacts/direct_sop_jev_invocation"}
	jev.Metrics = jevAssistedMetrics()
	if len(jev.Metrics) == 0 {
		return fmt.Errorf("no recorded JEV-207 observation available for the jev-assisted configuration")
	}

	julia := emitConfig{Name: "julia", Classification: "untested", Reason: "no production connector: end-to-end adapter benchmarking requires JEV-209"}
	nimble := emitConfig{Name: "nimble", Classification: "untested", Reason: "no production connector: end-to-end adapter benchmarking requires JEV-209"}

	ev := emitEvidence{
		Version:        1,
		Command:        "go test -count=1 -run '^TestDeterministicBenchmarkFixtures$' ./internal/decision/",
		Argv:           []string{"go", "test", "-run", "^TestDeterministicBenchmarkFixtures$", "./internal/decision/"},
		Revision:       os.Getenv("JEV208_REVISION"),
		Inputs:         inputs,
		Configurations: []emitConfig{deterministic, jev, julia, nimble},
	}
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(full, append(data, '\n'), 0o644)
}

// repoRootT walks up from the test's working directory to the module root.
func repoRootT() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// jevAssistedMetrics reads the recorded JEV-207 observation: the live latency from
// the committed evidence and, when the Gate 4B proxy counter is provided, the
// inference-call count. Only genuinely recorded values are returned.
func jevAssistedMetrics() map[string]float64 {
	m := map[string]float64{}
	root := repoRootT()
	if data, err := os.ReadFile(filepath.Join(root, "docs/reports/evidence/JEV-WAVE2-DOGFOOD.json")); err == nil {
		var parsed struct {
			Scenarios []struct {
				ID        string `json:"id"`
				LatencyMS int    `json:"latency_ms"`
			} `json:"scenarios"`
		}
		if json.Unmarshal(data, &parsed) == nil {
			for _, s := range parsed.Scenarios {
				if s.ID == "direct_sop_jev_invocation" && s.LatencyMS > 0 {
					m["latency_ms"] = float64(s.LatencyMS)
				}
			}
		}
	}
	if ps := os.Getenv("JEV208_PROXY_STATE"); ps != "" {
		if b, err := os.ReadFile(ps); err == nil {
			var st struct {
				Count int `json:"count"`
			}
			if json.Unmarshal(b, &st) == nil && st.Count > 0 {
				m["inference_calls"] = float64(st.Count)
			}
		}
	}
	return m
}
