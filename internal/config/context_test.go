package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestContextEfficiencyParsesAndDefaults proves the opt-in context-efficiency block
// parses and defaults to off.
func TestContextEfficiencyParsesAndDefaults(t *testing.T) {
	dir := t.TempDir()

	enabled := filepath.Join(dir, "enabled.yaml")
	if err := os.WriteFile(enabled, []byte("version: 1\nproject:\n  name: x\ncontext:\n  decision_memory: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(enabled)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.ContextEfficiency.DecisionMemory {
		t.Error("decision_memory must parse as true")
	}

	absent := filepath.Join(dir, "absent.yaml")
	if err := os.WriteFile(absent, []byte("version: 1\nproject:\n  name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg2, err := Load(absent)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg2.ContextEfficiency.DecisionMemory {
		t.Error("decision_memory must default to false")
	}
}
