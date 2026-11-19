package ollamaagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/redact"
)

// NativeAgent is the in-process form of the Ollama tool harness: it satisfies
// agent.Agent, so SOP's lifecycle drives the controlled tool loop directly,
// without a subprocess command agent. It is the intended native execution path
// (harness: tool, provider: ollama). The installed-binary command path
// (cmd/sop-ollama-agent) stays available for projects that deliberately run the
// agent as a separate process.
//
// Every Generate call constructs a fresh Harness, so invocation-scoped state —
// mutation evidence, phase, no-progress fingerprints, the working-tree
// baseline — can never cross from one invocation into another.
type NativeAgent struct {
	cfg       Config
	root      string
	diag      io.Writer
	newClient func(Config) chatClient
}

// NewNativeAgent returns a NativeAgent rooted at root. Diagnostics (the failed
// invocation's trace and audit summary) go to diag; a nil diag discards them.
func NewNativeAgent(cfg Config, root string, diag io.Writer) *NativeAgent {
	return newNativeAgent(cfg, root, diag, func(c Config) chatClient { return newOllamaClient(c) })
}

// NewNativeOMLXAgent returns the governed oMLX variant: the same bounded tool
// loop, driven by the OpenAI-compatible chat transport that oMLX serves. It is
// the only oMLX path that advertises IMPLEMENT/FIX, because it is the only one
// that can execute governed edits and ground a structured outcome in observed
// repository change.
func NewNativeOMLXAgent(cfg Config, root string, diag io.Writer) *NativeAgent {
	return newNativeAgent(cfg, root, diag, func(c Config) chatClient { return newOpenAIChatClient(c) })
}

func newNativeAgent(cfg Config, root string, diag io.Writer, newClient func(Config) chatClient) *NativeAgent {
	if diag == nil {
		diag = io.Discard
	}
	return &NativeAgent{cfg: cfg, root: root, diag: diag, newClient: newClient}
}

// NativeAgentFromEnv builds the native agent from the environment and the
// configured model. Model precedence is SOP_AGENT_MODEL, then SOP_OLLAMA_MODEL,
// then the configured model, then the built-in default — the same precedence
// the rest of the stack documents; no model is hard-wired here. The repository
// root is the process working directory (SOP runs in the project root).
// cargoAgentToolEnabled is the project's independent cargo.agent_tool permission;
// it is passed through to the shared tool harness unchanged.
func NativeAgentFromEnv(configuredModel string, cargoAgentToolEnabled bool) (*NativeAgent, error) {
	return NativeAgentFromEnvWithPolicy(configuredModel, cargoAgentToolEnabled, redact.Policy{})
}

// NativeAgentFromEnvWithPolicy is NativeAgentFromEnv with the loaded LLM02 C1
// model-input policy. The composition root resolves the project configuration
// and external inventory; the agent layer receives only the resulting policy.
func NativeAgentFromEnvWithPolicy(configuredModel string, cargoAgentToolEnabled bool, policy redact.Policy) (*NativeAgent, error) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	cfg.CargoAgentToolEnabled = cargoAgentToolEnabled
	cfg.Redaction = policy
	if model := strings.TrimSpace(os.Getenv(agent.EnvAgentModel)); model != "" {
		cfg.Model = model
	} else if strings.TrimSpace(os.Getenv(agent.EnvOllamaModel)) == "" {
		if model := strings.TrimSpace(configuredModel); model != "" {
			cfg.Model = model
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return NewNativeAgent(cfg, root, os.Stderr), nil
}

// NativeOMLXAgentFromEnvWithPolicy builds the governed oMLX agent. It reuses the
// shared harness configuration (budgets, command policy, output bounds) and
// replaces only the endpoint, model, timeout, and identity with the oMLX values.
// The resulting agent advertises the full capability set because it executes
// through the same controlled tool loop as the native Ollama harness.
func NativeOMLXAgentFromEnvWithPolicy(configuredModel string, cargoAgentToolEnabled bool, policy redact.Policy) (*NativeAgent, error) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	cfg.Provider = "mlx"
	cfg.ModelEnv = agent.EnvMLXModel
	cfg.CargoAgentToolEnabled = cargoAgentToolEnabled
	cfg.Redaction = policy
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv(agent.EnvMLXBaseURL)), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultOMLXBaseURL
	}
	cfg.APIKey = strings.TrimSpace(os.Getenv(agent.EnvMLXAPIKey))
	d, derr := agent.MLXTimeoutFromEnv()
	if derr != nil {
		return nil, derr
	}
	cfg.Timeout = d
	toggle, err := agent.MLXThinkingEnabled()
	if err != nil {
		return nil, err
	}
	cfg.EnableThinking = toggle
	// Use the shared resolver so the governed path and the text-only MLX path
	// apply the same documented environment > configuration precedence.
	cfg.Model = agent.ProviderModelFromEnv(agent.ProviderMLX, configuredModel)
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("mlx: model is required (set %s or agent.model)", agent.EnvMLXModel)
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return NewNativeOMLXAgent(cfg, root, os.Stderr), nil
}

// Capabilities declares the full set: the tool loop can plan, review, and —
// through the controlled tools — mutate the repository for IMPLEMENT and FIX.
func (n *NativeAgent) Capabilities() agent.Capabilities { return agent.AllCapabilities() }

// Generate runs one capability invocation in-process and maps the harness result
// onto the same Response shape the command provider produces, including the
// parsed structured outcome for mutating capabilities. A harness failure for a
// mutating capability is reported as a structured outcome (needs_human when the
// invocation changed nothing, so SOP requeues rather than blocks); a PLAN/REVIEW
// failure is an error, because those capabilities return schema documents.
func (n *NativeAgent) Generate(ctx context.Context, req agent.Request) (agent.Response, error) {
	if err := req.Validate(); err != nil {
		return agent.Response{}, err
	}

	// One Harness per invocation: nothing crosses invocation boundaries. The
	// mutation-evidence accumulator is owned here so the repository paths this
	// invocation changed can be returned as authoritative task change evidence.
	h := NewWithClient(n.cfg, n.root, n.newClient(n.cfg))
	ev := &mutationEvidence{}
	content, _, err := h.CompleteWithEvidence(ctx, req, ev)
	if err != nil {
		h.FlushAudit(n.diag)
		h.FlushTrace(n.diag)
		// A provider-reported truncation is preserved on the Response and the
		// structured outcome. The model's content (when any) is incomplete, so it
		// can never become a successful completion; the caller's existing bounded
		// classification decides what happens next.
		var truncated *agent.TruncationError
		isTruncated := errors.As(err, &truncated)
		if isTruncated && truncated.Capability == "" {
			truncated.Capability = req.Capability
		}
		if wantsOutcome(req.Capability) {
			status := agent.OutcomeFailed
			var incomplete *changeIncompleteError
			if errors.As(err, &incomplete) {
				status = agent.OutcomeNeedsHuman
			}
			return agent.Response{
				Content:      "\"" + err.Error() + "\"",
				Outcome:      &agent.Outcome{Status: status, Reason: err.Error()},
				ChangedFiles: ev.reportedPaths(),
				Truncated:    isTruncated,
			}, nil
		}
		return agent.Response{Truncated: isTruncated}, err
	}
	return agent.Response{Content: content, Outcome: agent.ParseOutcome(content), ChangedFiles: ev.reportedPaths(), VerifiedAlreadySatisfied: ev.alreadySatisfied}, nil
}

// compile-time assertion: the native agent is a full-capability SOP agent.
var _ agent.Agent = (*NativeAgent)(nil)
var _ agent.Declarer = (*NativeAgent)(nil)
