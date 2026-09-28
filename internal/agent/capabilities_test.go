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

func TestIsRepositoryMutation(t *testing.T) {
	mutating := map[Capability]bool{Implement: true, Fix: true}
	for _, capability := range capabilityOrder {
		want := mutating[capability]
		if got := IsRepositoryMutation(capability); got != want {
			t.Errorf("IsRepositoryMutation(%s) = %v, want %v", capability, got, want)
		}
	}
}

func TestCapabilitiesMutating(t *testing.T) {
	if AllCapabilities().Mutating() != true {
		t.Error("AllCapabilities should report a mutating capability")
	}
	if NewCapabilities(Plan, DesignTests, DiagnoseFailure, Review).Mutating() {
		t.Error("text-only capabilities should not report mutation")
	}
	if !NewCapabilities(Plan, Fix).Mutating() {
		t.Error("a set containing Fix should report mutation")
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

func TestOllamaCapabilities(t *testing.T) {
	o, err := NewOllama("http://127.0.0.1:11434", "qwen3:8b", 0)
	if err != nil {
		t.Fatalf("NewOllama failed: %v", err)
	}
	caps := CapabilitiesOf(o)
	if caps.Supports(Implement) {
		t.Error("Ollama must not advertise IMPLEMENT")
	}
	if caps.Supports(Fix) {
		t.Error("Ollama must not advertise FIX")
	}
	for _, want := range []Capability{Plan, DesignTests, DiagnoseFailure, Review} {
		if !caps.Supports(want) {
			t.Errorf("Ollama should advertise %s", want)
		}
	}
	if caps.Mutating() {
		t.Error("Ollama's declared set must not include repository mutation")
	}
}

func TestLlamaCppCapabilities(t *testing.T) {
	c, err := NewLlamaCpp("http://127.0.0.1:8080", "local", "", 0)
	if err != nil {
		t.Fatalf("NewLlamaCpp failed: %v", err)
	}
	caps := CapabilitiesOf(c)
	if caps.Supports(Implement) {
		t.Error("LlamaCpp must not advertise IMPLEMENT")
	}
	if caps.Supports(Fix) {
		t.Error("LlamaCpp must not advertise FIX")
	}
	for _, want := range []Capability{Plan, DesignTests, DiagnoseFailure, Review} {
		if !caps.Supports(want) {
			t.Errorf("LlamaCpp should advertise %s", want)
		}
	}
	if caps.Mutating() {
		t.Error("LlamaCpp's declared set must not include repository mutation")
	}
}

func TestCommandAgentCapabilities(t *testing.T) {
	a := NewCommandAgent("printf '%s' 'ok'")
	caps := CapabilitiesOf(a)
	for _, capability := range capabilityOrder {
		if !caps.Supports(capability) {
			t.Errorf("command agent must support %s", capability)
		}
	}
	if !caps.Supports(Implement) || !caps.Supports(Fix) {
		t.Error("command agent must keep supporting IMPLEMENT and FIX")
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

func TestCheckedRejectsMutatingCapabilitiesForOllama(t *testing.T) {
	o, err := NewOllama("http://127.0.0.1:11434", "qwen3:8b", 0)
	if err != nil {
		t.Fatalf("NewOllama failed: %v", err)
	}
	checked := NewChecked(o)
	for _, capability := range []Capability{Implement, Fix} {
		_, err := checked.Generate(context.Background(), validRequest(capability))
		if err == nil {
			t.Fatalf("expected %s to be rejected", capability)
		}
		if !strings.Contains(err.Error(), "does not support capability "+string(capability)) {
			t.Errorf("error = %q, want it to name %s", err, capability)
		}
		if !strings.Contains(err.Error(), "PLAN") {
			t.Errorf("error = %q, want it to list the supported set", err)
		}
	}
}

func TestCheckedRejectsMutatingCapabilitiesForLlamaCpp(t *testing.T) {
	c, err := NewLlamaCpp("http://127.0.0.1:8080", "local", "", 0)
	if err != nil {
		t.Fatalf("NewLlamaCpp failed: %v", err)
	}
	checked := NewChecked(c)
	for _, capability := range []Capability{Implement, Fix} {
		_, err := checked.Generate(context.Background(), validRequest(capability))
		if err == nil {
			t.Fatalf("expected %s to be rejected", capability)
		}
		if !strings.Contains(err.Error(), "does not support capability "+string(capability)) {
			t.Errorf("error = %q, want it to name %s", err, capability)
		}
		if !strings.Contains(err.Error(), "PLAN") {
			t.Errorf("error = %q, want it to list the supported set", err)
		}
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
