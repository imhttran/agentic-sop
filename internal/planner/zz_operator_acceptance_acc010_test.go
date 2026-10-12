//go:build acceptance

// Operator-authored acceptance check for the narrowed ACC-010: multi-file Files
// values become bare individual paths (backticks and trailing sentence punctuation
// stripped), and every existing execution-mode shape keeps working. These cases are
// operator-owned copies of the behaviour, so editing an ordinary test cannot satisfy
// them.

package planner

import (
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func accOp010TaskPlan(files string) string {
	return "# P\n\n## Project\np\n\n## Summary\ns\n\n## Tasks\n\n" +
		"### T-001 — Stage\n" +
		"- **Objective:** Do it.\n" +
		"- **Files:** " + files + "\n" +
		"- **Depends on:** —\n" +
		"- **Acceptance:** it works.\n"
}

func TestOperatorAcceptanceACC010FilesBecomeBarePaths(t *testing.T) {
	cases := map[string][]string{
		"`cmd/sop/main.go`, `internal/app/app.go`.": {"cmd/sop/main.go", "internal/app/app.go"},
		"`a/x.go`, `a/y.go`":                        {"a/x.go", "a/y.go"},
		"`b/z.go`; `b/w.go`;":                       {"b/z.go", "b/w.go"},
		"`c/one.go`.":                               {"c/one.go"},
		"c/plain.go, c/other.go.":                   {"c/plain.go", "c/other.go"},
	}
	for files, want := range cases {
		plan, err := PlanFromMarkdown(accOp010TaskPlan(files))
		if err != nil || plan == nil || len(plan.Stages) != 1 {
			t.Fatalf("Files %q: plan=%+v err=%v", files, plan, err)
		}
		if got := plan.Stages[0].Deliverables; !reflect.DeepEqual(got, want) {
			t.Errorf("Files %q: Deliverables = %q, want %q", files, got, want)
		}
	}
}

func TestOperatorAcceptanceACC010ExecutionModesPreserved(t *testing.T) {
	rendered := func(body string) string {
		return "# P\n\n## Project\np\n\n## Summary\ns\n\n## S001 — Stage\n\nDo it.\n\n" +
			"### Dependencies\n\n- none\n\n### Acceptance Criteria\n\n- works\n\n### Execution\n\n" + body + "\n"
	}
	inline := func(mode string) string {
		return "# P\n\n## Project\np\n\n## Summary\ns\n\n## Tasks\n\n### T-001 — Stage\n" +
			"- **Objective:** Do it.\n- **Depends on:** —\n- **Acceptance:** it works.\n" +
			"- **Execution:** " + mode + "\n"
	}
	cases := []struct {
		name, doc string
		want      domain.ExecutionMode
	}{
		{"rendered verify-first", rendered("- verify-first"), "verify-first"},
		{"rendered done", rendered("- done"), "done"},
		{"rendered prose before mode", rendered("This stage runs as\n- verify-first"), "verify-first"},
		{"rendered indented note after mode", rendered("- done\n  (already merged)"), "done"},
		{"inline verify-first", inline("verify-first"), "verify-first"},
		{"inline done", inline("done"), "done"},
	}
	for _, tc := range cases {
		plan, err := PlanFromMarkdown(tc.doc)
		if err != nil || plan == nil || len(plan.Stages) != 1 {
			t.Fatalf("%s: plan=%+v err=%v", tc.name, plan, err)
		}
		if got := plan.Stages[0].ExecutionMode; got != tc.want {
			t.Errorf("%s: ExecutionMode = %q, want %q", tc.name, got, tc.want)
		}
	}
}
