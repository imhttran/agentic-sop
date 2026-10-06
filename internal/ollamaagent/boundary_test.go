package ollamaagent

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/budget"
)

// repeat returns s n times, for scripting a fake model.
func scriptN(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

// toolActivityCount counts executed tool activity events (one per executed tool
// call), which is the harness's own record of tool executions.
func toolActivityCount(events []activity.Event) int {
	n := 0
	for _, e := range events {
		switch e.Stage {
		case activity.StageDiscover, activity.StageChange, activity.StageValidate:
			n++
		}
	}
	return n
}

// readOnlyTool is a tool call that never mutates, so each turn is stale: the
// run makes no repository progress.
const readOnlyTool = `{"tool":"git_status","args":{}}`

// TestImplementIterationBoundary proves the IMPLEMENT hard iteration ceiling is
// enforced at the boundary: the harness executes exactly N model turns and never
// attempts an N+1th, driven through the canonical budget path
// (budget.Budget -> Config.EffectiveBudget -> policyFor).
func TestImplementIterationBoundary(t *testing.T) {
	const n = 3
	dir := t.TempDir()
	f, srv := newFakeOllama(t, scriptN(readOnlyTool, 8)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100 // keep the tool-call limit out of the way
	cfg.Budget = budget.Defaults()
	cfg.Budget.ImplementIterations = n
	cfg.Budget.StaleIterations = 100 // let the iteration ceiling fire first

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil {
		t.Fatal("the iteration ceiling must stop a run that never completes")
	}
	if got := f.count(); got != n {
		t.Errorf("model turns = %d, want exactly %d (N-1 and N run; N+1 must not)", got, n)
	}
}

// TestFixIterationBoundary proves the FIX hard iteration ceiling is enforced the
// same way.
func TestFixIterationBoundary(t *testing.T) {
	const n = 3
	dir := t.TempDir()
	f, srv := newFakeOllama(t, scriptN(readOnlyTool, 8)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100
	cfg.Budget = budget.Defaults()
	cfg.Budget.FixIterations = n
	cfg.Budget.StaleIterations = 100

	_, err := New(cfg, dir).Execute(context.Background(), fixRequest())
	if err == nil {
		t.Fatal("the FIX iteration ceiling must stop a run that never completes")
	}
	if got := f.count(); got != n {
		t.Errorf("model turns = %d, want exactly %d", got, n)
	}
}

// TestStaleIterationBoundary proves the no-progress (stale) guard fires exactly
// at the configured stale limit: N stale turns execute, and there is no N+1th.
// The IMPLEMENT_NO_PROGRESS semantics remain BLOCK / not retryable / no human /
// diagnostic preserved.
func TestStaleIterationBoundary(t *testing.T) {
	const n = 3
	dir := t.TempDir()
	f, srv := newFakeOllama(t, scriptN(readOnlyTool, 8)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100
	cfg.Budget = budget.Defaults()
	cfg.Budget.ImplementIterations = 100 // let the stale guard fire first
	cfg.Budget.StaleIterations = n

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "IMPLEMENT_NO_PROGRESS") {
		t.Fatalf("err = %v, want the IMPLEMENT_NO_PROGRESS stop", err)
	}
	if got := f.count(); got != n {
		t.Errorf("model turns = %d, want exactly the stale limit %d (N+1 must not execute)", got, n)
	}
}

// TestToolCallBoundary proves the tool-call ceiling is enforced at the boundary:
// exactly N tool calls execute, and the N+1th attempt is refused (the model turn
// happens, but the tool does not run). The invariant is executed <= N.
func TestToolCallBoundary(t *testing.T) {
	const n = 3
	dir := t.TempDir()
	f, srv := newFakeOllama(t, scriptN(readOnlyTool, 8)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = n
	cfg.Budget = budget.Defaults()
	cfg.Budget.ImplementIterations = 100
	cfg.Budget.StaleIterations = 100

	sink := &recordSink{}
	ctx := activity.WithRecorder(context.Background(), activity.New("T", sink))
	_, err := New(cfg, dir).Execute(ctx, implementRequest())
	if err == nil || !strings.Contains(err.Error(), "tool-call limit") {
		t.Fatalf("err = %v, want a tool-call-limit error", err)
	}
	executed := toolActivityCount(sink.events)
	if executed > n {
		t.Errorf("executed tool calls = %d, want <= the configured %d", executed, n)
	}
	if executed != n {
		t.Errorf("executed tool calls = %d, want exactly %d (the N+1th is refused)", executed, n)
	}
	// The model was asked once more (the turn that attempted the refused call).
	if got := f.count(); got != n+1 {
		t.Errorf("model turns = %d, want %d (N executions + the refused attempt)", got, n+1)
	}
}
