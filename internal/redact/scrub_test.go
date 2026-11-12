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
