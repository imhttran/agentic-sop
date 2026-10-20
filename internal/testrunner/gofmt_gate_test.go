package testrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestGofmtListingGateFailsOnUnformattedFile proves the generated shell-free
// formatting check is a real gate: gofmt exits 0 while listing an unformatted file,
// so the runner must judge it by the listing, not by the exit status.
func TestGofmtListingGateFailsOnUnformattedFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.go")
	if err := os.WriteFile(bad, []byte("package bad\n\nfunc  f( ) {  return  }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := New(dir).Run(context.Background(), Check{Category: Lint, Command: "gofmt -l ."})
	if res.Status != Fail {
		t.Fatalf("status = %s (exit %d, stdout %q), want FAIL for an unformatted file", res.Status, res.ExitCode, res.Stdout)
	}
}

// TestGofmtListingGatePassesOnFormattedTree: a formatted tree lists nothing, so the
// check is met and the gate is not a blanket failure.
func TestGofmtListingGatePassesOnFormattedTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.go"), []byte("package ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := New(dir).Run(context.Background(), Check{Category: Lint, Command: "gofmt -l ."})
	if res.Status != Pass {
		t.Fatalf("status = %s (stdout %q), want PASS for a formatted tree", res.Status, res.Stdout)
	}
}

// TestGofmtListingGateFailsInNestedModule proves the generated nested-module
// check ("cd '<dir>' && gofmt -l .") is still judged by its listing, so an
// unformatted file in a subfolder module fails the gate.
func TestGofmtListingGateFailsInNestedModule(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "svc")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "bad.go"), []byte("package bad\n\nfunc  f( ) {  return  }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := New(dir).Run(context.Background(), Check{Category: Lint, Command: "cd 'svc' && gofmt -l ."})
	if res.Status != Fail {
		t.Fatalf("status = %s (exit %d, stdout %q), want FAIL for an unformatted nested file", res.Status, res.ExitCode, res.Stdout)
	}
}
