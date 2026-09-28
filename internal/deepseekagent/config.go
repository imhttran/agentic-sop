// Package deepseekagent is a minimal, temporary coding-agent harness that lets
// SOP's command provider use a local Ollama model
// (deepseek-v4.1-flash:cloud) as its implementation agent.
//
// It is an implementation adapter, not a workflow authority. It reads and writes
// repository files and runs bounded, allow-listed commands through controlled
// tools, then reports a structured outcome. SOP keeps ownership of task state,
// dependencies, retries, validation, review, quality gates, and human approval;
// this harness never touches .agent-sdlc state and never commits, pushes, or
// rewrites history.
//
// The whole package is intentionally self-contained: it exists only so an
// existing command-agent path can drive a plan, and is expected to be superseded
// by the Agent Harness V2 work.
package deepseekagent

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Defaults for the bootstrap harness. The model settings (base URL, model,
// timeout) are configurable through the shared SOP_OLLAMA_* variables; the loop
// bounds are fixed constants so the agent cannot run away.
const (
	// DefaultModel is the model this bootstrap harness targets.
	DefaultModel = "deepseek-v4.1-flash:cloud"
	// DefaultBaseURL is Ollama's default local address.
	DefaultBaseURL = "http://127.0.0.1:11434"

	defaultTimeout        = 120 * time.Second
	defaultCommandTimeout = 120 * time.Second
	defaultMaxIterations  = 24
	defaultMaxToolCalls   = 40
	defaultMaxOutputBytes = 256 << 10 // 256 KiB of tool output kept per call

	maxSearchMatches     = 200
	maxListEntries       = 500
	maxSearchFileBytes   = 512 << 10
	maxHTTPResponseBytes = 1 << 20
)

// Config is the harness's settings.
type Config struct {
	// BaseURL and Model identify the Ollama endpoint and model.
	BaseURL string
	Model   string
	// Timeout bounds a single model/API call.
	Timeout time.Duration
	// MaxIterations bounds model turns; MaxToolCalls bounds tool executions.
	MaxIterations int
	MaxToolCalls  int
	// CommandTimeout bounds a single run_command execution.
	CommandTimeout time.Duration
	// MaxOutputBytes bounds the output kept from a tool call.
	MaxOutputBytes int
}

// ConfigFromEnv reads the harness configuration from the environment. It reuses
// the same SOP_OLLAMA_* variables the Ollama provider uses, so a project already
// configured for Ollama needs no extra setup.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:        DefaultBaseURL,
		Model:          DefaultModel,
		Timeout:        defaultTimeout,
		MaxIterations:  defaultMaxIterations,
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
	return cfg, nil
}
