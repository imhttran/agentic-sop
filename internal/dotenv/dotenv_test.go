package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	content := `
# a comment
SOP_MODEL_DEFAULT_CLASS=medium

export SOP_MODEL_SMALL_NAME = qwen3:4b
SOP_MODEL_MEDIUM_NAME="glm-5.3-flash:cloud"
SOP_MODEL_LARGE_NAME='deepseek-v4.1-flash:cloud'
EMPTY=
`
	vars, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[string]string{
		"SOP_MODEL_DEFAULT_CLASS": "medium",
		"SOP_MODEL_SMALL_NAME":    "qwen3:4b",
		"SOP_MODEL_MEDIUM_NAME":   "glm-5.3-flash:cloud",
		"SOP_MODEL_LARGE_NAME":    "deepseek-v4.1-flash:cloud",
		"EMPTY":                   "",
	}
	if len(vars) != len(want) {
		t.Fatalf("got %d vars, want %d: %v", len(vars), len(want), vars)
	}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("%s = %q, want %q", k, vars[k], v)
		}
	}
}

func TestParseRejectsMalformedLine(t *testing.T) {
	if _, err := Parse("NOT_A_PAIR\n"); err == nil {
		t.Fatal("expected an error for a line without '='")
	}
	if _, err := Parse("1BAD=value\n"); err == nil {
		t.Fatal("expected an error for an invalid variable name")
	}
}

func TestLoadFileMissingIsNotAnError(t *testing.T) {
	vars, err := LoadFile(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("missing file must not be an error: %v", err)
	}
	if vars != nil {
		t.Fatalf("missing file must return nil, got %v", vars)
	}
}

func TestLoadDirReadsEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vars, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if vars["A"] != "1" {
		t.Fatalf("A = %q, want 1", vars["A"])
	}
}

func TestApplyLeavesProcessEnvironmentUntouched(t *testing.T) {
	t.Setenv("DOTENV_TEST_KEEP", "process")
	vars := map[string]string{
		"DOTENV_TEST_KEEP": "file",
		"DOTENV_TEST_NEW":  "file",
	}
	t.Cleanup(func() { os.Unsetenv("DOTENV_TEST_NEW") })

	applied := Apply(vars)

	if got := os.Getenv("DOTENV_TEST_KEEP"); got != "process" {
		t.Errorf("process environment was overridden: got %q, want process", got)
	}
	if got := os.Getenv("DOTENV_TEST_NEW"); got != "file" {
		t.Errorf("new variable not applied: got %q, want file", got)
	}
	if len(applied) != 1 || applied[0] != "DOTENV_TEST_NEW" {
		t.Errorf("applied = %v, want [DOTENV_TEST_NEW]", applied)
	}
}
