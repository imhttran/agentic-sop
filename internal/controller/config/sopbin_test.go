package config

import (
	"os"
	"testing"
)

// TestResolveSOPBin covers SOP_BIN precedence: an explicit SOP_BIN value wins over
// the current-binary default, and an unset (or blank) SOP_BIN resolves to the
// currently running executable rather than the literal "sop" command.
func TestResolveSOPBin(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	t.Setenv("SOP_BIN", "/custom/sop")
	if got := ResolveSOPBin(); got != "/custom/sop" {
		t.Errorf("explicit SOP_BIN = %q, want /custom/sop", got)
	}

	if err := os.Unsetenv("SOP_BIN"); err != nil {
		t.Fatal(err)
	}
	if got := ResolveSOPBin(); got != exe {
		t.Errorf("unset SOP_BIN = %q, want current executable %q", got, exe)
	}

	// A whitespace-only SOP_BIN is treated as unset and falls back to the current
	// binary.
	t.Setenv("SOP_BIN", "   ")
	if got := ResolveSOPBin(); got != exe {
		t.Errorf("blank SOP_BIN = %q, want current executable %q", got, exe)
	}
}
