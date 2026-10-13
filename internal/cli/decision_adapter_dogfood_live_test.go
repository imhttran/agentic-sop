package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
)

// TestAdapterDogfoodLiveNimble is the bounded Gate W3-4 Nimble live dogfood. It is
// gated behind JEV213_LIVE=1 and writes the credential-free evidence artifact.
// It exercises the REAL production path: decisionProviderFromConfig -> connector
// -> real adapter subprocess -> bridge.Interpret -> decision.Validate/Route.
func TestAdapterDogfoodLiveNimble(t *testing.T) {
	if os.Getenv("JEV213_LIVE") != "1" {
		t.Skip("live dogfood disabled (set JEV213_LIVE=1)")
	}
	adapter := os.Getenv("JEV213_ADAPTER")
	evidenceRel := os.Getenv("JEV213_EVIDENCE")
	revision := os.Getenv("JEV213_REVISION")

	root := repoRootCLI()
	evidencePath := filepath.Join(root, filepath.FromSlash(evidenceRel))

	cfg := config.Config{Decision: config.DecisionConfig{
		Provider:        "adapter",
		Enabled:         bp(true),
		AdapterCommand:  []string{adapter},
		AdapterProvider: "nimble",
		AdapterModel:    "nimble",
	}}
	p, err := decisionProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("decisionProviderFromConfig: %v", err)
	}
	if p.Name() != "adapter" {
		t.Fatalf("provider = %q, want adapter", p.Name())
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	d, decErr := p.Decide(ctx, decision.Request{UseCase: "change-risk", Subject: "bounded Gate W3-4 Nimble dogfood"})
	latencyMS := int(time.Since(start).Milliseconds())

	// Deterministic governance on the returned evidence.
	nimble := evConfig{Name: "nimble-live", SampleSize: 1}
	if decErr == nil {
		if ve := decision.Validate(d); ve != nil {
			nimble.Classification = "untested"
			nimble.Reason = "live evidence rejected by governance validation: " + ve.Error()
		} else {
			target := decision.Route(decision.Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}, d)
			nimble.Classification = "measured"
			nimble.Reason = fmt.Sprintf("live Nimble decision: choice=%s confidence=%.3f -> %s", d.Choice, d.Confidence, target)
			nimble.Metrics = map[string]float64{"latency_ms": float64(latencyMS)}
			// Measured requires a resolvable artifact.
			artRel := "docs/reports/evidence/artifacts/nimble-dogfood"
			artDir := filepath.Join(root, filepath.FromSlash(artRel))
			if err := os.MkdirAll(artDir, 0o755); err != nil {
				t.Fatalf("artifact dir: %v", err)
			}
			obs := map[string]any{
				"provider": "nimble", "model": "nimble",
				"choice": string(d.Choice), "confidence": d.Confidence,
				"governance_target": string(target), "latency_ms": latencyMS,
				"validation": "PASS",
			}
			b, _ := json.MarshalIndent(obs, "", "  ")
			if err := os.WriteFile(filepath.Join(artDir, "observation.json"), append(b, '\n'), 0o644); err != nil {
				t.Fatalf("write observation: %v", err)
			}
			nimble.ArtifactPath = artRel
			t.Logf("nimble measured: choice=%s conf=%.3f target=%s latency=%dms", d.Choice, d.Confidence, target, latencyMS)
		}
	} else {
		nimble.Classification = "untested"
		nimble.Reason = "live Nimble attempt failed (class-only): " + decErr.Error()
		t.Logf("nimble attempt failed: %v", decErr)
	}
	nimble.SampleSize = 0
	if nimble.Classification == "measured" {
		nimble.SampleSize = 1
	}

	ev := evEvidence{
		Version:  1,
		Command:  "go test -count=1 -run ^TestAdapterDogfoodLiveNimble$ ./internal/cli/",
		Argv:     []string{"go", "test", "-run", "^TestAdapterDogfoodLiveNimble$", "./internal/cli/"},
		Revision: revision,
		Provider: "adapter", Model: "nimble",
		Configurations: []evConfig{
			{Name: "connector-offline", Classification: "simulated",
				Reason:  "offline integration against a deterministic fake adapter (TestAdapterConnectorIntegration)",
				Metrics: map[string]float64{"inference_calls": 0}},
			nimble,
			{Name: "julia-live", Classification: "untested",
				Reason: "no ONNX model and onnxruntime are available; Julia live inference is unavailable"},
		},
	}
	if err := writeEv(evidencePath, ev); err != nil {
		t.Fatalf("write evidence: %v", err)
	}
	t.Logf("wrote evidence to %s", evidencePath)
}

func repoRootCLI() string {
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

type evEvidence struct {
	Version        int        `json:"version"`
	Command        string     `json:"command"`
	Argv           []string   `json:"argv"`
	Revision       string     `json:"revision"`
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	Configurations []evConfig `json:"configurations"`
}

type evConfig struct {
	Name           string             `json:"name"`
	Classification string             `json:"classification"`
	SampleSize     int                `json:"sample_size,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	ArtifactPath   string             `json:"artifact_path,omitempty"`
}

func writeEv(path string, ev evEvidence) error {
	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
