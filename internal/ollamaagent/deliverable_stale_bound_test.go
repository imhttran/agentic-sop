package ollamaagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Deliverable-aware stale bound.
//
// Observed defect (CLOSE-009): a task whose declared deliverable was genuinely
// missing was terminated IMPLEMENT_NO_PROGRESS after the configured stale bound
// (5) even though the harness escalates a missing deliverable only at
// implementNowAfter (12). The escalation could therefore never fire, and the
// invocation was never told to create the deliverable that was blocking it.
//
// These tests pin the coherence rule and its scope: the stale bound is raised
// only while a declared deliverable is missing AND the escalation has not yet
// been issued. Every other case keeps the configured bound, so genuine
// NO_PROGRESS behavior is unchanged.

const owedDeliverable = "docs/reports/t/out.md"

func deliverableRequest(path string) agent.Request {
	req := implementRequest()
	req.Deliverables = []string{path}
	return req
}

func isNoProgress(err error) bool {
	var stalled *noProgressError
	return errors.As(err, &stalled)
}

// TestStaleLimitRaisesOnlyForAnOwedDeliverable is the rule itself.
func TestStaleLimitRaisesOnlyForAnOwedDeliverable(t *testing.T) {
	for _, tc := range []struct {
		name         string
		deliverables []string
		instructed   bool
		exists       bool
		want         int
	}{
		{"no declared deliverable", nil, false, false, maxNoProgressIterations},
		{"declared deliverable already present", []string{owedDeliverable}, false, true, maxNoProgressIterations},
		{"declared deliverable owed and not yet instructed", []string{owedDeliverable}, false, false, implementNowAfter},
		{"declared deliverable owed but escalation already issued", []string{owedDeliverable}, true, false, maxNoProgressIterations},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.exists {
				writeFile(t, dir, owedDeliverable, "# report\n")
			}
			h := New(testConfig("http://127.0.0.1:1"), dir)
			st := newExecutionState()
			st.finalization.implementInstructed = tc.instructed
			if got := h.staleLimitFor(tc.deliverables, st, maxNoProgressIterations); got != tc.want {
				t.Fatalf("staleLimitFor = %d, want %d", got, tc.want)
			}
		})
	}
}

// Regression 1: missing deliverable + discovery-only early turns -> the write
// escalation happens before stale termination, so the deliverable is produced.
// Before the fix this run died NO_PROGRESS at the configured bound.
func TestMissingDeliverableEscalatesBeforeStaleBlock(t *testing.T) {
	dir := t.TempDir()
	// implementNowAfter-1 distinct failed reads: stale turns 1..11, one short of
	// the escalation point and well past the configured stale bound.
	responses := distinctToolCalls(implementNowAfter - 1)
	responses = append(responses,
		fmt.Sprintf(`{"tool":"write_file","args":{"path":%q,"content":"# report\n"}}`, owedDeliverable),
		`{"status":"completed","summary":"wrote the report","changes_expected":true}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	content, err := New(cfg, dir).Execute(context.Background(), deliverableRequest(owedDeliverable))
	if err != nil {
		t.Fatalf("Execute failed: %v (the escalation must happen before stale termination)", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want the completion outcome", content)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(owedDeliverable))); err != nil {
		t.Errorf("deliverable not written: %v", err)
	}
	if got := fake.count(); got <= implementNowAfter {
		t.Errorf("model turns = %d, want more than %d (the write comes at the escalation)", got, implementNowAfter)
	}
}

// Regression 2: the deferral is bounded. With a missing deliverable and no write
// even after the escalation, the run still ends NO_PROGRESS.
func TestMissingDeliverableWithoutAWriteStillBlocks(t *testing.T) {
	dir := t.TempDir()
	responses := distinctToolCalls(implementNowAfter + 8)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), deliverableRequest(owedDeliverable))
	if !isNoProgress(err) {
		t.Fatalf("err = %v, want IMPLEMENT_NO_PROGRESS after the write opportunity", err)
	}
	if !strings.Contains(err.Error(), "termination=no_progress") {
		t.Errorf("err = %q, want a no-progress termination", err)
	}
	if got := fake.count(); got > implementNowAfter+maxNoProgressIterations {
		t.Errorf("model turns = %d, want a bounded stop near %d", got, implementNowAfter+maxNoProgressIterations)
	}
}

// Regression 3: a declared deliverable that already exists is not an owed write,
// so the configured bound applies and no progress is manufactured.
func TestExistingDeliverableKeepsConfiguredBound(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, owedDeliverable, "# report\n")
	responses := distinctToolCalls(maxNoProgressIterations + 6)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), deliverableRequest(owedDeliverable))
	if !isNoProgress(err) {
		t.Fatalf("err = %v, want IMPLEMENT_NO_PROGRESS", err)
	}
	if got := fake.count(); got > maxNoProgressIterations+1 {
		t.Errorf("model turns = %d, want the configured bound %d (no deferral for an existing deliverable)", got, maxNoProgressIterations)
	}
}

// Regression 4: with no declared deliverable the configured bound is untouched,
// so the global NO_PROGRESS behavior is unchanged.
func TestNoDeliverableKeepsConfiguredBound(t *testing.T) {
	dir := t.TempDir()
	responses := distinctToolCalls(maxNoProgressIterations + 6)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if !isNoProgress(err) {
		t.Fatalf("err = %v, want IMPLEMENT_NO_PROGRESS", err)
	}
	if got := fake.count(); got > maxNoProgressIterations+1 {
		t.Errorf("model turns = %d, want the configured bound %d", got, maxNoProgressIterations)
	}
}

// Regression 5: failed mutations are never credited, with or without an owed
// deliverable, and the run still ends NO_PROGRESS.
func TestFailedMutationsWithMissingDeliverableAreNotCredited(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, implementNowAfter+8)
	for i := 0; i < implementNowAfter+8; i++ {
		// A distinct missing path each turn: every delete fails, so nothing is
		// credited and no turn repeats a fingerprint.
		responses = append(responses, fmt.Sprintf(`{"tool":"delete_file","args":{"path":"gone/f%d.txt"}}`, i))
	}
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), deliverableRequest(owedDeliverable))
	if !isNoProgress(err) {
		t.Fatalf("err = %v, want IMPLEMENT_NO_PROGRESS", err)
	}
	if !strings.Contains(err.Error(), "changed_files=0") {
		t.Errorf("err = %q, want changed_files=0 (a failed mutation is not progress)", err)
	}
	if got := fake.count(); got > implementNowAfter+maxNoProgressIterations {
		t.Errorf("model turns = %d, want a bounded stop", got)
	}
}
