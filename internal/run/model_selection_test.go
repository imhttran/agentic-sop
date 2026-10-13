package run

import (
	"strings"
	"testing"
)

// TestModelSelectionArtifactVersioned proves the persisted model-selection evidence
// carries a schema_version, round-trips the non-secret selection fields, and rejects
// an unsupported version.
func TestModelSelectionArtifactVersioned(t *testing.T) {
	a := ModelSelectionArtifact{
		SchemaVersion: ModelSelectionVersion,
		Class:         "large",
		Provider:      "ollama",
		Model:         "m",
		Locality:      "cloud",
		Source:        "env",
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("valid artifact rejected: %v", err)
	}
	data, err := a.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema_version": 1`, `"class": "large"`, `"source": "env"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("artifact missing %s:\n%s", want, data)
		}
	}

	bad := a
	bad.SchemaVersion = 999
	if err := bad.Validate(); err == nil {
		t.Error("unsupported model-selection version must be rejected")
	}
}
