package validate

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

func TestChecksOrderAndCategory(t *testing.T) {
	checks := Checks(config.Validation{
		Build: []string{"go build ./...", "  "},
		Test:  []string{"go test ./..."},
		Lint:  []string{"go vet ./..."},
	})

	want := []struct {
		category testrunner.Category
		command  string
	}{
		{testrunner.Build, "go build ./..."},
		{testrunner.UnitTest, "go test ./..."},
		{testrunner.Lint, "go vet ./..."},
	}
	if len(checks) != len(want) {
		t.Fatalf("got %d checks, want %d: %+v", len(checks), len(want), checks)
	}
	for i, w := range want {
		if checks[i].Category != w.category || checks[i].Command != w.command {
			t.Errorf("check[%d] = %+v, want %+v", i, checks[i], w)
		}
	}
}

func TestChecksEmpty(t *testing.T) {
	if got := Checks(config.Validation{}); len(got) != 0 {
		t.Errorf("Checks(empty) = %+v, want none", got)
	}
	if Enabled(config.Validation{}) {
		t.Error("Enabled(empty) = true, want false")
	}
	if !Enabled(config.Validation{Build: []string{"go build ./..."}}) {
		t.Error("Enabled(build) = false, want true")
	}
}

func TestRunFailFast(t *testing.T) {
	dir := t.TempDir()
	res := Run(context.Background(), dir, config.Validation{
		Build: []string{"false"},
		Test:  []string{"true"},
	})
	if res.Passed() {
		t.Fatal("suite passed, want failure")
	}
	if len(res.Results) != 1 {
		t.Fatalf("got %d results, want 1 (fail-fast)", len(res.Results))
	}
	if res.Results[0].Status != testrunner.Fail {
		t.Errorf("status = %s, want FAIL", res.Results[0].Status)
	}
}

func TestRunPassAndDiagnostics(t *testing.T) {
	dir := t.TempDir()

	ok := Run(context.Background(), dir, config.Validation{Test: []string{"true"}})
	if !ok.Passed() {
		t.Fatalf("expected pass, got %+v", ok)
	}

	bad := Run(context.Background(), dir, config.Validation{Lint: []string{"echo boom >&2; exit 1"}})
	if bad.Passed() {
		t.Fatal("expected failure")
	}
	if !strings.Contains(bad.Results[0].Stderr, "boom") {
		t.Errorf("stderr = %q, want it to include diagnostics", bad.Results[0].Stderr)
	}
}
