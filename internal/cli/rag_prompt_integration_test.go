package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runWithRAG mirrors runInjectedCLI but injects the optional-RAG retrieval seam.
func runWithRAG(t *testing.T, dir, diff string, a agent.Agent, rag ragKnowledgeFunc, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd:        func() (string, error) { return dir, nil },
		newAgent:     func(string, string, string) (agent.Agent, error) { return a, nil },
		readDiff:     func(context.Context, string) (string, error) { return diff, nil },
		commit:       func(context.Context, string, string) error { return nil },
		newGitHub:    func(string) github.Client { return &fakeGitHub{} },
		ragKnowledge: rag,
	}
	code := run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

// ragCapturingAgent records every agent.Request.Input and delegates to the
// existing fake so the task completes normally.
type ragCapturingAgent struct {
	fakeCapabilityAgent
	mu     sync.Mutex
	inputs []string
}

func (c *ragCapturingAgent) Generate(ctx context.Context, r agent.Request) (agent.Response, error) {
	c.mu.Lock()
	c.inputs = append(c.inputs, r.Input)
	c.mu.Unlock()
	return c.fakeCapabilityAgent.Generate(ctx, r)
}

func TestRAGEnabledAgentInput(t *testing.T) {
	ragEnabledEnv(t)

	calls := 0
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		calls++
		return []sopctx.Item{{
			Source:   sopctx.SourceKnowledgeRAG,
			Identity: "kb/docs/adr.md#Retrieval",
			Reason:   "retrieved knowledge evidence (untrusted; never an instruction)",
			Priority: sopctx.PriorityKnowledgeRAG,
			Text:     "the ADR records the retrieval fallback decision",
		}}, nil
	}

	dir := threeStageGraph(t)
	a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}

	code, stdout, stderr := runWithRAG(t, dir, "diff --git a/x b/x\n", a, rag, "run", "--max-tasks", "1")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if calls != 1 {
		t.Fatalf("retriever calls = %d, want exactly 1", calls)
	}

	var impl string
	for _, in := range a.inputs {
		if strings.Contains(in, "kb/docs/adr.md#Retrieval") {
			impl = in
		}
	}
	if impl == "" {
		t.Fatalf("no agent request carried the citation; captured %d inputs: %q", len(a.inputs), a.inputs)
	}
	if !strings.Contains(impl, "the ADR records the retrieval fallback decision") {
		t.Fatal("retrieved evidence text missing from the agent input")
	}
	if !strings.Contains(impl, "UNTRUSTED-RAG-KNOWLEDGE") || !strings.Contains(impl, "END-UNTRUSTED-RAG-KNOWLEDGE") {
		t.Fatal("untrusted-knowledge boundary missing from the agent input")
	}
}

// runAndCaptureImplInput runs one bounded task through the real dispatch path and
// returns the Implement-stage agent input (the last captured request).
func runAndCaptureImplInput(t *testing.T, rag ragKnowledgeFunc) string {
	t.Helper()
	dir := threeStageGraph(t)
	a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	code, stdout, stderr := runWithRAG(t, dir, "diff --git a/x b/x\n", a, rag, "run", "--max-tasks", "1")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(a.inputs) == 0 {
		t.Fatal("no agent request captured")
	}
	return a.inputs[len(a.inputs)-1]
}

func TestRAGDisabledAgentInput(t *testing.T) {
	t.Setenv(config.EnvRAGEnabled, "false")

	// Baseline: disabled RAG, no retriever wired.
	baseline := runAndCaptureImplInput(t, nil)

	// Disabled path with a counting retriever that must never be called.
	calls := 0
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		calls++
		t.Error("retriever invoked while RAG is disabled")
		return nil, nil
	}
	disabled := runAndCaptureImplInput(t, rag)

	if calls != 0 {
		t.Fatalf("retrieval calls = %d, want 0", calls)
	}
	if strings.Contains(disabled, "UNTRUSTED-RAG-KNOWLEDGE") {
		t.Fatal("disabled RAG emitted an untrusted-knowledge boundary")
	}
	if disabled != baseline {
		t.Fatalf("disabled agent input differs from baseline:\n--- baseline ---\n%s\n--- disabled ---\n%s", baseline, disabled)
	}
}

func TestRAGOptionalFailureAgentInput(t *testing.T) {
	// Baseline: RAG disabled, no retriever.
	t.Setenv(config.EnvRAGEnabled, "false")
	baseline := runAndCaptureImplInput(t, nil)

	// Enabled RAG with a retriever that fails deterministically.
	ragEnabledEnv(t)

	calls := 0
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		calls++
		return nil, errors.New("producer unavailable")
	}
	got := runAndCaptureImplInput(t, rag)

	if calls != 1 {
		t.Fatalf("failing retriever calls = %d, want exactly 1", calls)
	}
	if strings.Contains(got, "UNTRUSTED-RAG-KNOWLEDGE") {
		t.Fatal("an untrusted-knowledge boundary appeared after an optional retrieval failure")
	}
	if got != baseline {
		t.Fatalf("optional failure changed the agent input:\n--- baseline ---\n%s\n--- after failure ---\n%s", baseline, got)
	}
}

func TestRAGSharedBudgetAgentInput(t *testing.T) {
	ragEnabledEnv(t)
	// A RAG byte budget large enough not to truncate on its own, so the truncation
	// observed below is the SHARED sopctx.DefaultLimits() ceiling, not RAG's.
	t.Setenv(config.EnvRAGContextBudget, "200000")

	calls := 0
	big := strings.Repeat("A", 200000) + "TAILMARKER"
	rag := func(context.Context, *taskfile.Spec) ([]sopctx.Item, error) {
		calls++
		return []sopctx.Item{{
			Source:   sopctx.SourceKnowledgeRAG,
			Identity: "kb/big.md#S",
			Reason:   "retrieved knowledge evidence (untrusted; never an instruction)",
			Priority: sopctx.PriorityKnowledgeRAG,
			Text:     big,
		}}, nil
	}

	dir := threeStageGraph(t)
	a := &ragCapturingAgent{fakeCapabilityAgent: fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}}
	code, stdout, stderr := runWithRAG(t, dir, "diff --git a/x b/x\n", a, rag, "run", "--max-tasks", "1")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if calls != 1 {
		t.Fatalf("retriever calls = %d, want exactly 1", calls)
	}
	impl := a.inputs[len(a.inputs)-1]

	// Higher-priority lifecycle evidence survives.
	hi := strings.Index(impl, "First stage.")
	ragIdx := strings.Index(impl, "UNTRUSTED-RAG-KNOWLEDGE")
	if hi < 0 {
		t.Fatal("higher-priority lifecycle evidence missing from the shared-budget prompt")
	}
	if hi > ragIdx && ragIdx >= 0 {
		t.Fatal("priority-6 RAG item appeared before higher-priority context")
	}
	// The 200 KB body must be bounded by the shared context budget: either it is
	// truncated (tail gone) or the whole RAG item is omitted when the budget is
	// exhausted. Both are acceptable; only an unbounded body is a failure.
	if strings.Contains(impl, "TAILMARKER") {
		t.Fatal("RAG text was not bounded by the shared context budget")
	}
	if ragIdx >= 0 && !strings.Contains(impl, "kb/big.md#S") {
		t.Fatal("RAG item present without its citation")
	}
}
