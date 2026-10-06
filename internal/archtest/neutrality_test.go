package archtest

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/adaptiveroute"
	"github.com/imhttran/agentic-sop/internal/agent"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/normalize"
	"github.com/imhttran/agentic-sop/internal/prompt"
	"github.com/imhttran/agentic-sop/internal/promptcache"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// neutralAdapter is a test-only canonical agent. Its behavior is defined by its declared
// capabilities and its canned content, never by a provider name.
type neutralAdapter struct {
	name    string
	content string
	caps    agent.Capabilities
}

func (a neutralAdapter) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	return agent.Response{Content: a.content}, nil
}

func (a neutralAdapter) Capabilities() agent.Capabilities { return a.caps }

// fence wraps body in a fenced code block; the backticks are built from runes so the test
// source stays free of raw-string literals.
func fence(body string) string {
	f := strings.Repeat(string(rune(96)), 3)
	nl := string(rune(10))
	return f + "json" + nl + body + nl + f
}

// runCorePipeline drives the provider-neutral core: the Context Engine, retrieval, the
// Prompt Compiler, a canonical agent request, an adapter, and the Response Normalizer.
// It takes a canonical agent and never names a provider.
func runCorePipeline(t *testing.T, a agent.Agent, task string) (string, normalize.Result) {
	t.Helper()

	ctx := sopctx.FromInputs(sopctx.Inputs{
		TaskID: "S001",
		Task:   task,
		Plan:   "# Plan\n\n## S001 - do it",
	}, sopctx.DefaultLimits())

	candidates := []retrieval.Candidate{
		{ID: "symbol:pkg.Do", Source: retrieval.SourceSymbol, Path: "pkg/do.go", Text: "Do function pkg"},
		{ID: "file:pkg/do_test.go", Source: retrieval.SourceFile, Path: "pkg/do_test.go", Text: "do_test.go test"},
	}
	if ranked := retrieval.New(candidates).Search(retrieval.Query{Terms: []string{"do"}}, 0); len(ranked) == 0 {
		t.Fatal("retrieval returned no results")
	}

	compiled := prompt.Compile(prompt.Input{
		Capability: agent.Plan,
		Class:      prompt.Medium,
		Task:       task,
		Context:    ctx,
	})

	request := agent.Request{
		Capability:         agent.Plan,
		Task:               task,
		Input:              compiled.Evidence(),
		OutputRequirements: "Return JSON only.",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("canonical request invalid: %v", err)
	}

	response, err := a.Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("adapter returned an error: %v", err)
	}
	return compiled.Render(), normalize.Response(response.Content)
}

// TestSameCorePipelineAcrossAdapters proves the same core pipeline runs over two
// materially different canonical adapters, and that the core output does not depend on
// which adapter served it.
func TestSameCorePipelineAcrossAdapters(t *testing.T) {
	body := "{\"summary\":\"ok\"}"
	a := neutralAdapter{name: "adapter-A", content: body, caps: agent.NewCapabilities(agent.Plan)}
	b := neutralAdapter{name: "adapter-B", content: fence(body), caps: agent.NewCapabilities(agent.Plan)}

	promptA, normalizedA := runCorePipeline(t, a, "Plan the work")
	promptB, normalizedB := runCorePipeline(t, b, "Plan the work")

	if promptA != promptB {
		t.Errorf("the compiled prompt must not depend on the adapter:\n%s\n%s", promptA, promptB)
	}
	if normalizedA.Kind != normalize.KindJSON || normalizedB.Kind != normalize.KindJSON {
		t.Fatalf("both adapters must normalize to a JSON document: %v / %v", normalizedA.Kind, normalizedB.Kind)
	}
	if normalizedA.Content != normalizedB.Content || normalizedA.Content != body {
		t.Errorf("normalized output must be adapter-independent: %q vs %q, want %q", normalizedA.Content, normalizedB.Content, body)
	}
}

// TestCapabilityGateIsBehavioral proves the harness gates on a declared capability, not
// on a provider name: an adapter that does not declare a capability is rejected, and a
// declared one is served.
func TestCapabilityGateIsBehavioral(t *testing.T) {
	planOnly := neutralAdapter{name: "plan-only", content: "{}", caps: agent.NewCapabilities(agent.Plan)}
	checked := agent.NewChecked(planOnly)

	if _, err := checked.Generate(context.Background(), agent.Request{Capability: agent.Implement, Task: "mutate the repository"}); err == nil {
		t.Error("an adapter without the IMPLEMENT capability must be rejected")
	}
	if _, err := checked.Generate(context.Background(), agent.Request{Capability: agent.Plan, Task: "plan the work"}); err != nil {
		t.Errorf("a declared capability must be served: %v", err)
	}
}

// TestPromptCacheIsolatesProviderModel proves a cached result cannot leak between
// incompatible provider/model identities: the identity is data, and a different identity
// is a miss.
func TestPromptCacheIsolatesProviderModel(t *testing.T) {
	base := promptcache.Identity{
		Schema:     promptcache.SchemaVersion,
		Capability: "PLAN",
		Prompt:     "prompt-digest",
		Context:    "context-digest",
		Model:      "provider-A/model-A",
		Parameters: "default",
		Compiler:   "prompt-compiler/1",
	}
	other := base
	other.Model = "provider-B/model-B"

	if base.Digest() == other.Digest() {
		t.Fatal("provider/model identity must change the prompt-cache key")
	}

	cache := promptcache.New()
	if _, err := cache.Store(base, "result-from-A", "agent"); err != nil {
		t.Fatal(err)
	}
	if hit := cache.Lookup(base); !hit.Hit {
		t.Fatal("the same identity must hit")
	}
	if hit := cache.Lookup(other); hit.Hit {
		t.Error("provider/model B must not receive provider/model A's cached result")
	}
	if _, err := cache.Store(other, "result-from-B", "agent"); err != nil {
		t.Fatal(err)
	}
	if got := cache.Lookup(base); !got.Hit || got.Entry.Content != "result-from-A" {
		t.Errorf("A's entry must survive B's store: %+v", got.Entry)
	}
}

// TestAdaptiveRoutingIsCapabilityAndEvidenceDriven proves the routing policy selects a
// model CLASS from the capability and accumulated evidence, names no provider, and needs
// no provider implementation. Resolving a class to a concrete provider/model is a
// separate, configuration-driven step (internal/model).
func TestAdaptiveRoutingIsCapabilityAndEvidenceDriven(t *testing.T) {
	decision := adaptiveroute.Route(adaptiveroute.Input{
		Baseline:   model.ClassSmall,
		Capability: agent.Implement,
		Evidence: []adaptiveroute.Outcome{
			{Class: model.ClassSmall, Capability: agent.Implement, Success: false},
			{Class: model.ClassSmall, Capability: agent.Implement, Success: false},
			{Class: model.ClassSmall, Capability: agent.Implement, Success: false},
		},
	})
	if decision.Class != model.ClassMedium {
		t.Fatalf("class = %s, want medium (a failing baseline escalates)", decision.Class)
	}
	for _, s := range []string{decision.Reason, decision.Evidence} {
		for _, provider := range []string{"ollama", "openai", "anthropic", "claude", "qwen", "deepseek"} {
			if strings.Contains(strings.ToLower(s), provider) {
				t.Errorf("routing output must not name a provider: %q", s)
			}
		}
	}
}
