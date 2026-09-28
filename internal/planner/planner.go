package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
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

// RepairFunc is notified when an invalid generated plan is returned to the
// agent for correction: attempt is the 1-based repair number and cause is the
// deterministic validation error that triggered it. It exists so a caller can
// trace and account for plan recovery; the planner itself performs no I/O.
type RepairFunc func(attempt int, cause error)

// Planner converts a PRD into a validated Plan using an Agent.
type Planner struct {
	agent    agent.Agent
	onRepair RepairFunc
}

// New returns a Planner that uses the given Agent.
func New(a agent.Agent) *Planner {
	return &Planner{agent: a}
}

// OnRepair registers fn to be notified of each plan-repair attempt and returns
// the Planner so registration can be chained onto New.
func (p *Planner) OnRepair(fn RepairFunc) *Planner {
	p.onRepair = fn
	return p
}

// maxPlanRepairs bounds how many times an invalid plan is returned to the PLAN
// agent for correction before generation fails. The initial response is not a
// repair, so a plan is generated at most 1+maxPlanRepairs times; a valid plan
// never triggers a repair call.
const maxPlanRepairs = 2

// Generate validates the PRD, asks the agent for a plan, parses the JSON
// response, validates the Plan, and returns it. An invalid execution graph is
// returned to the agent for correction, bounded by maxPlanRepairs. It never
// writes files or touches workflow state.
func (p *Planner) Generate(ctx context.Context, prd string) (*Plan, error) {
	if strings.TrimSpace(prd) == "" {
		return nil, fmt.Errorf("prd is empty")
	}

	return p.generateValid(ctx, agent.Request{
		Capability:         agent.Plan,
		Task:               planTaskPrompt,
		Input:              prd,
		OutputRequirements: planOutputRequirements,
	})
}

// generateValid asks the agent for a plan and returns it only once it passes
// deterministic validation. When the response is not a valid execution graph,
// the deterministic error is returned to the agent in a follow-up request that
// asks for a correction; this is bounded by maxPlanRepairs so a persistently
// invalid plan fails cleanly instead of looping. Validation stays the sole
// authority: the agent is never asked to approve its own output.
func (p *Planner) generateValid(ctx context.Context, req agent.Request) (*Plan, error) {
	for repairs := 0; ; repairs++ {
		response, err := p.agent.Generate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent: %w", err)
		}
		plan, err := decodePlan(response.Content)
		if err == nil {
			return plan, nil
		}
		if repairs >= maxPlanRepairs {
			return nil, err
		}
		if p.onRepair != nil {
			p.onRepair(repairs+1, err)
		}
		req = planRepairRequest(req, response.Content, err)
	}
}

// decodePlan parses an agent's plan JSON and validates the execution graph
// (ids, references, cycles, non-empty). It is the deterministic authority on
// plan validity; no model is consulted.
func decodePlan(content string) (*Plan, error) {
	var plan Plan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("parse agent response: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return &plan, nil
}

// planRepairRequest shows the agent the plan it returned and the deterministic
// error that rejected it, and asks for a corrected plan. The original task,
// input, and output contract are preserved so the correction still answers the
// original request.
func planRepairRequest(original agent.Request, rejected string, cause error) agent.Request {
	var b strings.Builder
	if origin := strings.TrimSpace(original.Input); origin != "" {
		b.WriteString(origin)
		b.WriteString("\n\n")
	}
	if r := strings.TrimSpace(rejected); r != "" {
		b.WriteString("The plan you returned was:\n\n")
		b.WriteString(r)
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, `That plan was rejected by deterministic validation:

  %s

Return a corrected plan as JSON only. Fix exactly that problem and change
nothing else. Every dependency must reference another stage id in this plan,
stage ids must be unique, no stage may depend on itself, and the plan must have
no dependency cycles.`, strings.TrimSpace(cause.Error()))

	return agent.Request{
		Capability:         original.Capability,
		Task:               original.Task,
		Input:              b.String(),
		OutputRequirements: original.OutputRequirements,
	}
}
