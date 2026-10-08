package archtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RSH-005 provider/model-neutrality guard for the review and change-attribution
// seam in internal/cli.
//
// The existing guard in this package (TestArchitectureGuard and its corePackages
// inventory) protects the Phase 8 core and policy/governance packages by import
// graph. It does NOT cover the review and change-attribution seam in
// internal/cli (reviewscope.go, jev.go, review.go), which decides WHICH change set
// is offered to review and HOW changed files are attributed. RSH-002..RSH-004 touch
// that seam; this check proves those changes introduce no provider-, model-, or
// domain-specific behavior into it.
//
// The invariant: the seam is decided by declared capabilities, repository paths,
// and evidence — never by a provider name, a model name, or a domain branch. The
// guard is a stdlib-only source scan (offline, CI-safe), in the same style as the
// rest of this package.

// reviewSeamFiles are the internal/cli production files that implement the review
// input scoping and the change attribution seam. Each must exist (asserted by
// TestReviewSeamNeutralityFilesExist) so the check can never be vacuously satisfied.
var reviewSeamFiles = []string{
	"internal/cli/reviewscope.go",
	"internal/cli/jev.go",
	"internal/cli/review.go",
}

// providerModelTokens are the provider, model, and vendor names that must never
// appear in the provider-neutral review seam. Matching is case-insensitive so a
// literal provider/model reference is caught while unrelated identifiers are not
// affected by case.
var providerModelTokens = []string{
	"ollama",
	"openai",
	"anthropic",
	"claude",
	"qwen",
	"deepseek",
	"gemini",
	"llamacpp",
	"llama.cpp",
	"mlx",
	"gpt-4",
	"gpt-3",
}

// TestReviewSeamStaysProviderAndModelNeutral fails if a provider, model, or vendor
// name appears in the internal/cli review and change-attribution seam. The seam
// must be decided by paths, capabilities, and evidence, so a provider/model name
// there is a neutrality regression.
func TestReviewSeamStaysProviderAndModelNeutral(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range reviewSeamFiles {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		lower := strings.ToLower(string(src))
		for _, token := range providerModelTokens {
			if strings.Contains(lower, token) {
				t.Errorf("%s names provider/model token %q; the review and change-attribution seam must stay provider- and model-neutral", rel, token)
			}
		}
	}
}

// TestReviewSeamNeutralityFilesExist asserts every covered seam file exists, so the
// guard cannot silently protect nothing.
func TestReviewSeamNeutralityFilesExist(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range reviewSeamFiles {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || info.IsDir() {
			t.Errorf("declared review-seam file %s does not exist (err=%v)", rel, err)
		}
	}
}

// TestReviewSeamNeutralityDetectsRepresentativeToken is the negative control: the
// detection predicate must actually fire on a synthetic provider-naming source, so
// the guard cannot pass vacuously.
func TestReviewSeamNeutralityDetectsRepresentativeToken(t *testing.T) {
	src := "// served by ollama\npackage cli\n"
	lower := strings.ToLower(src)
	found := false
	for _, token := range providerModelTokens {
		if strings.Contains(lower, token) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("the neutrality predicate must flag a provider name; detection is vacuous")
	}
}
