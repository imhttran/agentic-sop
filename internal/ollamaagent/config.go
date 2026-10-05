// Package ollamaagent is a minimal, temporary coding-agent harness that lets
// SOP's command provider use a local Ollama model
// (deepseek-v4.1-flash:cloud) as its implementation agent.
//
// It is an implementation adapter, not a workflow authority. It reads and writes
// repository files and runs bounded, allow-listed commands through the shared
// toolharness controlled tools, then reports a structured outcome. SOP keeps
// ownership of task state, dependencies, retries, validation, review, quality
// gates, and human approval; this harness never touches .agent-sdlc/state.db and
// never commits, pushes, or rewrites history.
//
// The whole package is intentionally narrow: it exists only so an existing
// command-agent path can drive a plan, and is expected to be superseded by the
// Agent Harness V2 work. All repository access and command policy live in
// internal/toolharness, so there is one policy implementation, not two.
package ollamaagent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// Defaults for the bootstrap harness. The model settings (base URL, model,
// timeout) are configurable through the shared SOP_OLLAMA_* variables; the
// per-capability loop bounds live in PolicyFor, not in configuration, so the
// agent cannot run away and no single number governs every capability.
const (
	// DefaultModel is the model this bootstrap harness targets.
	DefaultModel = "deepseek-v4.1-flash:cloud"
	// DefaultBaseURL is Ollama's default local address.
	DefaultBaseURL = "http://127.0.0.1:11434"

	defaultTimeout        = 120 * time.Second
	defaultCommandTimeout = 120 * time.Second
	// defaultMaxToolCalls is a secondary bound on tool executions, independent of
	// the per-capability iteration budget.
	defaultMaxToolCalls   = 80
	defaultMaxOutputBytes = 256 << 10 // 256 KiB of tool output kept per call

	// defaultMaxAuditRecords bounds the in-memory audit trail kept per run.
	defaultMaxAuditRecords = 2048
	// defaultMaxTraceRecords bounds the in-memory per-turn trace kept per run.
	defaultMaxTraceRecords = 2048

	maxHTTPResponseBytes = 1 << 20

	// envToolAuditLog optionally names a file to append the durable tool audit
	// trail to. It is set by the operator and must not be SOP's state database:
	// the file is opened separately, so inspecting the audit never touches
	// .agent-sdlc/state.db.
	envToolAuditLog = "SOP_TOOL_AUDIT_LOG"
	// envToolTraceLog optionally names a file to append the per-turn trace to on a
	// failed run. SOP's command provider discards the harness's stderr, so without
	// this the turn history that explains a failure is lost; setting it gives every
	// failure a durable, inspectable trail. Like the audit sink it is operator-set
	// and never SOP's state database.
	envToolTraceLog = "SOP_OLLAMA_TRACE_LOG"
	// envWorkspaceRoots optionally declares additional task-authorized repository
	// roots as a JSON array of {"path":"<abs>","mode":"read"|"read-write"}. It is
	// trusted operator input, not model output: the model can only select an
	// authorized root by its exact canonical path.
	envWorkspaceRoots = "SOP_WORKSPACE_ROOTS"
)

// lookupEnv is indirected so tests can supply environment values without
// touching the process environment.
var lookupEnv = os.Getenv

// Config is the harness's settings.
type Config struct {
	// BaseURL and Model identify the Ollama endpoint and model.
	BaseURL string
	Model   string
	// Timeout bounds a single model/API call.
	Timeout time.Duration
	// MaxToolCalls bounds tool executions across the whole run.
	MaxToolCalls int
	// CommandTimeout bounds a single run_command execution.
	CommandTimeout time.Duration
	// MaxOutputBytes bounds the output kept from a tool call.
	MaxOutputBytes int
	// Roots are additional task-authorized repository roots (read-only unless a
	// root is explicitly read-write). They come from trusted configuration/input.
	Roots []toolharness.Root
}

// ConfigFromEnv reads the harness configuration from the environment. It reuses
// the same SOP_OLLAMA_* variables the Ollama provider uses, so a project already
// configured for Ollama needs no extra setup.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:        DefaultBaseURL,
		Model:          DefaultModel,
		Timeout:        defaultTimeout,
		MaxToolCalls:   defaultMaxToolCalls,
		CommandTimeout: defaultCommandTimeout,
		MaxOutputBytes: defaultMaxOutputBytes,
	}
	if v := strings.TrimSpace(os.Getenv(agent.EnvOllamaBaseURL)); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv(agent.EnvOllamaModel)); v != "" {
		cfg.Model = v
	}
	if v := strings.TrimSpace(os.Getenv(agent.EnvOllamaTimeout)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("invalid %s %q: want a positive duration (for example 2m)", agent.EnvOllamaTimeout, v)
		}
		cfg.Timeout = d
	}
	if v := strings.TrimSpace(os.Getenv(envWorkspaceRoots)); v != "" {
		roots, err := parseWorkspaceRoots(v)
		if err != nil {
			return Config{}, err
		}
		cfg.Roots = roots
	}
	return cfg, nil
}

// parseWorkspaceRoots parses envWorkspaceRoots. An unrecognized mode defaults to
// read-only, so a malformed entry can only narrow access, never widen it.
func parseWorkspaceRoots(v string) ([]toolharness.Root, error) {
	var raw []struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(v), &raw); err != nil {
		return nil, fmt.Errorf("invalid %s: want a JSON array of {\"path\":\"<abs>\",\"mode\":\"read\"|\"read-write\"}", envWorkspaceRoots)
	}
	var roots []toolharness.Root
	for _, r := range raw {
		path := strings.TrimSpace(r.Path)
		if path == "" {
			continue
		}
		mode := toolharness.RootMode(strings.ToLower(strings.TrimSpace(r.Mode)))
		if mode != toolharness.RootReadWrite {
			mode = toolharness.RootReadOnly
		}
		roots = append(roots, toolharness.Root{Path: path, Mode: mode})
	}
	return roots, nil
}

// configSource determines whether a configuration value came from an environment
// variable or is a hardcoded default.
func configSource(envVar string) string {
	if strings.TrimSpace(os.Getenv(envVar)) != "" {
		return "environment"
	}
	return "configuration"
}
