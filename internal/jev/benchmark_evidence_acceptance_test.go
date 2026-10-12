//go:build jev208_acceptance

package jev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are the registered JEV-208 acceptance checks. They validate the LIVE
// benchmark evidence artifact, so they are build-tagged: an ordinary
// `go test ./...` (which must stay green before evidence exists) excludes them,
// and they are invoked explicitly with -tags=jev208_acceptance. They FAIL when
// the artifact is missing, so a heading-only or prose-only report cannot produce
// VERIFIED.
//
// The deterministic validator they use (validate-/decode-/credential scanning)
// lives in benchmark_evidence_test.go and is proven by synthetic tests there.

func TestJEVBenchmarkEvidenceHasRequiredFields(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(benchmarkEvidencePath)))
	if err != nil {
		t.Fatalf("JEV-208 benchmark evidence artifact missing (%v); JEV-208 requires the comparative benchmark — this check correctly fails until it exists", err)
	}
	ev, err := decodeBenchmarkEvidence(data)
	if err != nil {
		t.Fatalf("decode JEV-208 benchmark evidence: %v", err)
	}
	if err := validateBenchmarkEvidence(ev, resolvingArtifactExists(root)); err != nil {
		t.Fatalf("JEV-208 benchmark evidence is incomplete: %v", err)
	}
}

func TestJEVBenchmarkEvidenceHasNoCredentials(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(benchmarkEvidencePath)))
	if err != nil {
		t.Fatalf("JEV-208 benchmark evidence artifact missing (%v)", err)
	}
	if hits := credentialMarkersIn(data); len(hits) > 0 {
		t.Errorf("JEV-208 benchmark evidence appears to contain credential material: %v", hits)
	}
}

// resolvingArtifactExists reports whether a referenced artifact resolves, relative
// to root or as an absolute path.
func resolvingArtifactExists(root string) func(string) bool {
	return func(p string) bool {
		if strings.TrimSpace(p) == "" {
			return false
		}
		if filepath.IsAbs(p) {
			_, err := os.Stat(p)
			return err == nil
		}
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		return err == nil
	}
}
