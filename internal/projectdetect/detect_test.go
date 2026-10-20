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
		if strings.Contains(cmd, "go build ./...") || strings.Contains(cmd, "go test ./...") ||
			strings.Contains(cmd, "go vet ./...") || strings.Contains(cmd, "gofmt -l .") {
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
	if !containsCommand(d.Validation.Build, "cd 'web' && npm run build") {
		t.Errorf("build = %v, want the nested web build", d.Validation.Build)
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
	if !reflect.DeepEqual(d.Validation, goValidation(".")) {
		t.Errorf("validation = %+v, want the established Go set %+v", d.Validation, goValidation("."))
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

func TestDetectNestedRustWithoutMakefile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "crates/app/Cargo.toml", "[package]\nname = \"app\"\n")
	d := Detect(dir)
	if d.Kind != KindRust {
		t.Fatalf("kind = %q, want rust", d.Kind)
	}
	for _, want := range []string{
		"cd 'crates/app' && cargo build --locked",
		"cd 'crates/app' && cargo test --locked",
		"cd 'crates/app' && cargo fmt --check",
		"cd 'crates/app' && cargo clippy --locked --all-targets -- -D warnings",
	} {
		all := append(append(append([]string{}, d.Validation.Build...), d.Validation.Test...), d.Validation.Lint...)
		if !containsCommand(all, want) {
			t.Errorf("missing nested Rust command %q in %+v", want, d.Validation)
		}
	}
	assertNoGo(t, d)
}

func TestDetectNestedGoWithoutMakefile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "services/api/go.mod", "module example.com/api\n\ngo 1.27\n")
	d := Detect(dir)
	if d.Kind != KindGo {
		t.Fatalf("kind = %q, want go", d.Kind)
	}
	for _, want := range []string{
		"cd 'services/api' && go build ./...",
		"cd 'services/api' && go test ./...",
		"cd 'services/api' && go vet ./...",
		"cd 'services/api' && gofmt -l .",
	} {
		all := append(append(append([]string{}, d.Validation.Build...), d.Validation.Test...), d.Validation.Lint...)
		if !containsCommand(all, want) {
			t.Errorf("missing nested Go command %q in %+v", want, d.Validation)
		}
	}
}

func TestDetectNestedNextJSWithoutMakefile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "apps/web/package.json", `{"scripts":{"build":"next build","e2e":"playwright test","typecheck":"tsc --noEmit"},"dependencies":{"next":"^15.0.0"}}`)
	write(t, dir, "apps/web/next.config.ts", "export default {}\n")
	d := Detect(dir)
	if d.Kind != KindNextJS {
		t.Fatalf("kind = %q, want nextjs", d.Kind)
	}
	if !containsCommand(d.Validation.Build, "cd 'apps/web' && npm run build") {
		t.Errorf("build = %v, want the nested package build", d.Validation.Build)
	}
	if !containsCommand(d.Validation.Test, "cd 'apps/web' && npm run e2e") {
		t.Errorf("test = %v, want the nested package e2e", d.Validation.Test)
	}
	if !containsCommand(d.Validation.Lint, "cd 'apps/web' && npm run typecheck") {
		t.Errorf("lint = %v, want the nested package typecheck", d.Validation.Lint)
	}
	assertNoGo(t, d)
}

func TestDetectRootManifestWinsOverNested(t *testing.T) {
	for _, tc := range []struct {
		name      string
		root      string
		nested    string
		wantBuild string
	}{
		{"go", "go.mod", "services/api/go.mod", "go build ./..."},
		{"rust", "Cargo.toml", "crates/app/Cargo.toml", "cargo build --locked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.name == "go" {
				write(t, dir, tc.root, "module example.com/root\n\ngo 1.27\n")
				write(t, dir, tc.nested, "module example.com/nested\n\ngo 1.27\n")
			} else {
				write(t, dir, tc.root, "[package]\nname = \"root\"\n")
				write(t, dir, tc.nested, "[package]\nname = \"nested\"\n")
			}
			d := Detect(dir)
			if !containsCommand(d.Validation.Build, tc.wantBuild) {
				t.Errorf("build = %v, want the root command %q", d.Validation.Build, tc.wantBuild)
			}
		})
	}
}

func TestDetectSelectsNextPackageOverUnrelatedRootPackage(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"build":"tools build"},"devDependencies":{"typescript":"^5.0.0"}}`)
	write(t, dir, "web/package.json", `{"scripts":{"build":"next build","e2e":"playwright test"},"dependencies":{"next":"^15.0.0"}}`)
	write(t, dir, "web/next.config.ts", "export default {}\n")
	d := Detect(dir)
	if d.Kind != KindNextJS {
		t.Fatalf("kind = %q, want nextjs", d.Kind)
	}
	if !containsCommand(d.Validation.Build, "cd 'web' && npm run build") {
		t.Errorf("build = %v, want the nested Next.js package, not the root tooling package", d.Validation.Build)
	}
	if containsCommand(d.Validation.Build, "npm run build") {
		t.Errorf("selected the unrelated root package: %v", d.Validation.Build)
	}
}

// TestDetectIgnoresAuxiliaryManifests proves side modules in test, example, and
// tooling folders do not make a Rust repository mixed or add their checks.
func TestDetectIgnoresAuxiliaryManifests(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[package]\nname = \"app\"\n")
	write(t, dir, "tools/gen/go.mod", "module example.com/gen\n")
	write(t, dir, "testdata/fixture/package.json", `{"scripts":{"build":"x"}}`)
	write(t, dir, "examples/demo/Cargo.toml", "[package]\nname = \"demo\"\n")
	write(t, dir, "tests/e2e/package.json", `{"scripts":{"test":"x"}}`)
	d := Detect(dir)
	if d.Kind != KindRust {
		t.Fatalf("kind = %q, want rust; auxiliary manifests must be ignored (%+v)", d.Kind, d.Manifests)
	}
	all := append(append(append([]string{}, d.Validation.Build...), d.Validation.Test...), d.Validation.Lint...)
	for _, cmd := range all {
		if strings.HasPrefix(cmd, "cd ") {
			t.Errorf("auxiliary folder command proposed: %q", cmd)
		}
	}
	assertNoGo(t, d)
}
