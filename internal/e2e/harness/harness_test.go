package harness

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestFakeProviderIsDeterministic proves the shared provider helper is
// deterministic: canned responses are returned in order and past-the-end calls
// fall back to a stable completed outcome. It anchors S0 (regression harness
// environment).
func TestFakeProviderIsDeterministic(t *testing.T) {
	p := NewFakeProvider(
		Response("first", Completed("a", false)),
		Response("second", Completed("b", true)),
	)

	for i, want := range []string{"first", "second"} {
		resp, err := p.Generate(context.Background(), agent.Request{
			Capability: agent.Plan,
			Task:       "t",
		})
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if resp.Content != want {
			t.Fatalf("call %d: content = %q, want %q", i, resp.Content, want)
		}
	}

	// A call past the canned responses gets a stable fallback, not a panic.
	resp, err := p.Generate(context.Background(), agent.Request{Capability: agent.Plan, Task: "t"})
	if err != nil {
		t.Fatalf("fallback: unexpected error: %v", err)
	}
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("fallback: outcome = %+v, want completed", resp.Outcome)
	}

	if got := p.Calls(); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
}

func TestClockIsExplicit(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewClock(start)
	if got := c.Now(); !got.Equal(start) {
		t.Fatalf("now = %v, want %v", got, start)
	}
	c.Advance(5 * time.Second)
	if got, want := c.Now(), start.Add(5*time.Second); !got.Equal(want) {
		t.Fatalf("after advance now = %v, want %v", got, want)
	}
}

func TestIDsAreStable(t *testing.T) {
	ids := NewIDs("stage")
	for _, want := range []string{"stage-1", "stage-2", "stage-3"} {
		if got := ids.Next(); got != want {
			t.Fatalf("next = %q, want %q", got, want)
		}
	}
}

func TestFakeProviderError(t *testing.T) {
	want := errors.New("boom")
	p := NewFakeProviderWithErr(want)
	_, err := p.Generate(context.Background(), agent.Request{Capability: agent.Plan, Task: "t"})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestCapabilitiesRestriction(t *testing.T) {
	p := NewFakeProvider().WithCapabilities(agent.NewCapabilities(agent.Review))
	if !p.Capabilities().Supports(agent.Review) {
		t.Fatal("expected REVIEW capability")
	}
	if p.Capabilities().Supports(agent.Plan) {
		t.Fatal("did not expect PLAN capability")
	}
}

func TestRepoCleanAndDirty(t *testing.T) {
	repo, err := NewRepo(t)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	dirty, err := repo.IsDirty()
	if err != nil {
		t.Fatalf("is dirty: %v", err)
	}
	if dirty {
		t.Fatal("fresh repo should be clean")
	}

	if _, err := repo.Dirty(); err != nil {
		t.Fatalf("dirty: %v", err)
	}
	dirty, err = repo.IsDirty()
	if err != nil {
		t.Fatalf("is dirty after modification: %v", err)
	}
	if !dirty {
		t.Fatal("modified repo should be dirty")
	}
}

func TestRepoUntrackedIsDirty(t *testing.T) {
	repo, err := NewRepo(t)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if _, err := repo.Untracked("extra.txt"); err != nil {
		t.Fatalf("untracked: %v", err)
	}
	dirty, err := repo.IsDirty()
	if err != nil {
		t.Fatalf("is dirty: %v", err)
	}
	if !dirty {
		t.Fatal("repo with an untracked file should be dirty")
	}
}

func TestAdapterContract(t *testing.T) {
	ok := &Adapter{Name: "ok"}
	resp, err := ok.Run(agent.Request{Capability: agent.Implement, Task: "t"})
	if err != nil {
		t.Fatalf("ok adapter error: %v", err)
	}
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v, want completed", resp.Outcome)
	}

	fail := &Adapter{Name: "fail", ExitCode: 2, Stderr: "bad"}
	if _, err := fail.Run(agent.Request{Capability: agent.Implement, Task: "t"}); err == nil {
		t.Fatal("failing adapter should return an error")
	}

	malformed := &Adapter{Name: "malformed", Malformed: true}
	if _, err := malformed.Run(agent.Request{Capability: agent.Implement, Task: "t"}); err == nil {
		t.Fatal("malformed adapter should return an error")
	}
}

// TestFakeProviderConcurrentUseIsRaceFree proves the shared provider helper is
// safe for concurrent use: many goroutines generate at once, every request is
// recorded, and the call count is exact. Run under the race detector, it guards
// the concurrency-sensitive shared state. S11.
func TestFakeProviderConcurrentUseIsRaceFree(t *testing.T) {
	const n = 32
	p := NewFakeProvider()

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			if _, err := p.Generate(context.Background(), agent.Request{Capability: agent.Plan, Task: "t"}); err != nil {
				t.Errorf("generate %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if got := p.Calls(); got != n {
		t.Fatalf("calls = %d, want %d", got, n)
	}
	if got := len(p.Requests()); got != n {
		t.Fatalf("recorded requests = %d, want %d", got, n)
	}
}
