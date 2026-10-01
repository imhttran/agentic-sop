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
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// These tests pin the Phase 5 bounded-escalation integration: when escalation is
// enabled and an attempt fails a quality gate, SOP retries the task on the next
// larger model class, persists one attempt record per try, keeps the initial
// routing decision, and never escalates on a safety boundary, an infrastructure
// error, or past the ladder. With escalation disabled (the default) nothing
// changes.

// escalationFailConfig makes every attempt fail the deterministic validation, so
// the bounded fix loop exhausts and the failure is classified (AutoFixExhausted).
// It enables the pre-execution checkpoint so routing has evidence.
const escalationFailConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"

// scriptedEscalationAgent reviews a diff containing marker as clean and any other
// diff as a blocking finding, and reports a successful change for plan/implement.
// It makes the first attempt fail and the escalated attempt pass deterministically,
// without any prose heuristics.
type scriptedEscalationAgent struct{ marker string }

func (a *scriptedEscalationAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		return agent.Response{Content: "changed files"}, nil
	case agent.Fix:
		return agent.Response{Content: "fixed"}, nil
	case agent.Review:
		if strings.Contains(r.Input, a.marker) {
			return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
		}
		return agent.Response{Content: `{"summary":"bug","findings":[{"severity":"HIGH","title":"nil deref","file":"a.go","line":3}]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// escalationRun is the outcome of an injected escalation CLI run.
type escalationRun struct {
	code   int
	stdout string
	stderr string
	// models is every model the agent factory was asked to build, in order.
	models []string
}

// runEscalation runs `sop run --task TASK.md` with a factory that records the
// models it builds. readDiff returns diffBefore until the escalatedModel agent has
// been built, after which it returns diffAfter — so the first attempt sees the tree
// that fails and the escalated attempt sees the tree that passes.
func runEscalation(t *testing.T, dir, diffBefore, diffAfter, escalatedModel string, jevFake jev.Analyzer, args ...string) escalationRun {
	t.Helper()
	var out, errOut bytes.Buffer
	var models []string
	escalated := false
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(harness, provider, model string) (agent.Agent, error) {
			models = append(models, model)
			if model == escalatedModel {
				escalated = true
			}
			return &scriptedEscalationAgent{marker: "escalated"}, nil
		},
		readDiff: func(context.Context, string) (string, error) {
			if escalated {
				return diffAfter, nil
			}
			return diffBefore, nil
		},
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	if jevFake != nil {
		d.newJEVAnalyzer = fakeJEV(jevFake)
	}
	code := run(args, &out, &errOut, d)
	return escalationRun{code: code, stdout: out.String(), stderr: errOut.String(), models: models}
}

// attemptsOf reads a task's persisted attempt records.
func attemptsOf(t *testing.T, dir, taskID string) []runpkg.AttemptRecord {
	t.Helper()
	return runpkg.ReadAttemptRecordsAt(filepath.Join(dir, stateDirName, "runs", taskID))
}

// TestEscalationDisabledIsUnchanged proves that with escalation off (the default),
// a failing task writes no attempt records and prints no recovery block: existing
// behavior is unchanged.
func TestEscalationDisabledIsUnchanged(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, escalationFailConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, "Recovery:") || strings.Contains(res.stdout, "Retrying") {
		t.Errorf("escalation must be silent when disabled: %q", res.stdout)
	}
	if got := attemptsOf(t, dir, "T001"); len(got) != 0 {
		t.Errorf("attempt records written while escalation is disabled: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "runs", "T001", "attempts")); err == nil {
		t.Error("attempts/ created while escalation is disabled")
	}
}

// TestEscalationSmallToMediumPasses is Scenario B: the SMALL attempt fails, SOP
// escalates to MEDIUM, the MEDIUM attempt passes, and both attempts are recorded.
func TestEscalationSmallToMediumPasses(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "diff --git a/a.go b/a.go\n+escalated\n", "glm-5.3-flash:cloud", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitOK {
		t.Fatalf("code=%d, want ok; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "Task routing: small") {
		t.Errorf("stdout missing the initial SMALL routing: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "Recovery:") || !strings.Contains(res.stdout, "ESCALATE") {
		t.Errorf("stdout missing the recovery decision: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "Retrying (attempt 2):") {
		t.Errorf("stdout missing the retry block: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "glm-5.3-flash:cloud") {
		t.Errorf("stdout missing the escalated model: %q", res.stdout)
	}

	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 2 {
		t.Fatalf("attempts = %+v, want 2", attempts)
	}
	if attempts[0].Attempt != 1 || attempts[0].Class != "small" || attempts[0].Model != "qwen3:4b" {
		t.Errorf("attempt 1 = %+v, want small/qwen3:4b", attempts[0])
	}
	if attempts[0].Result != runpkg.AttemptFailed {
		t.Errorf("attempt 1 result = %q, want failed", attempts[0].Result)
	}
	if attempts[0].Action != "escalate" {
		t.Errorf("attempt 1 action = %q, want escalate", attempts[0].Action)
	}
	if attempts[0].FailureStage != "review" {
		t.Errorf("attempt 1 failure stage = %q, want review", attempts[0].FailureStage)
	}
	if attempts[1].Attempt != 2 || attempts[1].Class != "medium" || attempts[1].Model != "glm-5.3-flash:cloud" {
		t.Errorf("attempt 2 = %+v, want medium/glm-5.3-flash:cloud", attempts[1])
	}
	if attempts[1].Result != runpkg.AttemptPassed {
		t.Errorf("attempt 2 result = %q, want passed", attempts[1].Result)
	}

	// The initial routing decision is preserved: an escalation never overwrites it.
	art := readRoutingArtifact(t, dir)
	if art.Class != "small" || art.Source != runpkg.RoutingSourcePolicy {
		t.Errorf("routing.json = %+v, want the initial small/policy decision", art)
	}

	// The report distinguishes the initial class from the attempts.
	if _, out, _ := runCLI(t, dir, "report", "T001"); !strings.Contains(out, "Execution attempts:") ||
		!strings.Contains(out, "Initial class:") || !strings.Contains(out, "escalated SMALL→MEDIUM: 1") {
		t.Errorf("sop report missing the attempt history: %q", out)
	}
}

// TestEscalationLadderExhaustsAtLarge is Scenario C: SMALL, MEDIUM, and LARGE all
// fail, no fourth class runs, and the task ends at the existing boundary.
func TestEscalationLadderExhaustsAtLarge(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, escalationFailConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	if got := strings.Count(res.stdout, "Retrying (attempt"); got != 2 {
		t.Errorf("retry blocks = %d, want 2 (small->medium, medium->large): %q", got, res.stdout)
	}
	if !strings.Contains(res.stdout, "no larger model class is available") {
		t.Errorf("stdout missing the ladder-exhausted reason: %q", res.stdout)
	}

	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 3 {
		t.Fatalf("attempts = %d, want 3", len(attempts))
	}
	for i, want := range []struct{ class, model string }{
		{"small", "qwen3:4b"},
		{"medium", "glm-5.3-flash:cloud"},
		{"large", "deepseek-v4.1-flash:cloud"},
	} {
		if attempts[i].Class != want.class || attempts[i].Model != want.model {
			t.Errorf("attempt %d = %s/%s, want %s/%s", i+1, attempts[i].Class, attempts[i].Model, want.class, want.model)
		}
		if attempts[i].Result != runpkg.AttemptFailed {
			t.Errorf("attempt %d result = %q, want failed", i+1, attempts[i].Result)
		}
	}
}

// TestEscalationLimitBoundsTheLadder proves the configured bound is enforced
// independently of the ladder: with max_escalations=1 only one escalation happens.
func TestEscalationLimitBoundsTheLadder(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	t.Setenv(model.EnvMaxEscalations, "1")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, escalationFailConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if !strings.Contains(res.stdout, "maximum escalations reached") {
		t.Errorf("stdout missing the escalation-limit reason: %q", res.stdout)
	}
	if attempts := attemptsOf(t, dir, "T001"); len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2 (small, then one escalation to medium)", len(attempts))
	}
}

// TestEscalationZeroBoundDisables proves max_escalations=0 disables escalation even
// when the feature flag is on.
func TestEscalationZeroBoundDisables(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	t.Setenv(model.EnvMaxEscalations, "0")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, escalationFailConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if strings.Contains(res.stdout, "Retrying") {
		t.Errorf("escalation must not run with a zero bound: %q", res.stdout)
	}
	if attempts := attemptsOf(t, dir, "T001"); len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1 (no escalation)", len(attempts))
	}
}

// TestEscalationInfrastructureErrorDoesNotEscalate proves a provider/infrastructure
// error is not a quality-gate failure: SOP does not spend a larger model on it.
func TestEscalationInfrastructureErrorDoesNotEscalate(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	var out, errOut bytes.Buffer
	d := deps{
		getwd:          func() (string, error) { return dir, nil },
		newAgent:       func(string, string, string) (agent.Agent, error) { return errAgent{}, nil },
		readDiff:       func(context.Context, string) (string, error) { return "diff --git a/a.go b/a.go\n-old\n", nil },
		commit:         func(context.Context, string, string) error { return nil },
		newGitHub:      func(string) github.Client { return &fakeGitHub{} },
		newJEVAnalyzer: fakeJEV(jev.NewClearFake()),
	}
	if code := run([]string{"run", "--task", "TASK.md"}, &out, &errOut, d); code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", code, out.String())
	}
	if strings.Contains(out.String(), "Retrying") {
		t.Errorf("an infrastructure error must not escalate: %q", out.String())
	}
}

// errAgent fails every request with an infrastructure error.
type errAgent struct{}

func (errAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	return agent.Response{}, os.ErrDeadlineExceeded
}

// TestEscalationSafetyBoundaryDoesNotEscalate proves a security/destructive early
// evidence escalation (a human boundary) is never resolved by spending a larger
// model.
func TestEscalationSafetyBoundaryDoesNotEscalate(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "glm-5.3-flash:cloud", jev.NewDestructiveConcernFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if strings.Contains(res.stdout, "Retrying") {
		t.Errorf("a safety boundary must not escalate: %q", res.stdout)
	}
	if !strings.Contains(res.stdout+res.stderr, "NEEDS_HUMAN") {
		t.Errorf("expected the existing human boundary; stdout=%s stderr=%s", res.stdout, res.stderr)
	}
}

// TestEscalationManualOverrideDisablesEscalation proves an explicit --model-class
// override pins the class: the operator's choice is never silently replaced.
func TestEscalationManualOverrideDisablesEscalation(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, escalationFailConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "", jev.NewClearFake(), "run", "--task", "TASK.md", "--model-class", "small")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if strings.Contains(res.stdout, "Retrying") {
		t.Errorf("a manual override must not be escalated: %q", res.stdout)
	}
	if got := attemptsOf(t, dir, "T001"); len(got) != 0 {
		t.Errorf("attempt records written for a pinned manual override: %+v", got)
	}
}

// TestEscalatedSelectionIsValidatedBeforeExecution proves the escalated class is
// validated against the runtime before it runs: when the escalated model is absent,
// escalation stops, the escalated agent is never built, and SOP substitutes nothing.
func TestEscalatedSelectionIsValidatedBeforeExecution(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	// Only the SMALL model exists; the escalated MEDIUM model does not.
	t.Setenv(ollama.EnvBaseURL, ollamaTagsServer(t, "qwen3:4b").URL)

	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, validateRoutingConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "glm-5.3-flash:cloud", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "could not be applied") {
		t.Errorf("stdout missing the escalation-unavailable notice: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "no silent model substitution") {
		t.Errorf("stdout must record that no substitution happened: %q", res.stdout)
	}
	if !strings.Contains(strings.Join(res.models, " "), "qwen3:4b") {
		t.Errorf("the validated SMALL model should have been built: %v", res.models)
	}
	for _, m := range res.models {
		if m == "glm-5.3-flash:cloud" {
			t.Errorf("the unvalidated escalated model must never be built: %v", res.models)
		}
	}
}

// substitutedClassConfig configures the escalated class (medium) only partially and
// points the fallback at small, so the model layer's fallback policy would resolve
// "medium" to the small model. Recovery must refuse that substitution rather than
// downgrade the ladder.
const substitutedClassConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n  test:\n    - \"true\"\nmodels:\n  fallback_class: small\n  escalation_enabled: true\n  medium:\n    locality: cloud\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"

// TestEscalationRefusesSubstitutedClass proves an escalated attempt never silently
// accepts a different class from the model layer's fallback policy: a downgrade is
// not an escalation, so recovery stops and keeps the existing path.
func TestEscalationRefusesSubstitutedClass(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, substitutedClassConfig)

	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "glm-5.3-flash:cloud", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", res.code, res.stdout)
	}
	if strings.Contains(res.stdout, "Retrying") {
		t.Errorf("a substituted class must not be treated as an escalation: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "refuses a substituted class") {
		t.Errorf("stdout missing the substitution refusal: %q", res.stdout)
	}
	for _, m := range res.models {
		if m == "glm-5.3-flash:cloud" {
			t.Errorf("the substituted class must never be built: %v", res.models)
		}
	}
	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1 (the escalation never ran)", len(attempts))
	}
	if attempts[0].Action == "escalate" {
		t.Errorf("an unapplied escalation must not be recorded as escalate: %+v", attempts[0])
	}
}

// TestAttemptsAreResetPerRun proves the attempt records describe THIS run: a later
// run of the same task must not leave a previous run's attempts behind.
func TestAttemptsAreResetPerRun(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, escalationFailConfig)

	if res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n-old\n", "", "", jev.NewClearFake(), "run", "--task", "TASK.md"); len(attemptsOf(t, dir, "T001")) != 3 {
		t.Fatalf("first run attempts = %d, want 3; stdout=%s", len(attemptsOf(t, dir, "T001")), res.stdout)
	}

	// A second run against the same project (now passing on the first attempt).
	writeConfig(t, dir, preExecRoutingConfig)
	res := runEscalation(t, dir, "diff --git a/a.go b/a.go\n+escalated\n", "", "", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitOK {
		t.Fatalf("code=%d, want ok; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 1 {
		t.Fatalf("attempts after the second run = %d, want 1 (no stale records)", len(attempts))
	}
	if attempts[0].Class != "small" || attempts[0].Result != runpkg.AttemptPassed {
		t.Errorf("attempt = %+v, want one passing small attempt", attempts[0])
	}
	if _, out, _ := runCLI(t, dir, "report", "T001"); strings.Contains(out, "escalated SMALL→MEDIUM") {
		t.Errorf("report still shows a stale escalation: %q", out)
	}
}

// TestEscalationNoChangesProducesEscalates proves SOP's own deterministic no-change
// verdict participates in bounded recovery: when a SMALL attempt claims success but
// produces no repository change, the failure is classified (NO_CHANGES_PRODUCED) and
// escalated to MEDIUM rather than failing closed, while the initial routing decision
// is preserved.
func TestEscalationNoChangesProducesEscalates(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.EnvEscalationEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	// Attempt 1 changes nothing (diffBefore is empty); the escalated MEDIUM attempt
	// produces a reviewable diff, so it passes.
	res := runEscalation(t, dir, "", "diff --git a/a.go b/a.go\n+escalated\n", "glm-5.3-flash:cloud", jev.NewClearFake(), "run", "--task", "TASK.md")
	if res.code != exitOK {
		t.Fatalf("code=%d, want ok; stdout=%s stderr=%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "Task routing: small") {
		t.Errorf("stdout missing the initial SMALL routing: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "Recovery:") || !strings.Contains(res.stdout, "Action: ESCALATE") {
		t.Errorf("stdout missing the recovery decision: %q", res.stdout)
	}
	// The escalate reason is only produced for a classified implementation failure,
	// so it proves the no-change verdict was classified rather than failing closed.
	if !strings.Contains(res.stdout, "Reason: implementation validation failed") {
		t.Errorf("stdout missing the classified escalation reason: %q", res.stdout)
	}
	if !strings.Contains(res.stdout, "Retrying (attempt 2):") {
		t.Errorf("stdout missing the retry block: %q", res.stdout)
	}

	attempts := attemptsOf(t, dir, "T001")
	if len(attempts) != 2 {
		t.Fatalf("attempts = %+v, want 2", attempts)
	}
	if attempts[0].Class != "small" || attempts[0].Model != "qwen3:4b" || attempts[0].Result != runpkg.AttemptFailed {
		t.Errorf("attempt 1 = %+v, want a failed small/qwen3:4b attempt", attempts[0])
	}
	if attempts[0].Action != "escalate" {
		t.Errorf("attempt 1 action = %q, want escalate (the no-change verdict is now classified)", attempts[0].Action)
	}
	if attempts[1].Class != "medium" || attempts[1].Model != "glm-5.3-flash:cloud" || attempts[1].Result != runpkg.AttemptPassed {
		t.Errorf("attempt 2 = %+v, want a passing medium/glm-5.3-flash:cloud attempt", attempts[1])
	}

	// The initial routing decision is preserved: an escalation never overwrites it.
	art := readRoutingArtifact(t, dir)
	if art.Class != "small" || art.Source != runpkg.RoutingSourcePolicy {
		t.Errorf("routing.json = %+v, want the initial small/policy decision", art)
	}
}

// TestNoChangesClassificationKeepsRecoveryOff proves the no-change classification is
// additive: with escalation off, the task still stops at the existing boundary and
// renders no recovery, while the failure is now classified rather than left empty.
func TestNoChangesClassificationKeepsRecoveryOff(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}

	code, stdout, _ := runInjectedCLI(t, dir, "   \n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want error; stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "no repository changes") {
		t.Errorf("stdout missing the no-change reason: %q", stdout)
	}
	if !strings.Contains(stdout, "classification: AUTO_FIX (NO_CHANGES_PRODUCED") {
		t.Errorf("the no-change verdict should be classified: %q", stdout)
	}
	if strings.Contains(stdout, "Retrying") {
		t.Errorf("recovery must be silent when escalation is off: %q", stdout)
	}
}

// TestEscalationDisabledKeepsReportUnchanged proves an unrouted task's report is
// unchanged when escalation is off: no attempt section is rendered.
func TestEscalationDisabledKeepsReportUnchanged(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if _, out, _ := runCLI(t, dir, "report", "T001"); strings.Contains(out, "Execution attempts:") {
		t.Errorf("report should render no attempts when escalation is off: %q", out)
	}
}
