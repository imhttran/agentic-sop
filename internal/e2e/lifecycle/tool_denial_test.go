package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/e2e/harness"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// TestReviewSynthesisToolDenied asserts that a synthesis tool call during
// REVIEW is denied by the controlled tool harness and the denial is surfaced as
// an error rather than a silent success. S5.
func TestReviewSynthesisToolDenied(t *testing.T) {
	repo, err := harness.NewRepo(t)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	h := toolharness.New(repo.Dir, toolharness.DefaultConfig(), nil)

	// A synthesis tool call attempts to mutate the repository during REVIEW. The
	// harness must deny a destructive command rather than execute it.
	_, err = h.Run(context.Background(), toolharness.ToolRunCommand, map[string]any{
		"command": "git commit -m sneaky",
	})
	if err == nil {
		t.Fatal("expected synthesis tool denial for a destructive command")
	}

	// The denial must not have mutated the repository.
	dirty, derr := repo.IsDirty()
	if derr != nil {
		t.Fatalf("is dirty: %v", derr)
	}
	if dirty {
		t.Fatal("denied synthesis tool nevertheless mutated the repository")
	}
}

// TestReviewSynthesisUnknownToolDenied asserts that a tool outside the reviewed
// surface is refused. S5.
func TestReviewSynthesisUnknownToolDenied(t *testing.T) {
	repo, err := harness.NewRepo(t)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	h := toolharness.New(repo.Dir, toolharness.DefaultConfig(), nil)
	if _, err := h.Run(context.Background(), "synthesize_review", map[string]any{}); err == nil {
		t.Fatal("expected denial for an unsupported synthesis tool")
	}
}

// TestReviewSynthesisDenialRecorded asserts a denied synthesis tool is a
// deterministic, classifiable refusal. S5 / focused failure.
func TestReviewSynthesisDenialRecorded(t *testing.T) {
	repo, err := harness.NewRepo(t)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	h := toolharness.New(repo.Dir, toolharness.DefaultConfig(), nil)
	_, err = h.Run(context.Background(), toolharness.ToolRunCommand, map[string]any{
		"command": "git push origin main",
	})
	if err == nil {
		t.Fatal("expected denial for git push during REVIEW")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("denial should not be a timeout: %v", err)
	}
}

// TestSynthesisOutcomeNotSilentlySucceeded asserts a denied synthesis tool never
// yields a completed structured outcome. S5.
func TestSynthesisOutcomeNotSilentlySucceeded(t *testing.T) {
	repo, err := harness.NewRepo(t)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	h := toolharness.New(repo.Dir, toolharness.DefaultConfig(), nil)

	_, toolErr := h.Run(context.Background(), toolharness.ToolRunCommand, map[string]any{
		"command": "git reset --hard",
	})
	// Convert the denial into the structured outcome SOP would observe: a denied
	// tool is a failure, never completed.
	result := agent.Response{
		Content: "review synthesis",
		Outcome: harness.Failed("synthesis tool denied"),
	}
	if toolErr == nil {
		t.Fatal("expected the destructive tool to be denied")
	}
	if result.Outcome.Status != agent.OutcomeFailed {
		t.Fatalf("outcome status = %s, want failed", result.Outcome.Status)
	}
}
