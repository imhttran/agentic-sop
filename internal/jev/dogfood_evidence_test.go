package jev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// This file holds the DETERMINISTIC JEV-207 evidence *validator* and its unit
// tests. It contains no live-evidence acceptance test, so an ordinary
// `go test ./...` stays green without a live dogfood artifact.
//
// The live-evidence acceptance checks (TestJEVDogfoodEvidenceHasExecutionProof,
// TestJEVDogfoodEvidenceHasNoCredentials) live in the build-tagged file
// dogfood_evidence_acceptance_test.go. They are invoked explicitly by the
// registered JEV-207 acceptance checks with -tags=jev207_acceptance and FAIL when
// the live evidence artifact is missing. Synthetic fixtures are used ONLY for
// these validator/negative tests and are never represented as live evidence.

// dogfoodEvidencePath is the JEV-207 machine-readable evidence artifact,
// relative to the repository root.
const dogfoodEvidencePath = "docs/reports/evidence/JEV-WAVE2-DOGFOOD.json"

// requiredDogfoodScenarios is the closed set of scenarios JEV-207 must evidence.
var requiredDogfoodScenarios = []string{
	"direct_sop_jev_invocation",
	"provider_selection",
	"structured_evidence",
	"early_jev_routing",
	"retry",
	"provider_unavailability",
	"human_escalation",
	"fail_closed",
}

// dogfoodEvidence is the shape of the JEV-207 evidence artifact.
type dogfoodEvidence struct {
	Version          int               `json:"version"`
	Command          string            `json:"command"`
	Argv             []string          `json:"argv"`
	Revision         string            `json:"revision"`
	Provider         string            `json:"provider"`
	Model            string            `json:"model"`
	EndpointCategory string            `json:"endpoint_category"`
	Scenarios        []dogfoodScenario `json:"scenarios"`
}

// dogfoodScenario records one scenario's execution evidence. process_outcome is
// the recorded process result (command exit); governed_outcome is the SOP
// lifecycle/autonomy outcome. They are deliberately distinct fields.
type dogfoodScenario struct {
	ID              string `json:"id"`
	Classification  string `json:"classification"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	ExitStatus      *int   `json:"exit_status"`
	LatencyMS       *int   `json:"latency_ms"`
	ProcessOutcome  string `json:"process_outcome"`
	GovernedOutcome string `json:"governed_outcome"`
	ArtifactPath    string `json:"artifact_path"`
	Reason          string `json:"reason"`
}

var dogfoodSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// credentialMarkers are substrings that must never appear in an evidence
// artifact.
var credentialMarkers = []string{"api_key", "apikey", "authorization:", "bearer ", "password", "secret", "token=", "-----begin"}

// decodeDogfoodEvidence strictly decodes an evidence document, rejecting unknown
// fields so a shape drift cannot pass silently.
func decodeDogfoodEvidence(data []byte) (dogfoodEvidence, error) {
	var ev dogfoodEvidence
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil {
		return dogfoodEvidence{}, err
	}
	return ev, nil
}

// credentialMarkersIn returns the credential markers present in data.
func credentialMarkersIn(data []byte) []string {
	lower := strings.ToLower(string(data))
	var hits []string
	for _, m := range credentialMarkers {
		if strings.Contains(lower, m) {
			hits = append(hits, m)
		}
	}
	return hits
}

// validateDogfoodEvidence is the pure validator. artifactExists reports whether a
// referenced artifact resolves; synthetic tests inject a stub so the logic is
// proven without any real execution.
func validateDogfoodEvidence(ev dogfoodEvidence, artifactExists func(string) bool) error {
	if ev.Version != 1 {
		return fmt.Errorf("evidence version = %d, want 1", ev.Version)
	}
	if strings.TrimSpace(ev.Command) == "" {
		return errors.New("evidence records no command")
	}
	if len(ev.Argv) == 0 {
		return errors.New("evidence records no argv")
	}
	if !dogfoodSHARe.MatchString(ev.Revision) {
		return fmt.Errorf("revision %q is not a 40-hex commit", ev.Revision)
	}
	if strings.TrimSpace(ev.Provider) == "" || strings.TrimSpace(ev.Model) == "" {
		return errors.New("evidence records no provider/model identity")
	}
	if strings.TrimSpace(ev.EndpointCategory) == "" {
		return errors.New("evidence records no endpoint category")
	}
	if len(ev.Scenarios) == 0 {
		return errors.New("evidence records no scenarios")
	}
	seen := make(map[string]bool, len(ev.Scenarios))
	for _, s := range ev.Scenarios {
		if err := validateDogfoodScenario(s, artifactExists); err != nil {
			return fmt.Errorf("scenario %q: %w", s.ID, err)
		}
		seen[s.ID] = true
	}
	for _, want := range requiredDogfoodScenarios {
		if !seen[want] {
			return fmt.Errorf("required scenario %q is not evidenced", want)
		}
	}
	return nil
}

func validateDogfoodScenario(s dogfoodScenario, artifactExists func(string) bool) error {
	switch s.Classification {
	case "measured":
		if s.ExitStatus == nil {
			return errors.New("measured scenario has no process exit status")
		}
		if s.LatencyMS == nil {
			return errors.New("measured scenario has no latency")
		}
		if err := requireRFC3339Ordered(s.StartedAt, s.FinishedAt); err != nil {
			return err
		}
		if strings.TrimSpace(s.ProcessOutcome) == "" || strings.TrimSpace(s.GovernedOutcome) == "" {
			return errors.New("measured scenario must record process_outcome and governed_outcome distinctly")
		}
		if !artifactExists(s.ArtifactPath) {
			return fmt.Errorf("measured scenario references a missing artifact %q", s.ArtifactPath)
		}
	case "simulated":
		if strings.TrimSpace(s.Reason) == "" {
			return errors.New("simulated scenario must be identified (reason is required)")
		}
		if strings.TrimSpace(s.ArtifactPath) != "" && !artifactExists(s.ArtifactPath) {
			return fmt.Errorf("simulated scenario references a missing fixture %q", s.ArtifactPath)
		}
	case "untested":
		if strings.TrimSpace(s.Reason) == "" {
			return errors.New("untested scenario must give a reason")
		}
		if s.ExitStatus != nil || s.LatencyMS != nil {
			return errors.New("untested scenario must not carry invented metrics")
		}
	default:
		return fmt.Errorf("classification %q is not measured/simulated/untested", s.Classification)
	}
	return nil
}

func requireRFC3339Ordered(start, finish string) error {
	st, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return fmt.Errorf("started_at %q is not RFC3339", start)
	}
	ft, err := time.Parse(time.RFC3339, finish)
	if err != nil {
		return fmt.Errorf("finished_at %q is not RFC3339", finish)
	}
	if ft.Before(st) {
		return errors.New("finished_at precedes started_at")
	}
	return nil
}

// --- Synthetic validator tests (deterministic; never live evidence) ---

func syntheticDogfoodEvidence() dogfoodEvidence {
	scenario := func(id string) dogfoodScenario {
		return dogfoodScenario{
			ID:              id,
			Classification:  "measured",
			StartedAt:       "2026-01-01T00:00:00Z",
			FinishedAt:      "2026-01-01T00:00:02Z",
			ExitStatus:      intp(0),
			LatencyMS:       intp(2000),
			ProcessOutcome:  "process exit 0",
			GovernedOutcome: "task LOCAL_DONE",
			ArtifactPath:    "docs/reports/evidence/synthetic/" + id + ".json",
		}
	}
	ev := dogfoodEvidence{
		Version:          1,
		Command:          "sop",
		Argv:             []string{"run", "task.md"},
		Revision:         strings.Repeat("a", 40),
		Provider:         "ollama",
		Model:            "model-x",
		EndpointCategory: "local",
	}
	for _, id := range requiredDogfoodScenarios {
		ev.Scenarios = append(ev.Scenarios, scenario(id))
	}
	return ev
}

func intp(i int) *int { return &i }

func TestDogfoodEvidenceValidatorAcceptsWellFormedSynthetic(t *testing.T) {
	if err := validateDogfoodEvidence(syntheticDogfoodEvidence(), func(string) bool { return true }); err != nil {
		t.Fatalf("well-formed synthetic evidence rejected: %v", err)
	}
}

func TestDogfoodEvidenceValidatorRejectsMissingScenario(t *testing.T) {
	ev := syntheticDogfoodEvidence()
	ev.Scenarios = ev.Scenarios[:len(ev.Scenarios)-1]
	if err := validateDogfoodEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for a missing required scenario")
	}
}

func TestDogfoodEvidenceValidatorRejectsMeasuredWithoutExecution(t *testing.T) {
	ev := syntheticDogfoodEvidence()
	ev.Scenarios[0].ExitStatus = nil
	if err := validateDogfoodEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: measured scenario without an exit status")
	}
}

func TestDogfoodEvidenceValidatorRejectsUntestedWithMetrics(t *testing.T) {
	ev := syntheticDogfoodEvidence()
	ev.Scenarios[0].Classification = "untested"
	ev.Scenarios[0].Reason = "no reachable endpoint"
	if err := validateDogfoodEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection: untested scenario carrying metrics")
	}
}

func TestDogfoodEvidenceValidatorRejectsBadTimestamps(t *testing.T) {
	ev := syntheticDogfoodEvidence()
	ev.Scenarios[0].StartedAt = "yesterday"
	if err := validateDogfoodEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for a non-RFC3339 timestamp")
	}
}

func TestDogfoodEvidenceValidatorRejectsMissingArtifact(t *testing.T) {
	if err := validateDogfoodEvidence(syntheticDogfoodEvidence(), func(string) bool { return false }); err == nil {
		t.Fatal("expected rejection when a referenced artifact does not resolve")
	}
}

func TestDogfoodEvidenceValidatorRejectsUnsignedRevision(t *testing.T) {
	ev := syntheticDogfoodEvidence()
	ev.Revision = "abc123"
	if err := validateDogfoodEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for a non-40-hex revision")
	}
}

func TestDogfoodEvidenceValidatorRejectsUnknownClassification(t *testing.T) {
	ev := syntheticDogfoodEvidence()
	ev.Scenarios[0].Classification = "probably-fine"
	if err := validateDogfoodEvidence(ev, func(string) bool { return true }); err == nil {
		t.Fatal("expected rejection for an unknown classification")
	}
}

func TestDogfoodEvidenceDecoderRejectsMalformedJSON(t *testing.T) {
	if _, err := decodeDogfoodEvidence([]byte("{not json")); err == nil {
		t.Fatal("expected rejection of malformed JSON")
	}
}

func TestDogfoodEvidenceDecoderRejectsUnknownField(t *testing.T) {
	if _, err := decodeDogfoodEvidence([]byte(`{"version":1,"surprise":true}`)); err == nil {
		t.Fatal("expected rejection of an unknown field")
	}
}

func TestDogfoodEvidenceCredentialsDetected(t *testing.T) {
	hits := credentialMarkersIn([]byte(`{"note":"api_key=abc","h":"Bearer x"}`))
	if len(hits) == 0 {
		t.Fatal("expected credential markers to be detected")
	}
	if len(credentialMarkersIn([]byte(`{"note":"clean"}`))) != 0 {
		t.Fatal("expected no credential markers in clean data")
	}
}
