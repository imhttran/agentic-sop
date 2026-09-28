package lifecycle

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/e2e/harness"
)

// TestImplementInvocationIsolation asserts that state and mutations from one
// IMPLEMENT invocation do not leak into a subsequent invocation over the same
// repository. S3.
func TestImplementInvocationIsolation(t *testing.T) {
	repo, err := harness.NewRepo(t)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	// A harness whose behavior depends on the repository's dirty state. This
	// models an implementation adapter that detects prior changes.
	provider := &harness.FakeProvider{}
	provider.GenerateFunc = func(_ int, _ agent.Request) (agent.Response, error) {
		dirty, derr := repo.IsDirty()
		if derr != nil {
			return agent.Response{}, derr
		}
		return agent.Response{Outcome: harness.Completed("observed", dirty)}, nil
	}
	h := agent.HarnessFunc(func(ctx context.Context, req agent.Request) (agent.Response, error) {
		return provider.Generate(ctx, req)
	})

	// First invocation: repository is clean, so no change is expected.
	first, err := h.Execute(context.Background(), agent.Request{Capability: agent.Implement, Task: "first"})
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if first.Outcome.ChangesExpected {
		t.Fatal("first invocation should observe a clean repository")
	}

	// Second invocation over the same (still clean) repository must see the same
	// input state: the first call must not have contaminated it.
	second, err := h.Execute(context.Background(), agent.Request{Capability: agent.Implement, Task: "second"})
	if err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if second.Outcome.ChangesExpected {
		t.Fatal("second invocation leaked state from the first (isolation broken)")
	}

	// The provider saw exactly two isolated requests with distinct tasks.
	reqs := provider.Requests()
	if len(reqs) != 2 {
		t.Fatalf("recorded requests = %d, want 2", len(reqs))
	}
	if reqs[0].Task == reqs[1].Task {
		t.Fatalf("requests not isolated: both tasks = %q", reqs[0].Task)
	}
}

// TestImplementIsolationAcrossProviders asserts two independent providers do not
// share recorded state. S3.
func TestImplementIsolationAcrossProviders(t *testing.T) {
	a := harness.NewFakeProvider(harness.Response("a", harness.Completed("a", false)))
	b := harness.NewFakeProvider(harness.Response("b", harness.Completed("b", false)))

	if _, err := a.Generate(context.Background(), agent.Request{Capability: agent.Implement, Task: "a"}); err != nil {
		t.Fatalf("a generate: %v", err)
	}
	if _, err := b.Generate(context.Background(), agent.Request{Capability: agent.Implement, Task: "b"}); err != nil {
		t.Fatalf("b generate: %v", err)
	}

	if a.Calls() != 1 || b.Calls() != 1 {
		t.Fatalf("call counts leaked: a=%d b=%d, want 1 and 1", a.Calls(), b.Calls())
	}
	if len(a.Requests()) != 1 || len(b.Requests()) != 1 {
		t.Fatalf("request logs leaked: a=%d b=%d", len(a.Requests()), len(b.Requests()))
	}
}
