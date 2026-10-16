package cli

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
)

// plannerRecordingAgent records every agent request and completes deterministically, so a
// run can be driven while the requests SOP builds are inspected.
type plannerRecordingAgent struct {
	mu       sync.Mutex
	requests []agent.Request
}

func (a *plannerRecordingAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	a.mu.Lock()
	a.requests = append(a.requests, r)
	a.mu.Unlock()
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "created x"}, nil
	}
}

func (a *plannerRecordingAgent) implement() *agent.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.requests {
		if a.requests[i].Capability == agent.Implement {
			return &a.requests[i]
		}
	}
	return nil
}

// capturedPlannerNeedsHuman is the exact structured response a real model-driven executor
// returned in the bounded live planner test (provider ollama, model deepseek-v4.1-flash:cloud,
// capability IMPLEMENT, planner enabled, no repository change). The model recognized the
// ambiguity and asked for an operator decision, but used needs_human rather than the advertised
// needs_planner outcome. The reason text is reproduced verbatim so the regression pins the
// observed classification instead of an invented one.
const capturedPlannerNeedsHuman = `{"reason":"The task's central semantics — whether normalized.txt is sorted and whether duplicate lines are removed — are explicitly declared undecided operator preferences, and the task instructs 'Do not guess; clarify the intended ordering and duplicate handling before implementing.' No repository source defines these semantics, and the acceptance test (output_test.go / go test ./...) derives behavior solely from choices.txt, which does not exist. Choosing the ordering/duplicate policy is an authoritative operator decision, not an implementation detail I am authorized to make. Required validation 'go test ./...' currently fails for lack of choices.txt. An operator must specify (a) ordering (preserve input order vs sorted) and (b) duplicate handling (preserve duplicates vs remove duplicates) so I can write choices.txt and the matching normalized.txt.","status":"needs_human"}`

// TestCapturedNeedsHumanOutcomeContinues pins the policy the bounded live planner test
// exercised: a bare needs_human with no explicit human-boundary action and no repository change
// is a bounded continuation under the default BALANCED autonomy, never a human gate. The
// classifier deliberately treats the needs_human status alone as non-authoritative because
// SOP's own harness uses it to request a bounded requeue; only an explicit boundary action in
// the reason (security/destructive/requirements conflict) escalates to NEEDS_HUMAN. This test
// documents the recorded policy as intended, not as a lost structured boundary signal: no human
// approval is created and the task is requeued.
func TestCapturedNeedsHumanOutcomeContinues(t *testing.T) {
	outcome := agent.ParseOutcome(capturedPlannerNeedsHuman)
	if outcome == nil || outcome.Status != agent.OutcomeNeedsHuman {
		t.Fatalf("captured response parsed to %+v, want a needs_human outcome", outcome)
	}
	dir := budgetGraph(t)
	a := outcomeAgent{outcome: outcome}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("a bare needs_human with no explicit boundary must not require a human:\n%s", stdout)
	}
	for _, want := range []string{"run S001: CONTINUE", "classification: CONTINUE", "S001 CONTINUE (requeued)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	cls := readClassification(t, dir, "S001")
	if cls.Disposition != failure.Continue || cls.Kind != failure.IncompleteImplementation {
		t.Errorf("recorded classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", cls)
	}

	// No human gate: the approval listing is empty and the task is requeued, not blocked.
	if code, out, errS := runCLI(t, dir, "approvals", "--json"); code != exitOK {
		t.Fatalf("approvals code=%d stderr=%s", code, errS)
	} else {
		var doc struct {
			Approvals []json.RawMessage `json:"approvals"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("approvals not JSON: %v\n%s", err, out)
		}
		if len(doc.Approvals) != 0 {
			t.Errorf("approvals = %s, want none (a bare needs_human creates no gate)", out)
		}
	}
}

// TestPlannerAvailabilityReachesTheImplementRequest proves planner availability crosses the
// agent request boundary only when the planner is enabled: the harness learns it from the
// request, never from CLI configuration, and a disabled planner leaves the field at its zero
// value so the existing outcome contract is preserved.
func TestPlannerAvailabilityReachesTheImplementRequest(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
	}{
		{"enabled", true},
		{"disabled", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initProject(t, dir)
			seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
			config := "project:\n  name: \"demo\"\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n"
			if tc.enabled {
				config = "project:\n  name: \"demo\"\nplanner:\n  enabled: true\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n"
			}
			writeConfig(t, dir, config)

			a := &plannerRecordingAgent{}
			if code, _, errS := runInjectedCLI(t, dir, plannerDiff, a, "run"); code != exitOK {
				t.Fatalf("run code=%d stderr=%s", code, errS)
			}
			req := a.implement()
			if req == nil {
				t.Fatal("no IMPLEMENT request was made")
			}
			if req.PlannerAvailable != tc.enabled {
				t.Fatalf("PlannerAvailable=%v, want %v", req.PlannerAvailable, tc.enabled)
			}
		})
	}
}
