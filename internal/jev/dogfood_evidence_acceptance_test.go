//go:build jev207_acceptance

package jev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are the registered JEV-207 acceptance checks. They validate the LIVE
// dogfood evidence artifact, so they are build-tagged: an ordinary
// `go test ./...` (which must stay green without live evidence) excludes them,
// and they are invoked explicitly by the registered acceptance checks with
// -tags=jev207_acceptance. They FAIL when the evidence artifact is missing, so a
// heading-only prose report (or none) can never produce VERIFIED.
//
// The deterministic validator they use (validateDogfoodEvidence, decode-,
// credential scanning) lives in dogfood_evidence_test.go and is proven by
// synthetic tests there.

func TestJEVDogfoodEvidenceHasExecutionProof(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dogfoodEvidencePath)))
	if err != nil {
		t.Fatalf("JEV-207 evidence artifact missing (%v); JEV-207 requires a live dogfood run under Gate 4 — this check correctly fails until it exists", err)
	}
	ev, err := decodeDogfoodEvidence(data)
	if err != nil {
		t.Fatalf("decode JEV-207 evidence: %v", err)
	}
	if err := validateDogfoodEvidence(ev, artifactExists(root)); err != nil {
		t.Fatalf("JEV-207 evidence is not execution-backed: %v", err)
	}
}

func TestJEVDogfoodEvidenceHasNoCredentials(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dogfoodEvidencePath)))
	if err != nil {
		t.Fatalf("JEV-207 evidence artifact missing (%v)", err)
	}
	if hits := credentialMarkersIn(data); len(hits) > 0 {
		t.Errorf("JEV-207 evidence appears to contain credential material: %v", hits)
	}
}

func artifactExists(root string) func(string) bool {
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
