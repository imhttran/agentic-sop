package planner

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// countingAgent is an Agent that records every request it receives and never
// produces content. A deterministic compilation must make zero calls, so any
// fallback to the model path shows up as a non-zero count (and, without a
// scripted response, as an error).
type countingAgent struct {
	calls int
}

func (c *countingAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	c.calls++
	return agent.Response{Content: "SHOULD NOT BE USED"}, nil
}

// clefPlan is a structured plan written in the canonical rendered shape using
// multi-segment alphabetic ids (AS-CLEF-001, AS-CLEF-008-S1). It carries a
// completed predecessor artifact, an informational UNKNOWN discovery target with
// no requires gate, and status/validation prose that must not be read as a
// completion signal.
const clefPlan = `# Implementation Plan

## Project

Decision Boundary Readiness

## Summary

Harden the compiler for multi-segment stage ids.

## Capabilities

### Clef plan source — EXISTS

- Location: docs/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md
- Evidence: the plan is checked in and reviewed.

### Planflow equivalence — UNKNOWN

- Evidence: not inspected yet.
- Owner: planflow package (later step).

## AS-CLEF-001 — Widen the stage-id grammar

Accept multi-segment alphabetic ids in the compiler.

### Dependencies

None

### Deliverables

- Updated mdIDToken grammar

### Acceptance Criteria

- AS-CLEF-001 and AS-CLEF-008-S1 parse

### Execution

- done

## AS-CLEF-008 — Clarify the prerequisite rule

State that requires lists externally supplied prerequisites only.

### Dependencies

- AS-CLEF-001

### Status

Done (per the earlier report).

### Deliverables

- Updated prompt text

### Acceptance Criteria

- discovery work is not a requires entry

## AS-CLEF-008-S1 — Compile twice deterministically

Compile a structured plan without invoking the model.

### Dependencies

- AS-CLEF-008

### Validation

` + "`go test ./internal/planner` already passed." + `

### Deliverables

- Compiler hardening tests

### Acceptance Criteria

- zero model calls and byte-identical stage definitions
`

// TestCompileDeterministicClefPlanNoAgentCalls proves a structured plan with
// multi-segment ids compiles deterministically twice: the counting agent records
// zero Generate calls, and the serialized stage definitions are byte-identical
// across runs.
func TestCompileDeterministicClefPlanNoAgentCalls(t *testing.T) {
	agentDouble := &countingAgent{}
	p := New(agentDouble)

	first, err := p.Compile(context.Background(), clefPlan)
	if err != nil {
		t.Fatalf("first Compile failed: %v", err)
	}
	second, err := p.Compile(context.Background(), clefPlan)
	if err != nil {
		t.Fatalf("second Compile failed: %v", err)
	}
	if agentDouble.calls != 0 {
		t.Fatalf("agent calls = %d, want 0 for a deterministically parsed plan", agentDouble.calls)
	}

	firstStages, err := json.Marshal(first.Stages)
	if err != nil {
		t.Fatalf("marshal first stages: %v", err)
	}
	secondStages, err := json.Marshal(second.Stages)
	if err != nil {
		t.Fatalf("marshal second stages: %v", err)
	}
	if string(firstStages) != string(secondStages) {
		t.Errorf("stage definitions are not byte-identical across runs:\nfirst:  %s\nsecond: %s", firstStages, secondStages)
	}

	ids := make([]string, len(first.Stages))
	for i, s := range first.Stages {
		ids[i] = s.ID
	}
	want := []string{"AS-CLEF-001", "AS-CLEF-008", "AS-CLEF-008-S1"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("stage ids = %v, want %v", ids, want)
	}
}

// TestCompileClefPlanPreservesDependenciesAndNeverInfersCompletion proves the
// declared dependency edges survive compilation, the completion mode is read only
// from the explicit Execution field, and status/validation prose never implies
// completion.
func TestCompileClefPlanPreservesDependenciesAndNeverInfersCompletion(t *testing.T) {
	plan, err := New(&countingAgent{}).Compile(context.Background(), clefPlan)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	byID := map[string]Stage{}
	for _, s := range plan.Stages {
		byID[s.ID] = s
	}
	if got := byID["AS-CLEF-008"].Dependencies; !reflect.DeepEqual(got, []string{"AS-CLEF-001"}) {
		t.Errorf("AS-CLEF-008 dependencies = %v, want [AS-CLEF-001]", got)
	}
	if got := byID["AS-CLEF-008-S1"].Dependencies; !reflect.DeepEqual(got, []string{"AS-CLEF-008"}) {
		t.Errorf("AS-CLEF-008-S1 dependencies = %v, want [AS-CLEF-008]", got)
	}

	// The explicit Execution field is the only completion signal.
	if !byID["AS-CLEF-001"].ExecutionMode.Done() {
		t.Errorf("AS-CLEF-001 execution mode = %q, want done from the Execution field", byID["AS-CLEF-001"].ExecutionMode)
	}
	// "Status: Done (per the earlier report)." is documentation, not a completion
	// declaration: the stage must stay in its default (implement) mode.
	if mode := byID["AS-CLEF-008"].ExecutionMode; mode != "" {
		t.Errorf("AS-CLEF-008 execution mode = %q, want empty (Status prose is not a completion signal)", mode)
	}
	// "Validation: go test ... already passed." is likewise documentation.
	if mode := byID["AS-CLEF-008-S1"].ExecutionMode; mode != "" {
		t.Errorf("AS-CLEF-008-S1 execution mode = %q, want empty (Validation prose is not a completion signal)", mode)
	}
}

// TestCompileInformationalUnknownWithoutRequires proves an UNKNOWN discovery
// target may be inventoried as documentation without any stage requiring it: the
// plan compiles and validates, and no stage carries a requires entry for it.
func TestCompileInformationalUnknownWithoutRequires(t *testing.T) {
	plan, err := New(&countingAgent{}).Compile(context.Background(), clefPlan)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	for _, s := range plan.Stages {
		for _, r := range s.Requires {
			if r == "Planflow equivalence" {
				t.Errorf("stage %s requires the informational UNKNOWN capability %q; discovery must not gate execution", s.ID, r)
			}
		}
	}
}

// TestCompletedDependencyIsAStageDependencyNotAnUnknownRequirement proves a
// completed predecessor artifact is modeled as an ordinary stage dependency: the
// dependent stage references the producing stage id and does not declare an UNKNOWN
// capability requirement instead.
func TestCompletedDependencyIsAStageDependencyNotAnUnknownRequirement(t *testing.T) {
	plan, err := New(&countingAgent{}).Compile(context.Background(), clefPlan)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var found bool
	for _, s := range plan.Stages {
		if s.ID != "AS-CLEF-008" {
			continue
		}
		if !reflect.DeepEqual(s.Dependencies, []string{"AS-CLEF-001"}) {
			t.Fatalf("AS-CLEF-008 dependencies = %v, want the completed predecessor [AS-CLEF-001]", s.Dependencies)
		}
		if len(s.Requires) != 0 {
			t.Errorf("AS-CLEF-008 requires = %v, want none: a completed artifact is dependency evidence, not an UNKNOWN requirement", s.Requires)
		}
		found = true
	}
	if !found {
		t.Fatal("AS-CLEF-008 stage not found")
	}
}

// TestRequiredUnknownCapabilityStillRejected proves the hardening did not weaken
// validation: a stage that genuinely requires an UNKNOWN environmental runtime
// capability is still rejected with the deterministic error.
func TestRequiredUnknownCapabilityStillRejected(t *testing.T) {
	doc := "# Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n" +
		"## Capabilities\n\n" +
		"### Container runtime — UNKNOWN\n\n- Owner: environment\n\n" +
		"## AS-CLEF-002 — Build image\n\nBuild it.\n\n" +
		"### Requires\n\n- Container runtime\n\n" +
		"### Acceptance Criteria\n\n- image builds\n"
	_, err := New(&countingAgent{}).Compile(context.Background(), doc)
	if err == nil {
		t.Fatal("expected a genuinely required UNKNOWN capability to be rejected")
	}
	if !strings.Contains(err.Error(), "requires capability") || !strings.Contains(err.Error(), "UNKNOWN") {
		t.Errorf("err = %v, want the existing required-UNKNOWN rejection", err)
	}
}

// TestLegacyStageIDGrammarStillSupported proves the widened grammar retains every
// id shape the repository already uses.
func TestLegacyStageIDGrammarStillSupported(t *testing.T) {
	for in, want := range map[string]string{
		"S001 — Skeleton":              "S001",
		"P5-001 — Recovery vocabulary": "P5-001",
		"PREJEV012-S6 — Sub-stage":     "PREJEV012-S6",
		"AHV2001 --- Correct":          "AHV2001",
		"P3-002 --- Define early":      "P3-002",
		"AS-CLEF-001 — Widen":          "AS-CLEF-001",
		"AS-CLEF-008-S1 — Sub-stage":   "AS-CLEF-008-S1",
		"Tasks":                        "",
	} {
		id, _ := splitStageHeading(in)
		if id != want {
			t.Errorf("splitStageHeading(%q) = %q, want %q", in, id, want)
		}
	}
}

// TestMalformedDependencyStringsStillFail proves ambiguous dependency values are
// still rejected by isStageID/parseDependencies after the grammar widening.
func TestMalformedDependencyStringsStillFail(t *testing.T) {
	for _, value := range []string{"P1-001..P1-003", "all", "P1-001 (see the note)", "P1-002 through P1-004"} {
		doc := "## Project\n\nP\n\n## Summary\n\nS\n\n## Tasks\n\n### P1-001 — One\n\n- **Scope:** x\n- **Depends on:** " + value + "\n- **Acceptance:** it works.\n"
		_, err := PlanFromMarkdown(doc)
		if err == nil {
			t.Errorf("Depends on %q: expected an unreadable-dependency error", value)
			continue
		}
		if !strings.Contains(err.Error(), "unreadable dependency") {
			t.Errorf("Depends on %q: err = %v", value, err)
		}
	}
}
