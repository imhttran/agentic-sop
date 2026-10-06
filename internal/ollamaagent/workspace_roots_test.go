package ollamaagent

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

func TestParseWorkspaceRootsModes(t *testing.T) {
	roots, err := parseWorkspaceRoots(`[{"path":"/tmp/alpha","mode":"read-write"},{"path":"/tmp/beta","mode":"read"},{"path":"/tmp/gamma","mode":"bogus"},{"path":"","mode":"read-write"}]`)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(roots) != 3 {
		t.Fatalf("roots = %+v, want 3 (an empty path is skipped)", roots)
	}
	if roots[0].Path != "/tmp/alpha" || roots[0].Mode != toolharness.RootReadWrite {
		t.Errorf("read-write root = %+v", roots[0])
	}
	if roots[1].Path != "/tmp/beta" || roots[1].Mode != toolharness.RootReadOnly {
		t.Errorf("read root = %+v", roots[1])
	}
	if roots[2].Mode != toolharness.RootReadOnly {
		t.Errorf("an unknown mode must fail closed to read-only: %+v", roots[2])
	}
}

func TestParseWorkspaceRootsMalformedFailsClosed(t *testing.T) {
	if _, err := parseWorkspaceRoots(`{not json`); err == nil {
		t.Fatal("malformed SOP_WORKSPACE_ROOTS must be rejected, never silently grant access")
	}
}
