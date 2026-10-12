package commandprofile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// This file defines the JEV-213 offline/live dogfood evidence *contract* and its
// deterministic validator. It creates NO evidence artifact: the live artifact
// (docs/reports/evidence/JEV-WAVE3-ADAPTER-DOGFOOD.json) may only be produced by a
// future, authorized live run. Synthetic fixtures are used here solely to exercise
// the validator.
//
// The contract separates three buckets:
//   - simulated: an offline/deterministic result (reason required; no live
//     inference — inference_calls, if reported, must be 0);
//   - measured: a future live result (numeric metrics + execution evidence);
//   - untested: a configuration not exercised (reason required; no metrics).

const adapterDogfoodEvidencePath = "docs/reports/evidence/JEV-WAVE3-ADAPTER-DOGFOOD.json"

var requiredAdapterDogfoodConfigs = []string{"connector-offline", "julia-live", "nimble-live"}

var adapterDogfoodSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

type adapterDogfoodEvidence struct {
	Version        int                    `json:"version"`
	Command        string                 `json:"command"`
	Argv           []string               `json:"argv"`
	Revision       string                 `json:"revision"`
	Provider       string                 `json:"provider"`
	Model          string                 `json:"model"`
	Configurations []adapterDogfoodConfig `json:"configurations"`
}

type adapterDogfoodConfig struct {
	Name           string             `json:"name"`
	Classification string             `json:"classification"`
	SampleSize     int                `json:"sample_size,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	ArtifactPath   string             `json:"artifact_path,omitempty"`
}

func decodeAdapterDogfood(data []byte) (adapterDogfoodEvidence, error) {
	var ev adapterDogfoodEvidence
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return adapterDogfoodEvidence{}, err
	}
	return ev, nil
}

func validateAdapterDogfood(ev adapterDogfoodEvidence, artifactExists func(string) bool) error {
	if ev.Version != 1 {
		return fmt.Errorf("version = %d, want 1", ev.Version)
	}
	if strings.TrimSpace(ev.Command) == "" || len(ev.Argv) == 0 {
		return errors.New("records no command/argv")
	}
	if !adapterDogfoodSHARe.MatchString(ev.Revision) {
		return fmt.Errorf("revision %q is not a 40-hex commit", ev.Revision)
	}
	if strings.TrimSpace(ev.Provider) == "" || strings.TrimSpace(ev.Model) == "" {
		return errors.New("records no provider/model identity")
	}
	seen := map[string]bool{}
	for _, c := range ev.Configurations {
		if err := validateAdapterDogfoodConfig(c, artifactExists); err != nil {
			return fmt.Errorf("configuration %q: %w", c.Name, err)
		}
		seen[c.Name] = true
	}
	for _, want := range requiredAdapterDogfoodConfigs {
		if !seen[want] {
			return fmt.Errorf("required configuration %q is not present", want)
		}
	}
	return nil
}

func validateAdapterDogfoodConfig(c adapterDogfoodConfig, artifactExists func(string) bool) error {
	switch c.Classification {
	case "simulated":
		if strings.TrimSpace(c.Reason) == "" {
			return errors.New("simulated configuration must be identified (reason required)")
		}
		if v, ok := c.Metrics["inference_calls"]; ok && v != 0 {
			return errors.New("simulated configuration must not claim live inference (inference_calls must be 0)")
		}
	case "measured":
		if c.SampleSize < 1 {
			return errors.New("measured configuration must declare sample_size >= 1")
		}
		if len(c.Metrics) == 0 {
			return errors.New("measured configuration has no numeric metrics")
		}
		if strings.TrimSpace(c.ArtifactPath) == "" || !artifactExists(c.ArtifactPath) {
			return fmt.Errorf("measured configuration references missing execution evidence %q", c.ArtifactPath)
		}
	case "untested":
		if strings.TrimSpace(c.Reason) == "" {
			return errors.New("untested configuration must state a reason")
		}
		if len(c.Metrics) > 0 {
			return errors.New("untested configuration must not carry invented metrics")
		}
	default:
		return fmt.Errorf("classification %q is not simulated/measured/untested", c.Classification)
	}
	return nil
}

// --- Synthetic validator tests (no artifact is created) ---

func syntheticAdapterDogfood() adapterDogfoodEvidence {
	return adapterDogfoodEvidence{
		Version:  1,
		Command:  "go test -run TestAdapterConnectorIntegration ./internal/decision/commandprofile/",
		Argv:     []string{"go", "test"},
		Revision: strings.Repeat("a", 40),
		Provider: "in-process",
		Model:    "fake-adapter",
		Configurations: []adapterDogfoodConfig{
			{Name: "connector-offline", Classification: "simulated", Reason: "offline integration against a deterministic fake adapter", Metrics: map[string]float64{"inference_calls": 0}},
			{Name: "julia-live", Classification: "untested", Reason: "requires Gate W3-4 live authorization"},
			{Name: "nimble-live", Classification: "untested", Reason: "requires Gate W3-4 live authorization"},
		},
	}
}

func TestAdapterDogfoodValidatorAcceptsWellFormedSynthetic(t *testing.T) {
	if err := validateAdapterDogfood(syntheticAdapterDogfood(), func(string) bool { return true }); err != nil {
		t.Fatalf("well-formed synthetic rejected: %v", err)
	}
}

func TestAdapterDogfoodValidatorRejectsSimulatedLiveClaim(t *testing.T) {
	ev := syntheticAdapterDogfood()
	ev.Configurations[0].Metrics["inference_calls"] = 5
	if err := validateAdapterDogfood(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: simulated configuration claiming live inference")
	}
}

func TestAdapterDogfoodValidatorRejectsUntestedWithMetrics(t *testing.T) {
	ev := syntheticAdapterDogfood()
	ev.Configurations[1].Metrics = map[string]float64{"correct_rate": 1}
	if err := validateAdapterDogfood(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: untested configuration carrying metrics")
	}
}

func TestAdapterDogfoodValidatorRejectsMeasuredWithoutArtifact(t *testing.T) {
	ev := syntheticAdapterDogfood()
	ev.Configurations[1] = adapterDogfoodConfig{Name: "julia-live", Classification: "measured", SampleSize: 1, Metrics: map[string]float64{"latency_ms": 1}, ArtifactPath: "missing"}
	if err := validateAdapterDogfood(ev, func(string) bool { return false }); err == nil {
		t.Fatal("expected rejection: measured without resolvable evidence")
	}
}

func TestAdapterDogfoodValidatorRejectsMissingConfig(t *testing.T) {
	ev := syntheticAdapterDogfood()
	ev.Configurations = ev.Configurations[:1]
	if err := validateAdapterDogfood(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: missing required configuration")
	}
}

func TestAdapterDogfoodDecoderRejectsUnknownField(t *testing.T) {
	if _, err := decodeAdapterDogfood([]byte(`{"version":1,"surprise":true}`)); err == nil {
		t.Fatal("expected rejection of an unknown field")
	}
}
