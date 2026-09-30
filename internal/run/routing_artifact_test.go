package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wellFormedRouting() RoutingArtifact {
	return RoutingArtifact{
		Version:   RoutingArtifactVersion,
		Task:      "T001",
		Class:     "medium",
		Source:    RoutingSourcePolicy,
		Reasons:   []string{"moderate task; default class"},
		Provider:  "ollama",
		Model:     "glm-5.3-flash:cloud",
		Locality:  "cloud",
		Timestamp: "2024-01-01T00:00:00Z",
	}
}

func TestRoutingArtifactValidateAcceptsWellFormed(t *testing.T) {
	if err := wellFormedRouting().Validate(); err != nil {
		t.Fatalf("Validate failed on well-formed artifact: %v", err)
	}
}

func TestRoutingArtifactValidateFailsClosed(t *testing.T) {
	cases := map[string]func(a *RoutingArtifact){
		"version": func(a *RoutingArtifact) { a.Version = 999 },
		"source":  func(a *RoutingArtifact) { a.Source = "made_up" },
		"task":    func(a *RoutingArtifact) { a.Task = "" },
		"class":   func(a *RoutingArtifact) { a.Class = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := wellFormedRouting()
			mutate(&a)
			if err := a.Validate(); err == nil {
				t.Fatalf("Validate accepted an invalid %s", name)
			}
		})
	}
}

// TestWriteRoutingArtifact writes the decision beside the run's other artifacts
// and confirms it carries no credential.
func TestWriteRoutingArtifact(t *testing.T) {
	dir := t.TempDir()
	rn, err := New(dir, "T001")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := rn.WriteRoutingArtifact(wellFormedRouting()); err != nil {
		t.Fatalf("WriteRoutingArtifact: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(rn.Dir(), "routing.json"))
	if err != nil {
		t.Fatalf("read routing.json: %v", err)
	}
	if !strings.Contains(string(data), `"class": "medium"`) {
		t.Errorf("routing.json missing class: %s", data)
	}
	if low := strings.ToLower(string(data)); strings.Contains(low, "token") || strings.Contains(low, "api_key") || strings.Contains(low, "secret") {
		t.Errorf("routing.json appears to carry a credential: %s", data)
	}
}
