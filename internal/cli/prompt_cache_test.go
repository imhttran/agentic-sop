package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// countingPromptAgent counts Generate calls and serves the read-only text capabilities.
type countingPromptAgent struct {
	calls   int
	content string
}

func (a *countingPromptAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	a.calls++
	return agent.Response{Content: a.content}, nil
}

func (a *countingPromptAgent) Capabilities() agent.Capabilities {
	return agent.NewCapabilities(agent.Plan, agent.DesignTests, agent.DiagnoseFailure, agent.Review)
}

// TestPromptCacheHitsOnIdenticalReadOnlyPrompt proves sop prompt --cache serves an
// identical read-only prompt from the CTX-008 Prompt Result Cache.
func TestPromptCacheHitsOnIdenticalReadOnlyPrompt(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &countingPromptAgent{content: "the plan"}

	if code, _, se := runCLIWithAgent(t, dir, a, "prompt", "--cache", "Explain the architecture"); code != exitOK {
		t.Fatalf("first prompt: %s", se)
	}
	if a.calls != 1 {
		t.Fatalf("first prompt must call the agent once, got %d", a.calls)
	}

	code, stdout, se := runCLIWithAgent(t, dir, a, "prompt", "--cache", "Explain the architecture")
	if code != exitOK {
		t.Fatalf("second prompt: %s", se)
	}
	if a.calls != 1 {
		t.Errorf("an identical prompt must be served from cache; agent calls=%d", a.calls)
	}
	if !strings.Contains(stdout, "the plan") {
		t.Errorf("cached result missing: %q", stdout)
	}
}

// TestPromptCacheMissesOnDifferentPrompt proves a different prompt is a cache miss.
func TestPromptCacheMissesOnDifferentPrompt(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &countingPromptAgent{content: "x"}

	if code, _, _ := runCLIWithAgent(t, dir, a, "prompt", "--cache", "first"); code != exitOK {
		t.Fatal("first prompt failed")
	}
	if code, _, _ := runCLIWithAgent(t, dir, a, "prompt", "--cache", "second"); code != exitOK {
		t.Fatal("second prompt failed")
	}
	if a.calls != 2 {
		t.Errorf("a different prompt must not reuse the cache; agent calls=%d", a.calls)
	}
}

// TestPromptWithoutCacheAlwaysCallsAgent proves the cache is opt-in.
func TestPromptWithoutCacheAlwaysCallsAgent(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &countingPromptAgent{content: "x"}
	for i := 0; i < 2; i++ {
		if code, _, _ := runCLIWithAgent(t, dir, a, "prompt", "same"); code != exitOK {
			t.Fatal("prompt failed")
		}
	}
	if a.calls != 2 {
		t.Errorf("without --cache the agent must run every time; calls=%d", a.calls)
	}
}
