package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// AUTONOMY preventive-safety regressions: a prohibited operation is refused BEFORE
// it can take effect, leaves NO side effect, and is recorded as a denial in the
// audit. These reuse the existing toolharness authorization and audit mechanisms;
// no new enforcement is introduced.

func TestDeniedOperationsCauseNoSideEffect(t *testing.T) {
	root := t.TempDir()
	log := NewAuditLog(64)
	h := New(root, DefaultConfig(), log)
	ctx := context.Background()

	// A protected-path write is refused, and SOP's state database is not created.
	if _, err := h.Run(ctx, ToolCreateFile, map[string]any{"path": ".agent-sdlc/state.db", "content": "tampered"}); !errors.Is(err, ErrProtectedPath) {
		t.Fatalf("create state.db err = %v, want ErrProtectedPath", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-sdlc", "state.db")); !os.IsNotExist(err) {
		t.Errorf("state.db side effect: stat err = %v, want not-exist", err)
	}

	// An out-of-scope write (a path escaping the repository root) is refused, and
	// nothing is written outside the root.
	escape := filepath.Join(filepath.Dir(root), "escape.txt")
	if _, err := h.Run(ctx, ToolWriteFile, map[string]any{"path": "../escape.txt", "content": "x"}); err == nil {
		t.Fatal("out-of-scope write should be rejected")
	}
	if _, err := os.Stat(escape); !os.IsNotExist(err) {
		t.Errorf("escape-file side effect: stat err = %v, want not-exist", err)
	}

	// A destructive command is refused before execution.
	for _, cmd := range []string{"git commit -m x", "git push", "rm -rf .", "git reset --hard"} {
		if _, err := h.Run(ctx, ToolRunCommand, map[string]any{"command": cmd}); !errors.Is(err, errCommandNotAllowed) {
			t.Errorf("run_command(%q) err = %v, want errCommandNotAllowed", cmd, err)
		}
	}

	// Every policy refusal of a protected path or a destructive command is audited as
	// a denial, so the audit answers "what was refused". (An out-of-scope path escape
	// is refused as well; it is audited as an error, not a sentinel denial.)
	denied := 0
	for _, rec := range log.Records() {
		if rec.Action == ActionDeny && rec.Outcome == OutcomeDenied {
			denied++
		}
	}
	if denied < 5 {
		t.Errorf("audited denials = %d, want >= 5", denied)
	}
}
