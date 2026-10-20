package projectdetect

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsCommand(cmds []string, want string) bool {
	for _, c := range cmds {
		if c == want {
			return true
		}
	}
	return false
}

func assertNoGo(t *testing.T, d Detection) {
	t.Helper()
	for _, cmd := range append(append(append([]string{}, d.Validation.Build...), d.Validation.Test...), d.Validation.Lint...) {
		if strings.HasPrefix(cmd, "go ") || strings.HasPrefix(cmd, "gofmt ") {
			t.Fatalf("non-Go detection proposed a Go command: %q", cmd)
		}
	}
}

func TestDetectRustOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[package]\nname = \"app\"\n")
	d := Detect(dir)
	if d.Kind != KindRust {
		t.Fatalf("kind = %q, want %q", d.Kind, KindRust)
	}
	for _, want := range []string{"cargo build --locked", "cargo test --locked", "cargo fmt --check", "cargo clippy --locked --all-targets -- -D warnings"} {
		all := append(append(append([]string{}, d.Validation.Build...), d.Validation.Test...), d.Validation.Lint...)
		if !containsCommand(all, want) {
			t.Errorf("missing Rust check %q in %+v", want, d.Validation)
		}
	}
	assertNoGo(t, d)
}

func TestDetectNextJSOnly(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"build":"next build","typecheck":"tsc --noEmit","e2e":"playwright test"},"dependencies":{"next":"^15.0.0","react":"^19.0.0"}}`)
	write(t, dir, "next.config.ts", "export default {}\n")
	d := Detect(dir)
	if d.Kind != KindNextJS {
		t.Fatalf("kind = %q, want %q", d.Kind, KindNextJS)
	}
	if !containsCommand(d.Validation.Build, "npm run build") {
		t.Errorf("build = %v, want npm run build", d.Validation.Build)
	}
	if !containsCommand(d.Validation.Test, "npm run e2e") {
		t.Errorf("test = %v, want npm run e2e", d.Validation.Test)
	}
	if !containsCommand(d.Validation.Lint, "npm run typecheck") {
		t.Errorf("lint = %v, want npm run typecheck", d.Validation.Lint)
	}
	assertNoGo(t, d)
}

func TestDetectNestedNextJS(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "web/package.json", `{"scripts":{"build":"next build"},"dependencies":{"next":"^15.0.0"}}`)
	write(t, dir, "web/next.config.ts", "export default {}\n")
	d := Detect(dir)
	if d.Kind != KindNextJS {
		t.Fatalf("kind = %q, want %q (manifests %v)", d.Kind, KindNextJS, d.Manifests)
	}
	if !containsCommand(d.Validation.Build, "npm run build") {
		t.Errorf("build = %v, want npm run build", d.Validation.Build)
	}
}

func TestDetectMixedRustAndNextJSWithMakeTargets(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[package]\nname = \"tran\"\n")
	write(t, dir, "web/package.json", `{"scripts":{"build":"next build","typecheck":"tsc --noEmit","e2e":"playwright test"},"dependencies":{"next":"^15.0.0"}}`)
	write(t, dir, "web/next.config.ts", "export default {}\n")
	write(t, dir, "Makefile", ".PHONY: rust-check web-check web-e2e test\nrust-check:\n\tcargo test --locked\nweb-check:\n\tnpm run build\nweb-e2e:\n\tnpm run e2e\ntest:\n\tcargo test --locked\n")

	d := Detect(dir)
	if d.Kind != KindMixed {
		t.Fatalf("kind = %q, want %q", d.Kind, KindMixed)
	}
	all := append(append(append([]string{}, d.Validation.Build...), d.Validation.Test...), d.Validation.Lint...)
	for _, want := range []string{
		"cargo fmt --check",
		"cargo test --locked",
		"cargo clippy --locked --all-targets -- -D warnings",
		"make rust-check",
		"make web-check",
		"make web-e2e",
	} {
		if !containsCommand(all, want) {
			t.Errorf("missing required mixed check %q in %+v", want, d.Validation)
		}
	}
	if len(d.Conflicts) != 0 {
		t.Errorf("unexpected conflicts: %v", d.Conflicts)
	}
	assertNoGo(t, d)
}

func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/app\n\ngo 1.27\n")
	d := Detect(dir)
	if d.Kind != KindGo {
		t.Fatalf("kind = %q, want %q", d.Kind, KindGo)
	}
	if !reflect.DeepEqual(d.Validation, goValidation()) {
		t.Errorf("validation = %+v, want the established Go set %+v", d.Validation, goValidation())
	}
}

func TestDetectUnknownNeverFallsBackToGo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "README.md", "# empty\n")
	d := Detect(dir)
	if d.Kind != KindUnknown {
		t.Fatalf("kind = %q, want %q", d.Kind, KindUnknown)
	}
	if !d.Empty() {
		t.Errorf("unknown project proposed commands: %+v", d.Validation)
	}
	if len(d.Warnings) == 0 {
		t.Error("unknown project must report a warning")
	}
	assertNoGo(t, d)
}

func TestDetectStaleMakeTargetIsAConflict(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "README.md", "# empty\n")
	write(t, dir, "Makefile", "rust-check:\n\tcargo test --locked\n")
	d := Detect(dir)
	if d.Kind != KindUnknown {
		t.Fatalf("kind = %q, want %q", d.Kind, KindUnknown)
	}
	if len(d.Conflicts) == 0 || !strings.Contains(strings.Join(d.Conflicts, " "), "rust-check") {
		t.Errorf("conflicts = %v, want a rust-check conflict", d.Conflicts)
	}
	if containsCommand(d.Validation.Build, "make rust-check") {
		t.Error("a conflicting rust-check target must not be proposed")
	}
}

func TestDetectPackageManagerFromLockfile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"build":"next build"},"dependencies":{"next":"^15.0.0"}}`)
	write(t, dir, "pnpm-lock.yaml", "lockfileVersion: '9.0'\n")
	d := Detect(dir)
	if !containsCommand(d.Validation.Build, "pnpm run build") {
		t.Errorf("build = %v, want pnpm run build", d.Validation.Build)
	}
}

func TestDetectIgnoresDependencyDirectories(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "README.md", "# empty\n")
	write(t, dir, "node_modules/dep/package.json", `{"scripts":{"build":"x"}}`)
	d := Detect(dir)
	if d.Kind != KindUnknown {
		t.Fatalf("kind = %q, want unknown; dependency manifests must be ignored", d.Kind)
	}
}
