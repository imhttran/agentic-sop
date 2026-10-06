package cli

import (
	"strings"
	"testing"
)

// TestRunGateVectorRejects proves the CTX-009 gate is exposed as a model-free command
// that reports its ADOPT/REJECT decision and exits zero on a rejection.
func TestRunGateVectorRejects(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	a := &planCountAgent{}

	code, out, errOut := runCLIWithAgent(t, dir, a, "gate", "vector")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if a.calls != 0 {
		t.Errorf("the vector gate invoked the model %d time(s); it must not", a.calls)
	}
	if !strings.Contains(out, "VECTOR_RETRIEVAL = REJECT") {
		t.Errorf("stdout = %q", out)
	}
}
