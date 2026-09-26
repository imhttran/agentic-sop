package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNotConfigured is returned by ProviderFromEnv when no external review
// provider is configured. It is a normal, non-fatal state: the orchestrator
// falls back to the internal provider.
var ErrNotConfigured = errors.New("no external review provider configured")

// EnvReviewCommand names the environment variable that configures an external
// review provider (e.g. Open Code Review). Its value is a trusted shell command
// that reads a JSON Request on stdin and writes findings JSON on stdout.
const EnvReviewCommand = "SOP_REVIEW_COMMAND"

// OCRProvider is a Provider backed by an external command such as Open Code
// Review. Like the agent harness, the command is trusted local configuration and
// is never built from untrusted input.
type OCRProvider struct {
	command string
}

// NewOCRProvider returns an OCRProvider that runs command via the shell.
func NewOCRProvider(command string) *OCRProvider {
	return &OCRProvider{command: command}
}

// ProviderFromEnv returns the external review provider configured through
// EnvReviewCommand, or an error when it is unset. An unset variable is a
// normal, non-fatal state: the orchestrator uses the internal provider.
func ProviderFromEnv() (Provider, error) {
	command := strings.TrimSpace(os.Getenv(EnvReviewCommand))
	if command == "" {
		return nil, fmt.Errorf("%w: set %s", ErrNotConfigured, EnvReviewCommand)
	}
	return NewOCRProvider(command), nil
}

// Review runs the configured command, sending the request JSON on stdin and
// parsing findings JSON from stdout.
func (p *OCRProvider) Review(ctx context.Context, request Request) (Report, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return Report{}, fmt.Errorf("encode review request: %w", err)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", p.command)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Report{}, ctxErr
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return Report{}, fmt.Errorf("external review command failed: %w: %s", err, detail)
		}
		return Report{}, fmt.Errorf("external review command failed: %w", err)
	}

	return parseReport(stdout.String())
}
