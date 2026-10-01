package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// The local-first runtime fallback (SMALL): a local class runs its configured
// cloud fallback only when the local runtime cannot serve the primary model, and
// only through the injectable availability probe. These tests drive the probe
// deterministically, so no live Ollama server is required, and they prove the
// fallback is an AVAILABILITY switch — never a generation-failure recovery.

// usableLocal is the probe for "the local model is available": the fallback must
// not be used.
func usableLocal(config.Config, model.Selection) (bool, string) { return false, "" }

// unavailableLocal is the probe for "the local runtime cannot serve the model".
func unavailableLocal(config.Config, model.Selection) (bool, string) {
	return true, "ollama: unavailable"
}

// runRoutingFallbackCLI runs a task command with an injected working-tree diff,
// JEV analyzer, recording agent factory, and local-availability probe. It returns
// the model names the agent factory was asked to build, so a test can prove which
// model actually executed.
func runRoutingFallbackCLI(t *testing.T, dir, diff string, a agent.Agent, newJEV func(config.Config) (jev.Analyzer, error), probe func(config.Config, model.Selection) (bool, string), args ...string) (code int, stdout, stderr string, built []string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(_, _, m string) (agent.Agent, error) {
			if a == nil {
				return nil, errors.New("no agent configured")
			}
			built = append(built, m)
			return a, nil
		},
		readDiff:       func(context.Context, string) (string, error) { return diff, nil },
		commit:         func(context.Context, string, string) error { return nil },
		newGitHub:      func(string) github.Client { return &fakeGitHub{} },
		newJEVAnalyzer: newJEV,
		localProbe:     probe,
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String(), built
}

// builtContains reports whether the agent factory was asked to build model.
func builtContains(built []string, want string) bool {
	for _, m := range built {
		if m == want {
			return true
		}
	}
	return false
}

// TestRoutingSmallKeepsLocalModelWhenUsable proves the local model runs when the
// local runtime can serve it, and the cloud fallback is never invoked.
func TestRoutingSmallKeepsLocalModelWhenUsable(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr, built := runRoutingFallbackCLI(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), usableLocal, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: small (qwen3:4b; isolated low-risk task)") {
		t.Errorf("stdout missing the local SMALL routing line: %q", stdout)
	}
	if strings.Contains(stdout, "nemotron-3-nano:30b-cloud") {
		t.Errorf("the cloud fallback must not appear when the local model is usable: %q", stdout)
	}
	if !builtContains(built, "qwen3:4b") || builtContains(built, "nemotron-3-nano:30b-cloud") {
		t.Errorf("built models = %v, want the local model only", built)
	}
}

// TestRoutingSmallFallsBackToCloudWhenLocalUnavailable proves that a local class
// whose runtime cannot serve the primary model runs the configured cloud
// fallback, and records it as a cloud-fallback selection.
func TestRoutingSmallFallsBackToCloudWhenLocalUnavailable(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr, built := runRoutingFallbackCLI(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), unavailableLocal, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: small (nemotron-3-nano:30b-cloud; isolated low-risk task; local runtime unavailable; cloud fallback)") {
		t.Errorf("stdout missing the fallback routing line: %q", stdout)
	}
	if !builtContains(built, "nemotron-3-nano:30b-cloud") {
		t.Errorf("built models = %v, want the cloud fallback", built)
	}

	art := readRoutingArtifact(t, dir)
	if art.Class != "small" {
		t.Errorf("class = %q, want small (the fallback keeps the class)", art.Class)
	}
	if art.Model != "nemotron-3-nano:30b-cloud" || art.Locality != "cloud" {
		t.Errorf("routing artifact = %+v, want the cloud fallback model/locality", art)
	}
	if !strings.Contains(strings.Join(art.Reasons, ";"), model.ReasonLocalFallback) {
		t.Errorf("reasons = %v, want the local-fallback phrase recorded", art.Reasons)
	}
}

// TestRoutingFallbackOverrideFromEnv proves the fallback model is configurable
// through the environment, and that the recorded source is cloud-fallback.
func TestRoutingFallbackOverrideFromEnv(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv(model.ClassFallbackEnvKey(model.ClassSmall, model.FallbackFieldName), "other-cloud:latest")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr, built := runRoutingFallbackCLI(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), unavailableLocal, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: small (other-cloud:latest;") {
		t.Errorf("stdout missing the overridden fallback model: %q", stdout)
	}
	if !builtContains(built, "other-cloud:latest") {
		t.Errorf("built models = %v, want the configured fallback", built)
	}
}

// TestManualSmallOverrideUsesLocalFallback proves a manual --model-class override
// pins the CLASS but not the model within it, so the local-first fallback applies
// to an explicitly chosen class exactly as it does to a router-selected one.
func TestManualSmallOverrideUsesLocalFallback(t *testing.T) {
	clearModelEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, plainRoutingConfig)

	code, stdout, stderr, built := runRoutingFallbackCLI(t, dir, "diff --git a/x b/x\n", routingAgent(), nil, unavailableLocal, "run", "--task", "TASK.md", "--model-class", "small")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: small (nemotron-3-nano:30b-cloud; manual model-class override; local runtime unavailable; cloud fallback)") {
		t.Errorf("stdout missing the manual-override fallback line: %q", stdout)
	}
	if !builtContains(built, "nemotron-3-nano:30b-cloud") {
		t.Errorf("built models = %v, want the cloud fallback", built)
	}
	art := readRoutingArtifact(t, dir)
	if art.Class != "small" || art.Source != runpkg.RoutingSourceManual {
		t.Errorf("routing artifact = %+v, want small/manual_override", art)
	}
}

// TestLocalFallbackIsNotGenerationRecovery proves the fallback is an AVAILABILITY
// switch: a local model that passes readiness and then fails generation does NOT
// silently switch to the cloud fallback. The existing error handling stays
// authoritative.
func TestLocalFallbackIsNotGenerationRecovery(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr, built := runRoutingFallbackCLI(t, dir, "diff --git a/x b/x\n", failingImplementAgent{}, fakeJEV(jev.NewClearFake()), usableLocal, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("a generation failure must not succeed: stdout=%s", stdout)
	}
	if strings.Contains(stdout, "nemotron-3-nano:30b-cloud") || builtContains(built, "nemotron-3-nano:30b-cloud") {
		t.Errorf("a generation failure must not switch to the cloud fallback: stdout=%q built=%v", stdout, built)
	}
	if !builtContains(built, "qwen3:4b") {
		t.Errorf("built models = %v, want the local SMALL model that was proven ready", built)
	}
	if !strings.Contains(stderr+stdout, "generation failed") {
		t.Errorf("the original generation error must be surfaced: stdout=%q stderr=%q", stdout, stderr)
	}
}

// failingImplementAgent succeeds PLAN and every other capability but fails
// IMPLEMENT, modelling a local model that passes readiness and then fails during
// generation.
type failingImplementAgent struct{}

func (failingImplementAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Implement:
		return agent.Response{}, errors.New("local model generation failed")
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	default:
		return agent.Response{Content: `{"summary":"ok","findings":[]}`}, nil
	}
}

// TestRoutingTierModelsFromRouter pins the configured execution model per tier:
// SMALL local, MEDIUM and LARGE cloud.
func TestRoutingTierModelsFromRouter(t *testing.T) {
	clearModelEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, plainRoutingConfig)

	// MEDIUM is the safe default when the router is on but no evidence exists.
	t.Setenv(model.EnvRoutingEnabled, "true")
	code, stdout, stderr, _ := runRoutingFallbackCLI(t, dir, "diff --git a/x b/x\n", routingAgent(), nil, usableLocal, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: medium (nemotron-3-super:cloud;") {
		t.Errorf("stdout missing the medium cloud model: %q", stdout)
	}
}
