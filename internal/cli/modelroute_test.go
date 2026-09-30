package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
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
	setClassEnv(t, "medium", "glm-5.3-flash:cloud", "cloud")

	cfg := config.Default()
	res, err := applyModelRouting(&cfg, "")
	if err != nil {
		t.Fatalf("applyModelRouting: %v", err)
	}
	if !res.Active || res.Selection.Class != "medium" {
		t.Fatalf("res = %+v, want an active medium selection", res)
	}
	if cfg.Agent.Model != "glm-5.3-flash:cloud" {
		t.Fatalf("agent model = %q, want the routed model", cfg.Agent.Model)
	}
}

func TestApplyModelRoutingCLIOverride(t *testing.T) {
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
