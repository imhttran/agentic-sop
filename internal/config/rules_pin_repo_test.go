package config

import (
	"encoding/json"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// The repository's Semgrep rules_sha256 must come from the committed fetch
// manifest (trusted provenance), so updating one without the other fails here.
func TestRepoSemgrepRulesPinMatchesManifest(t *testing.T) {
	var cfg Config
	raw, err := os.ReadFile("../../.agent-sdlc/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		RulesDirSHA256 string `json:"rules_dir_sha256"`
	}
	raw, err = os.ReadFile("../../scripts/security/semgrep-rules-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, a := range cfg.Security.Adapters {
		if a.ID == "semgrep" {
			if a.RulesSHA256 != manifest.RulesDirSHA256 {
				t.Fatalf("semgrep rules_sha256 = %q, manifest rules_dir_sha256 = %q", a.RulesSHA256, manifest.RulesDirSHA256)
			}
			return
		}
	}
	t.Fatal("no semgrep adapter in .agent-sdlc/config.yaml")
}
