package eval

import (
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// TestAgenticEvalRepeatedDiscoveryIsBounded evaluates the committed fixture
// against a trace whose trajectory repeats an identical inspection. A repeated
// read is activity, not additional discovery progress, and none of the activity
// is repository progress.
func TestAgenticEvalRepeatedDiscoveryIsBounded(t *testing.T) {
	f, err := LoadFixture(filepath.Join("..", "..", "evals", "progress", "repeated-discovery.expect.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	tr := runtrace.Build(runtrace.Inputs{
		RunID: "T001",
		Iterations: []runtrace.Iteration{
			{Sequence: 1, Phase: "DISCOVER", Action: "reading", Observation: "pkg/a.go", Signal: "discovery.novel"},
			{Sequence: 2, Phase: "DISCOVER", Action: "reading", Observation: "pkg/a.go", Signal: "discovery.novel"}, // repeated
			{Sequence: 3, Phase: "DISCOVER", Action: "reading", Observation: "pkg/b.go", Signal: "discovery.novel"},
			{Sequence: 4, Phase: "DISCOVER", Action: "reading", Observation: "pkg/c.go"}, // activity only, no marker
		},
	})
	res := Evaluate(tr, f)
	if !res.Passed {
		for _, d := range res.Diagnostics {
			t.Errorf("%s", d.Message)
		}
		t.Fatalf("evaluation %q FAILED", f.Name)
	}
	if tr.ProgressSummary.Discovery != 2 {
		t.Errorf("discovery = %d, want 2 (the repeat is activity)", tr.ProgressSummary.Discovery)
	}
}
