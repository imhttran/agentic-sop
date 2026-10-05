package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectCapturedEvidenceAddsRawOutput(t *testing.T) {
	dir := t.TempDir()
	runDir := t.TempDir()
	report := "docs/reports/x.md"
	writeFileT(t, dir, report, "# Report\n\nModel prose here.\n")
	writeFileT(t, runDir, "command-evidence.jsonl",
		`{"command":"go test ./...","cwd":"/tmp/x","exit":0,"output":"ok\tpkg\t0.1s\n"}`+"\n")

	if err := injectCapturedEvidence(dir, []string{report}, runDir); err != nil {
		t.Fatal(err)
	}
	got := readT(t, filepath.Join(dir, filepath.FromSlash(report)))
	if !strings.Contains(got, capturedEvidenceMarker) || !strings.Contains(got, "ok\tpkg") {
		t.Fatalf("captured evidence not injected:\n%s", got)
	}
	// Idempotent: a second injection replaces the section, never duplicates it.
	if err := injectCapturedEvidence(dir, []string{report}, runDir); err != nil {
		t.Fatal(err)
	}
	again := readT(t, filepath.Join(dir, filepath.FromSlash(report)))
	if strings.Count(again, capturedEvidenceMarker) != 1 {
		t.Fatalf("marker count = %d, want 1:\n%s", strings.Count(again, capturedEvidenceMarker), again)
	}
}

func TestInjectCapturedEvidenceNoEvidenceIsNoOp(t *testing.T) {
	dir := t.TempDir()
	runDir := t.TempDir()
	report := "docs/reports/x.md"
	writeFileT(t, dir, report, "# Report\n")
	if err := injectCapturedEvidence(dir, []string{report}, runDir); err != nil {
		t.Fatal(err)
	}
	if got := readT(t, filepath.Join(dir, filepath.FromSlash(report))); strings.Contains(got, capturedEvidenceMarker) {
		t.Fatalf("no evidence must not inject a section: %s", got)
	}
}

func writeFileT(t *testing.T, base, rel, content string) {
	t.Helper()
	path := filepath.Join(base, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
