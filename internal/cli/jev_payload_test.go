package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
)

// TestJEVFixContextRendersCategoryAndActionableMessageDistinctly proves the end
// to end contract: FIX receives the actionable message, and the category is
// preserved beside it as separate metadata rather than replacing it.
func TestJEVFixContextRendersCategoryAndActionableMessageDistinctly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", fix: "fixed", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{
		{Status: jev.StatusFindings, Findings: []jev.Finding{
			jevFinding(review.High, "internal/x.go", 12, "file handle not closed", "os.Open without defer Close"),
		}},
		{Status: jev.StatusPass},
	}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(a.fixInputs) != 1 {
		t.Fatalf("fix invocations = %d, want 1", len(a.fixInputs))
	}
	in := a.fixInputs[0]
	if !strings.Contains(in, "Category: quality") {
		t.Errorf("FIX input missing the category metadata:\n%s", in)
	}
	if !strings.Contains(in, "Finding: file handle not closed") {
		t.Errorf("FIX input missing the actionable message:\n%s", in)
	}
	ci, fi := strings.Index(in, "Category: quality"), strings.Index(in, "Finding: file handle not closed")
	if ci < 0 || fi < 0 || ci > fi {
		t.Errorf("Category must render before Finding (ci=%d fi=%d):\n%s", ci, fi, in)
	}
}

// TestJEVPayloadBlockRendersDistinctCategoryAndFinding pins the exact rendering:
// a category line when one is known, its absence when it is not, and the message
// always present as the actionable Finding.
func TestJEVPayloadBlockRendersDistinctCategoryAndFinding(t *testing.T) {
	withCat := jevPayloadBlock(jev.FindingPayload{
		Severity: "HIGH", File: "a.go", Line: 3,
		Category: "quality", Finding: "leak", Evidence: "e",
	})
	want := "[HIGH] a.go:3\nCategory: quality\nFinding: leak\nEvidence: e"
	if withCat != want {
		t.Errorf("block = %q, want %q", withCat, want)
	}

	noCat := jevPayloadBlock(jev.FindingPayload{
		Severity: "HIGH", File: "a.go", Line: 3, Finding: "leak",
	})
	if strings.Contains(noCat, "Category:") {
		t.Errorf("absent category must not render a Category line: %q", noCat)
	}
	if !strings.Contains(noCat, "Finding: leak") {
		t.Errorf("message must still render: %q", noCat)
	}
}

// TestJEVPayloadsSortDeterministicallyAndKeepFields asserts the ordering stays
// deterministic (severity high-to-low, then file, then line) and that sorting
// never mutates the category or the actionable message.
func TestJEVPayloadsSortDeterministicallyAndKeepFields(t *testing.T) {
	in := []jev.FindingPayload{
		{Severity: "LOW", File: "b.go", Line: 2, Category: "c-low", Finding: "low"},
		{Severity: "HIGH", File: "b.go", Line: 5, Category: "c-high-b", Finding: "high-b"},
		{Severity: "CRITICAL", File: "a.go", Line: 9, Category: "c-crit", Finding: "crit"},
		{Severity: "HIGH", File: "a.go", Line: 1, Category: "c-high-a", Finding: "high-a"},
	}
	sortPayloads(in)

	gotOrder := make([]string, 0, len(in))
	for _, p := range in {
		gotOrder = append(gotOrder, p.Severity+":"+p.File)
	}
	wantOrder := []string{"CRITICAL:a.go", "HIGH:a.go", "HIGH:b.go", "LOW:b.go"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("order = %v, want %v", gotOrder, wantOrder)
		}
	}

	// Repeated sorts are stable: a second pass leaves the order unchanged.
	first := append([]jev.FindingPayload(nil), in...)
	sortPayloads(in)
	for i := range first {
		if in[i] != first[i] {
			t.Fatalf("sort is not stable at %d: %+v vs %+v", i, in[i], first[i])
		}
	}

	// Category and message survive sorting unchanged.
	for _, p := range in {
		if p.Category == "" || p.Finding == "" {
			t.Errorf("sorting dropped metadata: %+v", p)
		}
		if p.Finding == p.Category {
			t.Errorf("Finding collapsed into Category: %+v", p)
		}
	}
}

// TestJEVPayloadsFailClosedOnMalformedAndIncomplete asserts a result that is not
// a clean pass yields no payloads and no FIX section, so FIX is never handed a
// fabricated or ambiguous finding.
func TestJEVPayloadsFailClosedOnMalformedAndIncomplete(t *testing.T) {
	cases := map[string]jev.Result{
		"incomplete": {
			Status:   jev.StatusIncomplete,
			Findings: []jev.Finding{{Severity: review.High, Category: "quality", Path: "a.go", Line: 1, Message: "m"}},
		},
		"malformed": {
			// PASS with findings does not validate: a malformed result.
			Status:   jev.StatusPass,
			Findings: []jev.Finding{{Severity: review.High, Category: "quality", Path: "a.go", Line: 1, Message: "m"}},
		},
	}
	for name, res := range cases {
		ev := quality.NewJEVEvidence(res)
		if !ev.FailClosed {
			t.Fatalf("%s: expected fail-closed evidence", name)
		}
		re := &jevRunEvidence{TaskID: "t", RunID: "r", Evidence: &ev, Result: res}
		if got := re.fixPayloads(); got != nil {
			t.Errorf("%s: fail-closed payloads = %v, want nil", name, got)
		}
		if got := re.findingsSection([]string{"high", "critical"}); got != "" {
			t.Errorf("%s: fail-closed section = %q, want empty", name, got)
		}
	}
}

// TestJEVPayloadsAbsentWhenDisabled asserts the disabled path is inert: with no
// evidence (JEV disabled or absent), there are no payloads and no FIX section.
func TestJEVPayloadsAbsentWhenDisabled(t *testing.T) {
	var disabled *jevRunEvidence // JEV disabled: runOptionalJEV returns nil
	if got := disabled.fixPayloads(); got != nil {
		t.Errorf("disabled payloads = %v, want nil", got)
	}
	if got := disabled.findingsSection([]string{"high"}); got != "" {
		t.Errorf("disabled section = %q, want empty", got)
	}
	if got := disabled.gateEvidence(); got != nil {
		t.Errorf("disabled gate evidence = %v, want nil", got)
	}
}
