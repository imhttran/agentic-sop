package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EnvAgentCommand names the environment variable that configures the
// subprocess-backed agent. Its value is a shell command that reads a JSON
// Request on stdin and writes the model's raw output to stdout.
const EnvAgentCommand = "SOP_AGENT_COMMAND"

// legacyEnvAgentCommand is the pre-rename variable name, still accepted so
// existing configurations keep working.
const legacyEnvAgentCommand = "AGENT_SDLC_AGENT_COMMAND"

// CommandAgent is an Agent backed by an external command. It keeps the model
// provider out of the application: any harness that speaks the stdin/stdout
// contract can be configured through the environment.
type CommandAgent struct {
	command string
}

// NewCommandAgent returns a CommandAgent that runs command via the shell.
func NewCommandAgent(command string) *CommandAgent {
	return &CommandAgent{command: command}
}

// Capabilities declares that the command agent serves every capability.
func (a *CommandAgent) Capabilities() Capabilities { return AllCapabilities() }

// NewCommandAgentFromEnv builds a CommandAgent from EnvAgentCommand, falling
// back to the legacy variable name, and returns an error when neither is set.
func NewCommandAgentFromEnv() (Agent, error) {
	command := strings.TrimSpace(os.Getenv(EnvAgentCommand))
	if command == "" {
		command = strings.TrimSpace(os.Getenv(legacyEnvAgentCommand))
	}
	if command == "" {
		return nil, fmt.Errorf("no agent configured: set %s", EnvAgentCommand)
	}
	return NewCommandAgent(command), nil
}

// Generate validates the request, serialises it as JSON to the command's
// stdin, and returns the command's stdout as the response content. Context
// cancellation terminates the command and returns a distinguishable error.
func (a *CommandAgent) Generate(ctx context.Context, request Request) (Response, error) {
	if err := request.Validate(); err != nil {
		return Response{}, err
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return Response{}, fmt.Errorf("agent %s: encode request: %w", request.Capability, err)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", a.command)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Response{}, fmt.Errorf("agent %s canceled: %w", request.Capability, ctxErr)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return Response{}, fmt.Errorf("agent %s failed: %w: %s", request.Capability, err, detail)
		}
		return Response{}, fmt.Errorf("agent %s failed: %w", request.Capability, err)
	}

	// stderr is diagnostic only and is never merged into successful content.
	content := stdout.String()
	if strings.TrimSpace(content) == "" {
		return Response{}, fmt.Errorf("agent %s returned empty output", request.Capability)
	}

	return Response{Content: content, Outcome: parseOutcome(content)}, nil
}

// outcomeWire mirrors the structured execution outcome a command agent may return
// for a mutating capability.
type outcomeWire struct {
	Status          string `json:"status"`
	Summary         string `json:"summary"`
	Reason          string `json:"reason"`
	ChangesExpected *bool  `json:"changes_expected"`
}

// parseOutcome recognizes a structured execution outcome. It returns nil for a
// legacy prose response, for content that is not JSON, or for JSON without a
// recognized status, so the protocol change is backward compatible. When
// changes_expected is omitted for a completed outcome it defaults to true (the
// agent intended to change the repository), which is the stricter choice.
func parseOutcome(content string) *Outcome {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || trimmed[0] != '{' {
		return nil
	}

	var wire outcomeWire
	if err := json.Unmarshal([]byte(trimmed), &wire); err != nil {
		return nil
	}

	var status OutcomeStatus
	switch wire.Status {
	case string(OutcomeCompleted):
		status = OutcomeCompleted
	case string(OutcomeNeedsHuman):
		status = OutcomeNeedsHuman
	case string(OutcomeFailed):
		status = OutcomeFailed
	default:
		return nil
	}

	outcome := &Outcome{
		Status:  status,
		Summary: strings.TrimSpace(wire.Summary),
		Reason:  strings.TrimSpace(wire.Reason),
	}
	// ChangesExpected is meaningful only for a completed outcome; a completed
	// outcome without the field intends to change the repository (the stricter
	// default). For needs_human and failed it stays false.
	if status == OutcomeCompleted {
		outcome.ChangesExpected = true
		if wire.ChangesExpected != nil {
			outcome.ChangesExpected = *wire.ChangesExpected
		}
	}
	return outcome
}
