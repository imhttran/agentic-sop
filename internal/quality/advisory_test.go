package quality

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// TestRequiredCheckFailureNeverPasses proves a failed required check (build,
// required unit test, or required lint) can never yield a passing gate.
func TestRequiredCheckFailureNeverPasses(t *testing.T) {
	cases := []struct {
		name string
		p    config.Quality
		in   Input
	}{
		{"build", policy(true, 3), Input{BuildPassed: false, TestPassed: true}},
		{"required-test", policy(true, 3), Input{BuildPassed: true, TestPassed: false}},
		{"required-lint", policy(true, 3), Input{BuildPassed: true, TestPassed: true, LintRequired: true, LintPassed: false}},
	}
	for _, c := range cases {
		if got := Evaluate(c.p, c.in); got.Decision == Pass {
			t.Errorf("%s: Decision = Pass, want non-pass; reasons=%v", c.name, got.Reasons)
		}
	}
}

// TestOptionalTestFailureIsAdvisoryNotBlocking proves a failing unit test that
// policy does not require keeps a PASS verdict but is reported explicitly, so a
// passing gate never silently hides a red test.
func TestOptionalTestFailureIsAdvisoryNotBlocking(t *testing.T) {
	got := Evaluate(policy(false, 3), Input{BuildPassed: true, TestPassed: false})
	if got.Decision != Pass {
		t.Fatalf("Decision = %s, want Pass (tests not required); reasons=%v", got.Decision, got.Reasons)
	}
	if !strings.Contains(strings.Join(got.Advisories, " "), "unit tests failed") {
		t.Errorf("advisories = %v, want an explicit failing-unit-tests advisory", got.Advisories)
	}
}

// TestNoAdvisoryWhenOptionalTestPassed proves the advisory is specific to an
// actual optional failure and does not appear otherwise.
func TestNoAdvisoryWhenOptionalTestPassed(t *testing.T) {
	got := Evaluate(policy(false, 3), Input{BuildPassed: true, TestPassed: true})
	if got.Decision != Pass || len(got.Advisories) != 0 {
		t.Fatalf("Decision=%s advisories=%v, want Pass with no advisories", got.Decision, got.Advisories)
	}
}
