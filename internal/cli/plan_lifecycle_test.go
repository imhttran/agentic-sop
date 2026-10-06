package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// planCountAgent records how many times the model was invoked, so a plan lifecycle
// operation can prove it never runs a model.
type planCountAgent struct{ calls int }

func (a *planCountAgent) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	a.calls++
	return agent.Response{Content: "{}"}, nil
}

const planDocSecond = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nSecond plan.\n\n## S001 — Application skeleton\n\nCreate the skeleton.\n\n### Acceptance Criteria\n\n- Application starts\n"

// TestPlanActivateInstallsWithoutExecuting proves activation installs the task graph
// as the active plan and stops: no model call, no attempt, no run, and the first task
// is left runnable for a later `sop run`.
func TestPlanActivateInstallsWithoutExecuting(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	a := &planCountAgent{}

	code, out, errOut := runCLIWithAgent(t, dir, a, "plan", "activate", "PLAN.md")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if a.calls != 0 {
		t.Errorf("activation invoked the model %d time(s); it must not", a.calls)
	}
	if !strings.Contains(out, "State: ACTIVE") || !strings.Contains(out, "Nothing executed") {
		t.Errorf("stdout = %q", out)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	tasks, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %+v, want 1", tasks)
	}
	// PLANNED is the runnable state: READY is a transient promotion the scheduler
	// applies when it selects the task, so a fresh activation must leave the first task
	// PLANNED (a persisted READY task would not be re-selectable).
	if tasks[0].Status != domain.PLANNED {
		t.Errorf("status = %s, want PLANNED (runnable)", tasks[0].Status)
	}
	if tasks[0].Attempt != 0 || len(tasks[0].Attempts) != 0 {
		t.Errorf("activation recorded an attempt: %+v", tasks[0])
	}
}

// TestPlanSupersedeArchivesOldPlan proves an explicit supersession archives the
// active plan as SUPERSEDED and activates the new plan without executing it.
func TestPlanSupersedeArchivesOldPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}
	writeFile(t, dir, "PLAN2.md", planDocSecond)
	a := &planCountAgent{}

	code, out, errOut := runCLIWithAgent(t, dir, a, "plan", "supersede", "PLAN2.md")
	if code != exitOK {
		t.Fatalf("supersede: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if a.calls != 0 {
		t.Errorf("supersede invoked the model %d time(s); it must not", a.calls)
	}
	if !strings.Contains(out, "SUPERSEDED") || !strings.Contains(out, "State: ACTIVE") {
		t.Errorf("stdout = %q", out)
	}
}
