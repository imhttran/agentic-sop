//go:build jev213_acceptance

package commandprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are the JEV-213 live-dogfood acceptance checks. They validate the real
// evidence artifact and are build-tagged so an ordinary `go test ./...` (which
// must stay green before the artifact exists) excludes them.

func dogfoodRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root from %s", dir)
		}
		dir = parent
	}
}

func TestAdapterDogfoodEvidenceContract(t *testing.T) {
	root := dogfoodRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adapterDogfoodEvidencePath)))
	if err != nil {
		t.Fatalf("JEV-213 evidence artifact missing (%v)", err)
	}
	ev, err := decodeAdapterDogfood(data)
	if err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	exists := func(p string) bool {
		if strings.TrimSpace(p) == "" {
			return false
		}
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		return err == nil
	}
	if err := validateAdapterDogfood(ev, exists); err != nil {
		t.Fatalf("evidence violates the JEV-213 contract: %v", err)
	}
}

func TestAdapterDogfoodEvidenceHasNoCredentials(t *testing.T) {
	root := dogfoodRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adapterDogfoodEvidencePath)))
	if err != nil {
		t.Fatalf("JEV-213 evidence artifact missing (%v)", err)
	}
	low := strings.ToLower(string(data))
	for _, bad := range []string{"api_key", "apikey", "bearer", "password", "secret", "token=", "-----begin"} {
		if strings.Contains(low, bad) {
			t.Errorf("evidence appears to contain credential material: %q", bad)
		}
	}
}
