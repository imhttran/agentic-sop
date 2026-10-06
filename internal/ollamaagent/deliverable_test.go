package ollamaagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeliverableRequirementInUserPrompt(t *testing.T) {
	req := implementRequest()
	req.Deliverables = []string{"docs/reports/pre-performance-closure/CLOSE-003-sop-deterministic-baseline.md"}
	p := userPrompt(req)
	for _, want := range []string{"Required deliverable", req.Deliverables[0]} {
		if !strings.Contains(p, want) {
			t.Errorf("userPrompt missing %q:\n%s", want, p)
		}
	}
	if strings.Contains(userPrompt(implementRequest()), "Required deliverable") {
		t.Error("a task with no deliverable must not carry the standing requirement")
	}
}

func TestDeliverableMissingFailsClosed(t *testing.T) {
	dir := t.TempDir()
	h := New(testConfig("http://127.0.0.1:0"), dir)
	if !h.deliverableMissing([]string{"docs/reports/t/out.md"}) {
		t.Error("an absent deliverable must report missing")
	}
	if h.deliverableMissing(nil) {
		t.Error("no declared deliverable is never missing")
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs/reports/t"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "docs/reports/t/out.md", "# report\n")
	if h.deliverableMissing([]string{"docs/reports/t/out.md"}) {
		t.Error("a present deliverable must report present")
	}
}

// TestDeliverableWithholdsCommandTools proves a missing declared deliverable
// withholds command/git tools past the implement-now threshold and steers the run
// to write the deliverable, while file tools stay available and the written
// deliverable becomes an ordinary verified mutation.
func TestDeliverableWithholdsCommandTools(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", implementNowAfter)
	if err := os.MkdirAll(filepath.Join(dir, "docs/reports/t"), 0o755); err != nil {
		t.Fatal(err)
	}
	const deliverable = "docs/reports/t/out.md"

	responses := make([]string, 0, implementNowAfter+6)
	for i := 0; i < implementNowAfter; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses,
		`{"tool":"run_command","args":{"command":"go list ./..."}}`, // withheld: deliverable missing
		`{"tool":"write_file","args":{"path":"docs/reports/t/out.md","content":"# report\n"}}`,
		`{"tool":"run_command","args":{"command":"go list ./..."}}`, // allowed once it exists
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 50
	h := New(cfg, dir)
	req := implementRequest()
	req.Deliverables = []string{deliverable}

	content, err := h.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want completion", content)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(deliverable))); err != nil {
		t.Errorf("deliverable not written: %v", err)
	}
	denies := 0
	for _, r := range h.AuditRecords() {
		if r.Action == "deny" && r.Tool == "run_command" {
			denies++
		}
	}
	if denies != 1 {
		t.Errorf("run_command denials = %d, want exactly the one before the deliverable existed", denies)
	}
	if fake.count() != implementNowAfter+4 {
		t.Errorf("model turns = %d, want %d", fake.count(), implementNowAfter+4)
	}
}

// TestNoDeliverableKeepsCommandTools proves the withholding is scoped: without a
// declared deliverable, command tools stay available past the implement-now
// threshold (unchanged behavior).
func TestNoDeliverableKeepsCommandTools(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", implementNowAfter)
	responses := make([]string, 0, implementNowAfter+3)
	for i := 0; i < implementNowAfter; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses,
		`{"tool":"run_command","args":{"command":"go list ./..."}}`,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 50
	h := New(cfg, dir)

	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	for _, r := range h.AuditRecords() {
		if r.Action == "deny" {
			t.Errorf("unexpected denial without a declared deliverable: %+v", r)
		}
	}
	if fake.count() != implementNowAfter+3 {
		t.Errorf("model turns = %d, want %d", fake.count(), implementNowAfter+3)
	}
}
