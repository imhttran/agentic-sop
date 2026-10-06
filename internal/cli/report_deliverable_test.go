package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// Exercise the real native writer AND the production CLI mutation observer:
// writing a task-declared report must survive the outer completion/evidence gate.
func TestRunExplicitReportDeliverable(t *testing.T) {
	dir := newDirtyRepo(t)
	const report = "docs/reports/baseline/current.md"
	writeFile(t, dir, "TASK.md", "# T001 — Capture baseline\n\nCreate the baseline report.\n\n## Deliverables\n\n- "+report+" — current observed evidence\n\n## Acceptance criteria\n\n- The baseline contains the caller's current observation.\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - test -s "+report+"\n")
	writeRepoFile(t, dir, "docs/reports/unrelated/user.md", "user-owned evidence\n")
	t.Setenv("SOP_TASK_INPUTS", `{"T001":"Caller observation: the sibling checkout is unchanged."}`)

	srv := scriptedOllama(t,
		`{"tool":"write_file","args":{"path":"`+report+`","content":"# Baseline\nCaller observation: the sibling checkout is unchanged.\n"}}`,
		`{"status":"completed","summary":"created the requested report","changes_expected":true}`,
	)
	defer srv.Close()
	a := &reportInputAgent{hybridAgent: hybridAgent{native: ollamaagent.NewNativeAgent(ollamaagent.Config{
		BaseURL: srv.URL, Model: ollamaagent.DefaultModel, Timeout: 5 * time.Second,
		MaxToolCalls: 10, CommandTimeout: 10 * time.Second, MaxOutputBytes: 1 << 20,
	}, dir, io.Discard)}}
	d := defaultDeps()
	d.getwd = func() (string, error) { return dir, nil }
	d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }
	var out, errOut bytes.Buffer
	if code := run([]string{"run", "--task", "TASK.md"}, &out, &errOut, d); code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errOut)
	}
	if !strings.Contains(a.input, "Caller observation:") || !strings.Contains(a.task, "## Deliverables") {
		t.Fatalf("caller-owned deliverable/input dropped: task=%s input=%s", a.task, a.input)
	}
	if !strings.Contains(a.reviewInput, "# Baseline") || strings.Contains(a.reviewInput, "user-owned evidence") {
		t.Fatalf("review must see the requested report, not unrelated reports: %s", a.reviewInput)
	}
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "changed-files.json"))
	var paths []string
	if err != nil || json.Unmarshal(data, &paths) != nil || !sameStrings(paths, []string{report}) {
		t.Fatalf("task changes=%s, %v; want only %s", data, err, report)
	}
	for path, want := range map[string]string{
		"tracked.txt": "user edit\n", "untracked.txt": "scratch\n",
		"docs/reports/unrelated/user.md": "user-owned evidence\n",
	} {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(got) != want {
			t.Fatalf("pre-existing %s changed: %q, %v", path, got, err)
		}
	}
}

func TestDeclaredReportRequiresInvocationMutation(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "inherited report is not progress", true: "updating report is progress"}[mutate], func(t *testing.T) {
			dir := newDirtyRepo(t)
			const report = "docs/reports/baseline/current.md"
			writeRepoFile(t, dir, report, "pre-existing user evidence\n")
			writeFile(t, dir, "TASK.md", "# T001 — Report\n\nUpdate evidence.\n\n## Deliverables\n\n- "+report+"\n")
			writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - true\n")
			t.Setenv("SOP_TASK_INPUTS", `{"T001":"These observations alone do not prove completion."}`)
			a := invocationAgent{outcomeAgent: outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}, mutate: func() {
				if mutate {
					writeRepoFile(t, dir, report, "pre-existing user evidence\nnew observed evidence\n")
				}
			}}
			d := defaultDeps()
			d.getwd = func() (string, error) { return dir, nil }
			d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }
			var out, errOut bytes.Buffer
			code := run([]string{"run", "--task", "TASK.md"}, &out, &errOut, d)
			if (code == exitOK) != mutate {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errOut)
			}
			if !mutate && !strings.Contains(out.String(), "NO_CHANGES_PRODUCED") {
				t.Fatalf("missing fail-closed mutation outcome: %s", &out)
			}
		})
	}
}

func TestPlanTaskDeliverablesUseReconciledTaskIdentity(t *testing.T) {
	dir := t.TempDir()
	plan := planner.Plan{Project: "x", Summary: "Baseline", Stages: []planner.Stage{{
		ID: "T001", Title: "Baseline", Objective: "Record facts.",
		AcceptanceCriteria: []string{"Current evidence."}, Deliverables: []string{"docs/reports/baseline/current.md"},
	}}}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, dir, stateDirName+"/plan.json", string(data))
	task := &domain.Task{ID: "T001", Objective: "Record facts.", AcceptanceCriteria: "Current evidence."}
	if got := planTaskDeliverables(dir, task); !sameStrings(got, plan.Stages[0].Deliverables) {
		t.Fatalf("deliverables=%v", got)
	}
	task.Objective = "Different executed contract."
	if got := planTaskDeliverables(dir, task); len(got) != 0 {
		t.Fatalf("mismatched plan granted report ownership: %v", got)
	}
}

func TestReportScopeAndAccumulatedJEVContext(t *testing.T) {
	dir := t.TempDir()
	const report = "docs/reports/baseline/current.md"
	spec := &taskfile.Spec{Deliverables: []string{
		"`" + report + "` — evidence", ".agent-sdlc/state.db", "docs/reports/plan.md",
		"docs/reports", "docs/reports/baseline/../other.md", "/tmp/external.md",
	}}
	paths := taskReportDeliverables(spec)
	// A declared Markdown report is task-owned whether in a subdirectory or at the
	// top level (docs/reports/plan.md); a non-declared top-level report and SOP state
	// stay SOP-owned.
	want := []string{report, "docs/reports/plan.md"}
	if !sameStrings(paths, want) {
		t.Fatalf("report scope=%v, want %v", paths, want)
	}
	if got := taskChangedFiles([]string{report, "docs/reports/generated.md", ".agent-sdlc/state.db"}, paths...); !sameStrings(got, paths) {
		t.Fatalf("SOP output/state entered task evidence: %v", got)
	}
	writeRepoFile(t, dir, report, "current observed evidence\n")
	inv := buildJEVInvocation(spec, "", testrunner.SuiteResult{}, review.Report{}, paths, dir)
	if !strings.Contains(inv.RepositoryContext, "current observed evidence") {
		t.Fatalf("accumulated report disappeared from JEV: %+v", inv)
	}
	external := filepath.Join(t.TempDir(), "private.md")
	if err := os.WriteFile(external, []byte("must not leak"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := "docs/reports/baseline/link.md"
	if err := os.Symlink(external, filepath.Join(dir, link)); err != nil {
		t.Fatal(err)
	}
	if got := taskFileContext(dir, []string{link}, link); got != "" {
		t.Fatalf("escaping report supplied JEV context: %s", got)
	}
}

func TestTaskInputsAreBoundedDataForMatchingTask(t *testing.T) {
	for _, raw := range []string{`{`, `[]`, `{"T001":true}`, `{"":"facts"}`, strings.Repeat(" ", 32<<10) + "{}"} {
		t.Setenv("SOP_TASK_INPUTS", raw)
		if _, err := loadTaskInputs(); err == nil {
			t.Fatalf("accepted invalid/oversized input")
		}
	}
	t.Setenv("SOP_TASK_INPUTS", `{"T001":"caller facts"}`)
	inputs, err := loadTaskInputs()
	if err != nil {
		t.Fatal(err)
	}
	d := deps{taskInputs: inputs}
	if got := d.taskInput("T002", "original"); got != "original" {
		t.Fatalf("input spilled into another task: %s", got)
	}
	if got := d.taskInput("T001", "original"); !strings.Contains(got, "caller facts") || !strings.Contains(got, "original") {
		t.Fatalf("matching task lost context: %s", got)
	}
}

type reportInputAgent struct {
	hybridAgent
	input, task, reviewInput string
}

func (a *reportInputAgent) Generate(ctx context.Context, req agent.Request) (agent.Response, error) {
	if req.Capability == agent.Implement {
		a.input, a.task = req.Input, req.Task
	}
	if req.Capability == agent.Review {
		a.reviewInput = req.Input
	}
	return a.hybridAgent.Generate(ctx, req)
}
