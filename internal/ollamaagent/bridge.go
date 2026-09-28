package ollamaagent

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
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
	cfg  Config
	root string
	diag io.Writer
}

// NewNativeAgent returns a NativeAgent rooted at root. Diagnostics (the failed
// invocation's trace and audit summary) go to diag; a nil diag discards them.
func NewNativeAgent(cfg Config, root string, diag io.Writer) *NativeAgent {
	if diag == nil {
		diag = io.Discard
	}
	return &NativeAgent{cfg: cfg, root: root, diag: diag}
}

// NativeAgentFromEnv builds the native agent from the environment and the
// configured model. Model precedence is SOP_AGENT_MODEL, then SOP_OLLAMA_MODEL,
// then the configured model, then the built-in default — the same precedence
// the rest of the stack documents; no model is hard-wired here. The repository
// root is the process working directory (SOP runs in the project root).
func NativeAgentFromEnv(configuredModel string) (*NativeAgent, error) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
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

	// One Harness per invocation: nothing crosses invocation boundaries.
	h := New(n.cfg, n.root)
	content, _, err := h.Complete(ctx, req)
	if err != nil {
		h.FlushAudit(n.diag)
		h.FlushTrace(n.diag)
		if wantsOutcome(req.Capability) {
			status := agent.OutcomeFailed
			var incomplete *changeIncompleteError
			if errors.As(err, &incomplete) {
				status = agent.OutcomeNeedsHuman
			}
			return agent.Response{
				Content: "\"" + err.Error() + "\"",
				Outcome: &agent.Outcome{Status: status, Reason: err.Error()},
			}, nil
		}
		return agent.Response{}, err
	}
	return agent.Response{Content: content, Outcome: agent.ParseOutcome(content)}, nil
}

// compile-time assertion: the native agent is a full-capability SOP agent.
var _ agent.Agent = (*NativeAgent)(nil)
var _ agent.Declarer = (*NativeAgent)(nil)
