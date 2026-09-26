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

	return Response{Content: content}, nil
}
