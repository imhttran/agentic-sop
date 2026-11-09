package cli

import (
	"strings"
	"testing"

	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// TestValidationFailureContextBoundsHugeOutput proves a validation failure that
// streams far more than the context limit cannot make the model prompt unbounded.
func TestValidationFailureContextBoundsHugeOutput(t *testing.T) {
	limit := sopctx.DefaultLimits().MaxBytes
	huge := strings.Repeat("x", 3*limit)
	suite := testrunner.SuiteResult{Results: []testrunner.Result{{
		Category: testrunner.UnitTest,
		Command:  "go test ./...",
		Status:   testrunner.Fail,
		Stdout:   huge,
	}}}
	got := validationFailureContext(suite)
	if len(got) > 2*limit+2048 {
		t.Errorf("context length = %d, want at most %d", len(got), 2*limit+2048)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("context missing the truncation marker")
	}
}
