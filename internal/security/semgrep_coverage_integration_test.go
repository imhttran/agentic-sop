//go:build security_integration

package security

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// TestRealSemgrepCoverageContract proves the adapter consumes the pinned
// Semgrep's real coverage summary: a no-skip scan is COMPLETE, an in-scope
// .semgrepignore or size skip is PARTIAL with a named class, and a declared
// --exclude stays scope rather than a gap. If the scanner stops emitting this
// summary, these assertions fail instead of silently claiming COMPLETE.
func TestRealSemgrepCoverageContract(t *testing.T) {
	semgrepBin := integrationBinary(t, "SOP_SECURITY_SEMGREP_BIN", "semgrep")
	version := toolVersion(t, semgrepBin, "--version")
	rules := integrationRulesPath(t, "rules.yaml")
	t.Logf("semgrep=%s", version)

	run := func(t *testing.T, root string, excluded []string) Execution {
		t.Helper()
		adapter, err := BuildAdapter(config.SecurityAdapterConfig{
			ID: "semgrep", Category: "sast", Protocol: "builtin",
			Command: []string{semgrepBin}, Rules: rules, Version: version,
		}, BuildOptions{Timeout: 2 * time.Minute, MaxOutputBytes: 8 << 20})
		if err != nil {
			t.Fatal(err)
		}
		req := requestFor(t, root)
		req.Scope.Excluded = excluded
		exec, err := adapter.Scan(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if exec.Status != StatusCompleted {
			t.Fatalf("status = %s, want COMPLETED: %v", exec.Status, exec.Diagnostics)
		}
		return exec
	}
	diagnostics := func(exec Execution) string { return strings.Join(exec.Diagnostics, " ") }

	t.Run("no skip", func(t *testing.T) {
		root := t.TempDir()
		writeCoverageFixture(t, root, "main.go", "package probe\n\nfunc Clean() {}\n")
		exec := run(t, root, nil)
		if exec.Coverage != CoverageComplete {
			t.Fatalf("coverage = %s, want COMPLETE: %v", exec.Coverage, exec.Diagnostics)
		}
		if !strings.Contains(diagnostics(exec), "detected source language") {
			t.Errorf("diagnostics = %v, want language evidence", exec.Diagnostics)
		}
	})

	t.Run("semgrepignore skip", func(t *testing.T) {
		root := t.TempDir()
		writeCoverageFixture(t, root, "main.go", "package probe\n\nfunc Clean() {}\n")
		writeCoverageFixture(t, root, "ignored.go", "package probe\n\nfunc Ignored() {}\n")
		writeCoverageFixture(t, root, ".semgrepignore", "ignored.go\n")
		exec := run(t, root, nil)
		if exec.Coverage != CoveragePartial {
			t.Fatalf("coverage = %s, want PARTIAL for a .semgrepignore skip: %v", exec.Coverage, exec.Diagnostics)
		}
		if !strings.Contains(diagnostics(exec), ".semgrepignore") {
			t.Errorf("diagnostics = %v, want the .semgrepignore skip class", exec.Diagnostics)
		}
	})

	t.Run("oversized source file", func(t *testing.T) {
		root := t.TempDir()
		writeCoverageFixture(t, root, "main.go", "package probe\n\nfunc Clean() {}\n")
		var builder strings.Builder
		builder.WriteString("package probe\n")
		for builder.Len() < 1400*1024 {
			builder.WriteString("// padding line for the scanner size limit\n")
		}
		writeCoverageFixture(t, root, "big.go", builder.String())
		exec := run(t, root, nil)
		if exec.Coverage != CoveragePartial {
			t.Fatalf("coverage = %s, want PARTIAL for an oversized target: %v", exec.Coverage, exec.Diagnostics)
		}
		if !strings.Contains(diagnostics(exec), "Files larger than") {
			t.Errorf("diagnostics = %v, want the size-limit skip class", exec.Diagnostics)
		}
	})

	t.Run("declared exclusion stays scope", func(t *testing.T) {
		root := t.TempDir()
		writeCoverageFixture(t, root, "main.go", "package probe\n\nfunc Clean() {}\n")
		writeCoverageFixture(t, root, "excluded.go", "package probe\n\nfunc Excluded() {}\n")
		exec := run(t, root, []string{"excluded.go"})
		if exec.Coverage != CoverageComplete {
			t.Fatalf("coverage = %s, want COMPLETE for a declared exclusion: %v", exec.Coverage, exec.Diagnostics)
		}
		if !strings.Contains(diagnostics(exec), "declared out-of-scope") {
			t.Errorf("diagnostics = %v, want declared-exclusion accounting", exec.Diagnostics)
		}
		if len(exec.Provenance.Exclusions) != 1 || exec.Provenance.Exclusions[0] != "excluded.go" {
			t.Errorf("provenance exclusions = %v, want the configured pattern", exec.Provenance.Exclusions)
		}
	})
}

func writeCoverageFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
