package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// discoveryRecordingAgent records every agent request and defers to the ordinary
// outcome agent, so a run can be driven deterministically while the requests SOP
// builds are inspected.
type discoveryRecordingAgent struct {
	mu       sync.Mutex
	requests []agent.Request
	outcome  outcomeAgent
}

func (a *discoveryRecordingAgent) Generate(ctx context.Context, r agent.Request) (agent.Response, error) {
	a.mu.Lock()
	a.requests = append(a.requests, r)
	a.mu.Unlock()
	return a.outcome.Generate(ctx, r)
}

func (a *discoveryRecordingAgent) implementRequestForTask(id string) *agent.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.requests {
		if a.requests[i].Capability == agent.Implement && strings.Contains(a.requests[i].Task, id) {
			return &a.requests[i]
		}
	}
	return nil
}

// TestRunPerTaskDiscoveryAllowanceReachesAgentRequests proves two tasks in one `sop run`
// carry their own discovery allowance into the agent request, and that a task that
// declares none carries none (0), so a per-task setting neither leaks nor is silently
// defaulted into a sibling task.
func TestRunPerTaskDiscoveryAllowanceReachesAgentRequests(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), oneshotTwoTaskPlan)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "A", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 1, DiscoveryIterations: 30})
	seedTask(t, dir, &domain.Task{ID: "S002", Title: "B", Status: domain.PLANNED, MaxAttempts: 3})

	a := &discoveryRecordingAgent{outcome: outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}}
	// Both tasks execute their IMPLEMENT (recording the request) and then stop at a
	// task-local boundary; ONESHOT-005 continues the independent task.
	_, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if !strings.Contains(stdout, "Running: S002") {
		t.Fatalf("independent task S002 did not execute:\n%s", stdout)
	}

	first := a.implementRequestForTask("S001")
	if first == nil {
		t.Fatal("no IMPLEMENT request recorded for S001")
	}
	if first.DiscoveryIterations != 30 {
		t.Errorf("S001 discovery iterations = %d, want 30", first.DiscoveryIterations)
	}

	second := a.implementRequestForTask("S002")
	if second == nil {
		t.Fatal("no IMPLEMENT request recorded for S002")
	}
	if second.DiscoveryIterations != 0 {
		t.Errorf("S002 discovery iterations = %d, want 0 (no override; must not inherit S001's)", second.DiscoveryIterations)
	}
}

// TestRunTaskDiscoveryAllowanceFromPlan proves a per-stage allowance authored in the
// plan document flows through task construction and the scheduler into the agent
// request, end to end, for the task that declares it.
func TestRunTaskDiscoveryAllowanceFromPlan(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := "# Implementation Plan\n\n## Project\n\nW\n\n## Summary\n\nx\n\n" +
		"## S001 — A\n\nCreate A.\n\n### Dependencies\n\nNone\n\n### Acceptance Criteria\n\n- ok\n\n### Discovery iterations\n\n40 — multi-package\n\n" +
		"## S002 — B\n\nCreate B.\n\n### Dependencies\n\n- S001\n\n### Acceptance Criteria\n\n- ok\n"
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), plan)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")

	a := &discoveryRecordingAgent{outcome: outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}}
	_, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")

	first := a.implementRequestForTask("S001")
	if first == nil {
		t.Fatalf("no IMPLEMENT request recorded for S001\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if first.DiscoveryIterations != 40 {
		t.Errorf("S001 discovery iterations = %d, want 40 (from the plan)", first.DiscoveryIterations)
	}
}
