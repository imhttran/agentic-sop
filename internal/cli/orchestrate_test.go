package cli

// ORCH-011 opt-in path tests. They prove the multi-agent path is gated behind
// explicit configuration and is default-off, and that when enabled it is
// reachable and produces non-authoritative evidence. No live provider is used.

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// structuredAgent returns a completed structured outcome, so a worker produces a
// validated (accepted) claim.
type structuredAgent struct{}

func (structuredAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "done"}}, nil
}

// TestOrchestrateDisabledByDefault proves the opt-in path refuses unless the operator
// enables orchestration: with orchestration disabled, single-agent execution is the
// only path and nothing here runs.
func TestOrchestrateDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")

	code, _, stderr := runInjectedCLI(t, dir, "diff\n", structuredAgent{}, "orchestrate", "TASK.md")
	if code == exitOK {
		t.Fatalf("orchestrate must refuse when disabled; stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "disabled") {
		t.Errorf("stderr = %q, want a disabled diagnostic", stderr)
	}
}

// TestOrchestrateEnabledIsReachable proves that with orchestration explicitly enabled
// the multi-agent path is reachable and produces deterministic, non-authoritative
// evidence, without changing single-agent behavior.
func TestOrchestrateEnabledIsReachable(t *testing.T) {
	dir := t.TempDir()
	gitInitCommit(t, dir)
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\norchestration:\n  enabled: true\n")

	code, stdout, stderr := runInjectedCLI(t, dir, "  \n", structuredAgent{}, "orchestrate", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "SOP multi-agent orchestration") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "non-authoritative") {
		t.Errorf("stdout must state the claims are non-authoritative: %q", stdout)
	}
}
