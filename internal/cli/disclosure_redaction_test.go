package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// TestCheckEvidenceRedactsCredentialShapes proves a persisted acceptance-check
// log removes a credential-shaped canary even when the value is not present in the
// environment. The shared check-output boundary previously redacted only exact
// environment secret values.
func TestCheckEvidenceRedactsCredentialShapes(t *testing.T) {
	const canary = "sk-canary-not-a-real-secret"
	rn, cr, report := failingCheckRun(t, `"auth=sk-canary-not-a-real-secret budget exhausted"`)

	if log := readLog(t, rn, cr); strings.Contains(log, canary) || !strings.Contains(log, "[redacted]") {
		t.Fatalf("log artifact did not redact the credential-shaped canary:\n%s", log)
	}
	if out, _ := cr["Output"].(string); strings.Contains(out, canary) {
		t.Fatalf("excerpt leaks the credential-shaped canary: %s", out)
	}
	if strings.Contains(report, canary) {
		t.Fatalf("report leaks the credential-shaped canary:\n%s", report)
	}
	if log := readLog(t, rn, cr); !strings.Contains(log, "budget exhausted") {
		t.Fatalf("ordinary diagnostic text was lost:\n%s", log)
	}
}

// TestValidationFailureContextRedactsCredentialShapes proves the model-visible
// validation failure carries neither a known environment secret nor a
// credential-shaped canary, while ordinary failure text survives.
func TestValidationFailureContextRedactsCredentialShapes(t *testing.T) {
	const envSecret = "env-secret-value-42"
	const canary = "sk-canary-not-a-real-secret"
	t.Setenv("SOP_TEST_API_TOKEN", envSecret)

	suite := testrunner.SuiteResult{
		Status: testrunner.Fail,
		Results: []testrunner.Result{{
			Category: testrunner.UnitTest,
			Command:  "go test ./...",
			Status:   testrunner.Fail,
			ExitCode: 1,
			Stdout:   "token=" + envSecret + " auth=" + canary + " token budget exhausted",
			Stderr:   "ordinary assertion failure",
		}},
	}
	got := validationFailureContext(suite)

	for _, secret := range []string{envSecret, canary} {
		if strings.Contains(got, secret) {
			t.Fatalf("validation failure context leaks %q:\n%s", secret, got)
		}
	}
	for _, want := range []string{"[redacted]", "token budget exhausted", "ordinary assertion failure"} {
		if !strings.Contains(got, want) {
			t.Fatalf("validation failure context lost %q:\n%s", want, got)
		}
	}
}

// TestRedactDiagnosticTextKeepsOrdinaryText is the negative control: empty input
// stays empty and ordinary diagnostic prose is not rewritten.
func TestRedactDiagnosticTextKeepsOrdinaryText(t *testing.T) {
	if got := redactDiagnosticText(""); got != "" {
		t.Fatalf("empty input = %q, want empty", got)
	}
	const ordinary = "token budget exhausted; GET https://example.com/v1/status"
	if got := redactDiagnosticText(ordinary); got != ordinary {
		t.Fatalf("ordinary text changed: %q", got)
	}
}
