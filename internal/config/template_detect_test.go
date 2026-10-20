package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateForDirRustProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"app\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, detection := TemplateForDir("app", dir)
	if detection.Kind != "rust" {
		t.Fatalf("kind = %q, want rust", detection.Kind)
	}
	for _, want := range []string{"cargo fmt --check", "cargo test --locked", "cargo clippy --locked --all-targets -- -D warnings"} {
		if !strings.Contains(text, want) {
			t.Errorf("template does not contain %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "go build ./...") {
		t.Error("a Rust project template must not contain Go commands")
	}
	if _, err := Parse([]byte(text)); err != nil {
		t.Fatalf("generated template does not parse: %v", err)
	}
}

func TestTemplateForDirUnknownHasNoCommandsOrGoFallback(t *testing.T) {
	dir := t.TempDir()
	text, detection := TemplateForDir("empty", dir)
	if detection.Kind != "unknown" {
		t.Fatalf("kind = %q, want unknown", detection.Kind)
	}
	if strings.Contains(text, "go build ./...") || strings.Contains(text, "go test ./...") {
		t.Error("an unknown project template must not contain Go commands")
	}
	if !strings.Contains(text, "intentionally") {
		t.Error("an unknown project template must state that validation is intentionally empty")
	}
	cfg, err := Parse([]byte(text))
	if err != nil {
		t.Fatalf("generated template does not parse: %v", err)
	}
	if len(cfg.Validation.Build) != 0 || len(cfg.Validation.Test) != 0 || len(cfg.Validation.Lint) != 0 {
		t.Errorf("unknown project validation = %+v, want empty", cfg.Validation)
	}
}

func TestUpdateValidationPreservesOtherPolicyAndComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := `# operator header
version: 1
project:
  name: tran
  # keep this branch comment
  integration_branch: trunk
validation:
  build:
    - go build ./...
  test:
    - go test ./...
  lint:
    - go vet ./...
  integration:
    - make db-integration
  acceptance_checks:
    cargo: true
quality:
  max_fix_cycles: 7
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	want := Validation{
		Build: []string{"cargo build --locked", "make rust-check"},
		Test:  []string{"cargo test --locked"},
		Lint:  []string{"cargo fmt --check", "cargo clippy --locked --all-targets -- -D warnings"},
	}
	if err := UpdateValidation(path, want); err != nil {
		t.Fatalf("UpdateValidation: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(updated)
	if err != nil {
		t.Fatalf("updated configuration does not parse: %v", err)
	}
	if cfg.Project.Name != "tran" || cfg.Project.IntegrationBranch != "trunk" {
		t.Errorf("project policy changed: %+v", cfg.Project)
	}
	if cfg.Quality.MaxFixCycles != 7 {
		t.Errorf("max_fix_cycles = %d, want 7", cfg.Quality.MaxFixCycles)
	}
	if len(cfg.Validation.Integration) != 1 || cfg.Validation.Integration[0] != "make db-integration" {
		t.Errorf("integration commands changed: %v", cfg.Validation.Integration)
	}
	if cfg.Cargo.AcceptanceCheck == nil || !*cfg.Cargo.AcceptanceCheck {
		t.Errorf("acceptance_checks.cargo was not preserved: %+v", cfg.Cargo)
	}
	if len(cfg.Validation.Build) != 2 || cfg.Validation.Build[0] != "cargo build --locked" {
		t.Errorf("build = %v", cfg.Validation.Build)
	}
	if len(cfg.Validation.Test) != 1 || cfg.Validation.Test[0] != "cargo test --locked" {
		t.Errorf("test = %v", cfg.Validation.Test)
	}
	if len(cfg.Validation.Lint) != 2 {
		t.Errorf("lint = %v", cfg.Validation.Lint)
	}

	for _, comment := range []string{"# operator header", "# keep this branch comment"} {
		if !strings.Contains(string(updated), comment) {
			t.Errorf("comment %q was not preserved:\n%s", comment, updated)
		}
	}
}

func TestUpdateValidationRefusesInvalidConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nunknown_key: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateValidation(path, Validation{Test: []string{"true"}}); err == nil {
		t.Fatal("expected an invalid configuration to be refused")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "version: 1\nunknown_key: true\n" {
		t.Errorf("invalid configuration was modified:\n%s", got)
	}
}
