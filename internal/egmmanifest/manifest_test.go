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
	m, _ := newFixture(t)
	p := filepath.Join(t.TempDir(), "manifest.json")
	data, _ := json.Marshal(m)
	if err := os.WriteFile(p, append(data, []byte("{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := egmmanifest.Load(p); err == nil {
		t.Fatal("expected trailing-data error")
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
