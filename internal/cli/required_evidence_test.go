package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

type capturingSaver struct{ saved *domain.Task }

func (s *capturingSaver) Save(task *domain.Task) error {
	cp := *task
	s.saved = &cp
	return nil
}

type stubVerifier struct {
	missing []string
	err     error
}

func (v stubVerifier) VerifyRequiredEvidence(context.Context, *domain.Task) ([]string, error) {
	return v.missing, v.err
}

func requiredTask(evidence ...string) *domain.Task {
	return &domain.Task{ID: "T1", Status: domain.PLANNED, BlockedReason: domain.NO_REASON, RequiredEvidence: evidence}
}

func TestEnforceRequiredEvidenceNoRequirementIsNoop(t *testing.T) {
	saver := &capturingSaver{}
	var out, errOut bytes.Buffer

	blocked, code := enforceRequiredEvidence(context.Background(), saver, requiredTask(), nil, &out, &errOut)
	if blocked || code != 0 {
		t.Fatalf("no-op expected: blocked=%v code=%d", blocked, code)
	}
	if saver.saved != nil {
		t.Fatal("saver was called for a task with no required evidence")
	}
}

func TestEnforceRequiredEvidenceMissingVerifierBlocks(t *testing.T) {
	saver := &capturingSaver{}
	var out, errOut bytes.Buffer
	task := requiredTask("corpus:kb")

	blocked, code := enforceRequiredEvidence(context.Background(), saver, task, nil, &out, &errOut)
	if !blocked || code != exitError {
		t.Fatalf("expected BLOCKED: blocked=%v code=%d", blocked, code)
	}
	if task.Status != domain.BLOCKED {
		t.Fatalf("task status = %s, want BLOCKED", task.Status)
	}
	// The durable record is the persisted task: it must carry the reason.
	if saver.saved == nil || saver.saved.Status != domain.BLOCKED || saver.saved.BlockedReason != domain.REQUIRED_EVIDENCE_MISSING {
		t.Fatalf("blocked task not persisted with reason: %+v", saver.saved)
	}
	if !strings.Contains(out.String(), "corpus:kb") {
		t.Fatalf("missing identity not recorded in output: %q", out.String())
	}
	if task.BlockedDetail != "corpus:kb" {
		t.Fatalf("in-memory BlockedDetail = %q, want corpus:kb", task.BlockedDetail)
	}
	if saver.saved.BlockedDetail != "corpus:kb" {
		t.Fatalf("persisted BlockedDetail = %q, want corpus:kb", saver.saved.BlockedDetail)
	}
}

func TestEnforceRequiredEvidenceVerifierOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		missing []string
		err     error
		blocked bool
	}{
		{"satisfied", nil, nil, false},
		{"missing corpus", []string{"corpus:kb"}, nil, true},
		{"missing source", []string{"source:docs/"}, nil, true},
		{"authorization error", nil, errors.New("UNAUTHORIZED_SCOPE"), true},
		{"and semantics (one of two missing)", []string{"corpus:other"}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saver := &capturingSaver{}
			var out, errOut bytes.Buffer
			task := requiredTask("corpus:kb", "source:docs/")

			blocked, code := enforceRequiredEvidence(context.Background(), saver, task, stubVerifier{tc.missing, tc.err}, &out, &errOut)
			if blocked != tc.blocked {
				t.Fatalf("blocked=%v, want %v", blocked, tc.blocked)
			}
			if tc.blocked {
				if code != exitError || task.Status != domain.BLOCKED {
					t.Fatalf("code=%d status=%s", code, task.Status)
				}
			} else if code != 0 || task.Status != domain.PLANNED {
				t.Fatalf("clean pass changed task: code=%d status=%s", code, task.Status)
			}
		})
	}
}
