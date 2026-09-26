package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validRequest returns a minimal, valid request for a capability.
func validRequest(cap Capability) Request {
	return Request{Capability: cap, Task: "do the thing"}
}

func TestCommandAgentReturnsStdout(t *testing.T) {
	resp, err := NewCommandAgent("printf '%s' 'ok'").Generate(context.Background(), validRequest(Implement))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Content)
	}
}

func TestCommandAgentSerializesCapability(t *testing.T) {
	req := Request{Capability: DesignTests, Task: "add feature", Input: "prd", OutputRequirements: "json"}
	resp, err := NewCommandAgent("cat").Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var got Request
	if err := json.Unmarshal([]byte(resp.Content), &got); err != nil {
		t.Fatalf("stdin was not the JSON request: %v (%q)", err, resp.Content)
	}
	if got != req {
		t.Errorf("round-tripped request = %+v, want %+v", got, req)
	}
	if got.Capability != DesignTests {
		t.Errorf("capability = %q, want DESIGN_TESTS", got.Capability)
	}
}

func TestCommandAgentAcceptsAllCapabilities(t *testing.T) {
	caps := []Capability{DesignTests, Implement, DiagnoseFailure, Fix, Review, Plan}
	for _, cap := range caps {
		if _, err := NewCommandAgent("printf '%s' 'ok'").Generate(context.Background(), validRequest(cap)); err != nil {
			t.Errorf("capability %s rejected: %v", cap, err)
		}
	}
}

func TestCommandAgentRejectsUnknownCapabilityWithoutInvokingHarness(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "invoked")
	harness := NewCommandAgent("touch " + marker)

	_, err := harness.Generate(context.Background(), validRequest(Capability("DO_ANYTHING")))
	if err == nil {
		t.Fatal("expected error for unknown capability")
	}
	if !strings.Contains(err.Error(), "invalid agent capability") {
		t.Errorf("error = %q, want invalid-capability message", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("harness was invoked for an invalid capability")
	}
}

func TestCommandAgentRejectsBlankCapability(t *testing.T) {
	_, err := NewCommandAgent("printf '%s' 'ok'").Generate(context.Background(), Request{Task: "x"})
	if err == nil {
		t.Fatal("expected error for blank capability")
	}
}

func TestCommandAgentRejectsBlankTask(t *testing.T) {
	_, err := NewCommandAgent("printf '%s' 'ok'").Generate(context.Background(), Request{Capability: Implement, Task: "   "})
	if err == nil {
		t.Fatal("expected error for blank task")
	}
	if !strings.Contains(err.Error(), "blank task") {
		t.Errorf("error = %q, want blank-task message", err)
	}
}

func TestCommandAgentErrorOmitsStderrMergeOnSuccess(t *testing.T) {
	// Writes a warning to stderr and a real response to stdout.
	resp, err := NewCommandAgent("echo warn >&2; printf '%s' 'ok'").Generate(context.Background(), validRequest(Review))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Content = %q, want only stdout 'ok'", resp.Content)
	}
}

func TestCommandAgentFailureIncludesDiagnostics(t *testing.T) {
	_, err := NewCommandAgent("echo boom >&2; exit 3").Generate(context.Background(), validRequest(Fix))
	if err == nil {
		t.Fatal("expected error for a failing harness")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to include stderr diagnostics", err)
	}
	if !strings.Contains(err.Error(), "FIX") {
		t.Errorf("error = %q, want it to identify the capability", err)
	}
}

func TestCommandAgentRejectsEmptyOutput(t *testing.T) {
	_, err := NewCommandAgent("true").Generate(context.Background(), validRequest(Implement))
	if err == nil {
		t.Fatal("expected error for empty successful output")
	}
	if !strings.Contains(err.Error(), "empty output") {
		t.Errorf("error = %q, want empty-output message", err)
	}
}

func TestCommandAgentRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewCommandAgent("sleep 5").Generate(ctx, validRequest(Implement))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestNewCommandAgentFromEnv(t *testing.T) {
	t.Setenv(EnvAgentCommand, "")
	t.Setenv(legacyEnvAgentCommand, "")
	if _, err := NewCommandAgentFromEnv(); err == nil {
		t.Error("expected error when the command is unset")
	}

	t.Setenv(EnvAgentCommand, "  printf '%s' 'ok'  ")
	a, err := NewCommandAgentFromEnv()
	if err != nil {
		t.Fatalf("NewCommandAgentFromEnv failed: %v", err)
	}
	resp, err := a.Generate(context.Background(), validRequest(Plan))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if strings.TrimSpace(resp.Content) != "ok" {
		t.Errorf("Content = %q, want ok", resp.Content)
	}
}

func TestNewCommandAgentFromEnvFallsBackToLegacy(t *testing.T) {
	t.Setenv(EnvAgentCommand, "")
	t.Setenv(legacyEnvAgentCommand, "printf '%s' 'legacy'")

	a, err := NewCommandAgentFromEnv()
	if err != nil {
		t.Fatalf("NewCommandAgentFromEnv failed: %v", err)
	}
	resp, err := a.Generate(context.Background(), validRequest(Implement))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if strings.TrimSpace(resp.Content) != "legacy" {
		t.Errorf("Content = %q, want legacy", resp.Content)
	}
}

func TestRequestValidate(t *testing.T) {
	cases := []struct {
		name    string
		request Request
		wantErr bool
	}{
		{"valid", validRequest(DesignTests), false},
		{"blank capability", Request{Task: "x"}, true},
		{"unknown capability", validRequest(Capability("NOPE")), true},
		{"blank task", Request{Capability: Implement, Task: ""}, true},
		{"whitespace task", Request{Capability: Implement, Task: "  "}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
