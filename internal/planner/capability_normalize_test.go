package planner

import "testing"

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
