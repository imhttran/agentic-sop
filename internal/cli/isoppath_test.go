package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/planflow"
)

// TestIsSOPPathScopesReportsToTopLevel pins the boundary: SOP owns its runtime
// state and its TOP-LEVEL generated reports, but a directory tree under
// docs/reports/ is task content — so a task's writes into its own report subtree
// (and its fixtures) are its mutation evidence, not SOP-owned output.
func TestIsSOPPathScopesReportsToTopLevel(t *testing.T) {
	declared := planflow.ReportsDir + "/closed/x.md"
	cases := []struct {
		path string
		want bool
	}{
		{".agent-sdlc/state.db", true},
		{".agent-sdlc/runs/x", true},
		{planflow.ReportsDir, true},
		{planflow.ReportsDir + "/plan-x.md", true},
		{planflow.ReportsDir + "/closed/x.md", false},
		{planflow.ReportsDir + "/closed/workloads/A/go.mod", false},
		{"internal/cli/run.go", false},
	}
	for _, c := range cases {
		if got := isSOPPath(c.path, declared); got != c.want {
			t.Errorf("isSOPPath(%q) = %t, want %t", c.path, got, c.want)
		}
	}
}
