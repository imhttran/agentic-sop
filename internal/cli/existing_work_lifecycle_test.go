package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// existingWorkAgent reports the already-verified existing-work completion the harness
// produces for a task whose implementation already exists.
type existingWorkAgent struct{ ev *agent.CompletionEvidence }

func (a existingWorkAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement, agent.Fix:
		return agent.Response{
			Content:                  "verified existing implementation",
			VerifiedAlreadySatisfied: true,
			Outcome: &agent.Outcome{
				Status:     agent.OutcomeCompleted,
				Completion: agent.AlreadySatisfied,
				Evidence:   a.ev,
			},
		}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// TestExistingWorkLifecycleCompletesWithoutMutation covers the combined NEXT-002 shape at
// the lifecycle level: the implementation already exists, the task declares only
// descriptive deliverables, the attempt produces no working-tree change, the bounded
// existing-work verification is accepted, and the task reaches LOCAL_DONE with zero
// repository mutations and without a diff-based review.
func TestExistingWorkLifecycleCompletesWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	// The implementation already exists in the repository. Committed work is not part of
	// the working-tree change the lifecycle observes, which is the point of this test.
	writeFile(t, dir, "existing.go", "package x\n\n// already implemented and committed\n")
	seedTask(t, dir, &domain.Task{
		ID: "S001", Title: "Improve execution reliability", Objective: "o",
		Status: domain.PLANNED, MaxAttempts: 3,
		AcceptanceCriteria: "the lint command is harness-executable",
	})
	ev := &agent.CompletionEvidence{Acceptance: []agent.AcceptanceEvidence{{
		Criterion: "the lint command is harness-executable",
		Paths:     []string{"existing.go"},
	}}}

	// An empty diff: the attempt changes nothing.
	code, stdout, stderr := runInjectedCLI(t, dir, "", existingWorkAgent{ev: ev}, "run")
	if code != 0 {
		t.Fatalf("run: code=%d stderr=%s", code, stderr)
	}
	if strings.Contains(stdout, "reviewing changes") {
		t.Errorf("a verified existing-work completion must not enter diff-based review:\n%s", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	task, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != domain.LOCAL_DONE {
		t.Fatalf("task status = %s, want LOCAL_DONE (stdout=%s)", task.Status, stdout)
	}

	raw, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", "S001", "report.json"))
	if err != nil {
		t.Fatalf("read run report: %v", err)
	}
	var doc struct {
		RepositoryMutations *int                      `json:"repository_mutations"`
		AlreadySatisfied    *agent.CompletionEvidence `json:"already_satisfied"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode run report: %v", err)
	}
	if doc.RepositoryMutations == nil || *doc.RepositoryMutations != 0 {
		t.Fatalf("repository_mutations = %v, want 0 (existing work is never a mutation)", doc.RepositoryMutations)
	}
	if doc.AlreadySatisfied == nil {
		t.Fatal("the run report does not record the verified already-satisfied evidence")
	}
}
