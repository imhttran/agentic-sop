package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// memoryAgent records the Input of each IMPLEMENT invocation and serves the other
// capabilities from canned content.
type memoryAgent struct {
	plan       string
	implInputs []string
}

func (a *memoryAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: a.plan}, nil
	case agent.Implement:
		a.implInputs = append(a.implInputs, r.Input)
		return agent.Response{Content: "changed files"}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// TestRunIncludesDecisionMemoryWhenOptedIn proves context.decision_memory includes an
// applicable decision in the implement context.
func TestRunIncludesDecisionMemoryWhenOptedIn(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\ncontext:\n  decision_memory: true\n")
	if code, _, se := runCLI(t, dir, "memory", "add", "--decision", "Postgres is canonical storage", "--reason", "single source of truth", "--scope", "architecture"); code != exitOK {
		t.Fatalf("memory add: %s", se)
	}
	a := &memoryAgent{plan: validPlanJSON}

	if code, _, se := runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("run: %s", se)
	}
	if len(a.implInputs) == 0 {
		t.Fatal("no implement invocation recorded")
	}
	if !strings.Contains(a.implInputs[0], "Postgres is canonical storage") {
		t.Errorf("the implement context must include applicable decision memory:\n%s", a.implInputs[0])
	}
}

// TestRunOmitsDecisionMemoryByDefault proves the same decision is not injected when the
// facility is off (the default).
func TestRunOmitsDecisionMemoryByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	if code, _, se := runCLI(t, dir, "memory", "add", "--decision", "Postgres is canonical storage", "--reason", "single source of truth", "--scope", "architecture"); code != exitOK {
		t.Fatalf("memory add: %s", se)
	}
	a := &memoryAgent{plan: validPlanJSON}

	if code, _, se := runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("run: %s", se)
	}
	if len(a.implInputs) == 0 {
		t.Fatal("no implement invocation recorded")
	}
	if strings.Contains(a.implInputs[0], "Postgres is canonical storage") {
		t.Errorf("decision memory must be off by default:\n%s", a.implInputs[0])
	}
}
