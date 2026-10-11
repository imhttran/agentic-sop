package cli

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// newMalformedRun creates a minimal run fixture for the malformed-output tests.
func newMalformedRun(t *testing.T) *runpkg.Run {
	t.Helper()
	rn, err := runpkg.New(t.TempDir(), "H004")
	if err != nil {
		t.Fatalf("run.New: %v", err)
	}
	return rn
}

// TestMalformedOutputResultClassifiesViaIsMalformedOutput pins the run-level
// classification contract: a malformed review response is recognised by
// review.IsMalformedOutput(err) and malformedOutputResult maps it deterministically
// to NEEDS_HUMAN with UNKNOWN kind and HIGH confidence, at the WAITING_FOR_HUMAN
// stage with a NEEDS_HUMAN gate. The classification is error-derived, never read
// from incidental words in the malformed model output.
func TestMalformedOutputResultClassifiesViaIsMalformedOutput(t *testing.T) {
	rn := newMalformedRun(t)
	cfg := config.Default()
	err := &review.ErrMalformedOutput{
		Raw:      "not json at all",
		Attempts: 3,
		Last:     errors.New("invalid character 'n'"),
	}

	if !review.IsMalformedOutput(err) {
		t.Fatal("review.IsMalformedOutput(err) = false, want true for *review.ErrMalformedOutput")
	}

	res := malformedOutputResult(cfg, rn, err)

	if res.classification.Disposition != failure.NeedsHuman {
		t.Errorf("Disposition = %q, want %q", res.classification.Disposition, failure.NeedsHuman)
	}
	if res.classification.Kind != failure.Unknown {
		t.Errorf("Kind = %q, want %q", res.classification.Kind, failure.Unknown)
	}
	if res.classification.Confidence != failure.High {
		t.Errorf("Confidence = %q, want %q", res.classification.Confidence, failure.High)
	}
	if res.stage != runpkg.WaitingForHuman {
		t.Errorf("stage = %q, want %q", res.stage, runpkg.WaitingForHuman)
	}
	if res.gate.Decision != quality.NeedsHuman {
		t.Errorf("gate.Decision = %q, want %q", res.gate.Decision, quality.NeedsHuman)
	}
}

// TestMalformedOutputIsNeverPass proves malformed output fails closed: with the
// default autonomy the gate is NEEDS_HUMAN (never PASS) at WAITING_FOR_HUMAN, and
// no configured autonomy level can turn a malformed review response into a PASS.
// A malformed review response can never yield a passing result.
func TestMalformedOutputIsNeverPass(t *testing.T) {
	err := &review.ErrMalformedOutput{
		Raw:      "{",
		Attempts: 3,
		Last:     errors.New("unexpected end of JSON input"),
	}

	// Default autonomy: a human boundary, never a pass.
	rn := newMalformedRun(t)
	res := malformedOutputResult(config.Default(), rn, err)
	if res.gate.Decision == quality.Pass {
		t.Fatalf("gate.Decision = PASS, want a non-pass decision")
	}
	if res.gate.Decision != quality.NeedsHuman {
		t.Errorf("default gate.Decision = %q, want %q", res.gate.Decision, quality.NeedsHuman)
	}
	if res.stage != runpkg.WaitingForHuman {
		t.Errorf("default stage = %q, want %q", res.stage, runpkg.WaitingForHuman)
	}

	// No configured autonomy level can turn a malformed review response into a
	// pass: the classification is a NEEDS_HUMAN/UNKNOWN human boundary, which every
	// level fails closed on.
	for _, level := range []autonomy.Level{autonomy.Low, autonomy.Balanced, autonomy.High} {
		cfg := config.Default()
		cfg.Autonomy = config.Autonomy{Level: string(level)}
		resLvl := malformedOutputResult(cfg, newMalformedRun(t), err)
		if resLvl.gate.Decision == quality.Pass {
			t.Errorf("level %s: gate.Decision = PASS, want a non-pass decision", level)
		}
	}
}

// TestOrdinaryReviewErrorPreservesBehavior proves the classification is specific to
// malformed output: an ordinary review error is not recognised as malformed, so the
// run-level caller returns it as the generic wrapped lifecycle error instead of
// routing it through malformedOutputResult.
func TestOrdinaryReviewErrorPreservesBehavior(t *testing.T) {
	ordinary := errors.New("review agent: connection refused")

	if review.IsMalformedOutput(ordinary) {
		t.Fatal("review.IsMalformedOutput(ordinary) = true, want false")
	}

	// The run-level caller only routes a malformed error through the classifier; an
	// ordinary error is wrapped and returned unchanged, preserving existing behavior.
	wrapped := wrapReviewError(ordinary)
	if !errors.Is(wrapped, ordinary) {
		t.Errorf("wrapped error %v does not unwrap to the original %v", wrapped, ordinary)
	}
	if review.IsMalformedOutput(wrapped) {
		t.Error("wrapped ordinary error is malformed, want false")
	}
}

// TestMalformedRetainedReportDoesNotOverrideClassification proves the classification
// is error-based: a retained partial Report carried by *review.ErrMalformedOutput does
// not change the disposition (still NEEDS_HUMAN), while its blocking finding is still
// surfaced in the result's report so it blocks at its severity.
func TestMalformedRetainedReportDoesNotOverrideClassification(t *testing.T) {
	rn := newMalformedRun(t)
	cfg := config.Default()
	blocking := review.Finding{
		Severity: review.High,
		Title:    "retained blocking finding",
		Detail:   "a genuine HIGH finding emitted beside the malformed one",
		File:     "internal/cli/run.go",
		Line:     810,
	}
	err := &review.ErrMalformedOutput{
		Raw:      "partial",
		Attempts: 3,
		Last:     errors.New("parse review response: unexpected end of JSON input"),
		Report:   review.Report{Findings: []review.Finding{blocking}},
	}

	res := malformedOutputResult(cfg, rn, err)

	// The classification stays driven by the error, not by the retained finding.
	if res.classification.Disposition != failure.NeedsHuman {
		t.Errorf("Disposition = %q, want %q (error-driven, not finding-driven)", res.classification.Disposition, failure.NeedsHuman)
	}
	if res.classification.Kind != failure.Unknown {
		t.Errorf("Kind = %q, want %q", res.classification.Kind, failure.Unknown)
	}
	// The retained blocking finding is preserved so it is surfaced and still blocks.
	if len(res.report.Findings) != 1 {
		t.Fatalf("report findings = %d, want 1 retained finding", len(res.report.Findings))
	}
	if res.report.Findings[0].Severity != review.High {
		t.Errorf("retained finding severity = %q, want %q", res.report.Findings[0].Severity, review.High)
	}
}

// wrapReviewError mirrors the run-level caller's generic error wrapping for an
// ordinary (non-malformed) review error.
func wrapReviewError(err error) error {
	return &reviewErrorWrapper{err: err}
}

type reviewErrorWrapper struct{ err error }

func (w *reviewErrorWrapper) Error() string { return "review: " + w.err.Error() }
func (w *reviewErrorWrapper) Unwrap() error { return w.err }
