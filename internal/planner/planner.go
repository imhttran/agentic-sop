package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/normalize"
)

// planTaskPrompt is the stable instruction sent to the agent. It is an
// application asset, kept out of CLI code.
const planTaskPrompt = `Create an implementation plan from the PRD provided as input.

When the work integrates with or extends an existing system, do not assume its
capabilities. Discover them from the repository first, then plan against what
actually exists.

Requirements:
- Inspect the existing system's boundaries before committing to tasks: its
  application/service APIs, CLI commands, interfaces, persistence boundaries,
  scheduler/lifecycle ownership, external adapters, provider capabilities,
  authorization/approval operations, and existing read/write operations.
- Record every capability the work depends on with a status of EXISTS, PARTIAL,
  MISSING, or UNKNOWN and the repository evidence for that finding. When a
  capability is missing or partial, name the layer that owns providing it.
- Never describe a missing or unknown capability as if it already exists.
- A missing capability is not automatically a blocker. When the requested
  architecture already determines who owns it, record the gap and continue:
  scope the dependent task to an explicitly unsupported/placeholder contract, or
  create a correctly-owned prerequisite stage, preserving dependency order.
  Do not create an implementation task that depends on a capability nothing
  provides.
- Record important assumptions with their evidence and consequence.
- Keep the prerequisite classification exact: a stage's "requires" entry lists
  ONLY an actual externally supplied runtime, permission, tool, service, or
  artifact that must be available before the stage's work can begin.
  Repository or package inspection, searches, call-path tracing,
  types/interfaces/shapes discovery, configuration/defaults/policy discovery,
  adapter and test reconnaissance, and creating or inspecting report output are
  work: describe them in the stage's objective or acceptance criteria, not in
  "requires".
- A discovery target you cannot yet resolve may be recorded in the capability
  inventory as informational with status UNKNOWN and no stage "requires" entry,
  so it is visible without gating execution on it.
- A completed task artifact is reusable dependency evidence, not an unknown
  prerequisite: express it as a stage dependency (the dependent stage lists the
  producing stage id in "dependencies") instead of an UNKNOWN capability the
  stage requires before rediscovering it.
- Prefer decomposing work along natural boundaries (discovery/contract, read
  operations, command operations, missing capabilities, consumer wiring,
  verification) where they clarify ownership, so implementation does not have to
  rediscover its own architecture. Use only the boundaries the work justifies.
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
  "capabilities": [
    {
      "name": string,
      "status": "EXISTS" | "PARTIAL" | "MISSING" | "UNKNOWN",
      "evidence": string,
      "location": string,
      "owner": string,
      "gap": string,
      "resolution": string
    }
  ],
  "assumptions": [
    { "assumption": string, "evidence": string, "consequence": string }
  ],
  "stages": [
    {
      "id": string,
      "title": string,
      "objective": string,
      "dependencies": [string],
      "requires": [string],
      "deliverables": [string],
      "acceptance_criteria": [string],
      "kind": string
    }
  ]
}
Stage ids are unique; each dependency must reference another stage id in this plan.
"requires" is strictly the set of capabilities that are actual externally
supplied runtime, permission, tool, service, or artifact this stage needs before
its work can begin. Each entry must be a name declared in "capabilities", and a
stage must not depend on a capability whose status is UNKNOWN. Repository or
package inspection, searches, call-path tracing, types/interfaces/shapes
inspection, configuration/defaults/policy inspection, adapter and test
discovery, and report creation or inspection are work: put them in the stage's
"objective" or "acceptance_criteria", not in "requires". An informational UNKNOWN
discovery target may be listed in "capabilities" without any stage "requires"
entry. A completed task artifact is reusable dependency evidence: express it as
a stage dependency (list the producing stage id in "dependencies"), not as an
UNKNOWN capability the stage requires before rediscovery.
Give "owner" (the layer that must provide it) or "resolution"
for any capability that is not EXISTS.
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
// returned to the agent for correction, bounded by maxPlanRepairs. A plan that
// depends on a capability whose owner the requirements do not determine is a
// genuine human decision and is surfaced as such. It never writes files or
// touches workflow state.
func (p *Planner) Generate(ctx context.Context, prd string) (*Plan, error) {
	if strings.TrimSpace(prd) == "" {
		return nil, fmt.Errorf("prd is empty")
	}

	plan, err := p.generateValid(ctx, agent.Request{
		Capability:         agent.Plan,
		Task:               planTaskPrompt,
		Input:              prd,
		OutputRequirements: planOutputRequirements,
	})
	if err != nil {
		return nil, err
	}
	return acceptPlan(plan)
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
	data, err := normalize.JSON(content)
	if err != nil {
		return nil, fmt.Errorf("parse agent response: %w", err)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
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
// declaredCapabilityNames returns, best-effort, the capability names the rejected
// plan declared, so a repair can tell the agent exactly which names a stage's
// "requires" entry may use. A plan that does not parse yields none.
func declaredCapabilityNames(rejected string) []string {
	var p struct {
		Capabilities []struct {
			Name string `json:"name"`
		} `json:"capabilities"`
	}
	if json.Unmarshal([]byte(rejected), &p) != nil {
		return nil
	}
	var names []string
	for _, c := range p.Capabilities {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}

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
	if names := declaredCapabilityNames(rejected); len(names) > 0 {
		b.WriteString("The plan declares these capabilities:\n")
		for _, n := range names {
			fmt.Fprintf(&b, "  - %s\n", n)
		}
		b.WriteString("A stage's \"requires\" entries must name one of these capabilities exactly, or be removed.\n\n")
	}
	fmt.Fprintf(&b, `That plan was rejected by deterministic validation:

  %s

Return a corrected plan as JSON only. Fix exactly that problem and change
nothing else. Every dependency must reference another stage id in this plan,
stage ids must be unique, no stage may depend on itself, and the plan must have
no dependency cycles. A "requires" entry must be an actual externally supplied
runtime, permission, tool, service, or artifact; discovery, reconnaissance, and
report work belong in the objective or acceptance criteria instead.`, strings.TrimSpace(cause.Error()))

	return agent.Request{
		Capability:         original.Capability,
		Task:               original.Task,
		Input:              b.String(),
		OutputRequirements: original.OutputRequirements,
	}
}
