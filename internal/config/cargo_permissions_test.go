package config

import (
	"strings"
	"testing"
)

// TestCargoPermissionsDefaultOff proves that an omitted cargo block leaves both
// Cargo permissions off, so existing projects are unaffected (SOP-001).
func TestCargoPermissionsDefaultOff(t *testing.T) {
	const base = `version: 1
project:
  name: demo
`
	c, err := Parse([]byte(base))
	if err != nil {
		t.Fatalf("parse without cargo block: %v", err)
	}
	if c.CargoAgentToolEnabled() {
		t.Fatal("agent-tool Cargo permission must default off")
	}
	if c.CargoAcceptanceCheckEnabled() {
		t.Fatal("acceptance-check Cargo permission must default off")
	}

	// An explicit false is still off, and an empty block is off.
	c, err = Parse([]byte(base + "cargo:\n  agent_tool: false\n  acceptance_check: false\n"))
	if err != nil {
		t.Fatalf("parse explicit false: %v", err)
	}
	if c.CargoAgentToolEnabled() || c.CargoAcceptanceCheckEnabled() {
		t.Fatal("explicit false must resolve to disabled")
	}

	// An unknown key inside the cargo block is still rejected by strict decoding.
	if _, err := Parse([]byte(base + "cargo:\n  bogus: true\n")); err == nil {
		t.Fatal("unknown key inside cargo block must be rejected")
	}
}

// TestCargoLegacyPermissionsMigrate proves the deprecated spellings are mapped
// onto the canonical Cargo permissions, so an existing configuration does not
// silently lose (or gain) a permission. Conflicts fail closed.
func TestCargoLegacyPermissionsMigrate(t *testing.T) {
	const base = `version: 1
project:
  name: demo
`

	c, err := Parse([]byte(base + "agent:\n  tool_cargo: true\n"))
	if err != nil {
		t.Fatalf("parse legacy agent.tool_cargo: %v", err)
	}
	if !c.CargoAgentToolEnabled() || c.CargoAcceptanceCheckEnabled() {
		t.Fatalf("legacy agent permission = agent:%t acceptance:%t, want true/false", c.CargoAgentToolEnabled(), c.CargoAcceptanceCheckEnabled())
	}

	c, err = Parse([]byte(base + "validation:\n  acceptance_checks:\n    cargo: true\n"))
	if err != nil {
		t.Fatalf("parse legacy validation.acceptance_checks.cargo: %v", err)
	}
	if c.CargoAgentToolEnabled() || !c.CargoAcceptanceCheckEnabled() {
		t.Fatalf("legacy acceptance permission = agent:%t acceptance:%t, want false/true", c.CargoAgentToolEnabled(), c.CargoAcceptanceCheckEnabled())
	}

	// An explicit false legacy value stays off and does not widen access.
	c, err = Parse([]byte(base + "agent:\n  tool_cargo: false\nvalidation:\n  acceptance_checks:\n    cargo: false\n"))
	if err != nil {
		t.Fatalf("parse explicit false legacy: %v", err)
	}
	if c.CargoAgentToolEnabled() || c.CargoAcceptanceCheckEnabled() {
		t.Fatal("explicit false legacy values must remain disabled")
	}

	// Canonical and legacy values that agree are accepted.
	c, err = Parse([]byte(base + "agent:\n  tool_cargo: true\nvalidation:\n  acceptance_checks:\n    cargo: true\ncargo:\n  agent_tool: true\n  acceptance_check: true\n"))
	if err != nil || !c.CargoAgentToolEnabled() || !c.CargoAcceptanceCheckEnabled() {
		t.Fatalf("agreeing canonical/legacy values = %+v, %v", c, err)
	}

	// Disagreement is an error naming both spellings, never a silent choice.
	for name, doc := range map[string]string{
		"agent conflict":      "agent:\n  tool_cargo: true\ncargo:\n  agent_tool: false\n",
		"acceptance conflict": "validation:\n  acceptance_checks:\n    cargo: true\ncargo:\n  acceptance_check: false\n",
	} {
		_, err := Parse([]byte(base + doc))
		if err == nil || !strings.Contains(err.Error(), "conflicts") {
			t.Errorf("%s: err = %v, want a conflict error", name, err)
		}
	}

	// Unknown nested legacy keys are still rejected (strict decoding).
	if _, err := Parse([]byte(base + "validation:\n  acceptance_checks:\n    cargo: true\n    other: true\n")); err == nil {
		t.Fatal("unknown acceptance_checks key must be rejected")
	}
}

// TestCargoPermissionsIndependent proves each permission can be on while the
// other is off, in both directions (SOP-001).
func TestCargoPermissionsIndependent(t *testing.T) {
	const base = `version: 1
project:
  name: demo
`
	c, err := Parse([]byte(base + "cargo:\n  agent_tool: true\n  acceptance_check: false\n"))
	if err != nil {
		t.Fatalf("parse agent_tool only: %v", err)
	}
	if !c.CargoAgentToolEnabled() {
		t.Fatal("agent-tool permission should be on")
	}
	if c.CargoAcceptanceCheckEnabled() {
		t.Fatal("acceptance-check permission must stay off")
	}

	c, err = Parse([]byte(base + "cargo:\n  agent_tool: false\n  acceptance_check: true\n"))
	if err != nil {
		t.Fatalf("parse acceptance_check only: %v", err)
	}
	if c.CargoAgentToolEnabled() {
		t.Fatal("agent-tool permission must stay off")
	}
	if !c.CargoAcceptanceCheckEnabled() {
		t.Fatal("acceptance-check permission should be on")
	}
}
