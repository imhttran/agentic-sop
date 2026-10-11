package ollamaagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// CONV-004 correction (discriminating regression).
//
// The pre-mutation convergence enforcement must fire BEFORE the stale guard
// terminates the stage: a run that has exhausted its bounded discovery window and
// keeps inspecting must be denied its non-mutating repository tools (with a
// correction prompt) while the mutation tools remain available, so it can still
// converge on a mutation attempt or a truthful terminal outcome.
//
// The CONV-002/CONV-003 regressions pin the stop at implementNowAfter +
// staleIterations, but they pass for BOTH the inert implementation (whose
// enforcement threshold coincided with the stale threshold, so it never fired) and
// the corrected one. These tests add the missing discriminator: at least one
// non-mutating repository tool request is DENIED before the run terminates.
// Ordinary stale termination denies nothing, so the assertion fails against the
// inert implementation and passes against the corrected one.
//
// Model-free, deterministic, provider-neutral: the model is the in-process
// httptest fake (newFakeOllama) and all fixtures live in a per-test t.TempDir();
// no real model, provider, network, or Clef dependency is involved.
func TestConvergenceEnforcementFiresBeforeStaleTermination(t *testing.T) {
	cases := []struct {
		name   string
		req    agent.Request
		marker string
	}{
		{"implement", implementRequest(), "IMPLEMENT_NO_PROGRESS"},
		{"fix", fixRequest(), "FIX_NO_PROGRESS"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedToolFiles(t, dir, "pkg", implementNowAfter+maxNoProgressIterations+6)

			responses := make([]string, 0, implementNowAfter+maxNoProgressIterations+6)
			for i := 0; i < implementNowAfter+maxNoProgressIterations+6; i++ {
				responses = append(responses, readToolCall(i))
			}
			fake, srv := newFakeOllama(t, responses...)

			cfg := testConfig(srv.URL)
			cfg.MaxToolCalls = 200

			h := New(cfg, dir)
			_, err := h.Execute(context.Background(), tc.req)

			// The run is still bounded by the unchanged stale guard.
			var stalled *noProgressError
			if !errors.As(err, &stalled) {
				t.Fatalf("err = %v, want *noProgressError", err)
			}
			if !strings.Contains(err.Error(), tc.marker) {
				t.Errorf("err = %q, want %q", err, tc.marker)
			}
			if got, want := fake.count(), implementNowAfter+maxNoProgressIterations; got != want {
				t.Errorf("model turns = %d, want %d (implementNowAfter + staleIterations)", got, want)
			}

			// Discriminator: the convergence enforcement denied at least one
			// non-mutating repository tool request before termination. The inert
			// implementation records zero denials.
			denied, deniedRead := 0, 0
			for _, r := range h.AuditRecords() {
				if r.Outcome == "denied" {
					denied++
					if r.Tool == "read_file" {
						deniedRead++
					}
				}
			}
			if denied == 0 {
				t.Error("no denials: the convergence enforcement did not fire before the stale termination")
			}
			if deniedRead == 0 {
				t.Error("no denied read_file: the enforcement did not deny a non-mutating repository tool")
			}
		})
	}
}
