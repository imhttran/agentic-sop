// Package caveman is an optional handoff.Compressor adapter that delegates
// compression to a configured command. It is never mandatory: when the command
// is unavailable or unhealthy, handoff.Select falls back to the no-op compressor
// (auto) or skips compression and preserves the capsule (caveman), so task
// completion never depends on Caveman.
package caveman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/handoff"
)

// Runner executes a configured command, sending input on stdin and returning
// stdout. Commands are trusted project configuration (like a Makefile or
// SOP_AGENT_COMMAND); they are never built from task or model content.
type Runner interface {
	Run(ctx context.Context, command, input string) (string, error)
}

// Adapter implements handoff.Compressor and handoff.HealthChecker over Caveman.
type Adapter struct {
	healthCommand   string
	compressCommand string
	runner          Runner
}

// New returns an Adapter. An empty command disables the corresponding
// operation (health or compress).
func New(healthCommand, compressCommand string, runner Runner) *Adapter {
	return &Adapter{healthCommand: healthCommand, compressCommand: compressCommand, runner: runner}
}

// Name identifies the adapter in diagnostics.
func (a *Adapter) Name() string { return "Caveman" }

// Check performs a cheap, non-destructive readiness check: it requires a
// configured health command and runs it. It never compresses real task content.
func (a *Adapter) Check(ctx context.Context) error {
	if strings.TrimSpace(a.healthCommand) == "" {
		return errors.New("caveman: health command is not configured")
	}
	if a.runner == nil {
		return errors.New("caveman: no command runner configured")
	}
	if _, err := a.runner.Run(ctx, a.healthCommand, ""); err != nil {
		return fmt.Errorf("caveman: health check failed: %w", err)
	}
	return nil
}

// Compress sends the bundle as JSON to the configured compress command and parses
// the response, which may be a handoff.CompressedContext JSON object or plain
// text content.
func (a *Adapter) Compress(ctx context.Context, input handoff.ContextBundle) (handoff.CompressedContext, error) {
	if strings.TrimSpace(a.compressCommand) == "" {
		return handoff.CompressedContext{}, errors.New("caveman: compress command is not configured")
	}
	if a.runner == nil {
		return handoff.CompressedContext{}, errors.New("caveman: no command runner configured")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return handoff.CompressedContext{}, fmt.Errorf("caveman: marshal bundle: %w", err)
	}
	out, err := a.runner.Run(ctx, a.compressCommand, string(payload))
	if err != nil {
		return handoff.CompressedContext{}, err
	}
	var compressed handoff.CompressedContext
	if err := json.Unmarshal([]byte(out), &compressed); err == nil && compressed.Content != "" {
		return compressed, nil
	}
	return handoff.CompressedContext{Content: strings.TrimSpace(out)}, nil
}

// ShellRunner runs commands with `sh -c`, sending input on stdin and capturing
// stdout. Commands are trusted configuration, never untrusted input.
type ShellRunner struct{}

// Run executes command with input on stdin.
func (ShellRunner) Run(ctx context.Context, command, input string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", fmt.Errorf("caveman: command failed: %w: %s", err, detail)
		}
		return "", fmt.Errorf("caveman: command failed: %w", err)
	}
	return stdout.String(), nil
}
