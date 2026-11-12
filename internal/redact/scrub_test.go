package redact

import (
	"strings"
	"testing"
)

// TestScrubMergesOverlappingExactAndShapeSpans covers both ordering directions: an
// inventory value that extends past a token shape, and one that only prefixes a
// token. Either tail must not survive a single pass.
func TestScrubMergesOverlappingExactAndShapeSpans(t *testing.T) {
	policy := Policy{Enabled: true, Values: []string{"ghp_0123456789abcdef-tailX", "sk-abc"}}
	out := Scrub("a ghp_0123456789abcdef-tailX b sk-abcdef123456 c", nil, policy, Fixed("[M]"))
	for _, leaked := range []string{"tailX", "-tail", "def123456"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("tail %q survived: %q", leaked, out)
		}
	}
}

// TestPolicyCoversCasesTheDisabledPathRedacts checks the connector and suffix
// forms the precise rule missed, so enabling the policy does not redact less.
func TestPolicyCoversCasesTheDisabledPathRedacts(t *testing.T) {
	for _, in := range []string{"password for db: hunter2", "tokenvalue=abc123"} {
		if !strings.Contains(Scrub(in, nil, Policy{}, Fixed("[M]")), "[M]") {
			t.Fatalf("disabled path no longer redacts %q", in)
		}
		if got := Scrub(in, nil, Policy{Enabled: true}, Fixed("[M]")); strings.Contains(got, "hunter2") || strings.Contains(got, "abc123") {
			t.Fatalf("policy-on path leaked the value of %q: %q", in, got)
		}
	}
}

// TestPolicyKeepsSecretaryFalsePositiveSurviving keeps the approved C1
// false-positive behavior: an ordinary word that starts with a credential word is
// not redacted when the policy is enabled.
func TestPolicyKeepsSecretaryFalsePositiveSurviving(t *testing.T) {
	in := "secretary=quarterly-plan-2026"
	if got := Scrub(in, nil, Policy{Enabled: true}, Fixed("[M]")); got != in {
		t.Fatalf("policy-on path redacted an ordinary word: %q", got)
	}
}
