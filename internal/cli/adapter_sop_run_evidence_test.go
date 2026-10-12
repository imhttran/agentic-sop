package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// adapterSOPRunEvidencePath is the JEV-213/RD-01 governed-run evidence artifact,
// relative to the repository root.
const adapterSOPRunEvidencePath = "docs/reports/evidence/JEV-WAVE4-SOP-RUN-DOGFOOD.json"

var adapterRunSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

type sopRunEvidence struct {
	Version             int            `json:"version"`
	Command             string         `json:"command"`
	Argv                []string       `json:"argv"`
	Revision            string         `json:"revision"`
	Provider            string         `json:"provider"`
	Model               string         `json:"model"`
	GovernedOutcome     string         `json:"governed_outcome"`
	AgentLogicalCalls   int            `json:"agent_logical_calls"`
	AgentHTTPRequests   int            `json:"agent_http_requests"`
	AdapterHTTPRequests int            `json:"adapter_http_requests"`
	Configurations      []sopRunConfig `json:"configurations"`
}

type sopRunConfig struct {
	Name           string             `json:"name"`
	Classification string             `json:"classification"`
	SampleSize     int                `json:"sample_size,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
}

// TestAdapterSOPRunIntegrationHasEvidence validates the RD-01 governed-run
// evidence: a real SOP lifecycle was recorded, the adapter's consultation is
// classified truthfully, and the artifact is credential-free.
func TestAdapterSOPRunIntegrationHasEvidence(t *testing.T) {
	root := repoRootCLI()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adapterSOPRunEvidencePath)))
	if err != nil {
		t.Fatalf("RD-01 evidence artifact missing (%v)", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var ev sopRunEvidence
	if err := dec.Decode(&ev); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if ev.Version != 1 || strings.TrimSpace(ev.Command) == "" || len(ev.Argv) == 0 {
		t.Fatalf("evidence header incomplete: %+v", ev)
	}
	if !adapterRunSHARe.MatchString(ev.Revision) {
		t.Fatalf("revision %q is not a 40-hex commit", ev.Revision)
	}
	if strings.TrimSpace(ev.Provider) == "" || strings.TrimSpace(ev.Model) == "" || strings.TrimSpace(ev.GovernedOutcome) == "" {
		t.Fatalf("provider/model/governed_outcome required: %+v", ev)
	}
	needed := map[string]string{"governed-run": "measured", "adapter-decision-evidence": "", "adapter-offline": "simulated"}
	seen := map[string]sopRunConfig{}
	for _, c := range ev.Configurations {
		seen[c.Name] = c
	}
	for name, wantClass := range needed {
		c, ok := seen[name]
		if !ok {
			t.Errorf("missing configuration %q", name)
			continue
		}
		if wantClass != "" && c.Classification != wantClass {
			t.Errorf("configuration %q classification = %q, want %q", name, c.Classification, wantClass)
		}
		switch c.Classification {
		case "measured":
			if len(c.Metrics) == 0 {
				t.Errorf("measured configuration %q has no metrics", name)
			}
		case "untested", "simulated":
			if strings.TrimSpace(c.Reason) == "" {
				t.Errorf("%s configuration %q must state a reason", c.Classification, name)
			}
		default:
			t.Errorf("configuration %q has unknown classification %q", name, c.Classification)
		}
	}
	low := strings.ToLower(string(data))
	for _, bad := range []string{"api_key", "apikey", "bearer", "password", "secret", "token=", "-----begin"} {
		if strings.Contains(low, bad) {
			t.Errorf("evidence appears to contain credential material: %q", bad)
		}
	}
}
