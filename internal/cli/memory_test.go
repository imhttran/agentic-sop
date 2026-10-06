package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/decisionmemory"
)

// TestMemoryAddAndList proves a decision recorded for the current repository identity is
// listed as applicable.
func TestMemoryAddAndList(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	code, out, errOut := runCLI(t, dir, "memory", "add",
		"--decision", "Postgres is canonical storage",
		"--reason", "the controller delegates lifecycle authority",
		"--scope", "architecture", "--source", "ADR-001")
	if code != exitOK {
		t.Fatalf("add: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	code, out, errOut = runCLI(t, dir, "memory", "list")
	if code != exitOK {
		t.Fatalf("list: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "Postgres is canonical storage") || !strings.Contains(out, "1 applicable decision") {
		t.Errorf("list = %q", out)
	}
}

// TestMemoryRejectsMissingProvenance proves a decision without a reason is rejected.
func TestMemoryRejectsMissingProvenance(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	if code, _, _ := runCLI(t, dir, "memory", "add", "--decision", "x"); code != exitUsage {
		t.Errorf("a decision without --reason must be rejected with a usage error")
	}
}

// TestMemoryStaleReportedNotApplied proves a decision made under another repository state
// is never applied: current evidence outranks memory.
func TestMemoryStaleReportedNotApplied(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	store := decisionmemory.New(0)
	if _, err := store.Add(decisionmemory.Record{
		Decision:   "an old fact",
		Scope:      decisionmemory.ScopeArchitecture,
		Reason:     "recorded under a different state",
		Repository: "other-repository-state",
		Source:     "s",
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveDecisionStore(dir, store); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runCLI(t, dir, "memory", "list")
	if code != exitOK {
		t.Fatalf("list: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if strings.Contains(out, "an old fact") {
		t.Errorf("a stale decision must not be listed as applicable: %s", out)
	}
	if !strings.Contains(out, "stale decision") {
		t.Errorf("a stale decision must be reported: %s", out)
	}
}
