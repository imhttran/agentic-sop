package planner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

var (
	mdHeading  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	mdIDToken  = regexp.MustCompile(`^([A-Za-z]+[-_]?\d+(?:-[A-Za-z]+\d+)?[A-Za-z]?)\b`)
	mdBullet   = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	mdCheckbox = regexp.MustCompile(`^\[[ xX]\]\s*`)
)

// compileTaskPrompt asks the agent to normalize a human plan document into the
// machine plan. It is only used when deterministic compilation fails.
const compileTaskPrompt = `Convert the provided human PLAN.md into the machine plan JSON.
Preserve the stages, their ids, titles, objectives, dependencies, the
capabilities they require, deliverables, acceptance criteria, and execution mode,
along with any capability inventory and assumptions; do not invent or drop work.`

// PlanFromMarkdown deterministically compiles a human PLAN.md into a Plan. It
// understands the rendering produced by RenderMarkdown and close human variants:
// a "## Project" and "## Summary" section, optional "## Capabilities" and
// "## Assumptions" inventories, and one "## <id> — <title>" section per stage
// with optional Dependencies / Requires / Deliverables / Acceptance Criteria /
// Execution sub-sections. It does not validate; callers call Validate.
func PlanFromMarkdown(markdown string) (*Plan, error) {
	plan := &Plan{}
	var project, summary strings.Builder

	section := "" // "project" | "summary" | "stage" | "capabilities" | "assumptions" | ""
	type stageAcc struct {
		stage     Stage
		objective strings.Builder
		sub       string
	}
	var cur *stageAcc
	var curCap *Capability
	var curAsm *Assumption

	flushStage := func() {
		if cur != nil {
			cur.stage.Objective = strings.TrimSpace(cur.objective.String())
			plan.Stages = append(plan.Stages, cur.stage)
			cur = nil
		}
	}
	flushCap := func() {
		if curCap != nil {
			plan.Capabilities = append(plan.Capabilities, *curCap)
			curCap = nil
		}
	}
	flushAsm := func() {
		if curAsm != nil {
			plan.Assumptions = append(plan.Assumptions, *curAsm)
			curAsm = nil
		}
	}
	flush := func() {
		flushStage()
		flushCap()
		flushAsm()
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
				case "capabilities", "capability":
					section = "capabilities"
					continue
				case "assumptions", "assumption":
					section = "assumptions"
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
			default: // level >= 3: a sub-section within the current section
				if cur != nil {
					cur.sub = normalizeHeading(text)
					continue
				}
				switch section {
				case "capabilities":
					flushCap()
					name, status := splitNameStatus(text)
					curCap = &Capability{Name: name, Status: CapabilityStatus(status)}
				case "assumptions":
					flushAsm()
					curAsm = &Assumption{Assumption: strings.TrimSpace(text)}
				}
				continue
			}
		}

		switch section {
		case "project":
			project.WriteString(raw + "\n")
		case "summary":
			summary.WriteString(raw + "\n")
		case "capabilities":
			if curCap == nil {
				continue
			}
			key, val, ok := splitField(raw)
			if !ok {
				continue
			}
			switch key {
			case "status":
				curCap.Status = CapabilityStatus(val)
			case "location":
				curCap.Location = val
			case "owner":
				curCap.Owner = val
			case "evidence":
				curCap.Evidence = val
			case "gap":
				curCap.Gap = val
			case "resolution":
				curCap.Resolution = val
			}
		case "assumptions":
			if curAsm == nil {
				continue
			}
			key, val, ok := splitField(raw)
			if !ok {
				continue
			}
			switch key {
			case "evidence":
				curAsm.Evidence = val
			case "consequence":
				curAsm.Consequence = val
			}
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
			case "requires", "require", "needs":
				cur.stage.Requires = append(cur.stage.Requires, item)
			case "deliverables", "deliverable", "scope", "outputs":
				cur.stage.Deliverables = append(cur.stage.Deliverables, item)
			case "acceptance criteria", "acceptance", "acceptance criterion", "tests":
				cur.stage.AcceptanceCriteria = append(cur.stage.AcceptanceCriteria, item)
			case "execution", "execution mode":
				if mode, ok := domain.ParseExecutionMode(item); ok {
					cur.stage.ExecutionMode = mode
				}
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

// Compile returns a validated Plan from a human plan document. When the document
// is recognizably a plan (it parses into stages), its structure is authoritative:
// validation errors are surfaced, not silently repaired by the agent. Only when
// the document is not recognizable as a plan does the configured agent normalize
// it — and even then the result must pass the same deterministic validation.
func (p *Planner) Compile(ctx context.Context, markdown string) (*Plan, error) {
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("plan document is empty")
	}

	if parsed, _ := PlanFromMarkdown(markdown); parsed != nil && len(parsed.Stages) > 0 {
		if err := parsed.Validate(); err != nil {
			return nil, err
		}
		return acceptPlan(parsed)
	}

	if p == nil || p.agent == nil {
		return nil, fmt.Errorf("cannot compile plan document and no agent is available to normalize it")
	}

	plan, err := p.generateValid(ctx, agent.Request{
		Capability:         agent.Plan,
		Task:               compileTaskPrompt,
		Input:              markdown,
		OutputRequirements: planOutputRequirements,
	})
	if err != nil {
		return nil, err
	}
	return acceptPlan(plan)
}

// normalizeHeading lowercases a heading and collapses whitespace for lookup.
func normalizeHeading(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ":")
	return strings.Join(strings.Fields(s), " ")
}

// splitStageHeading separates a leading stage id from the rest of a heading. An
// id may carry one hierarchical sub-id segment (for example "PREJEV012-S6"), so
// an umbrella stage and its sub-stages can be named in one flat plan.
func splitStageHeading(text string) (id, title string) {
	m := mdIDToken.FindStringSubmatch(text)
	if m == nil {
		return "", ""
	}
	rest := strings.TrimLeft(strings.TrimSpace(text[len(m[1]):]), "-–—: \t")
	return m[1], strings.TrimSpace(rest)
}

// splitNameStatus separates a capability heading such as "CancelRun — MISSING"
// into its name and status. It accepts the separators RenderMarkdown emits and
// the close hand-written variants.
func splitNameStatus(text string) (name, status string) {
	for _, sep := range []string{" — ", " – ", " - ", " | ", ": "} {
		if i := strings.Index(text, sep); i >= 0 {
			return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+len(sep):])
		}
	}
	return strings.TrimSpace(text), ""
}

// splitField parses a rendered "- Key: value" field line into a normalized key
// and its value. It reports false for a line that carries no field.
func splitField(line string) (key, value string, ok bool) {
	item := listItem(line)
	i := strings.Index(item, ":")
	if i < 0 {
		return "", "", false
	}
	return normalizeHeading(item[:i]), strings.TrimSpace(item[i+1:]), true
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
