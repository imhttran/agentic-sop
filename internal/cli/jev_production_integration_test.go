package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// JEV-205 — Integration Validation.
//
// This file exercises the REAL production analyzer wiring
// (defaultDeps().newJEVAnalyzer -> jev.NewOllamaAnalyzerFromEnv -> OllamaAnalyzer)
// through the CLI lifecycle, rather than the sequencedJEV / earlyFakeAnalyzer test
// doubles the other tests use. The production construction path is:
//
//   - internal/cli/cli.go:      deps.newJEVAnalyzer; defaultDeps().newJEVAnalyzer
//     returns jev.NewOllamaAnalyzerFromEnv().
//   - internal/cli/jev.go:      runOptionalJEV gates on cfg.JEVActive(), consults
//     deps.newJEVAnalyzer, runs runpkg.RunJEV, and shapes jevEvidence().
//   - internal/jev/ollama.go:   NewOllamaAnalyzer(cfg, provider) builds the real
//     OllamaAnalyzer; Analyze() drives provider.ChatStructured and attaches
//     validated structured evidence via attachStructuredEvidence().
//   - internal/cli/early_jev.go: runJEVCheckpoint / runEarlyGate apply
//     autonomy.DecideEarly to structured evidence.
//
// Injection point (keeps the test hermetic): jev.NewOllamaAnalyzer is called with
// an injected, deterministic ChatProvider. The production OllamaAnalyzer is the
// object under test; only the transport is replaced, so no Claude and no live
// Ollama server is ever contacted. A config carrying BaseURL/Model/Timeout is
// required for NewOllamaAnalyzer to succeed.
//
// Observable fields asserted: jev.Result.StructuredEvidence (Version/Purpose/
// Status/Severity), quality.JEVEvidence.FailClosed + class-only Reason, and the
// deterministic autonomy.Decision produced by autonomy.DecideEarly.

// productionJEVConfig is the Ollama configuration the production analyzer needs.
// It is fixed and local so the test never depends on the environment.
func productionJEVConfig() ollamaagent.Config {
	return ollamaagent.Config{
		BaseURL: "http://127.0.0.1:11434",
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
}

// scriptedChatProvider is a deterministic ChatProvider injected into the REAL
// OllamaAnalyzer. It returns canned responses in order (repeating the last),
// recording the prompts it received. It touches no network.
type scriptedChatProvider struct {
	responses []string
	calls     int
	prompts   []string
}

func (p *scriptedChatProvider) next() string {
	i := p.calls
	p.calls++
	if len(p.responses) == 0 {
		return ""
	}
	if i >= len(p.responses) {
		i = len(p.responses) - 1
	}
	return p.responses[i]
}

func (p *scriptedChatProvider) Chat(_ context.Context, _ string, prompt string) (string, error) {
	p.prompts = append(p.prompts, prompt)
	return p.next(), nil
}

func (p *scriptedChatProvider) ChatStructured(_ context.Context, _ string, prompt string, _ json.RawMessage) (string, error) {
	p.prompts = append(p.prompts, prompt)
	return p.next(), nil
}

// newProductionAnalyzer builds the REAL production analyzer with the injected
// provider. The returned factory is exactly the shape deps.newJEVAnalyzer has, so
// it is wired into the CLI unchanged.
func newProductionAnalyzer(t *testing.T, provider jev.ChatProvider) func(config.Config) (jev.Analyzer, error) {
	t.Helper()
	return func(config.Config) (jev.Analyzer, error) {
		return jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
	}
}

// validStructuredJSON is a schema-valid model response carrying one typed finding
// and a versioned structured-evidence shape the analyzer will validate.
const validStructuredJSON = `{"status":"FINDINGS","summary":"one issue","findings":[{"id":"f1","severity":"HIGH","category":"security","path":"internal/x.go","line":7,"message":"credential logged","evidence":"log.Println(token)"}]}`

const cleanStructuredJSON = `{"status":"PASS","summary":"clean","findings":[]}`

// malformedOutput is never valid JSON, so the production analyzer exhausts its
// bounded corrective retries and fails closed with MALFORMED_OUTPUT.
const malformedOutput = "not json at all: <<<>>>"

// invalidCategoryJSON parses as JSON but carries an unrecognized category, so
// attachStructuredEvidence fails closed (validation rejects the category).
const invalidCategoryJSON = `{"status":"FINDINGS","summary":"x","findings":[{"id":"f1","severity":"HIGH","category":"definitely_not_a_category","path":"a.go","line":1,"message":"m","evidence":"e"}]}`

// TestJEVProductionAnalyzerIntegration drives the production analyzer wiring and
// asserts structured-evidence generation, fail-closed behavior, and early-JEV /
// autonomy routing integration — all without Claude or a live Ollama server.
func TestJEVProductionAnalyzerIntegration(t *testing.T) {
	t.Run("structured-evidence generation", func(t *testing.T) {
		provider := &scriptedChatProvider{responses: []string{validStructuredJSON}}
		analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}

		out := runpkg.RunJEV(context.Background(), analyzer, runpkg.JEVInvocation{
			Task:         "task under review",
			ChangedFiles: []string{"internal/x.go"},
		})
		if out.FailClosed() {
			t.Fatalf("valid output must not fail closed: err=%v", out.Error)
		}
		res := out.Result
		if res.StructuredEvidence == nil {
			t.Fatal("the production analyzer did not attach structured evidence")
		}
		ev := res.StructuredEvidence
		if ev.Version != jev.EvidenceVersion {
			t.Errorf("evidence version = %d, want %d", ev.Version, jev.EvidenceVersion)
		}
		if ev.Purpose != jev.PurposeQuality {
			t.Errorf("evidence purpose = %q, want %q", ev.Purpose, jev.PurposeQuality)
		}
		if ev.Status != jev.EvidenceFindings {
			t.Errorf("evidence status = %q, want %q", ev.Status, jev.EvidenceFindings)
		}
		if ev.Severity != jev.SeverityHigh {
			t.Errorf("evidence severity = %q, want %q", ev.Severity, jev.SeverityHigh)
		}
		if err := ev.Validate(); err != nil {
			t.Errorf("attached evidence must validate: %v", err)
		}

		// The same result maps to non-fail-closed gate evidence.
		gate := jevEvidence(out)
		if gate == nil || gate.FailClosed {
			t.Fatalf("valid output must yield non-fail-closed gate evidence; got %+v", gate)
		}
	})

	t.Run("fail-closed on malformed output", func(t *testing.T) {
		provider := &scriptedChatProvider{responses: []string{malformedOutput}}
		analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}

		out := runpkg.RunJEV(context.Background(), analyzer, runpkg.JEVInvocation{Task: "bad task"})
		if !out.FailClosed() {
			t.Fatal("malformed output must fail closed")
		}
		// The bounded corrective retry is exhausted before failing closed.
		if provider.calls != 3 {
			t.Errorf("provider calls = %d, want 3 (initial + bounded corrective retries)", provider.calls)
		}

		gate := jevEvidence(out)
		if gate == nil || !gate.FailClosed {
			t.Fatalf("malformed output must yield fail-closed gate evidence; got %+v", gate)
		}
		// The reason is class-only: it names MALFORMED_OUTPUT and never echoes the
		// raw response.
		if !strings.Contains(gate.Reason, string(jev.ErrMalformedOutput)) {
			t.Errorf("fail-closed reason = %q, want it to name %q", gate.Reason, jev.ErrMalformedOutput)
		}
		if strings.Contains(gate.Reason, "not json at all") {
			t.Errorf("fail-closed reason leaked raw provider text: %q", gate.Reason)
		}
	})

	t.Run("fail-closed on invalid structured evidence", func(t *testing.T) {
		provider := &scriptedChatProvider{responses: []string{invalidCategoryJSON}}
		analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}

		res, err := analyzer.Analyze(context.Background(), jev.Request{Task: "invalid evidence"})
		if err == nil {
			t.Fatalf("invalid structured evidence must fail closed; got result %+v", res)
		}
		if res.StructuredEvidence != nil {
			t.Errorf("invalid evidence must never be attached: %+v", res.StructuredEvidence)
		}
		var zero jev.Result
		if res.Status != zero.Status || len(res.Findings) != 0 {
			t.Errorf("fail-closed must return a zero Result, got %+v", res)
		}
	})

	t.Run("fail-closed never authorizes continuation through the CLI", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "TASK.md", runTaskFile)
		writeConfig(t, dir, jevEnabledConfig)
		a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}

		provider := &scriptedChatProvider{responses: []string{malformedOutput}}
		code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
			newProductionAnalyzer(t, provider), "run", "--task", "TASK.md")
		if code == exitOK {
			t.Fatalf("a fail-closed production JEV must not pass; stdout=%s stderr=%s", stdout, stderr)
		}
		// No fix context is authorized for a fail-closed JEV.
		if len(a.fixInputs) != 0 {
			t.Errorf("fail-closed JEV must not authorize FIX payloads; got %d", len(a.fixInputs))
		}
	})

	t.Run("valid production output continues and reaches FIX on blocking findings", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "TASK.md", runTaskFile)
		writeConfig(t, dir, jevEnabledConfig)
		a := &jevTestAgent{plan: validPlanJSON, impl: "impl", fix: "fixed", review: `{"summary":"clean","findings":[]}`}

		// First invocation reports a blocking finding; the second is clean, so the
		// fix loop resolves.
		provider := &scriptedChatProvider{responses: []string{validStructuredJSON, cleanStructuredJSON}}
		code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
			newProductionAnalyzer(t, provider), "run", "--task", "TASK.md")
		if code != exitOK {
			t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
		}
		if len(a.fixInputs) != 1 {
			t.Fatalf("fix invocations = %d, want 1", len(a.fixInputs))
		}
		if !strings.Contains(a.fixInputs[0], "credential logged") {
			t.Errorf("FIX input missing the production finding:\n%s", a.fixInputs[0])
		}
	})

	t.Run("early-JEV routing applies autonomy.DecideEarly", func(t *testing.T) {
		// The production analyzer's structured evidence is consumed by the early
		// checkpoint, which applies the deterministic autonomy bridge.
		provider := &scriptedChatProvider{responses: []string{validStructuredJSON}}
		analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}

		out := runpkg.RunJEV(context.Background(), analyzer, runpkg.JEVInvocation{Task: "early triage"})
		if out.Result.StructuredEvidence == nil {
			t.Fatal("production analyzer produced no structured evidence for early routing")
		}
		ev := *out.Result.StructuredEvidence
		if err := ev.Validate(); err != nil {
			t.Fatalf("early evidence must validate: %v", err)
		}

		// A HIGH security finding is blocking under fail_on=[high], so the
		// deterministic disposition requires a human boundary.
		decision := autonomy.DecideEarly(ev, []string{"high"}, autonomy.Policy{})
		if !decision.RequiresHuman {
			t.Errorf("a HIGH security finding must require a human; got %+v", decision)
		}

		// Determinism: the same inputs yield the same disposition.
		again := autonomy.DecideEarly(ev, []string{"high"}, autonomy.Policy{})
		if again != decision {
			t.Errorf("DecideEarly is not deterministic: %+v vs %+v", decision, again)
		}

		// And a clean result does not escalate.
		provider2 := &scriptedChatProvider{responses: []string{cleanStructuredJSON}}
		analyzer2, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider2)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}
		out2 := runpkg.RunJEV(context.Background(), analyzer2, runpkg.JEVInvocation{Task: "early triage clean"})
		if out2.Result.StructuredEvidence == nil {
			t.Fatal("clean run produced no structured evidence")
		}
		decision2 := autonomy.DecideEarly(*out2.Result.StructuredEvidence, []string{"high"}, autonomy.Policy{})
		if decision2.RequiresHuman {
			t.Errorf("a clean result must not escalate; got %+v", decision2)
		}
	})
}

// TestJEVProductionAnalyzerRequiresValidConfig pins that the production
// constructor fails closed with CONFIG_INVALID when it cannot build, so the
// integration test's hermetic config is genuinely required.
func TestJEVProductionAnalyzerRequiresValidConfig(t *testing.T) {
	if _, err := jev.NewOllamaAnalyzer(ollamaagent.Config{}, &scriptedChatProvider{}); err == nil {
		t.Fatal("NewOllamaAnalyzer must reject a missing base URL/model/timeout")
	}
	var pe *jev.ProviderError
	if _, err := jev.NewOllamaAnalyzer(ollamaagent.Config{}, &scriptedChatProvider{}); !errors.As(err, &pe) || pe.Kind != jev.ErrConfigInvalid {
		t.Fatalf("expected CONFIG_INVALID, got %v", err)
	}
}

// TestJEVProductionGateEvidenceShapesStructuredResult pins that the production
// analyzer's structured result is what the gate-shaped evidence is built from.
func TestJEVProductionGateEvidenceShapesStructuredResult(t *testing.T) {
	provider := &scriptedChatProvider{responses: []string{cleanStructuredJSON}}
	analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	out := runpkg.RunJEV(context.Background(), analyzer, runpkg.JEVInvocation{Task: "gate evidence"})
	gate := jevEvidence(out)
	if gate == nil {
		t.Fatal("a ran outcome must yield gate evidence")
	}
	if gate.FailClosed {
		t.Fatalf("a CLEAN result must not be fail-closed: %+v", gate)
	}
	// quality.NewJEVEvidence is the shaping function the CLI uses.
	_ = quality.NewJEVEvidence(out.Result)
}

// TestJEVProductionAnalyzerRouterIntegration drives the REAL production analyzer
// end to end into the ACTUAL router signal path: jev.NewOllamaAnalyzer ->
// runpkg.RunJEV -> validated Result.StructuredEvidence -> evidenceFrom ->
// router.SignalsFrom -> router.Decide. It pins the security-family HIGH model-class
// floor and deterministic routing, all through the in-process scriptedChatProvider
// (no Claude, no live Ollama server; JEV-205 Worker A).
func TestJEVProductionAnalyzerRouterIntegration(t *testing.T) {
	// securityLowJSON is a schema-valid response whose only finding is a
	// security-category concern stated at LOW severity. The router must still floor
	// it to HIGH risk (LARGE), because the security-family categories never route
	// below the conservative floor (internal/router/router.go securityFamily).
	const securityLowJSON = `{"status":"FINDINGS","summary":"security concern","findings":[{"id":"f1","severity":"LOW","category":"security","path":"internal/x.go","line":7,"message":"credential handled","evidence":"log.Println(token)"}]}`

	// nonSecurityLowJSON is the contrast: a LOW-severity finding with no
	// security-family category. It is NOT floored, so the stated severity is what
	// applies — which is what makes the security floor meaningful rather than an
	// artifact of the severity.
	const nonSecurityLowJSON = `{"status":"FINDINGS","summary":"minor note","findings":[{"id":"f1","severity":"LOW","category":"","path":"internal/x.go","line":3,"message":"minor note","evidence":"n/a"}]}`

	// analyzeAndRoute runs the REAL analyzer over resp through runpkg.RunJEV and
	// returns the fail-closed-checked outcome carrying validated structured evidence.
	analyzeAndRoute := func(t *testing.T, resp string) runpkg.JEVOutcome {
		t.Helper()
		provider := &scriptedChatProvider{responses: []string{resp}}
		analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}
		out := runpkg.RunJEV(context.Background(), analyzer, runpkg.JEVInvocation{
			Task:         "security-sensitive task",
			ChangedFiles: []string{"internal/x.go"},
		})
		if out.FailClosed() {
			t.Fatalf("valid output must not fail closed: err=%v", out.Error)
		}
		if out.Result.StructuredEvidence == nil {
			t.Fatal("the production analyzer did not attach structured evidence")
		}
		if err := out.Result.StructuredEvidence.Validate(); err != nil {
			t.Fatalf("attached evidence must validate: %v", err)
		}
		return out
	}

	// route folds a ran checkpoint's evidence through the SAME seam routing.go uses:
	// evidenceFrom -> router.SignalsFrom -> Signals.Merge -> router.Decide.
	route := func(out runpkg.JEVOutcome, task router.TaskSignals) (router.Signals, router.Decision) {
		gate := earlyGateResult{Ran: true, Provider: out.Provider, Evidence: *out.Result.StructuredEvidence}
		sig := router.SignalsFrom(nil, task)
		ev := evidenceFrom(gate)
		if ev == nil {
			t.Fatal("evidenceFrom dropped valid checkpoint evidence")
		}
		sig = sig.Merge(router.SignalsFrom(ev, task))
		return sig, router.Decide(sig)
	}

	t.Run("security category enforces the HIGH model-class floor", func(t *testing.T) {
		out := analyzeAndRoute(t, securityLowJSON)
		items := out.Result.StructuredEvidence.Items
		if len(items) != 1 {
			t.Fatalf("evidence items = %d, want 1", len(items))
		}
		// The floor below is driven by the analyzer's typed mapping, not by
		// hand-built evidence: pin the category and the (LOW) stated severity first,
		// so the escalation can only come from the security family.
		if items[0].Category != jev.CategorySecurity {
			t.Fatalf("analyzer category = %q, want %q", items[0].Category, jev.CategorySecurity)
		}
		if items[0].Severity != jev.SeverityLow {
			t.Fatalf("analyzer severity = %q, want %q (the floor must not rely on the stated severity)", items[0].Severity, jev.SeverityLow)
		}

		sig, dec := route(out, router.TaskSignals{})
		if sig.Risk != router.LevelHigh {
			t.Errorf("routing risk = %q, want %q (security-family HIGH floor)", sig.Risk, router.LevelHigh)
		}
		if dec.Class != model.ClassLarge {
			t.Errorf("routing class = %q, want %q (HIGH risk floor)", dec.Class, model.ClassLarge)
		}
		if !strings.Contains(router.ReasonsText(dec.Reasons), router.ReasonRiskHigh) {
			t.Errorf("routing reasons = %v, want them to include %q", dec.Reasons, router.ReasonRiskHigh)
		}
	})

	t.Run("LOW severity without a security category is not floored", func(t *testing.T) {
		sig, dec := route(analyzeAndRoute(t, nonSecurityLowJSON), router.TaskSignals{})
		if sig.Risk != router.LevelLow {
			t.Errorf("routing risk = %q, want %q (no security-family floor)", sig.Risk, router.LevelLow)
		}
		if dec.Class != model.ClassSmall {
			t.Errorf("routing class = %q, want %q (contrast with the security floor)", dec.Class, model.ClassSmall)
		}
	})

	t.Run("identical inputs route deterministically", func(t *testing.T) {
		sig1, dec1 := route(analyzeAndRoute(t, securityLowJSON), router.TaskSignals{})
		sig2, dec2 := route(analyzeAndRoute(t, securityLowJSON), router.TaskSignals{})
		if sig1 != sig2 {
			t.Errorf("router signals are not deterministic: %+v vs %+v", sig1, sig2)
		}
		if dec1.Class != dec2.Class {
			t.Errorf("router class is not deterministic: %q vs %q", dec1.Class, dec2.Class)
		}
		if router.ReasonsText(dec1.Reasons) != router.ReasonsText(dec2.Reasons) {
			t.Errorf("router reasons are not deterministic: %v vs %v", dec1.Reasons, dec2.Reasons)
		}
	})
}

// failureChatProvider is a minimal in-process jev.ChatProvider for the
// failure-path tests. It never touches the network: it either returns a fixed
// error from every chat call, or blocks until the call context is done and then
// surfaces the context error, exactly as a real transport reports a cancellation
// or an elapsed deadline.
type failureChatProvider struct {
	err   error
	block bool
}

func (p *failureChatProvider) Chat(ctx context.Context, _, _ string) (string, error) {
	return p.do(ctx)
}

func (p *failureChatProvider) ChatStructured(ctx context.Context, _, _ string, _ json.RawMessage) (string, error) {
	return p.do(ctx)
}

func (p *failureChatProvider) do(ctx context.Context) (string, error) {
	if p.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if p.err != nil {
		return "", p.err
	}
	return "", errors.New("failureChatProvider: no failure configured")
}

// TestJEVProductionAnalyzerFailurePaths covers the failure paths of the REAL
// production analyzer (jev.NewOllamaAnalyzer -> Analyze): a provider error, a
// caller cancellation, and an elapsed deadline. For each injected in-process
// failure (no Ollama) it asserts the analyzer and the CLI seam fail closed, a
// zero Result is returned, no continuation is authorized, and the persisted
// fail-closed reason is class-only -- never the raw provider error, endpoint,
// body, or token.
func TestJEVProductionAnalyzerFailurePaths(t *testing.T) {
	const (
		sentinelEndpoint = "http://sentinel-endpoint.invalid:11434/api/chat"
		sentinelBody     = "SENTINEL_BODY_do_not_persist"
		sentinelToken    = "SENTINEL_TOKEN_do_not_persist_9f3c"
	)

	// assertFailClosed proves one injected failure is fail-closed end to end: the
	// outcome carries a zero Result, the mapped gate evidence is fail-closed and
	// names only the bounded failure class, and no FIX payload is authorized.
	assertFailClosed := func(t *testing.T, out runpkg.JEVOutcome, wantClass jev.ProviderErrorKind) {
		t.Helper()
		if !out.FailClosed() {
			t.Fatalf("failure must yield a fail-closed outcome: err=%v", out.Error)
		}
		var zero jev.Result
		if out.Result.Status != zero.Status || len(out.Result.Findings) != 0 || out.Result.StructuredEvidence != nil {
			t.Fatalf("fail-closed must carry a zero Result, got %+v", out.Result)
		}

		gate := jevEvidence(out)
		if gate == nil || !gate.FailClosed {
			t.Fatalf("failure must yield fail-closed gate evidence; got %+v", gate)
		}
		if !strings.Contains(gate.Reason, string(wantClass)) {
			t.Errorf("fail-closed reason = %q, want bounded class %q", gate.Reason, wantClass)
		}
		for _, sentinel := range []string{sentinelEndpoint, sentinelBody, sentinelToken} {
			if strings.Contains(gate.Reason, sentinel) {
				t.Errorf("fail-closed reason leaked %q: %q", sentinel, gate.Reason)
			}
		}

		// The CLI seam refuses to project any FIX-bound payload for a fail-closed
		// JEV, so it can never authorize continuation.
		ev := &jevRunEvidence{Evidence: gate, Result: out.Result}
		if payloads := ev.fixPayloads(); len(payloads) != 0 {
			t.Errorf("fail-closed JEV must authorize no FIX payloads, got %d", len(payloads))
		}
	}

	t.Run("provider failure fails closed", func(t *testing.T) {
		providerErr := fmt.Errorf("ollama chat failed: dial %s: reply %s token=%s", sentinelEndpoint, sentinelBody, sentinelToken)
		provider := &failureChatProvider{err: providerErr}
		analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}

		res, err := analyzer.Analyze(context.Background(), jev.Request{Task: "provider failure"})
		if err == nil {
			t.Fatalf("a provider failure must fail closed; got result %+v", res)
		}
		if res.StructuredEvidence != nil {
			t.Errorf("a provider failure must never attach evidence: %+v", res.StructuredEvidence)
		}
		var pe *jev.ProviderError
		if !errors.As(err, &pe) || pe.Kind != jev.ErrProviderUnreachable {
			t.Fatalf("provider failure class = %v, want %q", err, jev.ErrProviderUnreachable)
		}

		out := runpkg.RunJEV(context.Background(), analyzer, runpkg.JEVInvocation{Task: "provider failure"})
		assertFailClosed(t, out, jev.ErrProviderUnreachable)

		// Through the full CLI lifecycle the same failure must not pass and must
		// never authorize a FIX.
		dir := t.TempDir()
		writeFile(t, dir, "TASK.md", runTaskFile)
		writeConfig(t, dir, jevEnabledConfig)
		a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
		code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
			newProductionAnalyzer(t, provider), "run", "--task", "TASK.md")
		if code == exitOK {
			t.Fatalf("a fail-closed production JEV must not pass; stdout=%s stderr=%s", stdout, stderr)
		}
		if len(a.fixInputs) != 0 {
			t.Errorf("fail-closed JEV must not authorize FIX payloads; got %d", len(a.fixInputs))
		}
	})

	t.Run("context cancellation fails closed", func(t *testing.T) {
		provider := &failureChatProvider{block: true}
		analyzer, err := jev.NewOllamaAnalyzer(productionJEVConfig(), provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}

		// Cancel the caller context while the provider call is in flight.
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		res, err := analyzer.Analyze(ctx, jev.Request{Task: "canceled"})
		if err == nil {
			t.Fatalf("a canceled context must fail closed; got result %+v", res)
		}
		if res.StructuredEvidence != nil {
			t.Errorf("a canceled context must never attach evidence: %+v", res.StructuredEvidence)
		}
		var pe *jev.ProviderError
		if !errors.As(err, &pe) || pe.Kind != jev.ErrCanceled {
			t.Fatalf("cancellation class = %v, want %q", err, jev.ErrCanceled)
		}

		out := runpkg.RunJEV(ctx, analyzer, runpkg.JEVInvocation{Task: "canceled"})
		assertFailClosed(t, out, jev.ErrCanceled)
	})

	t.Run("deadline exceeded fails closed", func(t *testing.T) {
		// A short timeout makes the analyzer's own bounded context elapse, so the
		// blocking provider surfaces context.DeadlineExceeded.
		cfg := ollamaagent.Config{
			BaseURL: "http://127.0.0.1:11434",
			Model:   "test-model",
			Timeout: 50 * time.Millisecond,
		}
		provider := &failureChatProvider{block: true}
		analyzer, err := jev.NewOllamaAnalyzer(cfg, provider)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}

		res, err := analyzer.Analyze(context.Background(), jev.Request{Task: "deadline"})
		if err == nil {
			t.Fatalf("an elapsed deadline must fail closed; got result %+v", res)
		}
		if res.StructuredEvidence != nil {
			t.Errorf("an elapsed deadline must never attach evidence: %+v", res.StructuredEvidence)
		}
		var pe *jev.ProviderError
		if !errors.As(err, &pe) || pe.Kind != jev.ErrTimeout {
			t.Fatalf("deadline class = %v, want %q", err, jev.ErrTimeout)
		}

		out := runpkg.RunJEV(context.Background(), analyzer, runpkg.JEVInvocation{Task: "deadline"})
		assertFailClosed(t, out, jev.ErrTimeout)
	})
}
