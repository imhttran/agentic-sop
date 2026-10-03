package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// reportSpec is a minimal rendered task spec for buildRunReport assertions.
func reportSpec(t *testing.T) *taskfile.Spec {
	t.Helper()
	spec, err := taskfile.Parse([]byte("# T001 --- Add widget\n\n## Objective\n\nAdd the widget.\n"))
	if err != nil {
		t.Fatalf("parse task spec: %v", err)
	}
	return spec
}

// TestBuildRunReportEmptySuiteConfiguredNotRun is the regression for the
// misleading branch: when checks ARE configured but the lifecycle stopped
// before validation ran (no results), the report must say NOT RUN rather than
// claiming no validation commands are configured.
func TestBuildRunReportEmptySuiteConfiguredNotRun(t *testing.T) {
	cfg := config.Default()
	cfg.Validation = config.Validation{Build: []string{"go build ./..."}, Test: []string{"go test ./..."}}

	res := lifeResult{stage: "FAILED", gate: quality.Result{Decision: quality.Fail, Reasons: []string{"implement failed"}}}
	out := buildRunReport(reportSpec(t), cfg, res)

	if strings.Contains(out, "No validation commands configured.") {
		t.Errorf("report falsely claims no commands configured:\n%s", out)
	}
	for _, want := range []string{"NOT RUN", "go build ./...", "go test ./..."} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

// TestBuildRunReportEmptySuiteUnconfigured keeps the original message when there
// genuinely are no configured checks.
func TestBuildRunReportEmptySuiteUnconfigured(t *testing.T) {
	cfg := config.Default()
	cfg.Validation = config.Validation{}

	res := lifeResult{stage: "FAILED", gate: quality.Result{Decision: quality.Fail, Reasons: []string{"implement failed"}}}
	out := buildRunReport(reportSpec(t), cfg, res)

	if !strings.Contains(out, "No validation commands configured.") {
		t.Errorf("report missing the no-configured-checks message:\n%s", out)
	}
	if strings.Contains(out, "NOT RUN") {
		t.Errorf("unconfigured project must not claim NOT RUN:\n%s", out)
	}
}

// TestBuildRunReportExecutedValidationUnchanged proves the executed-results path
// still renders each result exactly as before.
func TestBuildRunReportExecutedValidationUnchanged(t *testing.T) {
	cfg := config.Default()
	cfg.Validation = config.Validation{Build: []string{"go build ./..."}}

	res := lifeResult{
		stage: "PASSED",
		gate:  quality.Result{Decision: quality.Pass},
		suite: testrunner.SuiteResult{Results: []testrunner.Result{
			{Category: testrunner.Build, Command: "go build ./...", Status: testrunner.Pass},
		}},
	}
	out := buildRunReport(reportSpec(t), cfg, res)

	if !strings.Contains(out, "- PASS BUILD `go build ./...`") {
		t.Errorf("executed result line changed:\n%s", out)
	}
	if strings.Contains(out, "NOT RUN") || strings.Contains(out, "No validation commands configured.") {
		t.Errorf("executed results must not be reported as not-run:\n%s", out)
	}
}
