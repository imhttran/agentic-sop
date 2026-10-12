package failure

import "testing"

// TestClassifyMissingValidationIsAConfigurationFailure proves a missing required
// validation set is its own failure kind, not an implementation failure and not an
// approval: the change may be correct, but SOP has no evidence and must not pass.
func TestClassifyMissingValidationIsAConfigurationFailure(t *testing.T) {
	c := Classify(Evidence{Source: "VALIDATE", ValidationRequired: true, ValidationConfigured: false})
	if c.Kind != ValidationNotConfigured {
		t.Fatalf("kind = %s, want %s", c.Kind, ValidationNotConfigured)
	}
	if c.Disposition != Block {
		t.Fatalf("disposition = %s, want BLOCK (neither an implementation failure nor an approval)", c.Disposition)
	}
	if c.Disposition == NeedsHuman || c.Disposition == AutoFix {
		t.Fatalf("a missing validation set must not be a human approval or an auto-fix: %+v", c)
	}
}

// TestClassifyConfiguredValidationFallsThrough proves the configuration branch does
// not swallow an ordinary implementation failure: with validation configured, a red
// build classifies exactly as before.
func TestClassifyConfiguredValidationFallsThrough(t *testing.T) {
	c := Classify(Evidence{Source: "VALIDATE", ValidationRequired: true, ValidationConfigured: true, BuildFailed: true, MaxFixCycles: 3})
	if c.Kind == ValidationNotConfigured {
		t.Fatalf("a configured validation failure must not be a configuration failure: %+v", c)
	}
	if c.Kind != CompilerError {
		t.Fatalf("kind = %s, want %s", c.Kind, CompilerError)
	}
}

// TestClassifyMissingValidationOutranksVerificationEvidence: the configuration
// failure is named for what it is even when other evidence (for example absent test
// coverage) would otherwise fire.
func TestClassifyMissingValidationOutranksVerificationEvidence(t *testing.T) {
	c := Classify(Evidence{Source: "VALIDATE", ValidationRequired: true, ValidationConfigured: false, TestsMissing: true})
	if c.Kind != ValidationNotConfigured {
		t.Fatalf("kind = %s, want %s", c.Kind, ValidationNotConfigured)
	}
}
