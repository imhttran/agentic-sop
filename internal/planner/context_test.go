package planner

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// capContext is the authoritative compiled capability inventory used by the
// regression tests: two capabilities that already exist (one with a cosmetic
// spelling trap) and one genuinely unknown external runtime the plan records
// informationally.
func capContext(requires ...string) PlanContext {
	return PlanContext{
		Capabilities: []Capability{
			{Name: "Go toolchain", Status: CapabilityExists, Evidence: "go version go1.27.1"},
			{Name: "agentic-sop decision seam", Status: CapabilityExists, Evidence: "HEAD 1153913"},
			{Name: "MLX runtime", Status: CapabilityUnknown, Gap: "no native decision semantics"},
		},
		Requires: requires,
	}
}

// taskPlanJSON builds a minimal valid task-level plan for the tests.
func taskPlanJSON(capabilities string, stageBody string) string {
	return `{"project":"p","summary":"s","capabilities":[` + capabilities + `],"stages":[` + stageBody + `]}`
}

// The task prose mirrors the real CLEF-001 reproduction: a discovery task whose
// instructions name a provider registration/factory and existing adapters. Nothing
// in the prose is an external prerequisite.
const discoveryTask = `# CLEF-001 — Capture Adapter Repository Truth

Establish the exact current state of the repository.
- the current provider interface;
- existing provider implementations, including Nimble and Julia where present;
- the repository's current provider registration/factory and CLI selection
  mechanism;
- configuration and provider enablement behavior;
- the existing test structure and package layout.

## Acceptance criteria
- Repository state is recorded.
- The provider interface and request/result types are identified with file/symbol evidence.
- No production code is changed.`

// TestContextReachesPlanningBoundary proves the authoritative compiled capability
// inventory is delivered to the planning boundary: the agent request carries every
// authoritative capability name, the task's compiled requires, and the discovery
// rule. This is the primary fix — the planner now has the compiled capability model.
func TestContextReachesPlanningBoundary(t *testing.T) {
	stub := &stubAgent{content: taskPlanJSON("", `{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"]}`)}
	pctx := capContext("Go toolchain")

	if _, err := New(stub).GenerateWithContext(context.Background(), discoveryTask, pctx); err != nil {
		t.Fatalf("GenerateWithContext failed: %v", err)
	}

	input := stub.request.Input
	for _, want := range []string{
		"Authoritative plan context",
		"Go toolchain — EXISTS",
		"agentic-sop decision seam — EXISTS",
		"MLX runtime — UNKNOWN",
		"Capabilities this task's compiled stage already requires:",
		"do NOT declare it as\na capability or add it to \"requires\"",
	} {
		if !strings.Contains(input, want) {
			t.Errorf("authoritative context missing from the planning input: %q", want)
		}
	}
	if !strings.Contains(input, discoveryTask) {
		t.Errorf("the original task text must still be present in the planning input")
	}
}

// TestExistingCapabilityStaysExisting proves an authoritative EXISTS capability is
// never downgraded: a generated plan that re-declares a known capability as UNKNOWN
// (which would fail validation and trigger recurring capability repair) is
// reconciled to the authoritative EXISTS entry before validation, so it validates on
// the first response with no repair.
func TestExistingCapabilityStaysExisting(t *testing.T) {
	// The stage requires a capability the plan declares UNKNOWN. Without the
	// authoritative context this is the exact recurring-repair shape.
	content := taskPlanJSON(
		`{"name":"Go toolchain","status":"UNKNOWN"}`,
		`{"id":"S1","title":"t","objective":"o","requires":["Go toolchain"],"acceptance_criteria":["a"]}`,
	)
	scripted := &scriptedAgent{responses: []string{content}}
	plan, err := New(scripted).GenerateWithContext(context.Background(), "task", capContext("Go toolchain"))
	if err != nil {
		t.Fatalf("GenerateWithContext failed: %v", err)
	}
	if len(scripted.responses) != 0 || len(scripted.requests) != 1 {
		t.Fatalf("agent requests = %d, want 1 (no repair for a known capability)", len(scripted.requests))
	}
	if got := plan.Capabilities[0].Status; got != CapabilityExists {
		t.Errorf("capability status = %q, want EXISTS from the authoritative inventory", got)
	}
}

// TestKnownCapabilityNotUnknownByCosmeticWording proves a cosmetic name variant
// does not create a parallel UNKNOWN capability: a generated "Go  Toolchain"
// (different case and spacing) is matched to the authoritative "Go toolchain" by the
// same normalization validateCapabilities and CapabilityGaps use, and the stage
// requirement is canonicalized to the authoritative spelling.
func TestKnownCapabilityNotUnknownByCosmeticWording(t *testing.T) {
	content := taskPlanJSON(
		`{"name":"Go  Toolchain","status":"UNKNOWN"}`,
		`{"id":"S1","title":"t","objective":"o","requires":["Go  Toolchain"],"acceptance_criteria":["a"]}`,
	)
	plan, err := New(&scriptedAgent{responses: []string{content}}).
		GenerateWithContext(context.Background(), "task", capContext("Go toolchain"))
	if err != nil {
		t.Fatalf("GenerateWithContext failed: %v", err)
	}
	if got := plan.Capabilities[0].Name; got != "Go toolchain" {
		t.Errorf("capability name = %q, want the authoritative %q", got, "Go toolchain")
	}
	if got := plan.Stages[0].Requires[0]; got != "Go toolchain" {
		t.Errorf("requirement = %q, want it canonicalized to the authoritative name", got)
	}
	if got := len(plan.Capabilities); got != 1 {
		t.Errorf("capability count = %d, want 1 (no parallel entry)", got)
	}
}

// TestRequirementResolutionInjectsAuthoritativeDeclaration proves a stage
// requirement that resolves to an authoritative capability is guaranteed declared,
// even when the generated plan required it without declaring it, so validation sees
// one authoritative capability rather than triggering a repair.
func TestRequirementResolutionInjectsAuthoritativeDeclaration(t *testing.T) {
	content := taskPlanJSON(
		"",
		`{"id":"S1","title":"t","objective":"o","requires":["agentic-sop decision seam"],"acceptance_criteria":["a"]}`,
	)
	plan, err := New(&scriptedAgent{responses: []string{content}}).
		GenerateWithContext(context.Background(), "task", capContext("agentic-sop decision seam"))
	if err != nil {
		t.Fatalf("GenerateWithContext failed: %v", err)
	}
	if len(plan.Capabilities) != 1 || plan.Capabilities[0].Name != "agentic-sop decision seam" {
		t.Errorf("capabilities = %+v, want the authoritative declaration injected", plan.Capabilities)
	}
	if plan.Capabilities[0].Status != CapabilityExists {
		t.Errorf("injected capability status = %q, want EXISTS", plan.Capabilities[0].Status)
	}
}

// TestDiscoveryTargetsAreNotPrerequisites proves repository discovery named in the
// task prose does not become an external prerequisite. The planner is given the
// authoritative context, describes the discovery in the stage objective, and emits
// no capability or requires entry for it. Each case is a discovery target from the
// real CLEF-001 reproduction; none is a runtime, permission, tool, service, or
// artifact supplied before the work begins.
func TestDiscoveryTargetsAreNotPrerequisites(t *testing.T) {
	for _, target := range []string{
		"the repository's current provider registration/factory and CLI selection mechanism",
		"existing provider implementations, including Nimble and Julia where present",
		"configuration and provider enablement behavior",
		"the existing test structure and package layout",
		"the current provider interface and request/result types",
	} {
		objective := "Inspect and record " + target + " from source."
		content := taskPlanJSON(
			`{"name":"Go toolchain","status":"EXISTS"}`,
			`{"id":"S1","title":"Capture truth","objective":`+mustJSON(objective)+`,"requires":["Go toolchain"],"acceptance_criteria":["recorded"]}`,
		)
		plan, err := New(&scriptedAgent{responses: []string{content}}).
			GenerateWithContext(context.Background(), discoveryTask, capContext("Go toolchain"))
		if err != nil {
			t.Fatalf("%s: GenerateWithContext failed: %v", target, err)
		}
		for _, r := range plan.Stages[0].Requires {
			if strings.Contains(r, target) {
				t.Errorf("%s: discovery target became a prerequisite: %q", target, r)
			}
		}
		for _, c := range plan.Capabilities {
			if strings.Contains(c.Name, target) {
				t.Errorf("%s: discovery target became a capability: %q", target, c.Name)
			}
		}
	}
}

// TestGenuineMissingExternalCapabilityStillGoverned proves the fix preserves the
// governed missing-capability path: a stage that requires a genuinely missing
// external capability the authoritative inventory does not contain still reaches the
// existing ownership gate (a human decision) rather than being silently authorized
// or silently dropped.
func TestGenuineMissingExternalCapabilityStillGoverned(t *testing.T) {
	content := taskPlanJSON(
		`{"name":"Ollama SystemOne endpoint","status":"MISSING","gap":"not reachable"}`,
		`{"id":"S1","title":"t","objective":"o","requires":["Ollama SystemOne endpoint"],"acceptance_criteria":["a"]}`,
	)
	_, err := New(&scriptedAgent{responses: []string{content}}).
		GenerateWithContext(context.Background(), "task", capContext("Go toolchain"))
	if err == nil {
		t.Fatal("expected a genuinely missing external capability to reach the governed ownership gate")
	}
	if !IsNeedsHuman(err) {
		t.Errorf("err = %v, want the governed CapabilityGapError (NEEDS_HUMAN)", err)
	}
}

// TestAmbiguousUndeclaredCapabilityStillFailClosed proves an unverifiable capability
// the authoritative inventory does not contain is not authorized: a stage requiring
// an UNKNOWN capability is still rejected by deterministic validation (fail closed),
// so the deterministic layer never silently downgrades it to task-local work.
func TestAmbiguousUndeclaredCapabilityStillFailClosed(t *testing.T) {
	content := taskPlanJSON(
		`{"name":"Mystery runtime","status":"UNKNOWN","owner":"environment"}`,
		`{"id":"S1","title":"t","objective":"o","requires":["Mystery runtime"],"acceptance_criteria":["a"]}`,
	)
	// A single response is rejected; the planner asks for repairs until the bound.
	scripted := &scriptedAgent{responses: []string{content, content, content}}
	_, err := New(scripted).GenerateWithContext(context.Background(), "task", capContext("Go toolchain"))
	if err == nil {
		t.Fatal("expected an undeclared UNKNOWN requirement to fail closed")
	}
	if !strings.Contains(err.Error(), "UNKNOWN") {
		t.Errorf("err = %v, want the deterministic UNKNOWN-requirement rejection", err)
	}
	if IsNeedsHuman(err) {
		t.Errorf("err = %v, want a repairable validation error, not the ambiguous-owner human gate", err)
	}
}

// TestContextDoesNotAlterTaskIdentityOrGraph proves the deterministic reconciliation
// touches only capability semantics: stage ids, titles, dependency edges, execution
// mode, deliverables, and acceptance criteria survive generation-with-context
// unchanged.
func TestContextDoesNotAlterTaskIdentityOrGraph(t *testing.T) {
	content := taskPlanJSON(
		`{"name":"Go toolchain","status":"UNKNOWN"}`,
		`{"id":"CLEF-001","title":"Capture truth","objective":"o","dependencies":["CLEF-000"],"requires":["Go toolchain"],"deliverables":["d"],"acceptance_criteria":["a"],"execution_mode":"verify-first"},`+
			`{"id":"CLEF-000","title":"Base","objective":"o","acceptance_criteria":["a"]}`,
	)
	plan, err := New(&scriptedAgent{responses: []string{content}}).
		GenerateWithContext(context.Background(), "task", capContext("Go toolchain"))
	if err != nil {
		t.Fatalf("GenerateWithContext failed: %v", err)
	}
	if plan.Stages[0].ID != "CLEF-001" || plan.Stages[1].ID != "CLEF-000" {
		t.Errorf("stage ids changed: %q, %q", plan.Stages[0].ID, plan.Stages[1].ID)
	}
	if !reflect.DeepEqual(plan.Stages[0].Dependencies, []string{"CLEF-000"}) {
		t.Errorf("dependency graph changed: %v", plan.Stages[0].Dependencies)
	}
	if !plan.Stages[0].ExecutionMode.VerifyFirst() {
		t.Errorf("execution mode changed: %q", plan.Stages[0].ExecutionMode)
	}
	if !reflect.DeepEqual(plan.Stages[0].Deliverables, []string{"d"}) {
		t.Errorf("deliverables changed: %v", plan.Stages[0].Deliverables)
	}
	if !reflect.DeepEqual(plan.Stages[0].AcceptanceCriteria, []string{"a"}) {
		t.Errorf("acceptance criteria changed: %v", plan.Stages[0].AcceptanceCriteria)
	}
}

// TestRepeatedPlanningIsStable proves identical authoritative input plus an
// equivalent planner output is semantically stable at the deterministic boundary:
// two independent generations produce byte-identical stage definitions and
// capability semantics, so nothing in the fix introduces nondeterminism.
func TestRepeatedPlanningIsStable(t *testing.T) {
	content := taskPlanJSON(
		`{"name":"Go  Toolchain","status":"UNKNOWN"},{"name":"MLX runtime","status":"EXISTS"}`,
		`{"id":"S1","title":"t","objective":"o","requires":["Go toolchain"],"acceptance_criteria":["a"]}`,
	)
	gen := func() *Plan {
		plan, err := New(&scriptedAgent{responses: []string{content}}).
			GenerateWithContext(context.Background(), "task", capContext("Go toolchain"))
		if err != nil {
			t.Fatalf("GenerateWithContext failed: %v", err)
		}
		return plan
	}
	first, second := gen(), gen()

	firstCaps, _ := json.Marshal(first.Capabilities)
	secondCaps, _ := json.Marshal(second.Capabilities)
	if string(firstCaps) != string(secondCaps) {
		t.Errorf("capability semantics are not stable:\nfirst:  %s\nsecond: %s", firstCaps, secondCaps)
	}
	firstStages, _ := json.Marshal(first.Stages)
	secondStages, _ := json.Marshal(second.Stages)
	if string(firstStages) != string(secondStages) {
		t.Errorf("stage definitions are not stable:\nfirst:  %s\nsecond: %s", firstStages, secondStages)
	}
}

// TestEmptyContextMatchesGenerate proves the no-context entry point is unchanged:
// Generate and GenerateWithContext(PlanContext{}) issue the same request and produce
// the same plan, so every existing caller keeps its behavior.
func TestEmptyContextMatchesGenerate(t *testing.T) {
	stubA := &stubAgent{content: validJSON}
	stubB := &stubAgent{content: validJSON}

	viaGenerate, err := New(stubA).Generate(context.Background(), "A PRD about books.")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	viaContext, err := New(stubB).GenerateWithContext(context.Background(), "A PRD about books.", PlanContext{})
	if err != nil {
		t.Fatalf("GenerateWithContext failed: %v", err)
	}
	if stubA.request.Input != "A PRD about books." {
		t.Errorf("empty context must not prepend anything; input = %q", stubA.request.Input)
	}
	a, _ := json.Marshal(viaGenerate.Stages)
	b, _ := json.Marshal(viaContext.Stages)
	if string(a) != string(b) {
		t.Errorf("empty-context plan differs from Generate:\n%s\n%s", a, b)
	}
}

// TestPlanContextCarriesNoAuthority proves the context is capability information
// only: it never introduces a provider, model, approval, or lifecycle signal into
// the planning prompt, so provider/model identity cannot affect capability
// semantics.
func TestPlanContextCarriesNoAuthority(t *testing.T) {
	block := capContext("Go toolchain").promptBlock()
	for _, forbidden := range []string{"provider", "model", "approve", "approval", "merge", "commit", "lifecycle"} {
		if strings.Contains(strings.ToLower(block), forbidden) {
			t.Errorf("authoritative context block mentions %q; it must carry capability information only", forbidden)
		}
	}
}

// mustJSON marshals a Go string to a JSON string literal for embedding in a plan.
func mustJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
