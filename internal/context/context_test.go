package context

import "testing"

func ids(c Context) []string {
	out := []string{}
	for _, it := range c.Items() {
		out = append(out, it.Identity)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildEmptyIsValid(t *testing.T) {
	c := Build(nil, DefaultLimits())
	if !c.Empty() || c.Bytes() != 0 || c.Files() != 0 || c.Truncated() {
		t.Fatalf("empty context = %+v", c)
	}
	if c.Render() != "" {
		t.Errorf("render = %q, want empty", c.Render())
	}
	if len(c.Sources()) != 0 {
		t.Errorf("sources = %v, want none", c.Sources())
	}
}

func TestBuildOrderingIsTotalAndStable(t *testing.T) {
	items := []Item{
		{Source: SourceRepository, Identity: "b.go", Priority: PriorityRepository, Text: "b"},
		{Source: SourceTask, Identity: "T", Priority: PriorityLifecycle, Text: "task"},
		{Source: SourceRepository, Identity: "a.go", Priority: PriorityRepository, Text: "a"},
	}
	want := []string{"T", "a.go", "b.go"}
	if got := ids(Build(items, DefaultLimits())); !eq(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	// The same items in a different order yield the same context.
	rev := []Item{items[2], items[1], items[0]}
	if got := ids(Build(rev, DefaultLimits())); !eq(got, want) {
		t.Errorf("reversed order = %v, want %v", got, want)
	}
}

func TestBuildIsDeterministicAndModelFree(t *testing.T) {
	mk := func() []Item {
		return []Item{
			{Source: SourceTask, Identity: "T", Priority: PriorityLifecycle, Text: "x"},
			{Source: SourceRepository, Identity: "a", Priority: PriorityRepository, Text: "y"},
		}
	}
	c1, c2 := Build(mk(), DefaultLimits()), Build(mk(), DefaultLimits())
	if c1.Render() != c2.Render() || c1.Bytes() != c2.Bytes() || c1.Files() != c2.Files() {
		t.Error("same input must produce the same context")
	}
	// Construction is model-free by signature: Build takes no agent and no provider.
	_ = c1.Sources()
}

func TestBuildItemCeilingBoundary(t *testing.T) {
	items := []Item{
		{Source: SourceTask, Identity: "1", Priority: 1, Text: "a"},
		{Source: SourceTask, Identity: "2", Priority: 1, Text: "b"},
		{Source: SourceTask, Identity: "3", Priority: 1, Text: "c"},
	}
	// Ceiling N=2: fewer than N fits untouched; exactly N fits untouched; N+1 is cut.
	if c := Build(items[:1], Limits{MaxItems: 2}); len(c.Items()) != 1 || c.Truncated() {
		t.Errorf("N-1 items = %d truncated=%v, want 1/false", len(c.Items()), c.Truncated())
	}
	if c := Build(items[:2], Limits{MaxItems: 2}); len(c.Items()) != 2 || c.Truncated() {
		t.Errorf("N items = %d truncated=%v, want 2/false", len(c.Items()), c.Truncated())
	}
	if c := Build(items, Limits{MaxItems: 2}); len(c.Items()) != 2 || !c.Truncated() {
		t.Errorf("N+1 attempt = %d truncated=%v, want 2/true", len(c.Items()), c.Truncated())
	}
}

func TestBuildFileCeiling(t *testing.T) {
	items := []Item{
		{Source: SourceRepository, Identity: "a.go", Priority: 2, Text: "a"},
		{Source: SourceRepository, Identity: "a.go", Priority: 2, Text: "a2"},
		{Source: SourceRepository, Identity: "b.go", Priority: 2, Text: "b"},
		{Source: SourceRepository, Identity: "c.go", Priority: 2, Text: "c"},
	}
	c := Build(items, Limits{MaxFiles: 2})
	if c.Files() != 2 {
		t.Errorf("files = %d, want 2", c.Files())
	}
	if !c.Truncated() {
		t.Error("exceeding the file ceiling must be observable")
	}
}

func TestBuildByteCeilingTruncationIsDeterministic(t *testing.T) {
	items := []Item{
		{Source: SourceTask, Identity: "1", Priority: 1, Text: "aaaa"},
		{Source: SourceTask, Identity: "2", Priority: 1, Text: "bbbb"},
	}
	c := Build(items, Limits{MaxBytes: 6})
	if c.Bytes() != 6 {
		t.Errorf("bytes = %d, want 6", c.Bytes())
	}
	if !c.Truncated() {
		t.Error("truncation must be observable")
	}
	if got := c.Items()[1].Text; got != "bb" {
		t.Errorf("overflowing item = %q, want rune-safe prefix bb", got)
	}
	if c.Render() != Build(items, Limits{MaxBytes: 6}).Render() {
		t.Error("truncation must be deterministic")
	}
}

func TestBuildTruncatesRuneSafe(t *testing.T) {
	c := Build([]Item{{Source: SourceTask, Priority: 0, Text: "éééé"}}, Limits{MaxBytes: 5})
	if got := c.Items()[0].Text; got != "éé" {
		t.Errorf("rune-safe prefix = %q, want éé", got)
	}
}

func TestSourcesSummaryDeterministic(t *testing.T) {
	c := Build([]Item{
		{Source: SourceRepository, Identity: "a", Priority: 2, Text: "a"},
		{Source: SourceTask, Identity: "T", Priority: 1, Text: "task"},
	}, DefaultLimits())
	s := c.Sources()
	if len(s) != 2 || s[0].Source != SourceTask || s[1].Source != SourceRepository {
		t.Fatalf("sources = %+v, want task then repository", s)
	}
}

func TestFromInputsRepresentsAllFourSources(t *testing.T) {
	c := FromInputs(Inputs{
		TaskID: "T001", Task: "the task", Plan: "the plan", AcceptanceCriteria: []string{"a"},
		ChangedFiles: []string{"b.go", "a.go"},
		Execution:    []Item{{Source: SourceExecution, Identity: "exec", Priority: PriorityExecution, Text: "medium/ollama"}},
		Recovery:     []Item{{Source: SourceRecovery, Identity: "rec", Priority: PriorityRecovery, Text: "previous failure"}},
	}, DefaultLimits())
	srcs := map[Source]bool{}
	for _, s := range c.Sources() {
		srcs[s.Source] = true
	}
	for _, want := range []Source{SourceTask, SourceRepository, SourceExecution, SourceRecovery} {
		if !srcs[want] {
			t.Errorf("missing source %s", want)
		}
	}
	if c.Files() != 2 {
		t.Errorf("files = %d, want 2", c.Files())
	}
}

func TestFromInputsMissingOptionalSources(t *testing.T) {
	c := FromInputs(Inputs{TaskID: "T001", Task: "the task"}, DefaultLimits())
	if c.Empty() {
		t.Fatal("a task-only context is minimal, not empty")
	}
	if len(c.Items()) != 1 || c.Files() != 0 || c.Truncated() {
		t.Errorf("minimal context items=%d files=%d truncated=%v", len(c.Items()), c.Files(), c.Truncated())
	}
}
