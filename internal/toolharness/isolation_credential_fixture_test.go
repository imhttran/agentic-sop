package toolharness

// safeCommandCredentialFixture is a minimal package whose tests read the fake
// HOME's credential, an ordinary neighbour, and a pre-existing hardlink alias.
// The macOS and Linux SAFE-command tests share it so the two vectors cannot
// drift apart. It is driven by `go test`, which the command policy classifies
// SAFE: the credential read happens in a process the policy already allowed.
const safeCommandCredentialFixture = `package cred

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialRead(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", "probe"))
	if err != nil {
		t.Fatalf("read credential: %v", err)
	}
	t.Logf("credential=%s", string(b))
}

func TestOrdinaryRead(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "notes.txt"))
	if err != nil {
		t.Fatalf("read ordinary: %v", err)
	}
	t.Logf("ordinary=%s", string(b))
}

func TestHardlinkRead(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "alias.txt"))
	if err != nil {
		t.Fatalf("read hardlink: %v", err)
	}
	t.Logf("hardlink=%s", string(b))
}

func TestSymlinkRead(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "link.txt"))
	if err != nil {
		t.Fatalf("read symlink: %v", err)
	}
	t.Logf("symlink=%s", string(b))
}
`
