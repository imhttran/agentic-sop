package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
)

func TestParseRunArgsModelClass(t *testing.T) {
	var errBuf bytes.Buffer

	opts, ok := parseRunArgs([]string{"--model-class", "large", "--task", "T1.md"}, &errBuf)
	if !ok {
		t.Fatalf("parse failed: %s", errBuf.String())
	}
	if opts.modelClass != "large" || opts.taskArg != "T1.md" {
		t.Fatalf("opts = %+v", opts)
	}

	opts, ok = parseRunArgs([]string{"--model-class=small"}, &errBuf)
	if !ok || opts.modelClass != "small" {
		t.Fatalf("opts = %+v, ok = %v", opts, ok)
	}

	if _, ok := parseRunArgs([]string{"--model-class"}, &errBuf); ok {
		t.Fatal("a --model-class without a value must fail")
	}
}

func TestApplyModelRoutingInactiveByDefault(t *testing.T) {
	// The environment is process-global: the model-routing layer reads it directly,
	// so a developer's shell (or a loaded .env) could otherwise make this assertion
	// about the ABSENCE of configuration fail. Clear the routing namespace so the
	// test proves what it claims.
	clearModelEnv(t)

	cfg := config.Default()
	before := cfg.Agent

	res, err := applyModelRouting(&cfg, "")
	if err != nil {
		t.Fatalf("applyModelRouting: %v", err)
	}
	if res.Active {
		t.Fatal("routing must be inactive without models configuration or environment")
	}
	if cfg.Agent != before {
		t.Fatalf("agent changed while routing is inactive: %+v", cfg.Agent)
	}
}

// setClassEnv sets a fully specified class in the process environment.
func setClassEnv(t *testing.T, class, name, locality string) {
	t.Helper()
	prefix := "SOP_MODEL_" + strings.ToUpper(class) + "_"
	t.Setenv(prefix+"PROVIDER", "ollama")
	t.Setenv(prefix+"NAME", name)
	t.Setenv(prefix+"LOCALITY", locality)
}

func TestApplyModelRoutingFromEnvironment(t *testing.T) {
	// A leaked default class (for example SOP_MODEL_DEFAULT_CLASS from a developer's
	// .env) would outrank the per-class environment set below, so clear the routing
	// namespace first and set it explicitly.
	clearModelEnv(t)
	setClassEnv(t, "medium", "nemotron-3-super:cloud", "cloud")

	cfg := config.Default()
	res, err := applyModelRouting(&cfg, "")
	if err != nil {
		t.Fatalf("applyModelRouting: %v", err)
	}
	if !res.Active || res.Selection.Class != "medium" {
		t.Fatalf("res = %+v, want an active medium selection", res)
	}
	if cfg.Agent.Model != "nemotron-3-super:cloud" {
		t.Fatalf("agent model = %q, want the routed model", cfg.Agent.Model)
	}
}

func TestApplyModelRoutingCLIOverride(t *testing.T) {
	clearModelEnv(t)
	setClassEnv(t, "small", "small-model", "local")
	setClassEnv(t, "large", "large-model", "cloud")

	cfg := config.Default()
	res, err := applyModelRouting(&cfg, "large")
	if err != nil {
		t.Fatalf("applyModelRouting: %v", err)
	}
	if res.Selection.Class != "large" || cfg.Agent.Model != "large-model" {
		t.Fatalf("res = %+v, model = %q; want the large class", res, cfg.Agent.Model)
	}
}

func TestApplyModelRoutingInvalidClassIsActionable(t *testing.T) {
	clearModelEnv(t)
	t.Setenv("SOP_MODEL_DEFAULT_CLASS", "gigantic")
	cfg := config.Default()
	if _, err := applyModelRouting(&cfg, ""); err == nil {
		t.Fatal("expected an error for an unknown model class")
	}
}

func TestLoadDotEnvMissingIsNotAnError(t *testing.T) {
	if err := loadDotEnv(t.TempDir()); err != nil {
		t.Fatalf("a missing .env must not be an error: %v", err)
	}
}

func TestLoadDotEnvLoadsProjectFile(t *testing.T) {
	const key = "SOP_DOTENV_LOAD_TEST_KEY"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(key+"=loaded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv(key) })

	if err := loadDotEnv(dir); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv(key); got != "loaded" {
		t.Fatalf("%s = %q, want loaded", key, got)
	}
}

// clearModelEnv unsets every model-routing environment variable for the duration
// of a test and restores the original values afterwards, so a developer's shell
// (or another test) cannot leak SOP_MODEL_* into a routing assertion.
func clearModelEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		model.EnvDefaultClass, model.EnvFallbackClass, model.EnvAllowCloudFallbackForLocal,
		model.EnvRoutingEnabled, model.EnvEscalationEnabled, model.EnvMaxEscalations,
	}
	for _, c := range model.Classes {
		for _, f := range []string{"PROVIDER", "NAME", "LOCALITY"} {
			keys = append(keys, model.ClassEnvKey(c, f), model.ClassFallbackEnvKey(c, f))
		}
	}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { _ = os.Setenv(k, v) })
			_ = os.Unsetenv(k)
		}
	}
}

// readRunJSON reads and unmarshals a run artifact.
func readRunJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

// passingTaskRun is a one-task project whose lifecycle passes, so report.json is
// written. It returns the project directory and the injected agent.
func passingTaskRun(t *testing.T) (string, agent.Agent) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
	return dir, a
}

func TestRunRecordsModelSelectionEvidence(t *testing.T) {
	clearModelEnv(t)
	dir, a := passingTaskRun(t)

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md", "--model-class", "small")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Model class: small (local, source: cli; explicit CLI model class)") {
		t.Errorf("stdout missing the model-class line: %q", stdout)
	}

	runDir := filepath.Join(dir, stateDirName, "runs", "T001")

	// The standalone artifact records the non-secret selection.
	var sel model.Selection
	readRunJSON(t, filepath.Join(runDir, "model-selection.json"), &sel)
	want := model.Selection{
		Class:    model.ClassSmall,
		Provider: "ollama",
		Model:    "qwen3:4b",
		Locality: model.LocalityLocal,
		Source:   model.SourceCLI,
		Reason:   model.ReasonCLIClass,
	}
	if sel != want {
		t.Fatalf("model-selection.json = %+v, want %+v", sel, want)
	}

	// report.json carries the same evidence under model_selection.
	var doc runReportDoc
	readRunJSON(t, filepath.Join(runDir, "report.json"), &doc)
	if doc.ModelSelection == nil || *doc.ModelSelection != want {
		t.Fatalf("report.json model_selection = %+v, want %+v", doc.ModelSelection, want)
	}

	// `sop report` surfaces it, so the choice is auditable after the fact.
	if code, out, errOut := runCLI(t, dir, "report", "T001"); code != exitOK {
		t.Fatalf("report: code=%d stderr=%s", code, errOut)
	} else if !strings.Contains(out, "Model selection:") || !strings.Contains(out, "qwen3:4b") {
		t.Errorf("report output missing model selection: %q", out)
	}

	// No credential is present anywhere in the run artifacts.
	for _, name := range []string{"model-selection.json", "report.json"} {
		data, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(data)), "token") || strings.Contains(strings.ToLower(string(data)), "api_key") {
			t.Errorf("%s appears to carry a credential: %s", name, data)
		}
	}
}

func TestRunWithoutRoutingRecordsNoModelSelection(t *testing.T) {
	clearModelEnv(t)
	dir, a := passingTaskRun(t)

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "Model class:") {
		t.Errorf("inactive routing must not print a model class: %q", stdout)
	}

	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	if stateExists(filepath.Join(runDir, "model-selection.json")) {
		t.Error("model-selection.json written while routing is inactive")
	}
	var doc runReportDoc
	readRunJSON(t, filepath.Join(runDir, "report.json"), &doc)
	if doc.ModelSelection != nil {
		t.Errorf("report.json recorded model_selection while inactive: %+v", doc.ModelSelection)
	}
}
