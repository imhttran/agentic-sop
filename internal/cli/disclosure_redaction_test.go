package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/testrunner"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// TestCheckEvidenceRedactsCredentialShapes proves a persisted acceptance-check
// log removes a credential-shaped canary even when the value is not present in the
// environment. The shared check-output boundary previously redacted only exact
// environment secret values.
//
// Positive control: the same log keeps the ordinary "budget exhausted" text.
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

// TestToolOutputEnvCanaryDenied proves the approved narrow credential-path policy
// denies a synthetic .env read before content reaches the tool result. The
// ordinary-file control uses the same real path, and the harness audit records
// the denial without result content. Artifact persistence remains a separate
// limit covered by TestRunArtifactPersistsModelOutputVerbatim.
func TestToolOutputEnvCanaryDenied(t *testing.T) {
	const canary = "sk-llm02-env-canary-not-a-real-secret"
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SOP_TEST_API_KEY="+canary+"\n"), 0o600); err != nil {
		t.Fatalf("write synthetic .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("ordinary repository content"), 0o600); err != nil {
		t.Fatalf("write positive-control file: %v", err)
	}

	audit := toolharness.NewAuditLog(0)
	h := toolharness.New(root, toolharness.DefaultConfig(), audit)

	// Positive control: the same real path returns ordinary content.
	out, err := h.Run(context.Background(), toolharness.ToolReadFile, map[string]any{"path": "notes.txt"})
	if err != nil {
		t.Fatalf("positive-control read: %v", err)
	}
	if out != "ordinary repository content" {
		t.Fatalf("positive-control read = %q, want ordinary content", out)
	}

	out, err = h.Run(context.Background(), toolharness.ToolReadFile, map[string]any{"path": ".env"})
	if !errors.Is(err, toolharness.ErrSensitivePath) {
		t.Fatalf("read synthetic .env err = %v, want ErrSensitivePath", err)
	}
	if out != "" {
		t.Fatalf("read synthetic .env returned content despite the denial: %q", out)
	}

	records := audit.Records()
	if len(records) != 2 {
		t.Fatalf("audit recorded %d calls, want 2", len(records))
	}
	denied := false
	for _, rec := range records {
		if strings.Contains(rec.Request, ".env") && rec.Action == toolharness.ActionDeny {
			denied = true
		}
		if strings.Contains(rec.Request, canary) || strings.Contains(rec.Detail, canary) {
			t.Fatalf("harness audit persisted result content: %+v", rec)
		}
	}
	if !denied {
		t.Fatal("harness audit did not record the .env denial")
	}
	t.Logf("llm02 tool-output canary: credential_path_denied=true harness_audit_contains_canary=false")
}

// TestRunArtifactPersistsModelOutputVerbatim records the retained persistence
// path behind the .env canary reconciliation: model output is written to
// implementation.md as produced, so a model that repeats a tool result can put
// the value in a run artifact even though the harness audit does not. This is a
// GAP observation, not a redaction control.
func TestRunArtifactPersistsModelOutputVerbatim(t *testing.T) {
	const canary = "sk-llm02-env-canary-not-a-real-secret"
	rn, err := runpkg.Open(t.TempDir(), "LLM02-ARTIFACT")
	if err != nil {
		t.Fatalf("open run: %v", err)
	}
	content := "model output echoed the tool result: " + canary
	if err := rn.Write("implementation.md", content); err != nil {
		t.Fatalf("write implementation.md: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(rn.Dir(), "implementation.md"))
	if err != nil {
		t.Fatalf("read implementation.md: %v", err)
	}
	if string(data) != content {
		t.Fatalf("implementation.md = %q, want the model output verbatim", string(data))
	}
}

// TestCheckEvidenceRedactsEscapedQuotedCredential exercises the actual acceptance
// check path with `go test -json`: a quoted credential is JSON-escaped in the
// event Output, and redacting the escaped text would leave the value exposed and
// corrupt the JSON. The persisted log must be redacted and remain valid JSON.
func TestCheckEvidenceRedactsEscapedQuotedCredential(t *testing.T) {
	const canary = "canary-private-value"
	body := `"password=\"` + canary + `\""`
	rn, cr, report := failingCheckRun(t, body)

	log := readLog(t, rn, cr)
	if strings.Contains(log, canary) {
		t.Fatalf("log artifact leaks the escaped quoted credential:\n%s", log)
	}
	if !strings.Contains(log, "[redacted]") {
		t.Fatalf("log artifact has no redaction marker:\n%s", log)
	}
	if out, _ := cr["Output"].(string); strings.Contains(out, canary) {
		t.Fatalf("excerpt leaks the escaped quoted credential: %s", out)
	}
	if strings.Contains(report, canary) {
		t.Fatalf("report leaks the escaped quoted credential:\n%s", report)
	}
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		var event any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("persisted log line is not valid JSON: %v\n%s", err, line)
		}
	}
}
