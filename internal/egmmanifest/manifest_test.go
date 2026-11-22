package egmmanifest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

func TestValidateAcceptsSyntheticManifest(t *testing.T) {
	m, _ := newFixture(t)
	if err := m.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	mutate := map[string]func(*egmmanifest.Manifest){
		"unsupported schema": func(m *egmmanifest.Manifest) { m.SchemaVersion = 2 },
		"empty corpus":       func(m *egmmanifest.Manifest) { m.Corpus = "" },
		"no cases":           func(m *egmmanifest.Manifest) { m.Cases = nil },
		"duplicate id":       func(m *egmmanifest.Manifest) { m.Cases[1].ID = m.Cases[0].ID },
		"bad stratum":        func(m *egmmanifest.Manifest) { m.Cases[0].Stratum = "unknown" },
		"seeded none":        func(m *egmmanifest.Manifest) { m.Cases[0].SeededDefect = "none" },
		"seeded no answer":   func(m *egmmanifest.Manifest) { m.Cases[0].ExpectedFinding = nil },
		"control with class": func(m *egmmanifest.Manifest) { m.Cases[1].SeededDefect = "scope-violation" },
		"control no check":   func(m *egmmanifest.Manifest) { m.Cases[1].ControlCheck = "" },
		"uppercase hash":     func(m *egmmanifest.Manifest) { m.Cases[0].ArtifactSHA256 = strings.ToUpper(m.Cases[0].ArtifactSHA256) },
		"unsafe path":        func(m *egmmanifest.Manifest) { m.Cases[0].ArtifactPath = "../escape.md" },
		"leak asserted": func(m *egmmanifest.Manifest) {
			m.Cases[0].LeakChecks.ArtifactContainsAnswer = true
		},
		"bad severity": func(m *egmmanifest.Manifest) {
			m.Cases[0].ExpectedFinding.Severity = "SEVERE"
		},
	}
	for name, fn := range mutate {
		t.Run(name, func(t *testing.T) {
			m, _ := newFixture(t)
			fn(m)
			if err := m.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadValid(t *testing.T) {
	m, _ := newFixture(t)
	p := filepath.Join(t.TempDir(), "manifest.json")
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := egmmanifest.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(loaded.Cases))
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	m, _ := newFixture(t)
	p := filepath.Join(t.TempDir(), "manifest.json")
	data, _ := json.Marshal(m)
	data = []byte(strings.Replace(string(data), `"cases":`, `"unexpected":true,"cases":`, 1))
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := egmmanifest.Load(p); err == nil {
		t.Fatal("expected unknown-field error")
	}
}

func TestLoadRejectsTrailingData(t *testing.T) {
	for name, trailer := range map[string]string{
		"object":    "{}",
		"array":     "[]",
		"delimiter": "]",
		"scalar":    "0",
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := newFixture(t)
			p := filepath.Join(t.TempDir(), "manifest.json")
			data, _ := json.Marshal(m)
			if err := os.WriteFile(p, append(data, []byte(trailer)...), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := egmmanifest.Load(p); err == nil {
				t.Fatal("expected trailing-data error")
			}
		})
	}
}

// writeManifest marshals m, applies mutate to its generic form, and writes the result.
func writeManifest(t *testing.T, m *egmmanifest.Manifest, mutate func(map[string]any)) string {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	mutate(generic)
	data, err = json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadRequiresExplicitLeakChecks(t *testing.T) {
	eachCase := func(fn func(map[string]any)) func(map[string]any) {
		return func(m map[string]any) {
			cases, _ := m["cases"].([]any)
			for _, c := range cases {
				fn(c.(map[string]any))
			}
		}
	}
	mutations := map[string]func(map[string]any){
		"missing object": eachCase(func(c map[string]any) { delete(c, "leak_checks") }),
		"null object":    eachCase(func(c map[string]any) { c["leak_checks"] = nil }),
		"empty object":   eachCase(func(c map[string]any) { c["leak_checks"] = map[string]any{} }),
		"partial object": eachCase(func(c map[string]any) {
			c["leak_checks"] = map[string]any{"artifact_contains_answer": false}
		}),
		"non-boolean value": eachCase(func(c map[string]any) {
			c["leak_checks"] = map[string]any{
				"artifact_contains_answer":            false,
				"permitted_reference_contains_answer": false,
				"later_evidence_in_tree":              "false",
			}
		}),
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			m, _ := newFixture(t)
			if _, err := egmmanifest.Load(writeManifest(t, m, mutate)); err == nil {
				t.Fatal("manifest without explicit leak_checks was accepted")
			}
		})
	}
}

func TestLoadRejectsInvalidJSONAndMissing(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := egmmanifest.Load(bad); err == nil {
		t.Fatal("expected invalid-JSON error")
	}
	if _, err := egmmanifest.Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected missing-file error")
	}
}
