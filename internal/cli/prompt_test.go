package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// textOnlyAgent declares exactly the text-generation capabilities the
// OpenAI-compatible providers serve, so a mutating capability is rejected.
type textOnlyAgent struct{ content string }

func (t *textOnlyAgent) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	return agent.Response{Content: t.content}, nil
}

func (t *textOnlyAgent) Capabilities() agent.Capabilities {
	return agent.NewCapabilities(agent.Plan, agent.DesignTests, agent.DiagnoseFailure, agent.Review)
}

// promptRunDirs returns the prompt run directories under dir.
func promptRunDirs(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, stateDirName, "runs", "prompts", "*"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// TestPromptDefaultCapabilityIsReadOnly proves a bare prompt uses the conservative
// read-only default (PLAN) and never the mutating path.
func TestPromptDefaultCapabilityIsReadOnly(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{plan: "the plan"}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "Explain the architecture")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Capability: PLAN") {
		t.Errorf("default capability should be PLAN: %q", stdout)
	}
	if !strings.Contains(stdout, "the plan") {
		t.Errorf("result missing: %q", stdout)
	}

	dirs := promptRunDirs(t, dir)
	if len(dirs) != 1 {
		t.Fatalf("want one prompt run dir, got %v", dirs)
	}
	for _, name := range []string{"prompt.md", "result.md", "metadata.json"} {
		if !stateExists(filepath.Join(dirs[0], name)) {
			t.Errorf("missing artifact %s in %s", name, dirs[0])
		}
	}
}

// TestPromptExplicitReviewCapability proves an explicit read-only capability runs.
func TestPromptExplicitReviewCapability(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: "reviewed"}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Capability: REVIEW") || !strings.Contains(stdout, "reviewed") {
		t.Errorf("stdout = %q", stdout)
	}
}

// TestPromptFileInput proves --file is read and its content preserved.
func TestPromptFileInput(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "p.md", "  review the provider package  \n")
	a := &fakeCapabilityAgent{review: "ok"}

	code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "--file", "p.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	prompt, err := os.ReadFile(filepath.Join(promptRunDirs(t, dir)[0], "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "review the provider package") {
		t.Errorf("prompt.md should carry the file content:\n%s", prompt)
	}
}

func TestPromptInvalidInputs(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{plan: "x"}
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no prompt", []string{"prompt"}, exitUsage},
		{"unknown capability", []string{"prompt", "--capability", "deploy", "go"}, exitUsage},
		{"positional and file", []string{"prompt", "--file", "p.md", "go"}, exitUsage},
		{"missing file", []string{"prompt", "--file", "nope.md"}, exitError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, _ := runCLIWithAgent(t, dir, a, tc.args...)
			if code != tc.want {
				t.Fatalf("code=%d, want %d", code, tc.want)
			}
		})
	}
}

// TestPromptRoutingNoEvidenceIsMedium proves a prompt routes conservatively to
// MEDIUM when routing is enabled but no JEV evidence is available, and the routing
// artifact reuses the standard contract.
func TestPromptRoutingNoEvidenceIsMedium(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeConfig(t, dir, plainRoutingConfig)
	a := &fakeCapabilityAgent{review: "ok"}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Class: medium") {
		t.Errorf("stdout missing medium routing: %q", stdout)
	}
	var art struct {
		Version int    `json:"version"`
		Class   string `json:"class"`
		Source  string `json:"source"`
		Model   string `json:"model"`
	}
	readRunJSON(t, filepath.Join(promptRunDirs(t, dir)[0], "routing.json"), &art)
	if art.Class != "medium" || art.Model != "glm-5.3-flash:cloud" || art.Source != "policy" {
		t.Fatalf("routing = %+v, want medium/policy/glm-5.3-flash:cloud", art)
	}
}

// TestPromptRoutingSmallFromJEVMakesSmall proves typed JEV evidence drives the
// prompt route (it is evidence, not prose).
func TestPromptRoutingSmallFromJEVMakesSmall(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeConfig(t, dir, triageRoutingConfig)
	a := &fakeCapabilityAgent{review: "ok"}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "", a, fakeJEV(jev.NewClearFake()), "prompt", "--capability", "review", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Class: small") {
		t.Errorf("stdout missing small routing: %q", stdout)
	}
}

// TestPromptRoutingHighRiskIsLarge proves a cross-cutting typed evidence signal
// escalates the prompt to LARGE.
func TestPromptRoutingHighRiskIsLarge(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeConfig(t, dir, triageRoutingConfig)
	a := &fakeCapabilityAgent{review: "ok"}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "", a, fakeJEV(jev.NewScopeConcernFake()), "prompt", "--capability", "review", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Class: large") {
		t.Errorf("stdout missing large routing: %q", stdout)
	}
}

// TestPromptManualOverrideWins proves --model-class beats the router.
func TestPromptManualOverrideWins(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeConfig(t, dir, triageRoutingConfig)
	a := &fakeCapabilityAgent{review: "ok"}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "", a, fakeJEV(jev.NewClearFake()), "prompt", "--capability", "review", "--model-class", "large", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Class: large") {
		t.Errorf("manual override should win: %q", stdout)
	}
}

// TestPromptImplementCannotRunOnTextOnlyProvider proves the capability guard
// rejects IMPLEMENT on a provider that cannot serve it, instead of reinterpreting it.
func TestPromptImplementCannotRunOnTextOnlyProvider(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &textOnlyAgent{content: "should not run"}

	code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "implement", "Add caching")
	if code == exitOK {
		t.Fatalf("IMPLEMENT on a text-only provider must fail; stdout=%q", stderr)
	}
	if !strings.Contains(stderr, "cannot IMPLEMENT") {
		t.Errorf("stderr should name the rejected capability: %q", stderr)
	}
}

// TestPromptImplementReusesGovernedLifecycle proves a mutating prompt runs the
// existing implementation lifecycle (plan -> implement -> validate -> review ->
// gate) and produces the standard run artifacts, not a bare Generate shortcut.
func TestPromptImplementReusesGovernedLifecycle(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &recordingAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "prompt", "--capability", "implement", "Add caching to provider discovery")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("governed lifecycle should pass the gate: %q", stdout)
	}
	dirs := promptRunDirs(t, dir)
	if len(dirs) != 1 {
		t.Fatalf("want one prompt run dir, got %v", dirs)
	}
	for _, name := range []string{"prompt.md", "plan.md", "implementation.md", "report.json", "metadata.json"} {
		if !stateExists(filepath.Join(dirs[0], name)) {
			t.Errorf("governed prompt run missing %s", name)
		}
	}
}

// TestPromptProviderValidationBlocksAbsentModel proves the final prompt selection
// is validated and a definitely-absent model stops before any agent work.
func TestPromptProviderValidationBlocksAbsentModel(t *testing.T) {
	clearProviderEnv(t)
	srv := providerTestServer(t)
	t.Setenv(ollama.EnvBaseURL, srv.URL)
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nagent:\n  provider: ollama\n  model: not-installed:1b\nproviders:\n  validate: true\n")
	a := &countingAgent{}

	code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "review this")
	if code == exitOK {
		t.Fatal("an absent model must fail validation")
	}
	if a.calls != 0 {
		t.Errorf("the agent must not run when validation fails; calls=%d", a.calls)
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr should explain the absent model: %q", stderr)
	}
}

// TestPromptJSONOutput proves --json prints the structured result document.
func TestPromptJSONOutput(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: "the review"}

	code, stdout, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "--json", "review this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var doc promptResultDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if doc.Kind != "prompt" || doc.Capability != "review" || doc.Status != "completed" {
		t.Errorf("doc = %+v", doc)
	}
	if doc.Result != "the review" {
		t.Errorf("result = %q", doc.Result)
	}
	if !strings.Contains(doc.ReportPath, "prompts") || !strings.HasSuffix(doc.ResultPath, "result.md") {
		t.Errorf("paths = %q / %q", doc.ReportPath, doc.ResultPath)
	}
}

// TestPromptTriageEscalationStopsBeforeMutation proves an IMPLEMENT prompt that
// early JEV triage decides requires a human stops before any repository mutation,
// exactly as a task does (Phase 5.4 §6, §8).
func TestPromptTriageEscalationStopsBeforeMutation(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeConfig(t, dir, earlyJEVTriageConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposeTaskTriage, jev.CategoryAmbiguity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"prompt", "--capability", "implement", "Delete the old cache")
	if code == exitOK {
		t.Fatalf("escalated triage must not report success: stdout=%s stderr=%s", stdout, stderr)
	}
	if a.impl != 0 {
		t.Errorf("implementation ran %d times, want 0 (the prompt must not start)", a.impl)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN (early JEV triage)") {
		t.Errorf("stdout missing the triage human boundary: %q", stdout)
	}
}

// TestPromptTriageEscalationStopsReadOnly proves the same boundary applies to a
// read-only prompt: no model call is made.
func TestPromptTriageEscalationStopsReadOnly(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeConfig(t, dir, earlyJEVTriageConfig)
	a := &countingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposeTaskTriage, jev.CategorySecurity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"prompt", "--capability", "review", "review this")
	if code == exitOK {
		t.Fatalf("escalated triage must not report success: stdout=%s stderr=%s", stdout, stderr)
	}
	if a.calls != 0 {
		t.Errorf("the model was called %d times, want 0", a.calls)
	}
}

// TestPromptFileOutsideProjectRejected proves prompt input cannot read an arbitrary
// local file: a path outside the project directory is refused.
func TestPromptFileOutsideProjectRejected(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &fakeCapabilityAgent{review: "x"}
	for _, file := range []string{outside, filepath.Join("..", "..", "etc", "passwd")} {
		code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "--file", file)
		if code == exitOK {
			t.Fatalf("--file %s must be rejected", file)
		}
		if !strings.Contains(stderr, "outside the project directory") {
			t.Errorf("--file %s: stderr = %q", file, stderr)
		}
	}
}

// TestTaskRunUnaffectedByWorkItem is the Phase 5.4 §43 regression: introducing the
// WorkItem adapter must not change existing `sop run` behavior. A planned task still
// runs through runs/<id>/ and never writes under runs/prompts/.
func TestTaskRunUnaffectedByWorkItem(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &recordingAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !stateExists(filepath.Join(dir, stateDirName, "runs", "T001", "report.json")) {
		t.Error("the task run must still write runs/T001/report.json")
	}
	if dirs := promptRunDirs(t, dir); len(dirs) != 0 {
		t.Errorf("a task run must not write prompt artifacts: %v", dirs)
	}
}

// TestPromptReportInspectsRun proves `sop report <prompt-run-id>` renders a
// read-only prompt run through the existing report command.
func TestPromptReportInspectsRun(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: "ok"}
	if code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "review this"); code != exitOK {
		t.Fatalf("setup failed: %s", stderr)
	}
	id := filepath.Base(promptRunDirs(t, dir)[0])

	code, out, stderr := runCLI(t, dir, "report", "prompts/"+id)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"Kind: prompt", "Capability: review", "Status: completed"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}
