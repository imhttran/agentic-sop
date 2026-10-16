package quality

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// gateTestPolicy builds the deterministic default quality policy used by the
// PostgreSQL-category GATE tests: tests required, a three-cycle fix budget, and
// the standard blocking severities. It mirrors the built-in default so the tests
// exercise the gate exactly as a default project would.
func gateTestPolicy() config.Quality {
	requireTests := true
	return config.Quality{
		RequireTests: &requireTests,
		MaxFixCycles: config.DefaultMaxFixCycles,
		FailOn:       []string{"critical", "high"},
	}
}

// TestPostgresConfiguredButNotExecutedFailsClosed proves that merely configuring
// a PostgreSQL integration command is not enough: the classified check must have
// executed and passed in this run. A configured-but-not-executed (or skipped)
// check fails closed.
func TestPostgresConfiguredButNotExecutedFailsClosed(t *testing.T) {
	res := Evaluate(gateTestPolicy(), Input{
		BuildPassed:          true,
		TestPassed:           true,
		ValidationRequired:   true,
		ValidationConfigured: true,
		PostgresRequired:     true,
		PostgresConfigured:   true,
		PostgresPassed:       false,
	})
	if res.Decision != Fail {
		t.Fatalf("configured-but-not-executed PostgreSQL integration must fail closed, got %v", res.Decision)
	}
	if !containsSubstring(res.Reasons, "did not pass") {
		t.Fatalf("expected a did-not-pass reason, got %v", res.Reasons)
	}
}

// TestPostgresRequiredAndPassedAllowsPass proves the positive path: when the
// category is required, configured, and the classified check executed and
// passed, the gate proceeds to its ordinary decision instead of failing on the
// category.
func TestPostgresRequiredAndPassedAllowsPass(t *testing.T) {
	res := Evaluate(gateTestPolicy(), Input{
		BuildPassed:          true,
		TestPassed:           true,
		ValidationRequired:   true,
		ValidationConfigured: true,
		PostgresRequired:     true,
		PostgresConfigured:   true,
		PostgresPassed:       true,
	})
	if res.Decision != Pass {
		t.Fatalf("a required, configured, executed, passing PostgreSQL integration check must pass, got %v (%v)", res.Decision, res.Reasons)
	}
}

// TestPostgresGatePolicyReadsNoRetrievalMetric asserts that the PostgreSQL
// validation policy path is independent of retrieval quality: the required-
// category decision is a pure function of changed paths, so a retrieval-only
// change never acquires the category, while a database-sensitive change does.
func TestPostgresGatePolicyReadsNoRetrievalMetric(t *testing.T) {
	paths := []string{"internal/retrieval/retrieval.go"}
	if PostgresIntegrationRequired(paths) {
		t.Fatal("a retrieval-only change must not require the PostgreSQL integration category")
	}
	if !PostgresIntegrationRequired([]string{"internal/store/schema.sql"}) {
		t.Fatal("a database-sensitive change must require the category")
	}
}

// containsSubstring reports whether any reason contains want.
func containsSubstring(reasons []string, want string) bool {
	for _, r := range reasons {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}
