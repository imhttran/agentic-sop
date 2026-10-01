package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// These tests pin the Phase 4 hardening contract that provider validation sees the
// FINAL per-task routing selection, not just the run-level default: when the
// automatic router is enabled, the class a task routes to is the selection
// validated before it executes.

// validateRoutingConfig enables the pre-execution checkpoint (so routing has
// evidence) and opts into provider validation, with high-severity early findings
// kept non-blocking.
const validateRoutingConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nproviders:\n  validate: true\nearly_jev:\n  enabled: true\n  mode: review\n  fail_on:\n    - critical\n  gates:\n    pre_execution: true\n"

// ollamaTagsServer serves Ollama's probe endpoints with exactly the named models,
// so a test can make discovery authoritative about which models exist.
func ollamaTagsServer(t *testing.T, names ...string) *httptest.Server {
	t.Helper()
	entries := make([]string, 0, len(names))
	for _, n := range names {
		entries = append(entries, fmt.Sprintf(`{"name":%q}`, n))
	}
	tags := `{"models":[` + strings.Join(entries, ",") + `]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.1"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(tags))
		case "/api/show":
			_, _ = w.Write([]byte(`{"capabilities":["completion","tools"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestPerTaskValidationValidatesRoutedSelection proves the FINAL routed selection
// is what validation sees: the run-level default (medium) is absent from the
// runtime, but the task routes to large, which is present, so the run proceeds. If
// validation only checked the startup default, the run would fail.
func TestPerTaskValidationValidatesRoutedSelection(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	// Only the LARGE model exists; the default MEDIUM model does not.
	t.Setenv(ollama.EnvBaseURL, ollamaTagsServer(t, "deepseek-v4.1-flash:cloud").URL)

	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, validateRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewScopeConcernFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: large") {
		t.Errorf("stdout missing the routed LARGE selection: %q", stdout)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("stdout missing PASS: %q", stdout)
	}
}

// TestPerTaskValidationStopsOnAbsentRoutedModel proves validation of the final
// selection fails closed: the routed LARGE model is absent from the runtime, so the
// task stops before implementation — SOP never substitutes a different model.
func TestPerTaskValidationStopsOnAbsentRoutedModel(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	// Only the default MEDIUM model exists; the routed LARGE model does not.
	t.Setenv(ollama.EnvBaseURL, ollamaTagsServer(t, "nemotron-3-super:cloud").URL)

	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, validateRoutingConfig)

	a := &recordingAgent{}
	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a, fakeJEV(jev.NewScopeConcernFake()), "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "deepseek-v4.1-flash:cloud") {
		t.Errorf("stderr should name the absent routed model: %q", stderr)
	}
	if len(a.inputs) != 0 {
		t.Errorf("implementation must not run when the routed model fails validation; inputs=%v", a.inputs)
	}
	if strings.Contains(stdout, "PASS") {
		t.Errorf("a failed validation must not report PASS: %q", stdout)
	}
}

// TestValidationDisabledIsUnchangedWhenRoutingOn proves that with validation off
// (the default), an enabled router neither probes nor blocks: a dead endpoint is
// never contacted and the run proceeds as before.
func TestValidationDisabledIsUnchangedWhenRoutingOn(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	// A dead endpoint would fail validation if it were (wrongly) probed.
	t.Setenv(ollama.EnvBaseURL, "http://127.0.0.1:1")

	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig) // no providers.validate

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("validation must be a no-op when disabled: code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Task routing: small") {
		t.Errorf("stdout missing routing line: %q", stdout)
	}
}

// TestStartupValidationStillRunsWhenRoutingDisabled preserves the pre-existing
// behavior: with the automatic router off, the run-level/default selection is the
// one validated, and an absent model stops the run up front.
func TestStartupValidationStillRunsWhenRoutingDisabled(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "false")
	t.Setenv(ollama.EnvBaseURL, ollamaTagsServer(t, "qwen3:4b").URL)

	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nagent:\n  provider: ollama\n  model: absent:1b\nproviders:\n  validate: true\n")

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", routingAgent(), "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "absent:1b") {
		t.Errorf("stderr should name the absent default model: %q", stderr)
	}
}
