package agent

import (
	"context"
	"strings"
	"testing"
)

// restrictedAgent declares a subset of capabilities.
type restrictedAgent struct {
	caps Capabilities
}

func (a restrictedAgent) Generate(_ context.Context, _ Request) (Response, error) {
	return Response{Content: "ok"}, nil
}

func (a restrictedAgent) Capabilities() Capabilities { return a.caps }

// plainAgent declares nothing.
type plainAgent struct{}

func (plainAgent) Generate(_ context.Context, _ Request) (Response, error) {
	return Response{Content: "ok"}, nil
}

func TestCapabilitiesSupportsAndList(t *testing.T) {
	caps := NewCapabilities(Fix, Plan)
	if !caps.Supports(Plan) || !caps.Supports(Fix) {
		t.Error("expected Plan and Fix to be supported")
	}
	if caps.Supports(Review) {
		t.Error("Review should not be supported")
	}
	// List is in the canonical order, not insertion order.
	if got := strings.Join(capNames(caps.List()), ","); got != "PLAN,FIX" {
		t.Errorf("List() = %q, want PLAN,FIX", got)
	}
}

func TestAllCapabilities(t *testing.T) {
	all := AllCapabilities()
	for _, capability := range capabilityOrder {
		if !all.Supports(capability) {
			t.Errorf("AllCapabilities missing %s", capability)
		}
	}
}

func TestCapabilitiesOf(t *testing.T) {
	declared := NewCapabilities(Plan)
	if got := CapabilitiesOf(restrictedAgent{caps: declared}); !got.Supports(Plan) || got.Supports(Fix) {
		t.Errorf("CapabilitiesOf(restricted) = %v", got.List())
	}
	// An agent that declares nothing is assumed to serve everything.
	if got := CapabilitiesOf(plainAgent{}); len(got.List()) != len(capabilityOrder) {
		t.Errorf("CapabilitiesOf(plain) = %v, want all", got.List())
	}
}

func TestCheckedRejectsUnsupportedCapability(t *testing.T) {
	checked := NewChecked(restrictedAgent{caps: NewCapabilities(Plan)})

	_, err := checked.Generate(context.Background(), validRequest(Fix))
	if err == nil {
		t.Fatal("expected an error for an unsupported capability")
	}
	if !strings.Contains(err.Error(), "does not support capability FIX") {
		t.Errorf("error = %q, want it to name the capability", err)
	}
	if !strings.Contains(err.Error(), "PLAN") {
		t.Errorf("error = %q, want it to list the supported set", err)
	}
}

func TestCheckedDelegatesSupportedCapability(t *testing.T) {
	checked := NewChecked(restrictedAgent{caps: NewCapabilities(Plan)})
	resp, err := checked.Generate(context.Background(), validRequest(Plan))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Content)
	}
	if !checked.Capabilities().Supports(Plan) {
		t.Error("Checked.Capabilities should report the wrapped set")
	}
}

func TestCheckedValidatesFirst(t *testing.T) {
	checked := NewChecked(restrictedAgent{caps: NewCapabilities(Plan)})
	if _, err := checked.Generate(context.Background(), Request{Capability: Plan, Task: "  "}); err == nil {
		t.Error("expected a validation error before capability routing")
	}
}

// capNames renders capabilities as strings for assertions.
func capNames(caps []Capability) []string {
	names := make([]string, len(caps))
	for i, capability := range caps {
		names[i] = string(capability)
	}
	return names
}
