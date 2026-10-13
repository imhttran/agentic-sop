package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSessionFileEnv covers the resolution of SOP_CONTROLLER_SESSION_FILE: unset uses the
// ignored default, an explicit path is honored, and an explicit empty value disables
// persistence (sessions then end on restart).
func TestSessionFileEnv(t *testing.T) {
	if err := os.Unsetenv("SOP_CONTROLLER_SESSION_FILE"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(".run", "sessions.json")
	if got := sessionFileEnv(); got != want {
		t.Errorf("default = %q, want %q", got, want)
	}

	t.Setenv("SOP_CONTROLLER_SESSION_FILE", "")
	if got := sessionFileEnv(); got != "" {
		t.Errorf("explicit empty should disable persistence, got %q", got)
	}

	t.Setenv("SOP_CONTROLLER_SESSION_FILE", "/tmp/x/sessions.json")
	if got := sessionFileEnv(); got != "/tmp/x/sessions.json" {
		t.Errorf("explicit path = %q", got)
	}
}
