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
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/store"
)

// This file proves the ACC-003 acceptance contract end-to-end through the
// scheduled-task path (sop run on a persisted task): a plan-declared acceptance
// check is evaluated by the EXISTING deriveOutcome / authorizeAcceptanceCommand /
// runAcceptanceArgv seam, and its verdict is recorded in outcome.json and
// report.md. A passing SAFE check yields VERIFIED with the check's criterion
// listed; a failing SAFE check yields PARTIAL with the failure evidence; an
// unauthorized (non-SAFE, shell-syntax) check yields HOLD and is never executed.
//
// A stage always declares free-text acceptance criteria (Plan.Validate requires at
// least one), and the persisted task is seeded with the newline-joined criteria so
// the plan stage reconciles with it by identity. When the stage declares a
// deterministic check, the free-text criteria are recorded as informational while
// the check is the verifiable evidence, so a passing check plus free-text criteria
// still records VERIFIED.

// outcomeReport reads the run's report.md.
func outcomeReport(t *testing.T, dir, id string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", id, "report.md"))
	if err != nil {
		t.Fatalf("read report.md: %v", err)
	}
	return string(b)
}

// outcomeArtifactStatus reads the run's outcome.json status.
func outcomeArtifactStatus(t *testing.T, dir, id string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", id, "outcome.json"))
	if err != nil {
		t.Fatalf("read outcome.json: %v", err)
	}
	var doc struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal outcome.json: %v", err)
	}
	return doc.Status
}

// acceptanceCriteria is the free-text acceptance criterion declared by every stage
// the fixture builds. Plan.Validate requires at least one, and planTaskStage
// reconciles the stage with the persisted task by the newline-joined criteria, so
// the same slice seeds both.
var acceptanceCriteria = []string{"the objective is demonstrably met"}

// seededTaskCriteria is the task.AcceptanceCriteria (newline-joined) that matches
// acceptanceCriteria, so the plan stage reconciles with the seeded task.
func seededTaskCriteria() string {
	return strings.Join(acceptanceCriteria, "\n")
}

// planWithChecks builds a single-stage plan carrying the given acceptance checks
// (plus the free-text acceptance criteria Plan.Validate requires), matching by
// identity (ID + Objective + AcceptanceCriteria) the task that reconcilePlanTask
// seeds. Checks are separated by \x00 so a check containing spaces is preserved as
// one item.
func planWithChecks(t *testing.T, dir, checks string) {
	t.Helper()
	stage := planner.Stage{
		ID:                 "T001",
		Title:              "Acceptance",
		Objective:          "Do it.",
		AcceptanceCriteria: acceptanceCriteria,
	}
	if checks != "" {
		for _, c := range strings.Split(checks, "\x00") {
			stage.AcceptanceChecks = append(stage.AcceptanceChecks, c)
		}
	}
	plan := planner.Plan{Project: "x", Summary: "S", Stages: []planner.Stage{stage}}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	writePlanFile(t, dir, string(data))
}

// writePlanFile writes .agent-sdlc/plan.json.
func writePlanFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(stateDirName, "plan.json"), content)
}

// reconcilePlanTask seeds the persisted task whose identity the plan stage must
// match for acceptance checks to be recovered.
func reconcilePlanTask(t *testing.T, dir string) *domain.Task {
	t.Helper()
	task := &domain.Task{
		ID:                 "T001",
		Title:              "Acceptance",
		Objective:          "Do it.",
		AcceptanceCriteria: seededTaskCriteria(),
		Status:             domain.READY,
		MaxAttempts:        3,
	}
	seedTask(t, dir, task)
	return task
}

// scheduledAcceptanceAgent is a capability agent whose IMPLEMENT actually writes
// a repository file, so the lifecycle observes a governed change and reaches the
// outcome step where deriveOutcome evaluates the plan-declared checks. Without an
// observed change SOP fails closed with NO_CHANGES_PRODUCED before any outcome is
// recorded, which would make this contract untestable.
type scheduledAcceptanceAgent struct {
	dir string
}

func (a *scheduledAcceptanceAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement, agent.Fix:
		if err := os.WriteFile(filepath.Join(a.dir, "impl.txt"), []byte("done\n"), 0o644); err != nil {
			return agent.Response{}, err
		}
		return agent.Response{Content: "implemented", Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "implemented", ChangesExpected: true}}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// scheduledAcceptanceDeps returns the boundaries the scheduled-task lifecycle
// needs, with the injected agent and the real working-tree observers so the
// agent's written file is observed as a change.
func scheduledAcceptanceDeps(dir string, a agent.Agent) deps {
	d := defaultDeps()
	d.getwd = func() (string, error) { return dir, nil }
	d.newAgent = func(string, string, string) (agent.Agent, error) { return a, nil }
	d.commit = func(context.Context, string, string) error { return nil }
	d.newGitHub = func(string) github.Client { return &fakeGitHub{} }
	return d
}

// runScheduledAcceptance drives the scheduled-task lifecycle once for a persisted
// task, returning the exit code and the combined output.
func runScheduledAcceptance(t *testing.T, dir string, task *domain.Task) (int, string) {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &scheduledAcceptanceAgent{dir: dir}
	d := scheduledAcceptanceDeps(dir, a)

	var out, errOut bytes.Buffer
	code := runScheduledTask(context.Background(), dir, cfg, a, d, st, task, newRunSession(), &out, &errOut)
	return code, out.String() + errOut.String()
}

// scheduledAcceptanceFixture seeds a git repository containing a minimal buildable
// Go module (so `go vet ./...` / `go test ./...` checks run deterministically
// against a real module) plus the project state and validation config.
func scheduledAcceptanceFixture(t *testing.T) string {
	t.Helper()
	dir := newDirtyRepo(t)
	initProject(t, dir)
	goModuleFixture(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	return dir
}

// TestScheduledAcceptancePassingCheckVerified proves a plan-declared passing SAFE
// check (go vet ./...) records outcome VERIFIED in outcome.json and report.md, the
// report lists the check's criterion as met, and the free-text criterion is listed
// as informational.
func TestScheduledAcceptancePassingCheckVerified(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	planWithChecks(t, dir, "go vet ./...")
	task := reconcilePlanTask(t, dir)

	code, out := runScheduledAcceptance(t, dir, task)
	if code != exitOK {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "outcome: VERIFIED") {
		t.Errorf("stdout missing VERIFIED outcome: %q", out)
	}
	if got := outcomeArtifactStatus(t, dir, "T001"); got != "VERIFIED" {
		t.Errorf("outcome.json status = %q, want VERIFIED", got)
	}
	report := outcomeReport(t, dir, "T001")
	if !strings.Contains(report, "go vet ./...") {
		t.Errorf("report.md does not list the check's criterion:\n%s", report)
	}
	if !strings.Contains(report, "informational") {
		t.Errorf("report.md does not list the free-text criterion as informational:\n%s", report)
	}
}

// TestScheduledAcceptancePassingGoTestCheckVerified proves a `go test` check that
// runs a passing test contributes to VERIFIED.
func TestScheduledAcceptancePassingGoTestCheckVerified(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	writeFile(t, dir, "t_test.go", "package t\n\nimport \"testing\"\n\nfunc TestT(t *testing.T) {}\n")
	planWithChecks(t, dir, "go test ./...")
	task := reconcilePlanTask(t, dir)

	_, out := runScheduledAcceptance(t, dir, task)
	if !strings.Contains(out, "outcome: VERIFIED") {
		t.Errorf("stdout missing VERIFIED outcome: %q", out)
	}
	if got := outcomeArtifactStatus(t, dir, "T001"); got != "VERIFIED" {
		t.Errorf("outcome.json status = %q, want VERIFIED", got)
	}
}

// TestScheduledAcceptanceNoTestExecutedPartial proves a `go test` check that runs
// no test (`-run NoSuchTest`) yields PARTIAL with `no test executed` evidence.
func TestScheduledAcceptanceNoTestExecutedPartial(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	writeFile(t, dir, "t_test.go", "package t\n\nimport \"testing\"\n\nfunc TestT(t *testing.T) {}\n")
	planWithChecks(t, dir, "go test -run NoSuchTest ./...")
	task := reconcilePlanTask(t, dir)

	_, out := runScheduledAcceptance(t, dir, task)
	if !strings.Contains(out, "outcome: PARTIAL") {
		t.Errorf("stdout missing PARTIAL outcome: %q", out)
	}
	if got := outcomeArtifactStatus(t, dir, "T001"); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
	report := outcomeReport(t, dir, "T001")
	if !strings.Contains(report, "no test executed") {
		t.Errorf("report.md missing 'no test executed' evidence:\n%s", report)
	}
}

// TestScheduledAcceptanceNoTestFilesPartial proves a `go test` check against a
// package with no test files yields PARTIAL with `no test executed` evidence. The
// fixture's Go module carries no test file here (the fixture's shared test file is
// removed) so `go test ./...` reports "no test files" and runs zero tests.
func TestScheduledAcceptanceNoTestFilesPartial(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	// The shared fixture declares a passing test so `go vet`/`go test` checks can
	// execute one; remove it so this package has no test files at all.
	if err := os.Remove(filepath.Join(dir, "t_test.go")); err != nil {
		t.Fatalf("remove test file: %v", err)
	}
	planWithChecks(t, dir, "go test ./...")
	task := reconcilePlanTask(t, dir)

	_, _ = runScheduledAcceptance(t, dir, task)
	if got := outcomeArtifactStatus(t, dir, "T001"); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
	report := outcomeReport(t, dir, "T001")
	if !strings.Contains(report, "no test executed") {
		t.Errorf("report.md missing 'no test executed' evidence:\n%s", report)
	}
}

// TestScheduledAcceptanceSkippedTestPartial proves a `go test` check whose every
// test is skipped yields PARTIAL with `no test executed` evidence.
func TestScheduledAcceptanceSkippedTestPartial(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	writeFile(t, dir, "t_test.go", "package t\n\nimport \"testing\"\n\nfunc TestT(t *testing.T) { t.Skip(\"skip\") }\n")
	planWithChecks(t, dir, "go test -v ./...")
	task := reconcilePlanTask(t, dir)

	_, _ = runScheduledAcceptance(t, dir, task)
	if got := outcomeArtifactStatus(t, dir, "T001"); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
	report := outcomeReport(t, dir, "T001")
	if !strings.Contains(report, "no test executed") {
		t.Errorf("report.md missing 'no test executed' evidence:\n%s", report)
	}
}

// TestScheduledAcceptanceFailingCheckPartial proves a failing SAFE check yields
// PARTIAL with the failure evidence recorded. The lifecycle still completes for a
// PARTIAL verdict (a human gate before commit), so the verdict is asserted from the
// recorded outcome rather than the exit code.
func TestScheduledAcceptanceFailingCheckPartial(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	// go build ./... on a module with a deliberate syntax error fails.
	writeFile(t, dir, "broken.go", "package t\n\nfunc broken( {\n")
	planWithChecks(t, dir, "go build ./...")
	task := reconcilePlanTask(t, dir)

	_, out := runScheduledAcceptance(t, dir, task)
	if !strings.Contains(out, "outcome: PARTIAL") {
		t.Errorf("stdout missing PARTIAL outcome: %q", out)
	}
	if got := outcomeArtifactStatus(t, dir, "T001"); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
	report := outcomeReport(t, dir, "T001")
	if !strings.Contains(report, "go build ./...") || !strings.Contains(report, "not met") {
		t.Errorf("report.md missing failure evidence:\n%s", report)
	}
}

// TestScheduledAcceptanceUnauthorizedCheckHolds proves each unauthorized (non-SAFE)
// check yields HOLD and is never executed: the marker file it would have created
// does not exist after the run.
func TestScheduledAcceptanceUnauthorizedCheckHolds(t *testing.T) {
	for _, check := range []string{
		"go vet ./... && touch pwned",
		"sh -c \"touch pwned\"",
		"go run .",
		"git push",
	} {
		t.Run(check, func(t *testing.T) {
			dir := scheduledAcceptanceFixture(t)
			planWithChecks(t, dir, check)
			task := reconcilePlanTask(t, dir)

			_, _ = runScheduledAcceptance(t, dir, task)

			if got := outcomeArtifactStatus(t, dir, "T001"); got != "HOLD" {
				t.Errorf("outcome.json status = %q, want HOLD", got)
			}
			if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
				t.Fatalf("marker file pwned exists: unauthorized check %q was executed", check)
			}
		})
	}
}

// TestScheduledAcceptanceStageMismatchNil proves a plan whose stage no longer
// matches the persisted task yields no checks (nil).
func TestScheduledAcceptanceStageMismatchNil(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	// Plan stage's objective differs from the persisted task's, so no stage matches.
	plan := planner.Plan{Project: "x", Summary: "S", Stages: []planner.Stage{{
		ID: "T001", Title: "Acceptance", Objective: "DIFFERENT",
		AcceptanceCriteria: acceptanceCriteria,
		AcceptanceChecks:   []string{"go vet ./..."},
	}}}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	writePlanFile(t, dir, string(data))
	task := reconcilePlanTask(t, dir)

	if got := planTaskAcceptanceChecks(dir, task); got != nil {
		t.Errorf("planTaskAcceptanceChecks on mismatch = %v, want nil", got)
	}
}

// TestScheduledAcceptanceNoDeclaredChecksUnchanged proves a task with no declared
// checks behaves as today: no checks are recovered and the outcome is unchanged.
func TestScheduledAcceptanceNoDeclaredChecksUnchanged(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	planWithChecks(t, dir, "")
	task := reconcilePlanTask(t, dir)

	if got := planTaskAcceptanceChecks(dir, task); got != nil {
		t.Errorf("planTaskAcceptanceChecks with no declared checks = %v, want nil", got)
	}
	code, out := runScheduledAcceptance(t, dir, task)
	if code != exitOK {
		t.Fatalf("code=%d out=%s, want exitOK for a task with no declared checks", code, out)
	}
}
