package planner

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// boundaryPlanJSON is the dogfood scenario: a controller-to-SOP boundary whose
// upstream capabilities were discovered before synthesis. One read capability
// exists, one is partial, and two SOP-owned lifecycle operations are missing but
// explicitly owned — so the plan records the gaps and proceeds.
const boundaryPlanJSON = `{
  "project": "SOP Controller",
  "summary": "Add human control to the controller dashboard.",
  "capabilities": [
    {"name": "GetTasks", "status": "EXISTS", "location": "internal/sopclient", "evidence": "Client.Project reads the task graph"},
    {"name": "CancelRun", "status": "MISSING", "owner": "SOP", "gap": "SOP has no cancellation operation", "resolution": "the controller exposes CancelRun as unsupported until SOP provides it"},
    {"name": "ApproveTask", "status": "MISSING", "owner": "SOP", "gap": "SOP has no approval operation", "resolution": "the controller exposes ApproveTask as unsupported until SOP provides it"},
    {"name": "TaskCommand", "status": "PARTIAL", "owner": "SOP", "evidence": "read operations exist; no command counterpart", "gap": "no command operation for the read model", "resolution": "scope the command stage to the read contract until SOP adds the command"}
  ],
  "assumptions": [
    {"assumption": "SOP owns lifecycle state transitions", "evidence": "internal/scheduler", "consequence": "the controller must not implement missing lifecycle operations itself"}
  ],
  "stages": [
    {"id": "S1", "title": "Boundary contract", "objective": "Define the controller-to-SOP boundary.", "dependencies": [], "requires": ["GetTasks", "CancelRun", "ApproveTask"], "deliverables": ["boundary doc"], "acceptance_criteria": ["boundary documented"]},
    {"id": "S2", "title": "Command wiring", "objective": "Wire controller commands to SOP.", "dependencies": ["S1"], "requires": ["TaskCommand"], "deliverables": ["handlers"], "acceptance_criteria": ["commands resolve"]}
  ]
}`

// TestGenerateAcceptsDiscoveredCapabilities covers the EXISTS, MISSING-with-owner,
// and PARTIAL cases at once: discovery found the real state, and synthesis planned
// against it without a human decision.
func TestGenerateAcceptsDiscoveredCapabilities(t *testing.T) {
	plan, err := New(&stubAgent{content: boundaryPlanJSON}).Generate(context.Background(), "controller PRD")
	if err != nil {
		t.Fatalf("Generate failed on a plan with owned capability gaps: %v", err)
	}

	got := map[string]Capability{}
	for _, c := range plan.Capabilities {
		got[c.Name] = c
	}
	if len(got) != 4 {
		t.Fatalf("capabilities = %d, want 4", len(got))
	}
	if got["GetTasks"].Status != CapabilityExists {
		t.Errorf("GetTasks status = %q, want EXISTS", got["GetTasks"].Status)
	}
	if c := got["CancelRun"]; c.Status != CapabilityMissing || c.Owner != "SOP" {
		t.Errorf("CancelRun = %+v, want MISSING owned by SOP", c)
	}
	if c := got["ApproveTask"]; c.Status != CapabilityMissing || c.Owner != "SOP" {
		t.Errorf("ApproveTask = %+v, want MISSING owned by SOP", c)
	}
	if c := got["TaskCommand"]; c.Status != CapabilityPartial {
		t.Errorf("TaskCommand status = %q, want PARTIAL (the read/command split)", c.Status)
	}

	// The existing capability is referenced by the stage that depends on it; the
	// missing ones are not pretended to exist but are still depended on under a
	// recorded resolution.
	if !contains(plan.Stages[0].Requires, "GetTasks") {
		t.Errorf("stage S1 requires = %v, want it to reference GetTasks", plan.Stages[0].Requires)
	}
	if len(plan.Assumptions) != 1 || !strings.Contains(plan.Assumptions[0].Assumption, "lifecycle") {
		t.Errorf("assumptions = %+v, want the recorded ownership assumption", plan.Assumptions)
	}

	handled, needsHuman := plan.CapabilityGaps()
	if len(needsHuman) != 0 {
		t.Errorf("needsHuman = %+v, want none: every gap is owned", needsHuman)
	}
	if len(handled) != 3 {
		t.Errorf("handled gaps = %d, want 3 (CancelRun, ApproveTask, TaskCommand)", len(handled))
	}
}

// TestGeneratePreservesUnknownCapability proves an UNKNOWN finding stays explicit
// and is not silently promoted to EXISTS.
func TestGeneratePreservesUnknownCapability(t *testing.T) {
	const unknown = `{"project":"p","summary":"s","capabilities":[{"name":"Telemetry","status":"UNKNOWN","evidence":"no provider adapter found"}],"stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"]}]}`
	plan, err := New(&stubAgent{content: unknown}).Generate(context.Background(), "a prd")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(plan.Capabilities) != 1 || plan.Capabilities[0].Status != CapabilityUnknown {
		t.Fatalf("capabilities = %+v, want one UNKNOWN finding", plan.Capabilities)
	}
}

// TestGenerateRejectsRequiringUnknownCapability proves a stage cannot be built on
// a capability discovery could not verify.
func TestGenerateRejectsRequiringUnknownCapability(t *testing.T) {
	const reqUnknown = `{"project":"p","summary":"s","capabilities":[{"name":"Telemetry","status":"UNKNOWN"}],"stages":[{"id":"S1","title":"t","objective":"o","requires":["Telemetry"],"acceptance_criteria":["a"]}]}`
	_, err := New(&stubAgent{content: reqUnknown}).Generate(context.Background(), "a prd")
	if err == nil {
		t.Fatal("expected a validation error for depending on an UNKNOWN capability")
	}
	if !strings.Contains(err.Error(), "UNKNOWN") {
		t.Errorf("error = %q, want it to name the UNKNOWN capability", err)
	}
}

// TestGenerateGenuineAmbiguityNeedsHuman proves a required capability with no
// determined owner is surfaced as a human decision, and that the ambiguity is not
// sent back to the agent for a repair round that could invent an owner.
func TestGenerateGenuineAmbiguityNeedsHuman(t *testing.T) {
	const ambiguous = `{"project":"p","summary":"s","capabilities":[{"name":"Reindex","status":"MISSING","evidence":"no reindex operation found"}],"stages":[{"id":"S1","title":"t","objective":"o","requires":["Reindex"],"acceptance_criteria":["a"]}]}`

	stub := &scriptedAgent{responses: []string{ambiguous, "SHOULD NOT BE USED"}}
	_, err := New(stub).Generate(context.Background(), "a prd")
	if err == nil {
		t.Fatal("expected NEEDS_HUMAN for an ownerless required capability")
	}
	if !IsNeedsHuman(err) {
		t.Errorf("err = %v, want a CapabilityGapError", err)
	}
	if !strings.Contains(err.Error(), "NEEDS_HUMAN") || !strings.Contains(err.Error(), "Reindex") {
		t.Errorf("err = %q, want a NEEDS_HUMAN diagnostic naming Reindex", err)
	}
	if len(stub.requests) != 1 {
		t.Errorf("agent calls = %d, want 1: a genuine ambiguity is not a repair round", len(stub.requests))
	}
}

// TestGeneratePlanWithoutCapabilitiesIsUnaffected proves the inventory is
// optional: a plan with no upstream dependencies needs no capability discovery.
func TestGeneratePlanWithoutCapabilitiesIsUnaffected(t *testing.T) {
	plan, err := New(&stubAgent{content: validJSON}).Generate(context.Background(), "a prd")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(plan.Capabilities) != 0 || len(plan.Assumptions) != 0 {
		t.Errorf("capabilities/assumptions = %+v/%+v, want empty", plan.Capabilities, plan.Assumptions)
	}
	if handled, needsHuman := plan.CapabilityGaps(); len(handled) != 0 || len(needsHuman) != 0 {
		t.Errorf("gaps = %v/%v, want none", handled, needsHuman)
	}
}

// TestGeneratePromptRequiresCapabilityDiscovery proves the output contract and
// task instruct the agent to inventory capabilities, so synthesis consumes
// discovery instead of assuming the architecture.
func TestGeneratePromptRequiresCapabilityDiscovery(t *testing.T) {
	stub := &stubAgent{content: validJSON}
	if _, err := New(stub).Generate(context.Background(), "a prd"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(stub.request.OutputRequirements, "capabilities") ||
		!strings.Contains(stub.request.OutputRequirements, "MISSING") {
		t.Errorf("output contract does not describe the capability inventory:\n%s", stub.request.OutputRequirements)
	}
	if !strings.Contains(stub.request.Task, "capabilit") {
		t.Errorf("task does not ask for capability discovery:\n%s", stub.request.Task)
	}
}

// TestValidateCapabilityInventory covers the deterministic guards on the
// inventory and the stages that depend on it.
func TestValidateCapabilityInventory(t *testing.T) {
	base := func() *Plan {
		return &Plan{
			Project: "P", Summary: "S",
			Stages: []Stage{{ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"}}},
		}
	}

	cases := []struct {
		name    string
		mutate  func(*Plan)
		wantErr string
	}{
		{"unknown status", func(p *Plan) {
			p.Capabilities = []Capability{{Name: "C", Status: "PROBABLY"}}
		}, "unknown status"},
		{"duplicate capability", func(p *Plan) {
			p.Capabilities = []Capability{{Name: "C", Status: CapabilityExists}, {Name: "C", Status: CapabilityMissing}}
		}, "duplicate capability"},
		{"empty name", func(p *Plan) {
			p.Capabilities = []Capability{{Name: " ", Status: CapabilityExists}}
		}, "empty name"},
		{"undeclared requires", func(p *Plan) {
			p.Stages[0].Requires = []string{"Ghost"}
		}, "undeclared capability"},
		{"empty assumption", func(p *Plan) {
			p.Assumptions = []Assumption{{Assumption: "  "}}
		}, "assumption 0 is empty"},
		{"lowercase status is accepted", func(p *Plan) {
			p.Capabilities = []Capability{{Name: "C", Status: "exists"}}
			p.Stages[0].Requires = []string{"C"}
		}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			tc.mutate(p)
			err := p.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestCompileSurfacesCapabilityAmbiguity proves the NEEDS_HUMAN policy also
// applies when a document is normalized by the agent.
func TestCompileSurfacesCapabilityAmbiguity(t *testing.T) {
	const ambiguous = `{"project":"p","summary":"s","capabilities":[{"name":"Reindex","status":"MISSING"}],"stages":[{"id":"S1","title":"t","objective":"o","requires":["Reindex"],"acceptance_criteria":["a"]}]}`
	_, err := New(fakeAgent{content: ambiguous}).Compile(context.Background(), "not a plan")
	if err == nil || !IsNeedsHuman(err) {
		t.Fatalf("Compile() = %v, want a NEEDS_HUMAN capability gap", err)
	}
}

// TestCapabilityGapsClassification pins the handled/needs-human split.
func TestCapabilityGapsClassification(t *testing.T) {
	plan := &Plan{
		Project: "P", Summary: "S",
		Capabilities: []Capability{
			{Name: "Owned", Status: CapabilityMissing, Owner: "SOP"},
			{Name: "Resolved", Status: CapabilityMissing, Resolution: "represent as unsupported"},
			{Name: "Ownerless", Status: CapabilityMissing},
			{Name: "Present", Status: CapabilityExists},
			{Name: "Unrequired", Status: CapabilityMissing}, // inventory, not required
		},
		Stages: []Stage{{ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"}, Requires: []string{"Owned", "Resolved", "Ownerless", "Present"}}},
	}
	handled, needsHuman := plan.CapabilityGaps()
	if got := names(handled); !reflect.DeepEqual(got, []string{"Owned", "Resolved"}) {
		t.Errorf("handled = %v, want [Owned Resolved]", got)
	}
	if got := names(needsHuman); !reflect.DeepEqual(got, []string{"Ownerless"}) {
		t.Errorf("needsHuman = %v, want [Ownerless]", got)
	}
}

// TestPlanMarkdownRoundTripsCapabilities proves the human rendering carries the
// capability inventory, assumptions, and per-stage requirements, and that they
// parse back.
func TestPlanMarkdownRoundTripsCapabilities(t *testing.T) {
	original := &Plan{
		Project: "P", Summary: "S",
		Capabilities: []Capability{
			{Name: "CancelRun", Status: CapabilityMissing, Owner: "SOP", Evidence: "no `sop cancel`", Gap: "no cancellation op", Resolution: "represent as unsupported"},
			{Name: "GetTasks", Status: CapabilityExists, Location: "internal/sopclient"},
		},
		Assumptions: []Assumption{{Assumption: "SOP owns lifecycle", Evidence: "internal/scheduler", Consequence: "controller delegates"}},
		Stages: []Stage{{
			ID: "S1", Title: "Boundary", Objective: "Define it.",
			Requires: []string{"CancelRun", "GetTasks"}, AcceptanceCriteria: []string{"documented"},
		}},
	}

	got, err := PlanFromMarkdown(original.RenderMarkdown())
	if err != nil {
		t.Fatalf("PlanFromMarkdown failed: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("round-tripped plan invalid: %v", err)
	}

	if len(got.Capabilities) != 2 {
		t.Fatalf("capabilities = %+v, want 2", got.Capabilities)
	}
	if got.Capabilities[0] != original.Capabilities[0] {
		t.Errorf("capability[0] = %+v, want %+v", got.Capabilities[0], original.Capabilities[0])
	}
	if got.Capabilities[1] != original.Capabilities[1] {
		t.Errorf("capability[1] = %+v, want %+v", got.Capabilities[1], original.Capabilities[1])
	}
	if !reflect.DeepEqual(got.Assumptions, original.Assumptions) {
		t.Errorf("assumptions = %+v, want %+v", got.Assumptions, original.Assumptions)
	}
	if !reflect.DeepEqual(got.Stages[0].Requires, original.Stages[0].Requires) {
		t.Errorf("requires = %v, want %v", got.Stages[0].Requires, original.Stages[0].Requires)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func names(caps []Capability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, c.Name)
	}
	return out
}
