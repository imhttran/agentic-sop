package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `version: 1
project:
  name: book-rag
  integration_branch: trunk
agent:
  provider: ollama
validation:
  build:
    - go build ./...
  test:
    - go test ./...
review:
  engine: open-code-review
  delegation: true
quality:
  require_tests: false
  max_fix_cycles: 5
  fail_on:
    - critical
human:
  approval_before_commit: false
`

func TestParseValid(t *testing.T) {
	c, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Version != 1 || c.Project.Name != "book-rag" || c.Project.IntegrationBranch != "trunk" {
		t.Errorf("project = %+v", c.Project)
	}
	if c.Agent.Provider != "ollama" {
		t.Errorf("provider = %q", c.Agent.Provider)
	}
	if len(c.Validation.Test) != 1 || c.Validation.Test[0] != "go test ./..." {
		t.Errorf("validation.test = %v", c.Validation.Test)
	}
	if c.Review.Engine != "open-code-review" || !c.Review.Delegation {
		t.Errorf("review = %+v", c.Review)
	}
	if c.Quality.RequiresTests() {
		t.Error("require_tests = true, want explicit false")
	}
	if c.Quality.MaxFixCycles != 5 {
		t.Errorf("max_fix_cycles = %d, want 5", c.Quality.MaxFixCycles)
	}
	if len(c.Quality.FailOn) != 1 || c.Quality.FailOn[0] != "critical" {
		t.Errorf("fail_on = %v", c.Quality.FailOn)
	}
	if c.Human.RequiresApprovalBeforeCommit() {
		t.Error("approval_before_commit = true, want explicit false")
	}
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: solo\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Version != CurrentVersion {
		t.Errorf("version = %d, want %d", c.Version, CurrentVersion)
	}
	if c.Project.IntegrationBranch != "main" {
		t.Errorf("integration_branch = %q, want main", c.Project.IntegrationBranch)
	}
	if c.Agent.Provider != "command" {
		t.Errorf("provider = %q, want command", c.Agent.Provider)
	}
	if c.Review.Engine != "self" || c.Review.Delegation {
		t.Errorf("review = %+v, want self/false", c.Review)
	}
	if c.Quality.MaxFixCycles != DefaultMaxFixCycles {
		t.Errorf("max_fix_cycles = %d, want %d", c.Quality.MaxFixCycles, DefaultMaxFixCycles)
	}
	if !c.Quality.RequiresTests() {
		t.Error("require_tests defaulted to false, want true")
	}
	if !c.Human.RequiresApprovalBeforeCommit() {
		t.Error("approval_before_commit defaulted to false, want true")
	}
	if got := strings.Join(c.Quality.FailOn, ","); got != "critical,high" {
		t.Errorf("fail_on = %q, want critical,high", got)
	}
	if c.Decision.Provider != "deterministic" {
		t.Errorf("decision.provider = %q, want deterministic", c.Decision.Provider)
	}
	if c.Decision.Thresholds.RouteToStrongModel == 0 || c.Decision.Thresholds.RequireHuman == 0 {
		t.Errorf("decision thresholds not defaulted: %+v", c.Decision.Thresholds)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantMsg string
	}{
		{"empty file", "", "file is empty"},
		{"whitespace only", "   \n", "file is empty"},
		{"malformed", "project: [unterminated\n", "parse"},
		{"unknown key", "project:\n  name: a\nsecrets:\n  token: hunter2\n", "secrets"},
		{"missing project name", "version: 1\n", "project.name is required"},
		{"unknown version", "version: 99\nproject:\n  name: a\n", "unsupported version 99"},
		{"unknown provider", "project:\n  name: a\nagent:\n  provider: skynet\n", "unknown agent.provider"},
		{"unknown engine", "project:\n  name: a\nreview:\n  engine: clippy\n", "unknown review.engine"},
		{"unknown severity", "project:\n  name: a\nquality:\n  fail_on:\n    - blocker\n", "unknown quality.fail_on"},
		{"unknown decision provider", "project:\n  name: a\ndecision:\n  provider: skynet\n", "unknown decision.provider"},
		{"decision threshold out of range", "project:\n  name: a\ndecision:\n  thresholds:\n    require_human: 2\n", "within [0,1]"},
		{"negative cycles", "project:\n  name: a\nquality:\n  max_fix_cycles: -1\n", "must not be negative"},
		{"multiple documents", "project:\n  name: a\n---\nversion: 1\n", "multiple YAML documents"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantMsg)
			}
		})
	}
}

func TestTemplateParses(t *testing.T) {
	c, err := Parse([]byte(Template("book-rag")))
	if err != nil {
		t.Fatalf("generated template does not parse: %v", err)
	}
	if c.Project.Name != "book-rag" {
		t.Errorf("project name = %q, want book-rag", c.Project.Name)
	}
	if c.Agent.Provider != "command" {
		t.Errorf("provider = %q, want command", c.Agent.Provider)
	}
}

func TestTemplateDefaultsName(t *testing.T) {
	c, err := Parse([]byte(Template("  ")))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Project.Name != "project" {
		t.Errorf("name = %q, want project", c.Project.Name)
	}
}

func TestLoadAndNotFound(t *testing.T) {
	dir := t.TempDir()

	if _, err := LoadDir(dir); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadDir on empty dir = %v, want ErrNotFound", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), []byte("project:\n  name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if c.Project.Name != "x" {
		t.Errorf("name = %q, want x", c.Project.Name)
	}
}

func TestDefaultIsValid(t *testing.T) {
	c := Default()
	if c.Project.Name != "" {
		t.Fatal("test assumes Default has no project name")
	}
	c.Project.Name = "x"
	if err := c.Validate(); err != nil {
		t.Errorf("Default (named) should validate: %v", err)
	}
}
