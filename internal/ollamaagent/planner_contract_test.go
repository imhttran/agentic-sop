package ollamaagent

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// The planner outcome contract is advertised only when the planner is available for the
// invocation AND the stage supports a planner continuation (IMPLEMENT). These tests drive
// the contract text and the native bridge with canned model responses — they never call a
// live model, so they prove the plumbing, not model behavior.

// TestContractAdvertisesPlannerForSupportedStages pins the advertisement rule and proves the
// existing contract is preserved when the planner is unavailable or the stage is
// unsupported.
func TestContractAdvertisesPlannerForSupportedStages(t *testing.T) {
	cases := []struct {
		name      string
		cap       agent.Capability
		available bool
		want      bool
	}{
		{"implement enabled", agent.Implement, true, true},
		{"implement disabled", agent.Implement, false, false},
		{"fix enabled", agent.Fix, true, false}, // FIX has no continuation path
		{"fix disabled", agent.Fix, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := agent.Request{Capability: tc.cap, PlannerAvailable: tc.available}
			contract := contractFor(req)
			if got := strings.Contains(contract, "needs_planner"); got != tc.want {
				t.Fatalf("needs_planner advertised=%v, want %v:\n%s", got, tc.want, contract)
			}
			// Disabling the planner never removes the existing outcomes.
			for _, want := range []string{`"completed"`, `"needs_human"`, `"failed"`} {
				if !strings.Contains(contract, want) {
					t.Fatalf("base contract lost %s:\n%s", want, contract)
				}
			}
		})
	}
	// PLAN and REVIEW keep their own document contract.
	for _, cap := range []agent.Capability{agent.Plan, agent.Review} {
		if strings.Contains(contractFor(agent.Request{Capability: cap, PlannerAvailable: true}), "needs_planner") {
			t.Fatalf("%s advertised the planner outcome", cap)
		}
	}
}

// TestPlannerGuidanceKeepsHumanAuthorizationSeparate pins the "when to ask" guidance: the
// planner is for clarification only, authorization/human approval stays needs_human, the
// model must stop after asking, and the example uses the real PlannerRequest fields.
func TestPlannerGuidanceKeepsHumanAuthorizationSeparate(t *testing.T) {
	contract := contractFor(agent.Request{Capability: agent.Implement, PlannerAvailable: true})
	for _, want := range []string{"needs_human", "needs_planner", "FINAL", "return_stage", `"question"`, `"options"`} {
		if !strings.Contains(contract, want) {
			t.Fatalf("planner guidance missing %q:\n%s", want, contract)
		}
	}
}

// TestNeedsPlannerReachesTheNativeBridge proves a valid needs_planner response reaches
// OutcomeNeedsPlanner through the native bridge with its question fields intact, and that it
// never claims a repository change.
func TestNeedsPlannerReachesTheNativeBridge(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	const final = `{"status":"needs_planner","planner":{"question":"Sorted or preserve order?","options":["sorted","preserve order"],"kind":"order","return_stage":"IMPLEMENTING"}}`
	_, srv := newFakeOllama(t, final)

	req := agent.Request{Capability: agent.Implement, Task: "do it", Input: "context", OutputRequirements: "summarize", PlannerAvailable: true}
	resp, err := NewNativeAgent(testConfig(srv.URL), dir, nil).Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeNeedsPlanner {
		t.Fatalf("outcome = %+v content=%s, want needs_planner", resp.Outcome, resp.Content)
	}
	p := resp.Outcome.Planner
	if p == nil {
		t.Fatalf("planner request lost: %s", resp.Content)
	}
	if p.Question != "Sorted or preserve order?" || len(p.Options) != 2 || p.Options[0] != "sorted" || p.Options[1] != "preserve order" || p.Kind != "order" || p.ReturnStage != "IMPLEMENTING" {
		t.Fatalf("planner fields changed: %+v", p)
	}
	if resp.Outcome.ChangesExpected {
		t.Fatalf("a planner request must not claim a repository change: %+v", resp.Outcome)
	}
}

// TestNeedsPlannerRejectedWhenUnsupportedOrDisabled proves an unsupported needs_planner
// response is an explicit non-success: it is never wrapped as completed, never routed to a
// human approval boundary, and never carries a planner object.
func TestNeedsPlannerRejectedWhenUnsupportedOrDisabled(t *testing.T) {
	const final = `{"status":"needs_planner","planner":{"question":"q","options":["a","b"]}}`
	cases := []struct {
		name      string
		cap       agent.Capability
		available bool
	}{
		{"implement disabled", agent.Implement, false},
		{"fix enabled", agent.Fix, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			gitInit(t, dir)
			_, srv := newFakeOllama(t, final)
			req := agent.Request{Capability: tc.cap, Task: "t", Input: "i", OutputRequirements: "o", PlannerAvailable: tc.available}
			resp, err := NewNativeAgent(testConfig(srv.URL), dir, nil).Generate(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Outcome == nil {
				t.Fatalf("no outcome: %s", resp.Content)
			}
			if resp.Outcome.Status != agent.OutcomeFailed {
				t.Fatalf("outcome = %+v, want an explicit failed outcome (never completed, never needs_human)", resp.Outcome)
			}
			if !strings.Contains(resp.Outcome.Reason, "planner request rejected") {
				t.Fatalf("reason = %q, want a clear rejection reason", resp.Outcome.Reason)
			}
			if resp.Outcome.Planner != nil {
				t.Fatalf("a rejected request must not carry a planner object: %+v", resp.Outcome.Planner)
			}
		})
	}
}

// TestWriteBeforeInvalidPlannerRequestPreservesMutationEvidence proves that when the model
// performs a real repository write and then returns an invalid needs_planner request in the
// same invocation, the rejection is an explicit failure that preserves the observed mutation
// as evidence and never creates a planner question. The rejection message states only that
// the request was rejected — it does not claim that no work was performed.
func TestWriteBeforeInvalidPlannerRequestPreservesMutationEvidence(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, "seed.txt", "seed\n")
	commitAll(t, dir, "seed")

	// The model first performs a real write, then returns an invalid planner request
	// (a blank question) that this invocation cannot honor.
	fake, srv := newFakeOllama(t,
		`{"tool":"create_file","args":{"path":"new.txt","content":"created"}}`,
		`{"status":"needs_planner","planner":{"question":"  ","options":["a"]}}`,
	)
	h := New(testConfig(srv.URL), dir)
	req := agent.Request{Capability: agent.Implement, Task: "t", Input: "i", OutputRequirements: "o", PlannerAvailable: true}
	ev := &mutationEvidence{}
	content, _, err := h.CompleteWithEvidence(context.Background(), req, ev)
	if err != nil {
		t.Fatal(err)
	}
	if fake.count() != 2 {
		t.Fatalf("model calls = %d, want 2 (write then planner request)", fake.count())
	}

	out := agent.ParseOutcome(content)
	if out == nil || out.Status != agent.OutcomeFailed {
		t.Fatalf("outcome = %+v content=%s, want an explicit failed outcome", out, content)
	}
	if !strings.Contains(out.Reason, "planner request rejected") || !strings.Contains(out.Reason, "malformed planner request") {
		t.Fatalf("reason = %q, want a planner-rejection reason", out.Reason)
	}
	if strings.Contains(out.Reason, "no work was performed") {
		t.Fatalf("rejection reason must not claim no work was performed: %q", out.Reason)
	}
	if out.Planner != nil {
		t.Fatalf("a rejected request must not carry a planner object: %+v", out.Planner)
	}
	if out.ChangesExpected {
		t.Fatalf("a rejected planner request must not claim a repository change: %+v", out)
	}

	// The write that happened before the invalid request is preserved as evidence: the
	// rejection does not discard it, and it is never rewritten as a success.
	if !ev.observed {
		t.Fatalf("mutation evidence lost: %+v", ev)
	}
	if paths := ev.mutationPaths(); len(paths) != 1 || paths[0] != "new.txt" {
		t.Fatalf("mutationPaths = %v, want [new.txt]", paths)
	}
}

// TestMalformedPlannerRequestIsAnExplicitFailure proves a malformed planner request is an
// explicit failure even when the planner is enabled for IMPLEMENT: it is never completed and
// never creates a question.
func TestMalformedPlannerRequestIsAnExplicitFailure(t *testing.T) {
	cases := []struct{ name, final, want string }{
		{"no planner object", `{"status":"needs_planner"}`, "no planner object"},
		{"empty question", `{"status":"needs_planner","planner":{"question":"  ","options":["a"]}}`, "no question"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			gitInit(t, dir)
			_, srv := newFakeOllama(t, tc.final)
			req := agent.Request{Capability: agent.Implement, Task: "t", Input: "i", OutputRequirements: "o", PlannerAvailable: true}
			resp, err := NewNativeAgent(testConfig(srv.URL), dir, nil).Generate(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeFailed {
				t.Fatalf("outcome = %+v, want an explicit failed outcome", resp.Outcome)
			}
			if !strings.Contains(resp.Outcome.Reason, "malformed planner request") || !strings.Contains(resp.Outcome.Reason, tc.want) {
				t.Fatalf("reason = %q, want malformed/%s", resp.Outcome.Reason, tc.want)
			}
			if resp.Outcome.Planner != nil {
				t.Fatalf("a malformed request must not carry a planner object: %+v", resp.Outcome.Planner)
			}
		})
	}
}

// TestNeedsHumanStaysSeparateUnderThePlannerContract proves a human approval request is
// neither satisfied nor reclassified by the planner outcome.
func TestNeedsHumanStaysSeparateUnderThePlannerContract(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	_, srv := newFakeOllama(t, `{"status":"needs_human","reason":"approve the schema change"}`)
	req := agent.Request{Capability: agent.Implement, Task: "t", Input: "i", OutputRequirements: "o", PlannerAvailable: true}
	resp, err := NewNativeAgent(testConfig(srv.URL), dir, nil).Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeNeedsHuman {
		t.Fatalf("outcome = %+v, want needs_human", resp.Outcome)
	}
	if resp.Outcome.Planner != nil {
		t.Fatalf("needs_human must not carry a planner request: %+v", resp.Outcome.Planner)
	}
}
