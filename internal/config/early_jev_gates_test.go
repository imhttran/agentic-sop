package config

import (
	"strings"
	"testing"
)

// With the early layer enabled and gates.task_triage on, only the triage gate is
// active; the pre-execution gate stays off.
func TestEarlyJEVTaskTriageGateEnabled(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    task_triage: true\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !c.EarlyJEVTaskTriageEnabled() {
		t.Errorf("task triage gate should be enabled")
	}
	if c.EarlyJEVPreExecutionEnabled() {
		t.Errorf("pre-execution gate should stay off")
	}
}

func TestEarlyJEVPreExecutionGateEnabled(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.EarlyJEVTaskTriageEnabled() {
		t.Errorf("task triage gate should stay off")
	}
	if !c.EarlyJEVPreExecutionEnabled() {
		t.Errorf("pre-execution gate should be enabled")
	}
}

// Enablement is required for a gate: an early layer that is off runs no gate, and
// configuring a gate without enabling the layer fails clearly.
func TestEarlyJEVRunGateOffWithoutEnabled(t *testing.T) {
	if _, err := Parse([]byte("project:\n  name: a\nearly_jev:\n  gates:\n    task_triage: true\n")); err == nil {
		t.Fatalf("a gate configured without early_jev.enabled must fail clearly")
	} else if !strings.Contains(err.Error(), "early_jev.enabled") {
		t.Errorf("error = %v, want it to name early_jev.enabled", err)
	}
}

// Enabling the layer without any gate runs neither checkpoint.
func TestEarlyJEVRunGatesDefaultOff(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nearly_jev:\n  enabled: true\n  mode: review\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.EarlyJEVTaskTriageEnabled() || c.EarlyJEVPreExecutionEnabled() {
		t.Errorf("gates must default off: triage=%v preExec=%v", c.EarlyJEVTaskTriageEnabled(), c.EarlyJEVPreExecutionEnabled())
	}
}

// An unknown key nested under gates fails clearly (KnownFields), so a typo does
// not silently disable a checkpoint.
func TestEarlyJEVRunGateUnknownKeyRejected(t *testing.T) {
	if _, err := Parse([]byte("project:\n  name: a\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    tasktriage: true\n")); err == nil {
		t.Fatalf("an unknown gates key must fail clearly")
	}
}
