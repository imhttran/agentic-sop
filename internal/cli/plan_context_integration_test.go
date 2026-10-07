package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// planCapturingAgent behaves like fakeCapabilityAgent but records every PLAN
// request, so a test can assert the authoritative capability context reached the
// task-level planning boundary.
type planCapturingAgent struct {
	plan       string
	impl       string
	review     string
	planInputs []string
	planCalls  int
}

func (p *planCapturingAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		p.planCalls++
		p.planInputs = append(p.planInputs, r.Input)
		return agent.Response{Content: p.plan}, nil
	case agent.Implement:
		return agent.Response{Content: p.impl}, nil
	case agent.Review:
		return agent.Response{Content: p.review}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// taskPlanRequiresGoToolchain re-derives a capability the compiled plan already
// declares EXISTS as UNKNOWN and requires it. Without the authoritative context this
// is the recurring-capability-repair shape: validation rejects the UNKNOWN
// requirement and returns the plan to the agent for correction.
const taskPlanRequiresGoToolchain = `{"project":"p","summary":"s","capabilities":[{"name":"Go toolchain","status":"UNKNOWN"}],"stages":[{"id":"T001","title":"Add widget","objective":"Inspect the repository.","requires":["Go toolchain"],"acceptance_criteria":["widget works"]}]}`

// TestTaskRunPlanningReceivesAuthoritativeCapabilityContext proves the task-level
// planning boundary now receives the compiled plan's authoritative capability model,
// so a capability the plan already declares EXISTS is not downgraded to UNKNOWN (the
// exact shape that triggered recurring model-driven capability repair), and no
// repair round is spent on it.
func TestTaskRunPlanningReceivesAuthoritativeCapabilityContext(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	// The compiled plan is authoritative: Go toolchain EXISTS, and T001 already
	// requires it.
	plan := planner.Plan{
		Project: "x", Summary: "S",
		Capabilities: []planner.Capability{{Name: "Go toolchain", Status: planner.CapabilityExists, Evidence: "go1.27.1"}},
		Stages: []planner.Stage{{
			ID: "T001", Title: "Add widget", Objective: "Inspect the repository.",
			Requires: []string{"Go toolchain"}, AcceptanceCriteria: []string{"widget works"},
		}},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, stateDirName+"/plan.json", string(data))

	a := &planCapturingAgent{plan: taskPlanRequiresGoToolchain, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if a.planCalls != 1 {
		t.Errorf("plan calls = %d, want 1 (no repair for an authoritatively-known capability)", a.planCalls)
	}
	if strings.Contains(stdout, "invalid plan returned to the agent for correction") {
		t.Errorf("a known capability triggered a repair: %q", stdout)
	}
	if len(a.planInputs) == 0 {
		t.Fatal("no PLAN request captured")
	}
	for _, want := range []string{"Authoritative plan context", "Go toolchain — EXISTS"} {
		if !strings.Contains(a.planInputs[0], want) {
			t.Errorf("planning input missing %q:\n%s", want, a.planInputs[0])
		}
	}
}

// TestTaskRunPlanningWithoutPlanCarriesNoContext proves the fix is inert when no
// machine plan exists (an ad-hoc task run): the planning input is the task text
// alone and no authoritative context is prepended, so existing behavior is
// unchanged.
func TestTaskRunPlanningWithoutPlanCarriesNoContext(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	a := &planCapturingAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(a.planInputs) == 0 {
		t.Fatal("no PLAN request captured")
	}
	if strings.Contains(a.planInputs[0], "Authoritative plan context") {
		t.Errorf("an ad-hoc run with no machine plan must not carry authoritative context:\n%s", a.planInputs[0])
	}
}
