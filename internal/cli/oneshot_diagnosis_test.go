package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/verify"
)

// TestOneShotDiagnosisClassificationKinds pins the failure classifications the
// E2E diagnosis cites (E2E-001): the kinds named in the VER failure transitions
// exist and are non-empty, so a rename cannot silently change the diagnosis.
func TestOneShotDiagnosisClassificationKinds(t *testing.T) {
	for _, k := range []failure.Kind{
		failure.IncompleteImplementation,
		failure.AutoFixExhausted,
		failure.NoProgress,
		failure.ApprovalRequired,
	} {
		if k == "" {
			t.Errorf("empty failure kind")
		}
	}
}

// TestOneShotTierSelection pins the E2E-001 diagnosis (RC-3): with risk-based
// verification enabled, a task's plan-time shape alone does not reach LOW — the
// tier collapses to MEDIUM — so the configured LOW targeted set is unreachable
// without an operator verify-first declaration. It also pins that selectValidation
// is the full configured set when risk-based verification is disabled.
func TestOneShotTierSelection(t *testing.T) {
	on := true
	cfg := config.Config{}
	cfg.Verification.RiskBased = &on
	cfg.Verification.Low.Build = []string{"go build ./..."}
	cfg.Verification.Low.Test = []string{"go test ./internal/verify/..."}
	cfg.Validation.Build = []string{"go build ./..."}
	cfg.Validation.Test = []string{"go test ./..."}
	cfg.Validation.Lint = []string{"go vet ./..."}

	if got := verificationTier(cfg, &taskfile.Spec{}); got != verify.Medium {
		t.Fatalf("empty task tier = %q, want MEDIUM (RC-3)", got)
	}
	verifyFirst := &taskfile.Spec{ExecutionMode: domain.ExecutionVerifyFirst}
	if got := verificationTier(cfg, verifyFirst); got != verify.Low {
		t.Fatalf("verify-first tier = %q, want LOW", got)
	}
	if got := selectValidation(cfg, verifyFirst); len(got.Test) != 1 || got.Test[0] != "go test ./internal/verify/..." {
		t.Fatalf("LOW test set = %v, want the targeted set", got.Test)
	}
	off := false
	offcfg := cfg
	offcfg.Verification.RiskBased = &off
	if got := selectValidation(offcfg, &taskfile.Spec{}); len(got.Lint) != 1 || got.Test[0] != "go test ./..." {
		t.Fatalf("risk_based:false set = %+v, want the full set", got)
	}
}
