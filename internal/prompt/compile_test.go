package prompt

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
)

func sampleContext() sopctx.Context {
	return sopctx.Build([]sopctx.Item{
		{Source: sopctx.SourceTask, Identity: "T1", Reason: "the plan the task executes", Priority: sopctx.PriorityLifecycle, Text: "# Implementation Plan\n\n## S001 — do it"},
		{Source: sopctx.SourceRepository, Identity: "internal/x.go", Reason: "changed by this task", Priority: sopctx.PriorityRepository, Text: "internal/x.go"},
		{Source: sopctx.SourceRecovery, Identity: "validation-context", Reason: "the deterministic validation failure", Priority: sopctx.PriorityRecovery, Text: "Validation failed: BUILD\nmain.go:3: undefined: foo"},
	}, sopctx.DefaultLimits())
}

func sampleInput() Input {
	return Input{
		Capability:         agent.Implement,
		Class:              Medium,
		Task:               "Implement S001.",
		Context:            sampleContext(),
		Attempt:            "A previous attempt at this task did not complete:\n\nmade no repository change",
		Observations:       "sibling status: green",
		OutputRequirements: "Implement the plan in the working tree.",
	}
}

// TestCompileIsDeterministic proves identical input yields byte-identical output.
func TestCompileIsDeterministic(t *testing.T) {
	a, b := Compile(sampleInput()), Compile(sampleInput())
	if a.Render() != b.Render() || a.Digest != b.Digest || a.Bytes != b.Bytes {
		t.Fatalf("compilation is not deterministic:\n a=%s\n b=%s", a.Render(), b.Render())
	}
}

// TestSectionOrderIsFixed proves the canonical section order and instruction/task
// separation, and that Evidence excludes instructions and the task.
func TestSectionOrderIsFixed(t *testing.T) {
	c := Compile(sampleInput())
	var names []string
	for _, s := range c.Sections {
		names = append(names, s.Name)
	}
	want := []string{sectionInstructions, sectionTask, sectionContext, sectionAttempt, sectionObservations}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("section order = %v, want %v", names, want)
	}
	ev := c.Evidence()
	if strings.Contains(ev, "Capability;") || strings.Contains(ev, "Capability:") {
		t.Errorf("Evidence must not repeat the instructions:\n%s", ev)
	}
	if strings.Contains(ev, "Implement S001.") {
		t.Errorf("Evidence must not repeat the task:\n%s", ev)
	}
	for _, want := range []string{"# Context", "# Previous attempt", "# Caller observations", "Validation failed: BUILD", "made no repository change", "sibling status: green"} {
		if !strings.Contains(ev, want) {
			t.Errorf("Evidence missing %q:\n%s", want, ev)
		}
	}
}

// TestContextSectionCarriesProvenance proves the context section renders each item
// with its source, identity, and reason.
func TestContextSectionCarriesProvenance(t *testing.T) {
	c := Compile(sampleInput())
	s, ok := c.Section(sectionContext)
	if !ok {
		t.Fatal("context section missing")
	}
	for _, want := range []string{"### task: T1", "### repository: internal/x.go", "### recovery: validation-context", "Reason: the plan the task executes"} {
		if !strings.Contains(s.Body, want) {
			t.Errorf("context section missing %q:\n%s", want, s.Body)
		}
	}
}

// TestEmptyInputCompilesCleanly proves absent evidence leaves no heading.
func TestEmptyInputCompilesCleanly(t *testing.T) {
	c := Compile(Input{})
	if len(c.Sections) != 0 || c.Bytes != 0 || c.Render() != "" || c.Evidence() != "" {
		t.Fatalf("empty input produced %+v (render=%q)", c, c.Render())
	}
	if c.Truncated {
		t.Error("empty input must not report truncation")
	}
}

// TestBoundNMinusOneNPlusOne covers the byte ceiling boundary directly.
func TestBoundNMinusOneNPlusOne(t *testing.T) {
	secs := []Section{
		{Name: "a", Body: "aaaa", Bytes: 4},
		{Name: "b", Body: "bbbb", Bytes: 4},
		{Name: "c", Body: "cccc", Bytes: 4},
	}
	// N-1: only the first section fits; the second is truncated to the remaining 3 bytes.
	got, trunc := bound(secs, 9)
	if !trunc || len(got) != 2 || got[1].Body != "bbb" {
		t.Errorf("N-1 bound = %+v truncated=%v", got, trunc)
	}
	// N: two sections fit exactly, the third is dropped.
	got, trunc = bound(secs, 10)
	if !trunc || len(got) != 2 || got[1].Body != "bbbb" {
		t.Errorf("N bound = %+v truncated=%v", got, trunc)
	}
	// N+1: the third section fits only after truncation.
	got, trunc = bound(secs, 15)
	if !trunc || len(got) != 3 || got[2].Body != "ccc" {
		t.Errorf("N+1 bound = %+v truncated=%v", got, trunc)
	}
	// Exact ceiling: all three fit with nothing truncated.
	got, trunc = bound(secs, 16)
	if trunc || len(got) != 3 {
		t.Errorf("exact bound = %+v truncated=%v", got, trunc)
	}
	// Unbounded.
	if _, trunc := bound(secs, 0); trunc {
		t.Error("a zero ceiling must be unbounded")
	}
}

// TestTruncateRunesIsSafe proves truncation never splits a UTF-8 sequence.
func TestTruncateRunesIsSafe(t *testing.T) {
	s := "éééé" // each é is two bytes
	if got := truncateRunes(s, 5); got != "éé" {
		t.Errorf("truncateRunes(%q,5) = %q, want %q", s, got, "éé")
	}
	if got := truncateRunes(s, 8); got != s {
		t.Errorf("truncateRunes(%q,8) = %q, want unchanged", s, got)
	}
}

// TestClassBoundApplies proves the class selects a deterministic finite ceiling, and
// that a large context is truncated under SMALL but not under LARGE.
func TestClassBoundApplies(t *testing.T) {
	big := strings.Repeat("x", 40<<10)
	in := Input{
		Capability: agent.Plan,
		Class:      Small,
		Task:       "plan",
		Context:    sopctx.Build([]sopctx.Item{{Source: sopctx.SourceTask, Identity: "T", Priority: sopctx.PriorityLifecycle, Text: big}}, sopctx.Limits{}),
	}
	small := Compile(in)
	if !small.Truncated || small.Bytes > LimitsFor(Small).MaxBytes {
		t.Errorf("SMALL bound: truncated=%v bytes=%d limit=%d", small.Truncated, small.Bytes, LimitsFor(Small).MaxBytes)
	}
	in.Class = Large
	large := Compile(in)
	if large.Truncated {
		t.Errorf("LARGE must admit the context without truncation: bytes=%d", large.Bytes)
	}
}

// TestClassDefaultsAndParsing covers invalid/empty class fallback and parsing.
func TestClassDefaultsAndParsing(t *testing.T) {
	if c := Compile(Input{}); c.Class != Medium {
		t.Errorf("empty class = %q, want MEDIUM", c.Class)
	}
	if c := Compile(Input{Class: "bogus"}); c.Class != Medium {
		t.Errorf("invalid class = %q, want MEDIUM", c.Class)
	}
	for in, want := range map[string]Class{"small": Small, " Large ": Large, "MEDIUM": Medium} {
		got, ok := ParseClass(in)
		if !ok || got != want {
			t.Errorf("ParseClass(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := ParseClass("nope"); ok {
		t.Error("ParseClass should reject an unknown class")
	}
}
