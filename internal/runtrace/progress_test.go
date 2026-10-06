package runtrace

import "testing"

func countType(ps []ProgressSignal, t SignalType) int {
	n := 0
	for _, p := range ps {
		if p.Type == t {
			n++
		}
	}
	return n
}

// TestClassifyDiscoveryIsNovelAndBounded proves a first-seen inspection is a
// discovery signal, a repeated inspection does not multiply signals, and an
// inspection without a producer marker is activity only.
func TestClassifyDiscoveryIsNovelAndBounded(t *testing.T) {
	in := Inputs{Iterations: []Iteration{
		{Sequence: 1, Phase: "DISCOVER", Action: "reading", Observation: "a.go", Signal: "discovery.novel"},
		{Sequence: 2, Phase: "DISCOVER", Action: "reading", Observation: "a.go", Signal: "discovery.novel"}, // repeated
		{Sequence: 3, Phase: "DISCOVER", Action: "reading", Observation: "b.go", Signal: "discovery.novel"},
		{Sequence: 4, Phase: "DISCOVER", Action: "reading", Observation: "c.go"}, // activity, no marker
	}}
	got := classifyProgress(in)
	if n := countType(got, DiscoverySignal); n != 2 {
		t.Errorf("discovery signals = %d, want 2 (a.go first-seen + b.go)", n)
	}
	for _, p := range got {
		if p.Type == DiscoverySignal && p.Subtype != SubtypeNovelInspection {
			t.Errorf("discovery subtype = %q", p.Subtype)
		}
	}
}

// TestClassifyMutationVerifiedOnly proves a verified mutation is a signal and a
// plain command (activity) is not.
func TestClassifyMutationVerifiedOnly(t *testing.T) {
	in := Inputs{Iterations: []Iteration{
		{Sequence: 1, Phase: "VALIDATE", Action: "go version"},                                              // activity
		{Sequence: 2, Phase: "CHANGE", Action: "editing", Observation: "a.go", Signal: "mutation.verified"}, // progress
	}}
	got := classifyProgress(in)
	if n := countType(got, MutationSignal); n != 1 {
		t.Fatalf("mutation signals = %d, want 1", n)
	}
	if n := countType(got, VerificationSignal); n != 0 {
		t.Errorf("a command must not produce a verification signal, got %d", n)
	}
	if n := countType(got, TransitionSignal); n != 0 {
		t.Errorf("a VALIDATE activity must not be a lifecycle transition, got %d", n)
	}
}

// TestClassifyMutationFallsBackToChangedFiles proves that when the agent reports
// no per-mutation marker (a command agent), the invocation-attributed changed
// files are the mutation evidence.
func TestClassifyMutationFallsBackToChangedFiles(t *testing.T) {
	got := classifyProgress(Inputs{ChangedFiles: []string{"notes.txt"}})
	if n := countType(got, MutationSignal); n != 1 {
		t.Fatalf("mutation signals = %d, want 1", n)
	}
	if got[0].Evidence != "notes.txt" {
		t.Errorf("evidence = %q, want notes.txt", got[0].Evidence)
	}
}

// TestClassifyVerificationAndTransitions proves a passing validation is a
// verification signal and lifecycle stages are transition signals (VALIDATE is
// not, since it is shared with agent command activity).
func TestClassifyVerificationAndTransitions(t *testing.T) {
	in := Inputs{
		Iterations: []Iteration{
			{Sequence: 1, Phase: "PLAN"}, {Sequence: 2, Phase: "IMPLEMENT"},
			{Sequence: 3, Phase: "VALIDATE", Action: "go test ./..."},
			{Sequence: 4, Phase: "QUALITY"}, {Sequence: 5, Phase: "COMPLETE"},
		},
		Verification: []Verification{
			{Command: "go test ./...", Status: "PASS"},
			{Command: "go vet ./...", Status: "FAIL"}, // a failing check is not progress
		},
	}
	got := classifyProgress(in)
	if n := countType(got, TransitionSignal); n != 4 {
		t.Errorf("transition signals = %d, want 4 (PLAN, IMPLEMENT, QUALITY, COMPLETE)", n)
	}
	if n := countType(got, VerificationSignal); n != 1 {
		t.Errorf("verification signals = %d, want 1 (only the pass)", n)
	}
}

// TestBuildIncludesProgressAndSummary proves the trace carries the progress
// signals with a deterministic summary and the schema version is bumped.
func TestBuildIncludesProgressAndSummary(t *testing.T) {
	tr := Build(Inputs{
		Iterations:   []Iteration{{Sequence: 1, Phase: "PLAN"}, {Sequence: 2, Phase: "CHANGE", Observation: "a.go", Signal: "mutation.verified"}},
		Verification: []Verification{{Command: "true", Status: "PASS"}},
	})
	if tr.SchemaVersion != 2 {
		t.Errorf("schema version = %d, want 2", tr.SchemaVersion)
	}
	if tr.ProgressSummary.StateTransitions != 1 || tr.ProgressSummary.RepositoryMutations != 1 || tr.ProgressSummary.Verification != 1 {
		t.Errorf("summary = %+v", tr.ProgressSummary)
	}
	if len(tr.Progress) != 3 {
		t.Errorf("progress signals = %d, want 3", len(tr.Progress))
	}
	for i, p := range tr.Progress {
		if p.Sequence != i+1 {
			t.Errorf("signal %d sequence = %d", i, p.Sequence)
		}
	}
}
