package cli

import (
	"os"
	"regexp"
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

// TestMemoryAddRefusesUnreadableStore proves memory add never overwrites a decisions
// file it cannot round-trip (GAP-6): it fails and the file bytes are unchanged.
func TestMemoryAddRefusesUnreadableStore(t *testing.T) {
	for name, content := range map[string]string{
		"unknown schema": `{"schema":"unknown-future","records":[{"decision":"keep me"}]}`,
		"malformed json": `{"schema":"decisionmemory/1","records":[`,
		"invalid record": `{"schema":"decisionmemory/1","limit":4,"records":[{"decision":"keep me"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			initProject(t, dir)
			writeFileT(t, dir, ".agent-sdlc/context/decisions.json", content)
			path := decisionStorePath(dir)

			code, _, errOut := runCLI(t, dir, "memory", "add", "--decision", "new", "--reason", "r")
			if code != exitError || !strings.Contains(errOut, "refusing to overwrite") {
				t.Errorf("add: code=%d stderr=%s, want exitError refusing to overwrite", code, errOut)
			}
			if got, _ := os.ReadFile(path); string(got) != content {
				t.Errorf("file changed:\n%s", got)
			}
			// Reading for context still applies nothing and never rewrites the file.
			if code, out, _ := runCLI(t, dir, "memory", "list"); code != exitOK || !strings.Contains(out, "0 applicable decision") {
				t.Errorf("list: code=%d stdout=%s", code, out)
			}
			if got, _ := os.ReadFile(path); string(got) != content {
				t.Errorf("list changed the file:\n%s", got)
			}
		})
	}
}

// TestMemoryRemoveByID proves memory remove retires exactly the one decision whose ID
// memory list prints, and an unknown ID fails with the file unchanged (GAP-7).
func TestMemoryRemoveByID(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	for _, decision := range []string{"keep", "drop"} {
		if code, out, errOut := runCLI(t, dir, "memory", "add", "--decision", decision, "--reason", "r"); code != exitOK {
			t.Fatalf("add %s: code=%d stdout=%s stderr=%s", decision, code, out, errOut)
		}
	}
	_, out, _ := runCLI(t, dir, "memory", "list")
	m := regexp.MustCompile(`repository  drop\n(?:    .*\n)*?    id: ([0-9a-f]{64})\n`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no id listed for the drop decision: %s", out)
	}
	dropID := m[1]

	path := decisionStorePath(dir)
	before, _ := os.ReadFile(path)
	if code, _, errOut := runCLI(t, dir, "memory", "remove", "--id", strings.Repeat("0", 64)); code != exitError || !strings.Contains(errOut, "no decision with id") {
		t.Errorf("unknown id: code=%d stderr=%s, want exitError", code, errOut)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("an unknown id changed the file")
	}

	if code, out, errOut := runCLI(t, dir, "memory", "remove", "--id", dropID); code != exitOK {
		t.Fatalf("remove: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	_, out, _ = runCLI(t, dir, "memory", "list")
	if strings.Contains(out, "repository  drop") || !strings.Contains(out, "repository  keep") || !strings.Contains(out, "1 applicable decision") {
		t.Errorf("after remove, list = %q", out)
	}
}
