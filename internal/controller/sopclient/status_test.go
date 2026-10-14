package sopclient

import "testing"

// A missing SOP artifact must never imply PASS. This covers all four CTRL003
// status projections independently: validation, review, quality/JEV, and run
// status each report an explicit absence, not a passing value, when their
// backing SOP artifact is missing.
func TestMissingArtifactNeverImpliesPASS(t *testing.T) {
	if got := aggregateValidation(nil); got.Present || got.Level != StatusUnknown {
		t.Fatalf("aggregateValidation(nil) = %+v, want absent/UNKNOWN", got)
	}
	if got := aggregateReview(Review{}); got.Present || got.Level != StatusUnknown {
		t.Fatalf("aggregateReview(zero) = %+v, want absent/UNKNOWN", got)
	}
	if got := aggregateQuality(nil); got.Present || got.Level != StatusUnknown {
		t.Fatalf("aggregateQuality(nil) = %+v, want absent/UNKNOWN", got)
	}
	if got := (RunInfo{}).Status(); got.Present || got.Decision == "PASS" || got.Decision == "PASSED" {
		t.Fatalf("RunInfo{}.Status() = %+v, want absent run status, never a pass decision", got)
	}
}
