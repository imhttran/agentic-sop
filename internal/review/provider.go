package review

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/imhttran/agentic-sdlc/internal/agent"
)

// reviewOutputSchema is the output contract for the REVIEW capability.
const reviewOutputSchema = `Return JSON only (no prose, no markdown):
{"summary": string, "findings": [{"severity": "INFO|LOW|MEDIUM|HIGH|CRITICAL", "title": string, "detail": string, "file": string, "line": number, "suggestion": string}]}
Report only real findings; use severity CRITICAL/HIGH only for correctness, safety, or security problems.`

// AgentProvider reviews by asking an agent for structured findings.
type AgentProvider struct {
	agent agent.Agent
}

// NewAgentProvider returns a Provider backed by the agent REVIEW capability.
func NewAgentProvider(a agent.Agent) *AgentProvider {
	return &AgentProvider{agent: a}
}

// Review requests the REVIEW capability and parses the JSON findings.
func (p *AgentProvider) Review(ctx context.Context, request Request) (Report, error) {
	resp, err := p.agent.Generate(ctx, agent.Request{
		Capability:         agent.Review,
		Task:               "Review the implementation for the change under review.",
		Input:              request.text(),
		OutputRequirements: reviewOutputSchema,
	})
	if err != nil {
		return Report{}, fmt.Errorf("review agent: %w", err)
	}

	report, err := parseReport(resp.Content)
	if err != nil {
		return Report{}, err
	}
	return report, nil
}

type reportWire struct {
	Summary  string `json:"summary"`
	Findings []struct {
		Severity   string `json:"severity"`
		Title      string `json:"title"`
		Detail     string `json:"detail"`
		File       string `json:"file"`
		Line       int    `json:"line"`
		Suggestion string `json:"suggestion"`
	} `json:"findings"`
}

// parseReport deterministically parses an agent review response. Malformed
// output or an unknown severity is an error, never a silent pass.
func parseReport(content string) (Report, error) {
	var wire reportWire
	if err := json.Unmarshal([]byte(content), &wire); err != nil {
		return Report{}, fmt.Errorf("parse review response: %w", err)
	}

	report := Report{Summary: wire.Summary, Findings: make([]Finding, 0, len(wire.Findings))}
	for i, f := range wire.Findings {
		severity := Severity(f.Severity)
		if !severity.Valid() {
			return Report{}, fmt.Errorf("review finding %d has invalid severity %q", i, f.Severity)
		}
		report.Findings = append(report.Findings, Finding{
			Severity:   severity,
			Title:      f.Title,
			Detail:     f.Detail,
			File:       f.File,
			Line:       f.Line,
			Suggestion: f.Suggestion,
		})
	}
	return report, nil
}
