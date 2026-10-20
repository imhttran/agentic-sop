package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// TestInitDetectsRustValidation proves project detection reaches the generated
// configuration, so a Rust project never receives Go commands.
func TestInitDetectsRustValidation(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, dir, "Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n")
	code, stdout, stderr := runCLI(t, dir, "init")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "wrote .agent-sdlc/config.yaml") {
		t.Errorf("stdout = %q", stdout)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := append(append(append([]string{}, cfg.Validation.Build...), cfg.Validation.Test...), cfg.Validation.Lint...)
	joined := strings.Join(all, "\n")
	for _, want := range []string{"cargo fmt --check", "cargo test --locked", "cargo clippy --locked --all-targets -- -D warnings"} {
		if !strings.Contains(joined, want) {
			t.Errorf("generated validation missing %q: %v", want, all)
		}
	}
	if strings.Contains(joined, "go build ./...") || strings.Contains(joined, "go test ./...") {
		t.Errorf("Rust project received Go validation: %v", all)
	}
}

// TestInitDetectsMixedProjectValidation proves the Tran shape (Rust plus a
// Next.js app under web/ plus explicit Makefile targets) produces the required
// integration checks.
func TestInitDetectsMixedProjectValidation(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, dir, "Cargo.toml", "[package]\nname = \"tran\"\nversion = \"0.1.0\"\n")
	writeFileT(t, dir, "web/package.json", `{"scripts":{"build":"next build","typecheck":"tsc --noEmit","e2e":"playwright test"},"dependencies":{"next":"^15.3.0"}}`)
	writeFileT(t, dir, "web/next.config.ts", "export default {}\n")
	writeFileT(t, dir, "Makefile", "rust-check:\n\tcargo test --locked\nweb-check:\n\tnpm run build\nweb-e2e:\n\tnpm run e2e\n")

	code, _, stderr := runCLI(t, dir, "init")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := append(append(append([]string{}, cfg.Validation.Build...), cfg.Validation.Test...), cfg.Validation.Lint...)
	joined := strings.Join(all, "\n")
	for _, want := range []string{
		"cargo fmt --check",
		"cargo test --locked",
		"cargo clippy --locked --all-targets -- -D warnings",
		"make rust-check",
		"make web-check",
		"make web-e2e",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("generated validation missing %q:\n%v", want, all)
		}
	}
	if strings.Contains(joined, "go build ./...") || strings.Contains(joined, "go test ./...") {
		t.Errorf("mixed project received Go validation: %v", all)
	}
}

// TestInitUnknownProjectDoesNotFallBackToGo proves an unrecognized repository
// gets an empty validation policy and a reported warning.
func TestInitUnknownProjectDoesNotFallBackToGo(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, dir, "README.md", "# notes\n")
	code, _, stderr := runCLI(t, dir, "init")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Validation.Build)+len(cfg.Validation.Test)+len(cfg.Validation.Lint) != 0 {
		t.Errorf("unknown project validation = %+v, want empty", cfg.Validation)
	}
	if !strings.Contains(stderr, "validation is not configured") {
		t.Errorf("stderr = %q, want a detection warning", stderr)
	}
}

// TestInitPreservesExplicitValidation proves an explicit operator configuration
// is never replaced by detected defaults.
func TestInitPreservesExplicitValidation(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, dir, "Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n")
	writeFileT(t, dir, ".agent-sdlc/config.yaml", "version: 1\nproject:\n  name: custom\nvalidation:\n  test:\n    - custom-check\n")
	code, _, stderr := runCLI(t, dir, "init")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project.Name != "custom" || len(cfg.Validation.Test) != 1 || cfg.Validation.Test[0] != "custom-check" {
		t.Errorf("explicit configuration was replaced: project=%q validation=%+v", cfg.Project.Name, cfg.Validation)
	}
}

// TestInitRefreshValidationUpdatesOnlyDetection proves the explicit refresh
// operation replaces the generated command lists with detected ones while
// preserving every other policy key.
func TestInitRefreshValidationUpdatesOnlyDetection(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, dir, "Cargo.toml", "[package]\nname = \"app\"\nversion = \"0.1.0\"\n")
	writeFileT(t, dir, "web/package.json", `{"scripts":{"build":"next build","e2e":"playwright test"},"dependencies":{"next":"^15.3.0"}}`)
	writeFileT(t, dir, "Makefile", "rust-check:\n\tcargo test --locked\nweb-check:\n\tnpm run build\nweb-e2e:\n\tnpm run e2e\n")
	writeFileT(t, dir, ".agent-sdlc/config.yaml", "version: 1\nproject:\n  name: tran\n  integration_branch: trunk\nvalidation:\n  build:\n    - go build ./...\n  test:\n    - go test ./...\n  lint:\n    - go vet ./...\nquality:\n  max_fix_cycles: 7\n")

	code, stdout, stderr := runCLI(t, dir, "init", "--refresh-validation")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "updated .agent-sdlc/config.yaml validation for mixed") {
		t.Errorf("stdout = %q, want the detected project announcement", stdout)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project.Name != "tran" || cfg.Project.IntegrationBranch != "trunk" || cfg.Quality.MaxFixCycles != 7 {
		t.Errorf("non-validation policy changed: project=%+v quality=%+v", cfg.Project, cfg.Quality)
	}
	all := strings.Join(append(append(append([]string{}, cfg.Validation.Build...), cfg.Validation.Test...), cfg.Validation.Lint...), "\n")
	if strings.Contains(all, "go build ./...") || strings.Contains(all, "go test ./...") || strings.Contains(all, "go vet ./...") {
		t.Errorf("refresh kept Go commands: %v", all)
	}
	for _, want := range []string{"cargo test --locked", "make rust-check", "make web-check", "make web-e2e"} {
		if !strings.Contains(all, want) {
			t.Errorf("refresh missing %q: %v", want, all)
		}
	}
}

// TestInitRefreshValidationRefusesUnknownProject proves a refresh that detects
// nothing leaves the existing configuration byte for byte unchanged.
func TestInitRefreshValidationRefusesUnknownProject(t *testing.T) {
	dir := t.TempDir()
	const original = "version: 1\nproject:\n  name: py\nvalidation:\n  test:\n    - pytest\n"
	writeFileT(t, dir, ".agent-sdlc/config.yaml", original)

	code, _, stderr := runCLI(t, dir, "init", "--refresh-validation")
	if code == exitOK {
		t.Fatalf("refresh of an unknown project succeeded; stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "was not changed") {
		t.Errorf("stderr = %q, want the refusal", stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".agent-sdlc", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("configuration changed:\n%s", got)
	}
}
