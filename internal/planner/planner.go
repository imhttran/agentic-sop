package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sdlc/internal/agent"
)

// planTaskPrompt is the stable instruction sent to the agent. It is an
// application asset, kept out of CLI code.
const planTaskPrompt = `Create an implementation plan from the PRD provided as input.

Requirements:
- Break the work into implementation stages.
- Give each stage a clear objective.
- State explicit dependencies between stages.
- List concrete deliverables.
- Provide testable acceptance criteria.
- Avoid implementation outside the PRD scope.
- Prefer small stages.
- Preserve the PRD's terminology.`

// planOutputRequirements is the output contract. The agent must return JSON
// only, matching this schema; the application parses it into a Plan.
const planOutputRequirements = `Return JSON only (no prose, no markdown) matching:
{
  "project": string,
  "summary": string,
  "stages": [
    {
      "id": string,
      "title": string,
      "objective": string,
      "dependencies": [string],
      "deliverables": [string],
      "acceptance_criteria": [string],
      "kind": string
    }
  ]
}
Stage ids are unique; each dependency must reference another stage id in this plan.
kind is optional: use "environment" for a single development-environment/bootstrap
stage that all other stages depend on, and "feature" (or omit) otherwise.`

// Planner converts a PRD into a validated Plan using an Agent.
type Planner struct {
	agent agent.Agent
}

// New returns a Planner that uses the given Agent.
func New(a agent.Agent) *Planner {
	return &Planner{agent: a}
}

// Generate validates the PRD, asks the agent for a plan, parses the JSON
// response, validates the Plan, and returns it. It never writes files or
// touches workflow state.
func (p *Planner) Generate(ctx context.Context, prd string) (*Plan, error) {
	if strings.TrimSpace(prd) == "" {
		return nil, fmt.Errorf("prd is empty")
	}

	response, err := p.agent.Generate(ctx, agent.Request{
		Capability:         agent.Plan,
		Task:               planTaskPrompt,
		Input:              prd,
		OutputRequirements: planOutputRequirements,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: %w", err)
	}

	var plan Plan
	if err := json.Unmarshal([]byte(response.Content), &plan); err != nil {
		return nil, fmt.Errorf("parse agent response: %w", err)
	}

	if err := plan.Validate(); err != nil {
		return nil, err
	}

	return &plan, nil
}
