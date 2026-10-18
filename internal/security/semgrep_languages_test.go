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

func rustWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]\nname = \"fixture\"\nversion = \"0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func semgrepAdapterWithRules(t *testing.T, rules, report string, called *bool) Adapter {
	t.Helper()
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDSemgrep, Category: config.SecurityCategorySAST,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{"semgrep"},
		Rules: rules, Version: "1.99.0",
	}, BuildOptions{
		Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			if called != nil {
				*called = true
			}
			return []byte(report), nil, 0, nil
		}},
		Timeout: time.Second, MaxOutputBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestSemgrepRustOnlyRejectsGoRules(t *testing.T) {
	root := rustWorkspace(t)
	rules, err := filepath.Abs(filepath.Join("testdata", "real", "rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	adapter := semgrepAdapterWithRules(t, rules, `{"version":"1.99.0","results":[],"errors":[]}`, &called)
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("semgrep ran Go-only rules against Rust source")
	}
	if exec.Status != StatusFailed || exec.Coverage != CoverageUnknown {
		t.Fatalf("status/coverage = %s/%s, want FAILED/UNKNOWN", exec.Status, exec.Coverage)
	}
	diagnostics := strings.Join(exec.Diagnostics, " ")
	if !strings.Contains(diagnostics, "rust") || !strings.Contains(diagnostics, "UNSUPPORTED") {
		t.Errorf("diagnostics = %v, want explicit unsupported Rust analysis", exec.Diagnostics)
	}
}

func TestSemgrepRustRulesCoverRust(t *testing.T) {
	root := rustWorkspace(t)
	rules, err := filepath.Abs(filepath.Join("testdata", "rules-rust.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	adapter := semgrepAdapterWithRules(t, rules, `{"version":"1.99.0","results":[],"errors":[]}`, &called)
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("semgrep did not run with Rust rules")
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want COMPLETED/COMPLETE (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
}

func TestSemgrepMixedCoverageIsPartial(t *testing.T) {
	root := rustWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/mixed\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := filepath.Abs(filepath.Join("testdata", "real", "rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	adapter := semgrepAdapterWithRules(t, rules, `{"version":"1.99.0","results":[],"errors":[]}`, &called)
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("semgrep did not run the covered Go portion")
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoveragePartial {
		t.Fatalf("status/coverage = %s/%s, want COMPLETED/PARTIAL (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "rust") {
		t.Errorf("diagnostics = %v, want unsupported Rust note", exec.Diagnostics)
	}
}
