package cli

import (
	"strings"
	"testing"
)

// TestRunGateRetrievePasses proves the CTX-004 retrieval gate is exposed as a model-free
// command that reports an explicit PASS.
func TestRunGateRetrievePasses(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	a := &planCountAgent{}

	code, out, errOut := runCLIWithAgent(t, dir, a, "gate", "retrieve")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if a.calls != 0 {
		t.Errorf("the gate invoked the model %d time(s); it must not", a.calls)
	}
	if !strings.Contains(out, "RETRIEVAL_GATE = PASS") {
		t.Errorf("stdout = %q", out)
	}
}

// TestRunGateRetrieveJSON proves --json emits the deterministic decision document.
func TestRunGateRetrieveJSON(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	code, out, errOut := runCLI(t, dir, "gate", "retrieve", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "\"retrieval_gate\": \"PASS\"") {
		t.Errorf("json stdout = %q", out)
	}
}

// TestRunGateRetrieveRejectsBadK proves argument validation fails closed.
func TestRunGateRetrieveRejectsBadK(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	if code, _, _ := runCLI(t, dir, "gate", "retrieve", "--k", "0"); code != exitUsage {
		t.Errorf("code = %d, want %d", code, exitUsage)
	}
}
