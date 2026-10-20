package ollamaagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/budget"
)

func writeGoModule(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/verify\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "verify_test.go"), []byte("package verify\n\nimport \"testing\"\n\nfunc TestVerify(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRequiredValidationCommandIsVerificationProgress proves a verification-only
// task may run its configured validation without the stale guard treating that as
// no progress. The credit is per command: the first successful execution of each
// required command resets the streak, and repeating it cannot keep the run alive
// indefinitely.
func TestRequiredValidationCommandIsVerificationProgress(t *testing.T) {
	dir := t.TempDir()
	writeGoModule(t, dir)
	testCommand := `{"tool":"run_command","args":{"command":"go test ./..."}}`
	vetCommand := `{"tool":"run_command","args":{"command":"go vet ./..."}}`
	script := []string{testCommand, vetCommand, testCommand, vetCommand, testCommand, vetCommand}
	f, srv := newFakeOllama(t, script...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100
	cfg.Budget = budget.Defaults()
	cfg.Budget.ImplementIterations = 100
	cfg.Budget.StaleIterations = 3

	req := implementRequest()
	req.DiscoveryIterations = 1 // the discovery window contributes one reset only
	req.ValidationCommands = []string{"go test ./...", "go vet ./..."}

	_, err := New(cfg, dir).Execute(context.Background(), req)
	if err == nil {
		t.Fatal("a run that only repeats validation must still stop at the stale bound")
	}
	// Turn 1 is within the discovery window; turn 2 is the first verification
	// credit for the second required command; turns 3-5 are the three stale turns.
	// Without verification credit the run would have stopped at turn 4.
	if got := f.count(); got != 5 {
		t.Errorf("model turns = %d, want 5 (one discovery + one verification credit + 3 stale)", got)
	}
}

// TestUnrequiredCommandDoesNotEarnVerificationProgress proves the credit is limited
// to the task's configured validation commands, so an unrelated successful command
// remains a stale turn.
func TestUnrequiredCommandDoesNotEarnVerificationProgress(t *testing.T) {
	dir := t.TempDir()
	writeGoModule(t, dir)
	testCommand := `{"tool":"run_command","args":{"command":"go test ./..."}}`
	vetCommand := `{"tool":"run_command","args":{"command":"go vet ./..."}}`
	script := []string{testCommand, vetCommand, testCommand, vetCommand, testCommand, vetCommand}
	f, srv := newFakeOllama(t, script...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100
	cfg.Budget = budget.Defaults()
	cfg.Budget.ImplementIterations = 100
	cfg.Budget.StaleIterations = 3

	req := implementRequest()
	req.DiscoveryIterations = 1
	req.ValidationCommands = []string{"go build ./..."}

	_, err := New(cfg, dir).Execute(context.Background(), req)
	if err == nil {
		t.Fatal("an unrequired command must not keep a no-progress run alive")
	}
	if got := f.count(); got != 4 {
		t.Errorf("model turns = %d, want 4 (one discovery + 3 stale)", got)
	}
	// The diagnostic still carries the no-progress marker, so the recovery policy
	// and operator report are unchanged.
	if err != nil && !strings.Contains(err.Error(), "no repository progress") {
		t.Errorf("error = %v, want the no-progress diagnostic", err)
	}
}
