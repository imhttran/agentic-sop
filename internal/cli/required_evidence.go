package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// EvidenceVerifier verifies that the external retrieval evidence a task requires
// (domain.Task.RequiredEvidence) is available and authorized before the task
// runs. It is the seam the RAG client is connected to in Package 3; until then
// the verifier is nil and every required task fails closed. A verifier can only
// report what is missing or errored: it can never grant tools, mutation, or
// completion, and a model assertion is never accepted as evidence.
type EvidenceVerifier interface {
	// VerifyRequiredEvidence returns the requirement identities (as declared, for
	// example "corpus:kb") that could not be verified, plus any authorization or
	// retrieval error. An empty slice and a nil error means every requirement is
	// satisfied; anything else fails the task closed.
	VerifyRequiredEvidence(ctx context.Context, task *domain.Task) (missing []string, err error)
}

// enforceRequiredEvidence blocks a task whose declared required evidence cannot
// be verified, before it consumes evidence or runs its agent. It is a strict
// no-op when the task declares no required evidence, so existing behavior is
// unchanged. Because no verifier is wired until Package 3, a task with required
// evidence fails closed with domain.REQUIRED_EVIDENCE_MISSING.
func enforceRequiredEvidence(ctx context.Context, saver taskSaver, task *domain.Task, verifier EvidenceVerifier, stdout, stderr io.Writer) (bool, int) {
	if task == nil || len(task.RequiredEvidence) == 0 {
		return false, 0
	}

	var (
		missing []string
		verr    error
	)
	if verifier == nil {
		missing = append([]string{}, task.RequiredEvidence...)
		verr = fmt.Errorf("no evidence verifier is configured")
	} else {
		missing, verr = verifier.VerifyRequiredEvidence(ctx, task)
	}
	if verr == nil && len(missing) == 0 {
		return false, 0
	}

	detail := strings.Join(missing, ", ")
	if detail == "" && verr != nil {
		detail = verr.Error()
	}
	// Record the exact missing/unauthorized identities durably on the task so the
	// blocked record survives a restart and is visible via `sop task`/`sop report`.
	task.BlockedDetail = detail
	if err := blockTask(saver, task, domain.REQUIRED_EVIDENCE_MISSING); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return true, exitError
	}
	fmt.Fprintf(stdout, "%s BLOCKED (%s): required evidence unavailable: %s\n",
		task.ID, domain.REQUIRED_EVIDENCE_MISSING, detail)
	return true, exitError
}
