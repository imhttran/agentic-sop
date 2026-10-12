package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/store"
)

// SOP-LC-001 integration fixture: a plan whose 12 tasks are already implemented and
// committed must close in ONE `sop run` invocation, reusing validation evidence,
// invoking no agent, and invoking no JEV (there is no change to review).

// lc001Config enables JEV and counts validation runs by appending to a file OUTSIDE
// the project tree, so the working tree stays clean and validation reuse still holds.
func lc001Config(countPath string) string {
	return "project:\n  name: x\n" +
		"validation:\n  build:\n    - \"true\"\n  test:\n    - \"printf x >> " + countPath + "\"\n  lint:\n    - \"true\"\n" +
		"quality:\n  jev:\n    enabled: true\n    mode: review\n"
}

// lc001Plan renders an n-stage plan in which every stage is verify-first, so an
// already-satisfied tree completes each task with no agent.
func lc001Plan(n int) string {
	var b strings.Builder
	b.WriteString("# PLAN — SOP-LC-001 fixture\n\n## Project\n\nlc-001\n\n## Summary\n\nAlready-implemented verify-first tasks.\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "\n## LC-%03d — Task %d\n\nWork already exists in the tree.\n\n### Execution\n\n- verify-first\n\n### Acceptance Criteria\n\n- the configured validation passes\n", i, i)
	}
	return b.String()
}

func validationCount(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "x")
}

func openStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, stateDirName, stateFileName))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}

// TestSOPLC001OneRunClosesPlanOfCommittedWork is the end-to-end proof: one `sop run`
// completes and archives a 12-task plan of already-committed work, with no agent
// call, no JEV invocation (empty diff), and a single validation run reused across the
// twelve tasks.
func TestSOPLC001OneRunClosesPlanOfCommittedWork(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	count := filepath.Join(t.TempDir(), "count.txt")
	writeConfig(t, dir, lc001Config(count))
	writeFile(t, dir, "PLAN.md", lc001Plan(12))
	t.Setenv("SOP_HISTORICALIZE_ON_COMPLETION", "true")

	agent := &countingAgent{}
	// A fail-closed analyzer: if JEV were invoked on the empty diff it would return
	// ERROR and the gate would stop at NEEDS_HUMAN, so this plan could not close.
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusError}}}
	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "", agent,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil }, "run", "PLAN.md")
	if code != exitOK {
		t.Fatalf("run code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if agent.calls != 0 {
		t.Errorf("implementation agent calls = %d, want 0 for verify-first committed work", agent.calls)
	}
	if analyzer.calls != 0 {
		t.Errorf("JEV invocations = %d, want 0 (no reviewable diff)", analyzer.calls)
	}
	if n := validationCount(t, count); n != 1 {
		t.Errorf("validation runs = %d, want 1 (reused across 12 tasks)", n)
	}

	// The plan is archived COMPLETE with all 12 task records satisfied.
	if got := archivedDisposition(t, dir); got != "COMPLETE" {
		t.Errorf("archived disposition = %q, want COMPLETE", got)
	}
	tasks := archivedTasks(t, dir)
	if len(tasks) != 12 {
		t.Fatalf("archived tasks = %d, want 12", len(tasks))
	}
	for _, tk := range tasks {
		if !tk.IsSatisfied() {
			t.Errorf("archived task %s status = %s, want satisfied", tk.ID, tk.Status)
		}
	}
	// No human approval was created or bypassed.
	if code, _, _ := runInjectedCLIWithJEV(t, dir, "", &countingAgent{},
		func(config.Config) (jev.Analyzer, error) { return &sequencedJEV{}, nil }, "approvals", "--json"); code != exitOK {
		t.Errorf("approvals listing failed")
	}
}

// TestSOPLC001RepeatedRunIsIdempotent proves a second invocation performs no work:
// nothing re-runs, no agent call, and validation is not re-run (the graph is already
// satisfied). Historicalization is left off so the completed graph persists.
func TestSOPLC001RepeatedRunIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	count := filepath.Join(t.TempDir(), "count.txt")
	writeConfig(t, dir, lc001Config(count))
	writeFile(t, dir, "PLAN.md", lc001Plan(12))

	a1 := &countingAgent{}
	if code, _, stderr := runInjectedCLIWithJEV(t, dir, "", a1,
		func(config.Config) (jev.Analyzer, error) { return &sequencedJEV{}, nil }, "run", "PLAN.md"); code != exitOK {
		t.Fatalf("first run code=%d stderr=%s", code, stderr)
	}
	if n := validationCount(t, count); n != 1 {
		t.Fatalf("first run validation runs = %d, want 1", n)
	}

	// All twelve reached LOCAL_DONE; the plan is still active (historicalization off).
	st := openStore(t, dir)
	satisfied := 0
	all, _ := st.List()
	for _, tk := range all {
		if tk.IsSatisfied() {
			satisfied++
		}
	}
	st.Close()
	if satisfied != 12 {
		t.Fatalf("satisfied after first run = %d, want 12", satisfied)
	}

	a2 := &countingAgent{}
	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "", a2,
		func(config.Config) (jev.Analyzer, error) { return &sequencedJEV{}, nil }, "run", "PLAN.md")
	if code != exitOK {
		t.Fatalf("second run code=%d stderr=%s", code, stderr)
	}
	if a2.calls != 0 {
		t.Errorf("second run agent calls = %d, want 0", a2.calls)
	}
	if n := validationCount(t, count); n != 1 {
		t.Errorf("second run validation runs = %d, want 1 (no re-work)", n)
	}
	if !strings.Contains(stdout, "all tasks done") && !strings.Contains(stdout, "0 completed") {
		t.Errorf("second run did not report no work; stdout=%s", stdout)
	}
}

// TestSOPLC001InsufficientEvidenceFailsClosed proves nothing is fabricated: a
// verify-first task whose configured validation FAILS does not complete, and the run
// stops rather than recording a pass.
func TestSOPLC001InsufficientEvidenceFailsClosed(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"exit 1\"\n  test:\n    - \"true\"\n")
	writeFile(t, dir, "PLAN.md", lc001Plan(1))

	agent := &countingAgent{}
	analyzer := &sequencedJEV{}
	code, _, _ := runInjectedCLIWithJEV(t, dir, "", agent,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil }, "run", "PLAN.md")
	if code == exitOK {
		t.Error("a failing validation must not report success")
	}
	if agent.calls == 0 {
		t.Error("a failing verify-first task must fall back to the implementation agent")
	}
	st := openStore(t, dir)
	defer st.Close()
	if all, _ := st.List(); len(all) != 0 {
		for _, tk := range all {
			if tk.IsSatisfied() {
				t.Errorf("task %s = %s, want NOT satisfied (insufficient evidence)", tk.ID, tk.Status)
			}
		}
	}
	if got := archivedDisposition(t, dir); got != "" {
		t.Errorf("archived disposition = %q, want none (plan must remain open)", got)
	}
}

// archivedTasks reads the single archived plan's task records, or nil when none exist.
func archivedTasks(t *testing.T, dir string) []*domain.Task {
	t.Helper()
	root := filepath.Join(dir, stateDirName, "archive")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "tasks.json"))
		if err != nil {
			continue
		}
		var tasks []*domain.Task
		if json.Unmarshal(data, &tasks) == nil {
			return tasks
		}
	}
	return nil
}

// archivedDisposition returns the archived plan's lifecycle disposition, or "".
func archivedDisposition(t *testing.T, dir string) string {
	t.Helper()
	root := filepath.Join(dir, stateDirName, "archive")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "lifecycle.json"))
		if err != nil {
			continue
		}
		var lc struct {
			Disposition string `json:"disposition"`
		}
		if json.Unmarshal(data, &lc) == nil && lc.Disposition != "" {
			return lc.Disposition
		}
	}
	return ""
}
