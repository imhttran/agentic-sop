package planner

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestValidateCapabilitiesNormalizesReferences pins that a stage requirement may
// reference a declared capability by a case/whitespace variant, while a genuinely
// different name is still undeclared.
func TestValidateCapabilitiesNormalizesReferences(t *testing.T) {
	p := Plan{
		Capabilities: []Capability{{Name: "SOP CLI", Status: CapabilityStatus("EXISTS")}},
		Stages:       []Stage{{ID: "S1", Requires: []string{"sop   cli"}}},
	}
	if err := p.validateCapabilities(); err != nil {
		t.Fatalf("case/whitespace variant should match a declared capability: %v", err)
	}
	p2 := Plan{
		Capabilities: []Capability{{Name: "artifact", Status: CapabilityStatus("EXISTS")}},
		Stages:       []Stage{{ID: "S1", Requires: []string{"artifacts"}}},
	}
	if err := p2.validateCapabilities(); err == nil {
		t.Fatal("a differently-named capability must still be rejected")
	}
}

// TestPlanRepairNamesDeclaredCapabilities proves the repair instruction lists the
// capabilities the rejected plan declared, so a stage's "requires" can be aligned.
func TestPlanRepairNamesDeclaredCapabilities(t *testing.T) {
	rejected := `{"capabilities":[{"name":"CLOSE-009 raw measurement artifact","status":"EXISTS"}],"stages":[]}`
	req := planRepairRequest(agent.Request{Capability: agent.Plan, Task: "t", Input: "orig"}, rejected, errDeclared)
	if !strings.Contains(req.Input, "CLOSE-009 raw measurement artifact") || !strings.Contains(req.Input, "requires") {
		t.Fatalf("repair input does not name the declared capability:\n%s", req.Input)
	}
}

var errDeclared = errorString("plan: stage S1 requires undeclared capability \"x\"")

type errorString string

func (e errorString) Error() string { return string(e) }
