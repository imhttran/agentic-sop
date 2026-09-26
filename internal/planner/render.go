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

	for _, stage := range p.Stages {
		fmt.Fprintf(&b, "## %s — %s\n\n", stage.ID, stage.Title)
		b.WriteString(strings.TrimSpace(stage.Objective))
		b.WriteString("\n\n### Dependencies\n\n")
		b.WriteString(renderList(stage.Dependencies))
		b.WriteString("\n### Deliverables\n\n")
		b.WriteString(renderList(stage.Deliverables))
		b.WriteString("\n### Acceptance Criteria\n\n")
		b.WriteString(renderList(stage.AcceptanceCriteria))
		b.WriteString("\n")
	}

	return b.String()
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
