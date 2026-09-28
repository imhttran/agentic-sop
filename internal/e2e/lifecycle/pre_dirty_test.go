package lifecycle

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/e2e/harness"
)

// TestPreDirtyRepoAcrossCapabilities drives PLAN, IMPLEMENT, and REVIEW against a
// repository that is already dirty before each invocation, and asserts that the
// pre-existing changes are visible throughout, are never attributed to the
// invocation, and are preserved afterwards. S7.
func TestPreDirtyRepoAcrossCapabilities(t *testing.T) {
	for _, capability := range []agent.Capability{agent.Plan, agent.Implement, agent.Review} {
		t.Run(string(capability), func(t *testing.T) {
			repo, err := harness.NewRepo(t)
			if err != nil {
				t.Fatalf("new repo: %v", err)
			}
			// Pre-dirty the tree before the invocation: a modified tracked file and
			// an untracked file.
			if _, err := repo.Dirty(); err != nil {
				t.Fatalf("dirty: %v", err)
			}
			if _, err := repo.Untracked("pre-existing.txt"); err != nil {
				t.Fatalf("untracked: %v", err)
			}

			// The provider models an adapter that observes the repository at
			// invocation time: it must see the pre-existing dirt but report no
			// change of its own.
			provider := harness.NewFakeProvider()
			provider.GenerateFunc = func(_ int, req agent.Request) (agent.Response, error) {
				if req.Capability != capability {
					t.Fatalf("capability = %s, want %s", req.Capability, capability)
				}
				dirty, derr := repo.IsDirty()
				if derr != nil {
					return agent.Response{}, derr
				}
				if !dirty {
					t.Fatal("invocation saw a clean repository; pre-dirty state was lost")
				}
				return harness.Response(
					"observed pre-dirty tree",
					harness.Completed("no change of my own", false),
				), nil
			}
			h := agent.HarnessFunc(func(ctx context.Context, req agent.Request) (agent.Response, error) {
				return provider.Generate(ctx, req)
			})

			resp, err := h.Execute(context.Background(), agent.Request{Capability: capability, Task: "observe"})
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if resp.Outcome == nil || resp.Outcome.ChangesExpected {
				t.Fatalf("outcome = %+v, want a no-change completion over a pre-dirty tree", resp.Outcome)
			}

			// Pre-existing changes survive the invocation.
			dirty, derr := repo.IsDirty()
			if derr != nil {
				t.Fatalf("is dirty after invocation: %v", derr)
			}
			if !dirty {
				t.Fatal("repository is clean after the invocation; pre-existing changes were lost")
			}
		})
	}
}
