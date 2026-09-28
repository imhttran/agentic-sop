package ollamaagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// FIX lifecycle regression tests.
//
// The JEV002 failure was a continuously mutating FIX that ran to the 24-iteration
// ceiling in CHANGE, still executing write_file, and terminated with the generic
// iteration_limit (mutation_observed=true, last_action="write_file ..."). The fix
// makes the force-finalization cutoff independent of the shape of the turn that
// crosses it: a mutated run enters FINALIZE before its next model turn, its write
// tools are denied there, and the run either returns its structured outcome or
// fails with a finalization-specific reason. These tests pin that contract for FIX
// specifically, and for both public execution entry points.

// fixMutationWrites builds n scripted write_file turns to distinct files, so a
// model that keeps "repairing" performs genuinely different mutations rather than
// a repeated no-progress action.
func fixMutationWrites(prefix string, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf(
			`{"tool":"write_file","args":{"path":"%s-%d.txt","content":"mutation-%d"}}`,
			prefix, i, i,
		))
	}
	return out
}

// writeFileAudit tallies allowed and denied write_file tool calls in the run's
// audit trail. A denied write is the observable proof that a mutation was refused
// rather than executed.
func writeFileAudit(records []toolharness.AuditRecord) (allowed, denied int) {
	for _, r := range records {
		if r.Tool != toolharness.ToolWriteFile {
			continue
		}
		if r.Action == toolharness.ActionDeny {
			denied++
		} else {
			allowed++
		}
	}
	return allowed, denied
}

// TestFixForceFinalizeIsIndependentOfThresholdTurnShape is the JEV002 regression:
// a narration on the force-threshold turn must not delay FINALIZE past the point
// where the run can still finalize inside the ceiling. Before the fix the
// narration skipped the after-tool finalize check, FINALIZE was entered a turn
// late, and the mutated FIX ended as the generic iteration_limit.
func TestFixForceFinalizeIsIndependentOfThresholdTurnShape(t *testing.T) {
	dir := t.TempDir()

	responses := fixMutationWrites("f", fixForceFinalizeAfter-1) // iterations 1..19 mutate
	responses = append(responses, `I have completed the repair and am verifying it now.`)
	responses = append(responses, fixMutationWrites("g", 20)...) // keep writing forever

	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), fixRequest())
	if err == nil {
		t.Fatalf("expected a bounded failure, got success after %d turns", fake.count())
	}
	if strings.Contains(err.Error(), "iteration_limit") {
		t.Errorf("FIX ended as generic iteration_limit; force-finalization was bypassed: %v", err)
	}
	if !strings.Contains(err.Error(), "termination=finalization_limit") {
		t.Errorf("err = %v, want a finalization-specific termination", err)
	}

	records := h.TraceRecords()
	if !hasEvent(records, implementFinalizeEvent) {
		t.Errorf("FIX never entered FINALIZE: %+v", records)
	}

	// No mutation may execute at or after the force threshold (iteration 20): the
	// 19 pre-threshold writes run, later ones are denied.
	allowed, denied := writeFileAudit(h.AuditRecords())
	if allowed != fixForceFinalizeAfter-1 {
		t.Errorf("executed writes = %d, want %d (none at or after the threshold)", allowed, fixForceFinalizeAfter-1)
	}
	if denied == 0 {
		t.Error("no write_file was denied during FINALIZE")
	}
}

// TestFixFinalizationIsBoundedAndSpecific covers a FIX that repairs and then keeps
// requesting tools forever: it must stop with a finalization-specific reason after
// at most fixFinalizeTurns, not run to the generic iteration limit.
func TestFixFinalizationIsBoundedAndSpecific(t *testing.T) {
	dir := t.TempDir()

	_, srv := newFakeOllama(t, fixMutationWrites("h", fixForceFinalizeAfter+10)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), fixRequest())
	if err == nil {
		t.Fatal("expected a bounded failure")
	}
	if strings.Contains(err.Error(), "iteration_limit") {
		t.Errorf("FIX must not terminate as iteration_limit: %v", err)
	}
	for _, want := range []string{
		"termination=finalization_limit",
		"mutation_observed=true",
		fmt.Sprintf("finalization_turns=%d", fixFinalizeTurns),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}

	allowed, denied := writeFileAudit(h.AuditRecords())
	if allowed != fixForceFinalizeAfter-1 {
		t.Errorf("executed writes = %d, want %d", allowed, fixForceFinalizeAfter-1)
	}
	if denied != fixFinalizeTurns {
		t.Errorf("denied writes = %d, want %d (the finalization allowance)", denied, fixFinalizeTurns)
	}
}

// TestFixCannotReturnFromFinalizeToChange covers FINALIZE's terminality for FIX: a
// mutation requested after finalization is denied and never applied, and the run
// still returns its structured outcome.
func TestFixCannotReturnFromFinalizeToChange(t *testing.T) {
	dir := t.TempDir()

	responses := []string{`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`}
	responses = append(responses, distinctToolCalls(17)...) // interactions 2..18 -> FINALIZE
	responses = append(responses,
		`{"tool":"write_file","args":{"path":"more.txt","content":"too late"}}`,
		`{"status":"completed","summary":"fixed it","changes_expected":true}`,
	)

	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), fixRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want the structured outcome", content)
	}
	if _, err := os.Stat(filepath.Join(dir, "more.txt")); !os.IsNotExist(err) {
		t.Error("more.txt exists; a mutation during FINALIZE must be denied, not applied")
	}
	_, denied := writeFileAudit(h.AuditRecords())
	if denied == 0 {
		t.Error("the post-finalization write was not denied")
	}
	// The applied mutation is preserved: FINALIZE is terminal, it does not undo or
	// re-open the change.
	if got, err := os.ReadFile(filepath.Join(dir, "out.txt")); err != nil || string(got) != "x" {
		t.Errorf("out.txt = %q (err=%v), want the original change preserved", got, err)
	}
}

// TestFixLateFirstMutationIsFinalizationFailure covers a mutation that only
// arrives in the closing turns: FINALIZE is entered late, the loop exhausts in
// FINALIZE, and the run must still report a finalization-specific failure rather
// than the generic iteration limit.
func TestFixLateFirstMutationIsFinalizationFailure(t *testing.T) {
	dir := t.TempDir()

	responses := distinctToolCalls(fixForceFinalizeAfter + 1) // 21 discovery reads, no mutation
	responses = append(responses, fixMutationWrites("late", 6)...)

	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), fixRequest())
	if err == nil {
		t.Fatal("expected a bounded failure")
	}
	if strings.Contains(err.Error(), "iteration_limit") {
		t.Errorf("FIX exhausting inside FINALIZE must not report iteration_limit: %v", err)
	}
	for _, want := range []string{
		"termination=finalization_limit",
		"mutation_observed=true",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}

// TestFixLifecycleThroughCommandEntryPoint proves the command provider entry
// point (Run) obeys the same FIX lifecycle as the in-process Execute path: a
// continuously mutating FIX is force-finalized and reported as a finalization
// failure, never as the generic iteration limit.
func TestFixLifecycleThroughCommandEntryPoint(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	_, srv := newFakeOllama(t, fixMutationWrites("c", fixForceFinalizeAfter+10)...)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"FIX","task":"fix it","input":"validation failed","output_requirements":"outcome"}`
	var out, errOut strings.Builder
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"status":"failed"`) {
		t.Errorf("out = %q, want a failed structured outcome", out.String())
	}
	if !strings.Contains(out.String(), "termination=finalization_limit") {
		t.Errorf("out = %q, want a finalization-specific reason", out.String())
	}
	if strings.Contains(out.String(), "iteration_limit") {
		t.Errorf("out = %q, must not report the generic iteration limit", out.String())
	}
}

// TestImplementForceFinalizeStillAtItsOwnThreshold proves IMPLEMENT keeps its
// existing lifecycle with the shared engine change: a continuously mutating
// IMPLEMENT still finalizes at implementForceFinalizeAfter and fails with a
// finalization-specific reason, not the generic iteration limit.
func TestImplementForceFinalizeStillAtItsOwnThreshold(t *testing.T) {
	dir := t.TempDir()

	_, srv := newFakeOllama(t, fixMutationWrites("imp", implementForceFinalizeAfter+10)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), implementRequest())
	if err == nil {
		t.Fatal("expected a bounded failure")
	}
	if strings.Contains(err.Error(), "iteration_limit") {
		t.Errorf("IMPLEMENT must not terminate as iteration_limit: %v", err)
	}
	if !strings.Contains(err.Error(), "termination=finalization_limit") {
		t.Errorf("err = %v, want a finalization-specific termination", err)
	}

	allowed, denied := writeFileAudit(h.AuditRecords())
	if allowed != implementForceFinalizeAfter-1 {
		t.Errorf("executed writes = %d, want %d (none at or after the threshold)", allowed, implementForceFinalizeAfter-1)
	}
	if denied != implementFinalizeTurns {
		t.Errorf("denied writes = %d, want %d", denied, implementFinalizeTurns)
	}
}
