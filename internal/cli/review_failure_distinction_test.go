package cli

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
)

// TestReviewFailureDistinctionInfrastructureVsCode pins the boundary between a
// review INFRASTRUCTURE failure and a review CODE failure at the deterministic
// classifier. A review provider error is a failure to review: it is classified by
// failure.Classify — the same classifier runScheduledTask applies to the lifecycle
// error — as an infrastructure disposition (RETRY, or NEEDS_HUMAN when the error is
// unrecognized and therefore fails closed). It is never AUTO_FIX: only a genuine
// blocking finding (a code defect) enters the bounded fix loop.
func TestReviewFailureDistinctionInfrastructureVsCode(t *testing.T) {
	tests := []struct {
		name     string
		errText  string
		wantKind failure.Kind
		wantDisp failure.Disposition
		// forbid names dispositions that would misclassify this failure: a review
		// infrastructure/unknown failure must never be auto-fixed, and a transient
		// one must never be a human boundary or a silent continuation.
		forbid []failure.Disposition
	}{
		{
			name:     "connection refused is a transient provider failure",
			errText:  "review agent: connection refused",
			wantKind: failure.TransientProvider,
			wantDisp: failure.Retry,
			forbid:   []failure.Disposition{failure.AutoFix, failure.NeedsHuman},
		},
		{
			name:     "i/o timeout is a transient provider failure",
			errText:  "review provider: i/o timeout",
			wantKind: failure.TransientProvider,
			wantDisp: failure.Retry,
			forbid:   []failure.Disposition{failure.AutoFix, failure.NeedsHuman},
		},
		{
			name:     "temporarily unavailable is a transient provider failure",
			errText:  "review provider: temporarily unavailable",
			wantKind: failure.TransientProvider,
			wantDisp: failure.Retry,
			forbid:   []failure.Disposition{failure.AutoFix, failure.NeedsHuman},
		},
		{
			name:     "unrecognized review error fails closed to a human",
			errText:  "review agent: unsupported review verdict",
			wantKind: failure.Unknown,
			wantDisp: failure.NeedsHuman,
			forbid:   []failure.Disposition{failure.AutoFix, failure.Continue},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errText)
			// Classify exactly as runScheduledTask does for the wrapped lifecycle
			// error: Source "run", the error, no approval boundary.
			cls := failure.Classify(failure.Evidence{Source: "run", Err: err})

			if cls.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q (err %q)", cls.Kind, tt.wantKind, tt.errText)
			}
			if cls.Disposition != tt.wantDisp {
				t.Errorf("Disposition = %q, want %q (err %q)", cls.Disposition, tt.wantDisp, tt.errText)
			}
			for _, bad := range tt.forbid {
				if cls.Disposition == bad {
					t.Errorf("Disposition = %q for %q; a review infrastructure failure must not be classified as %q", cls.Disposition, tt.errText, bad)
				}
			}
		})
	}
}

// TestReviewDistinctionBlockingFindingIsCodeDefect pins the other half of the
// distinction. A genuine blocking review finding at a failing severity (HIGH under
// config.Default()'s fail_on = critical,high) is CODE evidence, not infrastructure:
// quality.Evaluate returns FAIL, which is what enters the bounded fix loop. The same
// evidence is AUTO_FIX at the classifier, categorically distinct from the RETRY
// infrastructure case above.
func TestReviewDistinctionBlockingFindingIsCodeDefect(t *testing.T) {
	cfg := config.Default()

	finding := review.Finding{
		Severity: review.High,
		Title:    "unresolved correctness finding",
		Detail:   "a genuine HIGH finding remains unresolved after review",
		File:     "internal/cli/run.go",
		Line:     42,
	}

	gate := quality.Evaluate(cfg.Quality, quality.Input{
		BuildPassed: true,
		TestPassed:  true,
		Unresolved:  []review.Finding{finding},
	})
	if gate.Decision != quality.Fail {
		t.Fatalf("gate.Decision = %q, want %q for a blocking HIGH finding (so the fix loop runs)", gate.Decision, quality.Fail)
	}

	// A blocking finding is a code defect: the classifier maps it to AUTO_FIX, the
	// disposition that drives the bounded fix loop — never an infrastructure retry.
	cls := failure.Classify(failure.Evidence{Source: "run", BlockingFindings: 1})
	if cls.Kind != failure.BlockingFindings {
		t.Errorf("Kind = %q, want %q", cls.Kind, failure.BlockingFindings)
	}
	if cls.Disposition != failure.AutoFix {
		t.Errorf("Disposition = %q, want %q", cls.Disposition, failure.AutoFix)
	}
}
