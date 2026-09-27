package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

var (
	mdHeading  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	mdIDToken  = regexp.MustCompile(`^([A-Za-z]+[-_]?\d+[A-Za-z]?)\b`)
	mdBullet   = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	mdCheckbox = regexp.MustCompile(`^\[[ xX]\]\s*`)
)

// compileTaskPrompt asks the agent to normalize a human plan document into the
// machine plan. It is only used when deterministic compilation fails.
const compileTaskPrompt = `Convert the provided human PLAN.md into the machine plan JSON.
Preserve the stages, their ids, titles, objectives, dependencies, deliverables,
and acceptance criteria; do not invent or drop work.`

// PlanFromMarkdown deterministically compiles a human PLAN.md into a Plan. It
// understands the rendering produced by RenderMarkdown and close human variants:
// a "## Project" and "## Summary" section, and one "## <id> — <title>" section
// per stage with optional Dependencies / Deliverables / Acceptance Criteria
// sub-sections. It does not validate; callers call Validate.
func PlanFromMarkdown(markdown string) (*Plan, error) {
	plan := &Plan{}
	var project, summary strings.Builder

	section := "" // "project" | "summary" | "stage" | ""
	type stageAcc struct {
		stage     Stage
		objective strings.Builder
		sub       string
	}
	var cur *stageAcc

	flush := func() {
		if cur != nil {
			cur.stage.Objective = strings.TrimSpace(cur.objective.String())
			plan.Stages = append(plan.Stages, cur.stage)
			cur = nil
		}
	}

	for _, raw := range strings.Split(markdown, "\n") {
		if m := mdHeading.FindStringSubmatch(raw); m != nil {
			level := len(m[1])
			text := strings.TrimSpace(m[2])

			switch {
			case level == 1:
				continue // document title; the project comes from "## Project"
			case level == 2:
				flush()
				switch normalizeHeading(text) {
				case "project":
					section = "project"
					continue
				case "summary":
					section = "summary"
					continue
				}
				id, title := splitStageHeading(text)
				if id == "" {
					section = "" // an unrecognized top-level section is ignored
					continue
				}
				cur = &stageAcc{stage: Stage{ID: id, Title: title}}
				section = "stage"
				continue
			default: // level >= 3: a sub-section within the current stage
				if cur != nil {
					cur.sub = normalizeHeading(text)
				}
				continue
			}
		}

		switch section {
		case "project":
			project.WriteString(raw + "\n")
		case "summary":
			summary.WriteString(raw + "\n")
		case "stage":
			if cur == nil {
				continue
			}
			item := listItem(raw)
			if item == "" {
				continue
			}
			switch cur.sub {
			case "dependencies", "dependency", "depends on":
				cur.stage.Dependencies = append(cur.stage.Dependencies, item)
			case "deliverables", "deliverable", "scope", "outputs":
				cur.stage.Deliverables = append(cur.stage.Deliverables, item)
			case "acceptance criteria", "acceptance", "acceptance criterion", "tests":
				cur.stage.AcceptanceCriteria = append(cur.stage.AcceptanceCriteria, item)
			default:
				// Only prose between the stage heading and its first sub-section is the
				// objective; an unrecognized sub-section is ignored rather than merged in.
				if cur.sub == "" {
					cur.objective.WriteString(raw + "\n")
				}
			}
		}
	}
	flush()

	plan.Project = strings.TrimSpace(project.String())
	plan.Summary = strings.TrimSpace(summary.String())
	return plan, nil
}

// Compile returns a validated Plan from a human plan document: it tries the
// deterministic compiler first, and only if that yields an invalid plan asks the
// configured agent to normalize the document.
func (p *Planner) Compile(ctx context.Context, markdown string) (*Plan, error) {
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("plan document is empty")
	}

	if plan, err := PlanFromMarkdown(markdown); err == nil {
		if err := plan.Validate(); err == nil {
			return plan, nil
		}
	}

	if p == nil || p.agent == nil {
		return nil, fmt.Errorf("cannot compile plan document and no agent is available to normalize it")
	}

	response, err := p.agent.Generate(ctx, agent.Request{
		Capability:         agent.Plan,
		Task:               compileTaskPrompt,
		Input:              markdown,
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

// normalizeHeading lowercases a heading and collapses whitespace for lookup.
func normalizeHeading(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ":")
	return strings.Join(strings.Fields(s), " ")
}

// splitStageHeading separates a leading stage id from the rest of a heading.
func splitStageHeading(text string) (id, title string) {
	m := mdIDToken.FindStringSubmatch(text)
	if m == nil {
		return "", ""
	}
	rest := strings.TrimLeft(strings.TrimSpace(text[len(m[1]):]), "-–—: \t")
	return m[1], strings.TrimSpace(rest)
}

// listItem strips a bullet/checkbox from a line and returns the item text, or ""
// when the line is blank or an explicit "none".
func listItem(line string) string {
	t := strings.TrimSpace(line)
	if t == "" {
		return ""
	}
	t = mdBullet.ReplaceAllString(t, "")
	t = mdCheckbox.ReplaceAllString(t, "")
	t = strings.TrimSpace(t)
	switch strings.ToLower(t) {
	case "", "-", "none", "n/a", "na", "tbd":
		return ""
	}
	return t
}
