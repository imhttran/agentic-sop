package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/store"
)

const promptReportConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n"

// TestValidatePromptDeliverables pins the declared-deliverable boundary: only a
// clean, explicit repository FILE inside the project is accepted. Traversal,
// absolute/external paths, directory-wide declarations, non-clean paths, and SOP's
// own state tree are refused, so a declaration can never exempt a whole directory or
// SOP state.
func TestValidatePromptDeliverables(t *testing.T) {
	for _, tc := range []struct {
		path string
		ok   bool
	}{
		{"docs/reports/PLAN-prompt.md", true},
		{"docs/reports/baseline/current.md", true},
		{"internal/cli/prompt.go", true},
		{"", false},
		{"../secret.md", false},
		{"docs/reports/../other.md", false},
		{"/tmp/external.md", false},
		{"docs/reports/", false},
		{"docs/reports", false},
		{".agent-sdlc/state.db", false},
		{".agent-sdlc", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got, err := validatePromptDeliverables([]string{tc.path})
			if (err == nil) != tc.ok {
				t.Fatalf("validatePromptDeliverables(%q) err=%v, want ok=%v", tc.path, err, tc.ok)
			}
			if tc.ok && !sameStrings(got, []string{tc.path}) {
				t.Fatalf("accepted path projected as %v, want %q", got, tc.path)
			}
			if !tc.ok && len(got) != 0 {
				t.Fatalf("rejected declaration returned paths: %v", got)
			}
		})
	}
}

// TestParsePromptDeliverables proves the option is repeatable and both the
// space-separated and `=` forms accumulate, preserving declaration order.
func TestParsePromptDeliverables(t *testing.T) {
	var stderr bytes.Buffer
	opts, ok := parsePromptArgs([]string{
		"--capability", "implement",
		"--deliverable", "docs/reports/a.md",
		"--deliverable=docs/reports/b.md",
		"write the reports",
	}, &stderr)
	if !ok {
		t.Fatalf("parse failed: %s", &stderr)
	}
	if want := []string{"docs/reports/a.md", "docs/reports/b.md"}; !sameStrings(opts.deliverables, want) {
		t.Fatalf("deliverables = %v, want %v", opts.deliverables, want)
	}

	stderr.Reset()
	if _, ok := parsePromptArgs([]string{"--deliverable"}, &stderr); ok {
		t.Fatal("a --deliverable without a value must be a usage error")
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Fatalf("stderr missing usage: %q", &stderr)
	}
}

// TestPromptDeliverableRejectedByCLI proves the boundary is enforced at the CLI: an
// unsafe declaration fails as a usage error before any run artifact is written, and
// a declaration on a read-only capability is refused rather than silently ignored.
func TestPromptDeliverableRejectedByCLI(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: cleanReviewJSON}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"traversal", []string{"prompt", "--capability", "implement", "--deliverable", "../secret.md", "go"}, "repository-relative"},
		{"external", []string{"prompt", "--capability", "implement", "--deliverable", "/tmp/secret.md", "go"}, "repository-relative"},
		{"directory-wide", []string{"prompt", "--capability", "implement", "--deliverable", "docs/reports", "go"}, "directory-wide"},
		{"SOP state", []string{"prompt", "--capability", "implement", "--deliverable", ".agent-sdlc/state.db", "go"}, "SOP state"},
		{"read-only capability", []string{"prompt", "--capability", "review", "--deliverable", "docs/reports/x.md", "go"}, "only valid with --capability implement"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCLIWithAgent(t, dir, a, tc.args...)
			if code != exitUsage {
				t.Fatalf("code=%d, want %d; stderr=%s", code, exitUsage, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Fatalf("stderr = %q, want %q", stderr, tc.want)
			}
		})
	}
	if dirs := promptRunDirs(t, dir); len(dirs) != 0 {
		t.Fatalf("a rejected declaration must not create a run: %v", dirs)
	}
}

// TestPromptDeclaredReportMutationEvidence is the SOP-PROMPT-REPORT-001 regression:
// a declared report counts as progress only when its content actually changes.
// An unchanged report, a merely announced output, an undeclared (SOP-owned) report,
// and SOP state never satisfy the no-changes check, so NO_CHANGES_PRODUCED is
// preserved for all of them.
func TestPromptDeclaredReportMutationEvidence(t *testing.T) {
	const report = "docs/reports/PROMPT-baseline.md"
	const otherReport = "docs/reports/PROMPT-generated-other.md"
	for _, tc := range []struct {
		name     string
		declare  bool
		mutate   string
		wantPass bool
	}{
		{"declared report changed is progress", true, "change", true},
		{"declared report unchanged is not progress", true, "same", false},
		{"declared report merely announced is not progress", true, "", false},
		{"undeclared top-level report is SOP-owned", false, "change", false},
		{"declared report unchanged, only a generated report changes", true, "other", false},
		{"declared report unchanged, only SOP state changes", true, "state", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearProviderEnv(t)
			dir := newDirtyRepo(t)
			writeRepoFile(t, dir, report, "pre-existing user evidence\n")
			writeConfig(t, dir, promptReportConfig)
			a := invocationAgent{outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
				Status: agent.OutcomeCompleted, ChangesExpected: true,
			}}, mutate: func() {
				switch tc.mutate {
				case "change":
					writeRepoFile(t, dir, report, "pre-existing user evidence\nnew observed evidence\n")
				case "same":
					writeRepoFile(t, dir, report, "pre-existing user evidence\n")
				case "other":
					writeRepoFile(t, dir, otherReport, "SOP-generated report\n")
				case "state":
					writeFile(t, dir, filepath.Join(".agent-sdlc", "diagnostic.txt"), "runtime evidence\n")
				}
			}}
			d := defaultDeps()
			d.getwd = func() (string, error) { return dir, nil }
			d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }

			args := []string{"prompt", "--capability", "implement"}
			if tc.declare {
				args = append(args, "--deliverable", report)
			}
			args = append(args, "Capture the baseline report")
			var out, errOut bytes.Buffer
			code := run(args, &out, &errOut, d)

			if (code == exitOK) != tc.wantPass {
				t.Fatalf("code=%d, want ok=%v; stdout=%s stderr=%s", code, tc.wantPass, &out, &errOut)
			}
			if !tc.wantPass && !strings.Contains(out.String(), "NO_CHANGES_PRODUCED") {
				t.Fatalf("missing fail-closed no-change outcome: %s", &out)
			}
			// Pre-existing unrelated dirty work is never attributed.
			if got, err := os.ReadFile(filepath.Join(dir, "tracked.txt")); err != nil || string(got) != "user edit\n" {
				t.Fatalf("unrelated user work changed: %q, %v", got, err)
			}
			dirs := promptRunDirs(t, dir)
			if len(dirs) != 1 {
				t.Fatalf("want one prompt run dir, got %v", dirs)
			}
			data, err := os.ReadFile(filepath.Join(dirs[0], "changed-files.json"))
			if tc.wantPass && tc.declare && tc.mutate == "change" {
				var paths []string
				if err != nil || json.Unmarshal(data, &paths) != nil || !sameStrings(paths, []string{report}) {
					t.Fatalf("task attribution = %s, %v; want only %s", data, err, report)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("no declared change, but task evidence was recorded: %s, %v", data, err)
			}
		})
	}
}

// TestPromptDeclaredReportProjectsIntoTaskSpec proves the declaration is projected
// into the lifecycle's task spec (not just parsed): the rendered task carries the
// declared deliverable, using the existing lifecycle mechanism.
func TestPromptDeclaredReportProjectsIntoTaskSpec(t *testing.T) {
	clearProviderEnv(t)
	const report = "docs/reports/PROMPT-projection.md"
	dir := newDirtyRepo(t)
	writeConfig(t, dir, promptReportConfig)
	a := invocationAgent{outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeCompleted, ChangesExpected: true,
	}}, mutate: func() { writeRepoFile(t, dir, report, "# baseline\n") }}
	d := defaultDeps()
	d.getwd = func() (string, error) { return dir, nil }
	d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }

	var out, errOut bytes.Buffer
	if code := run([]string{"prompt", "--capability", "implement", "--deliverable", report, "Write the report"}, &out, &errOut, d); code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errOut)
	}
	dirs := promptRunDirs(t, dir)
	if len(dirs) != 1 {
		t.Fatalf("want one prompt run dir, got %v", dirs)
	}
	data, err := os.ReadFile(filepath.Join(dirs[0], "task.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Deliverables") || !strings.Contains(string(data), report) {
		t.Fatalf("task spec does not carry the declared deliverable:\n%s", data)
	}
}

// TestScheduledTaskDeclaredReportCompatibility proves the existing scheduled-task
// report-deliverable path is unchanged by the prompt option: a reconciled plan's
// declared report still counts only a content change, and an unchanged report still
// fails closed.
func TestScheduledTaskDeclaredReportCompatibility(t *testing.T) {
	const report = "docs/reports/baseline/current.md"
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged report fails", true: "changed report passes"}[mutate], func(t *testing.T) {
			clearProviderEnv(t)
			dir := newDirtyRepo(t)
			initProject(t, dir)
			writeConfig(t, dir, promptReportConfig)
			writeRepoFile(t, dir, report, "pre-existing user evidence\n")

			plan := planner.Plan{Project: "x", Summary: "Baseline", Stages: []planner.Stage{{
				ID: "T001", Title: "Baseline", Objective: "Record the current state.",
				AcceptanceCriteria: []string{"Current observed evidence."},
				Deliverables:       []string{report},
			}}}
			data, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, dir, stateDirName+"/plan.json", string(data))

			task := &domain.Task{ID: "T001", Title: "Baseline", Objective: "Record the current state.",
				AcceptanceCriteria: "Current observed evidence.", Status: domain.READY, MaxAttempts: 3}
			st, err := store.Open(statePath(dir))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if err := st.Save(task); err != nil {
				t.Fatal(err)
			}

			a := invocationAgent{outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
				Status: agent.OutcomeCompleted, ChangesExpected: true,
			}}, mutate: func() {
				if mutate {
					writeRepoFile(t, dir, report, "pre-existing user evidence\nnew observed evidence\n")
				} else {
					writeRepoFile(t, dir, report, "pre-existing user evidence\n")
				}
			}}
			d := defaultDeps()
			d.getwd = func() (string, error) { return dir, nil }
			d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }
			cfg, err := loadConfigOrDefault(dir)
			if err != nil {
				t.Fatal(err)
			}

			var out, errOut bytes.Buffer
			code := runScheduledTask(context.Background(), dir, cfg, a, d, st, task, newRunSession(), &out, &errOut)
			if (code == exitOK) != mutate {
				t.Fatalf("code=%d, want ok=%v; stdout=%s stderr=%s", code, mutate, &out, &errOut)
			}
			if !mutate && !strings.Contains(out.String(), "NO_CHANGES_PRODUCED") {
				t.Fatalf("scheduled unchanged report must fail closed: %s", &out)
			}
		})
	}
}
