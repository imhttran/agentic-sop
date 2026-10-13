package config

import (
	"strings"
	"testing"
)

// TestParseDesignatedApprovers pins that the operator-designated approver list
// (SOP-HARD-04) is parsed, that it is empty by default (fail-closed), and that an
// unknown key is still rejected.
func TestParseDesignatedApprovers(t *testing.T) {
	base := "version: 1\nproject:\n  name: t\nagent:\n  harness: tool\n  provider: ollama\n  model: m\nreview:\n  engine: self\n"

	defaulted, err := Parse([]byte(base))
	if err != nil {
		t.Fatalf("parse without human block: %v", err)
	}
	if len(defaulted.Human.DesignatedApprovers) != 0 {
		t.Fatalf("designated approvers should default to empty, got %v", defaulted.Human.DesignatedApprovers)
	}

	withApprovers, err := Parse([]byte(base + "human:\n  designated_approvers:\n    - operator\n    - alice\n"))
	if err != nil {
		t.Fatalf("parse with designated approvers: %v", err)
	}
	got := withApprovers.Human.DesignatedApprovers
	if len(got) != 2 || got[0] != "operator" || got[1] != "alice" {
		t.Fatalf("designated approvers = %v, want [operator alice]", got)
	}
}

// TestParseRejectsUnknownHumanKey proves the new field did not loosen validation.
func TestParseRejectsUnknownHumanKey(t *testing.T) {
	base := "version: 1\nproject:\n  name: t\nagent:\n  harness: tool\n  provider: ollama\n  model: m\nreview:\n  engine: self\n"
	_, err := Parse([]byte(base + "human:\n  approvers_x:\n    - operator\n"))
	if err == nil || !strings.Contains(err.Error(), "approvers_x") {
		t.Fatalf("unknown human key should be rejected, got %v", err)
	}
}
