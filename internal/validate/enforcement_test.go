package validate

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// TestEnabledRequiresAtLeastOneCommand: an empty validation set is not "enabled",
// which is what lets the lifecycle distinguish "no validation configured" from
// "validation ran and passed".
func TestEnabledRequiresAtLeastOneCommand(t *testing.T) {
	if Enabled(config.Validation{}) {
		t.Error("an empty validation set must not be enabled")
	}
	if !Enabled(config.Validation{Build: []string{"go build ./..."}}) {
		t.Error("a configured build check must be enabled")
	}
}

// TestChecksOrderIsBuildTestLint pins the deterministic order (build, test, lint)
// and that a multi-command lint category (go vet plus a gofmt check) is preserved.
func TestChecksOrderIsBuildTestLint(t *testing.T) {
	v := config.Validation{
		Build: []string{"go build ./..."},
		Test:  []string{"go test ./..."},
		Lint:  []string{"go vet ./...", "test -z \"$(gofmt -l .)\""},
	}
	checks := Checks(v)
	want := []string{"go build ./...", "go test ./...", "go vet ./...", "test -z \"$(gofmt -l .)\""}
	if len(checks) != len(want) {
		t.Fatalf("checks = %v, want %v", checks, want)
	}
	for i, c := range checks {
		if c.Command != want[i] {
			t.Errorf("check[%d] = %q, want %q", i, c.Command, want[i])
		}
	}
}

// TestRunSuccessPasses: a configured check that succeeds reports PASS.
func TestRunSuccessPasses(t *testing.T) {
	res := Run(context.Background(), t.TempDir(), config.Validation{Build: []string{"true"}})
	if !res.Passed() {
		t.Fatalf("status = %s, want PASS", res.Status)
	}
}

// TestRunFailureIsNotPassed: a configured check that fails reports a non-pass.
func TestRunFailureIsNotPassed(t *testing.T) {
	res := Run(context.Background(), t.TempDir(), config.Validation{Build: []string{"false"}})
	if res.Passed() {
		t.Fatal("a failing configured check must not pass")
	}
}
