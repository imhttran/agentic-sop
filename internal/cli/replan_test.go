package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// These tests pin the Phase 7 bounded strategy-replanning integration: when
// replanning is enabled and an attempt fails a quality gate with a recoverable
// implementation failure, SOP changes strategy ONCE on the SAME class, hands the
// failed attempt's evidence forward, persists one attempt record per try, and then
// keeps the existing terminal behavior. Replanning is OFF by default, is bounded by
// MaxReplans, never changes the model, and never fires on a human boundary, a
// BLOCK, or a NO_PROGRESS.

// replanScriptAgent fails the code review on every attempt until a replan context
// has been seen, after which it passes (when passOnReplan). The harness prepends the
// "# Replan" marker to the replanned attempt's IMPLEMENT input, so the switch is
// deterministic across lifecycles and uses no prose heuristics.
type replanScriptAgent struct {
	passOnReplan bool
	replanned    bool
}

func (a *replanScriptAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		if strings.Contains(r.Input, "# Replan") {
			a.replanned = true
		}
		return agent.Response{Content: "changed files"}, nil
	case agent.Fix:
		return agent.Response{Content: "fixed"}, nil
	case agent.Review:
		if a.passOnReplan && a.replanned {
			return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
		}
		return agent.Response{Content: `{"summary":"bug","findings":[{"severity":"HIGH","title":"nil deref","file":"a.go","line":3}]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// runReplanCLI runs `sop run --task TASK.md` with an injected agent and JEV
// analyzer, and records every model the factory was asked to build. A replan never
// rebuilds the agent, so the model list stays stable across a replan.
func runReplanCLI(t *testing.T, dir, diff string, a agent.Agent, jevFake jev.Analyzer, args ...string) escalationRun {
	t.Helper()
	var out, errOut bytes.Buffer
	var models []string
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(_, _, mdl string) (agent.Agent, error) {
			models = append(models, mdl)
			return a, nil
		},
		readDiff:       func(context.Context, string) (string, error) { return diff, nil },
		commit:         func(context.Context, string, string) error { return nil },
		newGitHub:      func(string) github.Client { return &fakeGitHub{} },
		newJEVAnalyzer: fakeJEV(jevFake),
	}
	code := run(args, &out, &errOut, d)
	return escalationRun{code: code, stdout: out.String(), stderr: errOut.String(), models: models}
}

// TestReplanDisabledIsUnchanged proves that with replanning off (the default), a
// failing task writes no attempt records and prints no recovery block: existing
// behavior is unchanged.
func TestReplanDisabledIsUnchanged(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runReplanCLI(t, dir, evalDiff, &replanScriptAgent{}, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, "Recovery:") || strings.Contains(res.stdout, "replanning") {
		t.Errorf("replanning must be silent when disabled: %q", res.stdout)
	}
	if got := attemptsOf(t, dir, "T001"); len(got) != 0 {
		t.Errorf("attempt records written while replanning is disabled: %+v", got)
	}
}

// TestReplanSuccessAfterOneReplan is Case 1: the first attempt fails a recoverable
// implementation failure, SOP replans ONCE on the same class, the second attempt
// passes, and both attempts are recorded.
func TestReplanSuccessAfterOneReplan(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runReplanCLI(t, dir, evalDiff, &replanScriptAgent{passOnReplan: true}, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitOK {
		t.Fatalf("code=%d, want ok; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "Recovery: replanning (attempt 2) on the same class SMALL") {
		t.Errorf("stdout missing the replan announcement: %q", res.stdout)
	}

	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 2 {
		t.Fatalf("attempts = %+v, want 2", attempts)
	}
	if attempts[0].Attempt != 1 || attempts[0].Result != runpkg.AttemptFailed || attempts[0].Action != "replan" {
		t.Errorf("attempt 1 = %+v, want failed/replan", attempts[0])
	}
	if attempts[0].Class != "small" || attempts[0].Model != "qwen3:4b" {
		t.Errorf("attempt 1 class/model = %s/%s, want small/qwen3:4b", attempts[0].Class, attempts[0].Model)
	}
	// A replan keeps the class: the second attempt runs on the SAME model.
	if attempts[1].Attempt != 2 || attempts[1].Result != runpkg.AttemptPassed || attempts[1].Action != "" {
		t.Errorf("attempt 2 = %+v, want passed/empty", attempts[1])
	}
	if attempts[1].Class != "small" || attempts[1].Model != "qwen3:4b" {
		t.Errorf("attempt 2 class/model = %s/%s, want the unchanged small/qwen3:4b", attempts[1].Class, attempts[1].Model)
	}

	tr := loadEvalTrace(t, dir, "T001")
	if len(tr.Replans) != 1 || tr.Replans[0].FromAttempt != 1 || tr.Replans[0].ToAttempt != 2 {
		t.Errorf("trace replans = %+v, want one 1->2", tr.Replans)
	}
	runEvalFixture(t, "replan/success-after-replan.expect.json", tr)

	// A same-class replan is not an escalation: the report must show the replan
	// action and must NOT claim a class escalation.
	_, out, _ := runCLI(t, dir, "report", "T001")
	if !strings.Contains(out, "recovery: replan") {
		t.Errorf("report missing the replan recovery action: %q", out)
	}
	if strings.Contains(out, "success after escalation") {
		t.Errorf("a same-class replan must not be reported as an escalation: %q", out)
	}
}

// TestReplanFailsThenStops is Case 2: a permitted replan also fails, so no second
// replan occurs and the existing terminal behavior applies.
func TestReplanFailsThenStops(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runReplanCLI(t, dir, evalDiff, &replanScriptAgent{passOnReplan: false}, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if got := strings.Count(res.stdout, "replanning (attempt"); got != 1 {
		t.Errorf("replan announcements = %d, want 1: %q", got, res.stdout)
	}
	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2 (replan bound spent, no third attempt)", len(attempts))
	}
	if attempts[0].Action != "replan" {
		t.Errorf("attempt 1 action = %q, want replan", attempts[0].Action)
	}
	if attempts[1].Action != "none" {
		t.Errorf("attempt 2 action = %q, want none (no second replan)", attempts[1].Action)
	}
	tr := loadEvalTrace(t, dir, "T001")
	if len(tr.Replans) != 1 {
		t.Errorf("trace replans = %d, want 1", len(tr.Replans))
	}
	runEvalFixture(t, "replan/failed-replan.expect.json", tr)
}

// TestReplanZeroBoundDisables proves max_replans=0 disables replanning even when the
// feature flag is on.
func TestReplanZeroBoundDisables(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	t.Setenv(model.EnvMaxReplans, "0")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runReplanCLI(t, dir, evalDiff, &replanScriptAgent{}, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if strings.Contains(res.stdout, "replanning") {
		t.Errorf("replanning must not run with a zero bound: %q", res.stdout)
	}
	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1 (a zero bound disables the replan, not the attempt loop)", len(attempts))
	}
	if attempts[0].Action != "none" {
		t.Errorf("attempt 1 action = %q, want none", attempts[0].Action)
	}
}

// TestReplanBoundCapsTheLoop proves the configured MaxReplans bounds replanning, and
// the attempt-loop backstop (maxEscalationAttempts) bounds total attempts, so a
// replan can never multiply the execution envelope.
func TestReplanBoundCapsTheLoop(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	t.Setenv(model.EnvMaxReplans, "3")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runReplanCLI(t, dir, evalDiff, &replanScriptAgent{passOnReplan: false}, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) > maxEscalationAttempts {
		t.Fatalf("attempts = %d, must never exceed the loop backstop %d", len(attempts), maxEscalationAttempts)
	}
	replans := 0
	for _, a := range attempts {
		if a.Action == "replan" {
			replans++
		}
	}
	if replans > 3 {
		t.Fatalf("replans = %d, must never exceed the configured MaxReplans 3", replans)
	}
	if len(attempts) != maxEscalationAttempts {
		t.Errorf("attempts = %d, want %d (backstop reached)", len(attempts), maxEscalationAttempts)
	}
}

// TestReplanManualOverrideStillReplans proves a manual --model-class override pins
// the class (disabling ESCALATION) but does not disable a replan, which keeps the
// class and only changes the strategy.
func TestReplanManualOverrideStillReplans(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runReplanCLI(t, dir, evalDiff, &replanScriptAgent{passOnReplan: true}, jev.NewClearFake(), "run", "--task", "TASK.md", "--model-class", "small")
	if res.code != exitOK {
		t.Fatalf("code=%d, want ok; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 2 || attempts[0].Action != "replan" {
		t.Fatalf("attempts = %+v, want a single replan under a pinned class", attempts)
	}
}

// TestReplanComposesWithEscalation proves the two recoveries layer: a recoverable
// failure replans once on the same class, and the next failure escalates.
func TestReplanComposesWithEscalation(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, escalationFailConfig)

	res := runReplanCLI(t, dir, evalDiff, &replanScriptAgent{passOnReplan: false}, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) < 2 {
		t.Fatalf("attempts = %d, want at least 2", len(attempts))
	}
	if attempts[0].Action != "replan" || attempts[0].Class != "small" {
		t.Errorf("attempt 1 = %+v, want a replan on small", attempts[0])
	}
	// Attempt 2 is the replan (still on small); it failed and escalated FROM small.
	if attempts[1].Action != "escalate" || attempts[1].Class != "small" {
		t.Errorf("attempt 2 = %+v, want an escalation from small", attempts[1])
	}
	if len(attempts) < 3 || attempts[2].Class != "medium" {
		t.Errorf("attempt 3 = %+v, want the escalated attempt on medium", attempts)
	}
}

// TestReplanNeverForNoProgress proves the historical NO_PROGRESS regression is
// unchanged under replanning: repeated activity with no repository mutation blocks
// for operator intervention (not a human decision, not retryable) and never replans,
// with the diagnostic preserved.
func TestReplanNeverForNoProgress(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, preExecRoutingConfig)
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}

	res := runReplanCLI(t, dir, "diff\n", a, jev.NewClearFake(), "run")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if strings.Contains(res.stdout, "replanning") {
		t.Errorf("a NO_PROGRESS stop must never replan: %q", res.stdout)
	}
	for _, got := range attemptsOf(t, dir, "S001") {
		if got.Action == "replan" {
			t.Errorf("attempt recorded a replan for NO_PROGRESS: %+v", got)
		}
	}
	tr := loadEvalTrace(t, dir, "S001")
	if len(tr.Replans) != 0 {
		t.Errorf("trace replans = %+v, want none", tr.Replans)
	}
	if tr.Termination.Kind != "NO_PROGRESS" || tr.Termination.Disposition != "BLOCK" || tr.Termination.HumanRequired {
		t.Errorf("termination = %+v, want NO_PROGRESS/BLOCK/human_required=false", tr.Termination)
	}
	if !strings.Contains(tr.Termination.Diagnostic, "IMPLEMENT_NO_PROGRESS") {
		t.Errorf("diagnostic = %q, want the preserved IMPLEMENT_NO_PROGRESS marker", tr.Termination.Diagnostic)
	}
	runEvalFixture(t, "replan/no-progress-no-replan.expect.json", tr)
}

// TestReplanNeverForHumanBoundary proves a safety/authorization boundary is never
// resolved by changing strategy: it stays a human decision and never replans.
func TestReplanNeverForHumanBoundary(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)
	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: "the requested change is destructive and irreversible, so it requires authorization",
	}}

	res := runReplanCLI(t, dir, evalDiff, a, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if strings.Contains(res.stdout, "replanning") {
		t.Errorf("a human boundary must never replan: %q", res.stdout)
	}
	tr := loadEvalTrace(t, dir, "T001")
	if len(tr.Replans) != 0 {
		t.Errorf("trace replans = %+v, want none", tr.Replans)
	}
	if !tr.Termination.HumanRequired {
		t.Errorf("termination = %+v, want human_required=true", tr.Termination)
	}
	runEvalFixture(t, "replan/human-boundary-no-replan.expect.json", tr)
}

// TestReplanNoneOnSuccess is Case: a run that passes without a recoverable failure
// performs zero strategy changes even when replanning is enabled.
func TestReplanNoneOnSuccess(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvReplanEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	res := runReplanCLI(t, dir, evalDiff, a, jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitOK {
		t.Fatalf("code=%d, want ok; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	tr := loadEvalTrace(t, dir, "T001")
	if len(tr.Replans) != 0 {
		t.Errorf("trace replans = %+v, want none", tr.Replans)
	}
	if strings.Contains(res.stdout, "replanning") {
		t.Errorf("a passing run must not replan: %q", res.stdout)
	}
	runEvalFixture(t, "replan/no-replan.expect.json", tr)
}
