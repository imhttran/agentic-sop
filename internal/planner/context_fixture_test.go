package planner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// providerDiscoveryPlan is the fixture that represents the real
// provider-discovery failure semantics without depending on Clef: a compiled plan
// whose capability inventory declares the toolchain and the decision seam EXISTS and
// an MLX runtime UNKNOWN, and whose PROV-001 discovery task names a provider
// registration/factory and existing adapters (Nimble, Julia) in its prose while
// requiring only the toolchain.
func providerDiscoveryPlan(t *testing.T) *Plan {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "provider-discovery", "PLAN.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	plan, err := PlanFromMarkdown(string(data))
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if plan == nil || len(plan.Stages) == 0 {
		t.Fatal("fixture did not compile into a plan")
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("fixture plan invalid: %v", err)
	}
	return plan
}

// TestProviderDiscoveryFixtureCompilesDeterministically proves the real-shaped plan
// compiles deterministically with no model (the compiler boundary), keeps the
// capability inventory authoritative, and models the discovery task's only
// prerequisite as the declared toolchain: nothing the task exists to discover is a
// requires entry.
func TestProviderDiscoveryFixtureCompilesDeterministically(t *testing.T) {
	first := providerDiscoveryPlan(t)
	second := providerDiscoveryPlan(t)

	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatalf("compilation is not deterministic:\n%s\n%s", a, b)
	}

	status := map[string]CapabilityStatus{}
	for _, c := range first.Capabilities {
		status[c.Name] = c.Status
	}
	if status["Go build/test toolchain"] != CapabilityExists || status["Provider-neutral decision seam"] != CapabilityExists {
		t.Errorf("authoritative inventory = %v, want the toolchain and seam EXISTS", status)
	}
	if status["MLX runtime"] != CapabilityUnknown {
		t.Errorf("MLX runtime status = %q, want UNKNOWN", status["MLX runtime"])
	}

	var prov001 *Stage
	for i := range first.Stages {
		if first.Stages[i].ID == "PROV-001" {
			prov001 = &first.Stages[i]
		}
	}
	if prov001 == nil {
		t.Fatal("PROV-001 stage not found")
	}
	if len(prov001.Requires) != 1 || prov001.Requires[0] != "Go build/test toolchain" {
		t.Errorf("PROV-001 requires = %v, want only the declared toolchain", prov001.Requires)
	}
	for _, forbidden := range []string{"registration", "factory", "Nimble", "Julia", "provider interface", "configuration", "test structure"} {
		for _, r := range prov001.Requires {
			if strings.Contains(strings.ToLower(r), strings.ToLower(forbidden)) {
				t.Errorf("discovery target %q became a PROV-001 prerequisite: %q", forbidden, r)
			}
		}
	}

	handled, needsHuman := first.CapabilityGaps()
	if len(needsHuman) != 0 {
		t.Errorf("needsHuman = %+v, want none (the compiled plan is self-contained)", needsHuman)
	}
	for _, c := range handled {
		if c.Status == CapabilityUnknown && c.Name == "MLX runtime" {
			// The UNKNOWN runtime is informational unless a stage requires it; the
			// fixture does not, so it must not appear here.
			t.Errorf("the informational UNKNOWN MLX runtime was treated as a required gap")
		}
	}
}

// observedUnsafeTaskPlan is the shape the task-level planner produced in the real
// reproduction: it re-derives a capability the compiled plan already declares EXISTS
// as UNKNOWN and requires it, which deterministic validation rejects and returns to
// the agent for correction (the recurring repair).
const observedUnsafeTaskPlan = `{
  "project": "PROV-001 — Capture repository truth",
  "summary": "Discovery and report only.",
  "capabilities": [
    {"name":"Go build/test toolchain","status":"UNKNOWN"},
    {"name":"Provider registration/factory and CLI selection","status":"PARTIAL","owner":"wiring layer"}
  ],
  "stages": [
    {"id":"S1","title":"Capture","objective":"Inspect the repository.","requires":["Go build/test toolchain"],"acceptance_criteria":["recorded"]}
  ]
}`

// TestProviderDiscoveryFixtureContextPreventsRecurringRepair proves the fix at the
// real boundary: given the fixture's authoritative context, the same unsafe-shaped
// model output validates on the first response with no repair, the known capability
// keeps its authoritative EXISTS status, and the task's requirement resolves to the
// authoritative declaration. Without the context it is the recurring-repair shape.
func TestProviderDiscoveryFixtureContextPreventsRecurringRepair(t *testing.T) {
	compiled := providerDiscoveryPlan(t)
	pctx := PlanContext{Capabilities: compiled.Capabilities}
	for _, s := range compiled.Stages {
		if s.ID == "PROV-001" {
			pctx.Requires = s.Requires
		}
	}
	taskText := "# PROV-001 — Capture repository truth\n\nInspect and record the repository's provider registration/factory and existing adapters.\n"

	// After: authoritative context supplied → no repair, authoritative status kept.
	scripted := &scriptedAgent{responses: []string{observedUnsafeTaskPlan}}
	plan, err := New(scripted).GenerateWithContext(context.Background(), taskText, pctx)
	if err != nil {
		t.Fatalf("GenerateWithContext failed: %v", err)
	}
	if len(scripted.requests) != 1 {
		t.Errorf("agent requests = %d, want 1 (no repair with the authoritative context)", len(scripted.requests))
	}
	for _, c := range plan.Capabilities {
		if c.Name == "Go build/test toolchain" && c.Status != CapabilityExists {
			t.Errorf("the authoritative toolchain was downgraded to %q", c.Status)
		}
	}
	// The discovery target may remain informational inventory, but it must not become
	// a prerequisite: no stage may require it.
	for _, s := range plan.Stages {
		for _, r := range s.Requires {
			if strings.Contains(r, "registration/factory") {
				t.Errorf("the discovery target became a prerequisite: %q", r)
			}
		}
	}

	// Before: no context → deterministic validation rejects the UNKNOWN requirement.
	// Three identical responses model the bounded repair loop, which never converges.
	before := &scriptedAgent{responses: []string{observedUnsafeTaskPlan, observedUnsafeTaskPlan, observedUnsafeTaskPlan}}
	_, err = New(before).Generate(context.Background(), taskText)
	if err == nil {
		t.Fatal("expected the no-context path to fail on the re-derived UNKNOWN requirement")
	}
	if !strings.Contains(err.Error(), "UNKNOWN") {
		t.Errorf("no-context err = %v, want the UNKNOWN-requirement rejection", err)
	}
	if len(before.requests) < 2 {
		t.Errorf("no-context requests = %d, want the recurring repair loop", len(before.requests))
	}
}

// TestProviderDiscoveryFixturePlanningIsStable proves repeated task-level planning
// with the same authoritative input reaches the same result, so the fix removes the
// nondeterminism the defect manifested as.
func TestProviderDiscoveryFixturePlanningIsStable(t *testing.T) {
	compiled := providerDiscoveryPlan(t)
	pctx := PlanContext{Capabilities: compiled.Capabilities}
	for _, s := range compiled.Stages {
		if s.ID == "PROV-001" {
			pctx.Requires = s.Requires
		}
	}

	var results []string
	for i := 0; i < 3; i++ {
		plan, err := New(&scriptedAgent{responses: []string{observedUnsafeTaskPlan}}).
			GenerateWithContext(context.Background(), "task", pctx)
		if err != nil {
			t.Fatalf("iteration %d failed: %v", i, err)
		}
		stages, _ := json.Marshal(plan.Stages)
		caps, _ := json.Marshal(plan.Capabilities)
		results = append(results, string(stages)+"|"+string(caps))
	}
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("planning is not stable across repetitions:\n%q\n%q", results[0], results[i])
		}
	}
}
