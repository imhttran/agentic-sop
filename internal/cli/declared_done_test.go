package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// declaredAgent counts calls per capability so a test can prove a declared-complete
// task invokes no agent at all.
type declaredAgent struct {
	inner *fakeCapabilityAgent
	calls map[agent.Capability]int
}

func (c *declaredAgent) Generate(ctx context.Context, r agent.Request) (agent.Response, error) {
	if c.calls == nil {
		c.calls = map[agent.Capability]int{}
	}
	c.calls[r.Capability]++
	return c.inner.Generate(ctx, r)
}

// donePlanDoc declares one stage complete and leaves one stage to be implemented,
// the second depending on the first.
const donePlanDoc = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n" +
	"## S001 — Application skeleton\n\nAlready implemented.\n\n### Execution\n\n- done\n\n### Acceptance Criteria\n\n- Application starts\n\n" +
	"## S002 — Follow-up\n\nBuilds on the skeleton.\n\n### Dependencies\n\n- S001\n\n### Acceptance Criteria\n\n- Follow-up works\n"

// allDonePlanDoc is the BACKLOG item's case: every stage is already implemented.
const allDonePlanDoc = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nAlready implemented.\n\n### Execution\n\n- done\n\n### Acceptance Criteria\n\n- Application starts\n"

// TestRunDeclaresStageDone is the regression for BACKLOG "A fully-implemented plan
// cannot close through SOP": a plan whose stage is declared done is recorded as
// already satisfied, so the plan can complete without an agent — where an
// undeclared stage hits the no-change guard and ends BLOCKED.
func TestRunDeclaresStageDone(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), allDonePlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &declaredAgent{inner: &fakeCapabilityAgent{
		plan: validPlanJSON, impl: "nothing to change", fix: "nothing to change",
		review: `{"summary":"clean","findings":[]}`,
	}}

	code, stdout, stderr := runInjectedCLI(t, dir, "", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(a.calls) != 0 {
		t.Errorf("a declared-complete stage must invoke no agent, got %v", a.calls)
	}
	for _, want := range []string{"Declared done in the plan: 1 task(s)", "all tasks done"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.LOCAL_DONE {
		t.Errorf("status = %s, want LOCAL_DONE (recorded as already satisfied)", got.Status)
	}
	if got.ExecutionMode != domain.ExecutionDone {
		t.Errorf("execution mode = %q, want %q (the declaration is the record)", got.ExecutionMode, domain.ExecutionDone)
	}
	if !got.IsSatisfied() {
		t.Error("a declared-complete task must be satisfied")
	}
}

// TestRunDeclaredStageUnblocksDependants proves the declaration participates in the
// dependency graph: a dependant of a declared-complete stage still runs.
func TestRunDeclaredStageUnblocksDependants(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), donePlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &declaredAgent{inner: &fakeCapabilityAgent{
		plan: validPlanJSON, impl: "changed", fix: "changed",
		review: `{"summary":"clean","findings":[]}`,
	}}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if _, ran := a.calls[agent.Implement]; !ran {
		t.Errorf("the dependant stage must still be implemented, got calls=%v", a.calls)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s1, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if s1.Status != domain.LOCAL_DONE {
		t.Errorf("S001 status = %s, want LOCAL_DONE", s1.Status)
	}
	s2, err := st.Get("S002")
	if err != nil {
		t.Fatal(err)
	}
	if !s2.IsSatisfied() {
		t.Errorf("S002 status = %s, want a satisfied state", s2.Status)
	}
}

// TestRunTaskRejectsDeclaredDone proves a single --task run rejects the mode rather
// than silently implementing work the operator declared done.
func TestRunTaskRejectsDeclaredDone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", "# T001 --- Add widget\n\n## Objective\n\nAdd the widget.\n\n## Execution\n\n- done\n")
	writeConfig(t, dir, "project:\n  name: x\n")
	a := &declaredAgent{inner: &fakeCapabilityAgent{plan: validPlanJSON, impl: "x"}}

	code, _, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d (a --task run must reject execution_mode done)", code, exitError)
	}
	if !strings.Contains(stderr, "execution_mode `done`") {
		t.Errorf("stderr missing the actionable error: %s", stderr)
	}
}
