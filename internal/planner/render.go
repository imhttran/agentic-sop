package planner

import (
	"fmt"
	"strings"
)

// RenderMarkdown renders the Plan as the human-facing PLAN.md document. Output
// is deterministic and produced entirely by Go so presentation stays separate
// from the model's reasoning.
func (p *Plan) RenderMarkdown() string {
	var b strings.Builder

	b.WriteString("# Implementation Plan\n\n")
	b.WriteString("## Project\n\n")
	b.WriteString(strings.TrimSpace(p.Project))
	b.WriteString("\n\n## Summary\n\n")
	b.WriteString(strings.TrimSpace(p.Summary))
	b.WriteString("\n\n")

	if len(p.Capabilities) > 0 {
		b.WriteString("## Capabilities\n\n")
		for _, c := range p.Capabilities {
			fmt.Fprintf(&b, "### %s — %s\n\n", strings.TrimSpace(c.Name), normalizeCapabilityStatus(c.Status))
			renderField(&b, "Location", c.Location)
			renderField(&b, "Owner", c.Owner)
			renderField(&b, "Evidence", c.Evidence)
			renderField(&b, "Gap", c.Gap)
			renderField(&b, "Resolution", c.Resolution)
			b.WriteString("\n")
		}
	}

	if len(p.Assumptions) > 0 {
		b.WriteString("## Assumptions\n\n")
		for _, a := range p.Assumptions {
			fmt.Fprintf(&b, "### %s\n\n", strings.TrimSpace(a.Assumption))
			renderField(&b, "Evidence", a.Evidence)
			renderField(&b, "Consequence", a.Consequence)
			b.WriteString("\n")
		}
	}

	for _, stage := range p.Stages {
		fmt.Fprintf(&b, "## %s — %s\n\n", stage.ID, stage.Title)
		b.WriteString(strings.TrimSpace(stage.Objective))
		b.WriteString("\n\n### Dependencies\n\n")
		b.WriteString(renderList(stage.Dependencies))
		if len(stage.Requires) > 0 {
			b.WriteString("\n### Requires\n\n")
			b.WriteString(renderList(stage.Requires))
		}
		b.WriteString("\n### Deliverables\n\n")
		b.WriteString(renderList(stage.Deliverables))
		b.WriteString("\n### Acceptance Criteria\n\n")
		b.WriteString(renderList(stage.AcceptanceCriteria))
		if stage.ExecutionMode != "" {
			b.WriteString("\n### Execution\n\n")
			b.WriteString(renderList([]string{string(stage.ExecutionMode)}))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderField writes one optional "- Key: value" line, skipping an empty value.
func renderField(b *strings.Builder, key, value string) {
	if s := strings.TrimSpace(value); s != "" {
		fmt.Fprintf(b, "- %s: %s\n", key, s)
	}
}

func renderList(items []string) string {
	if len(items) == 0 {
		return "None\n"
	}
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	return b.String()
}
