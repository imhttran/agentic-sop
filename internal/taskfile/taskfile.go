// Package taskfile loads a single task from a Markdown file, so the workflow can
// start from a hand-written spec instead of only a generated plan.
//
// The format is plain Markdown: a title heading, optional named sections, and
// lists. Nothing is required beyond some content, so an ordinary Markdown file
// stays usable; recognized sections are extracted when present and everything
// else becomes the description.
package taskfile

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// Spec is the structured form of a Markdown task file.
type Spec struct {
	ID                 string
	Title              string
	Description        string
	Requirements       []string
	Deliverables       []string
	AcceptanceCriteria []string
	Constraints        []string
	Dependencies       []string
	// ExecutionMode is the optional execution mode ("" is the implement
	// default). An unrecognized value is ignored rather than failing, so an
	// unrelated "Execution" section cannot break a task file.
	ExecutionMode domain.ExecutionMode
}

var (
	headingRE  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	idTokenRE  = regexp.MustCompile(`^([A-Za-z]+[-_]?\d+(?:-[A-Za-z]+\d+)?[A-Za-z]?)\b`)
	bulletRE   = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	checkboxRE = regexp.MustCompile(`^\[[ xX]\]\s*`)
)

// section collects the body lines of one heading and is keyed by its name.
type section struct {
	key   string
	lines []string
}

// Parse extracts a Spec from Markdown bytes. It fails only for an empty file;
// every other shape degrades to title + description.
func Parse(data []byte) (*Spec, error) {
	raw := string(data)
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("taskfile: file is empty")
	}

	var (
		spec     Spec
		sections []section
		preamble []string
		seenH1   bool
	)

	for _, line := range strings.Split(raw, "\n") {
		if m := headingRE.FindStringSubmatch(line); m != nil {
			text := strings.TrimSpace(m[2])
			if len(m[1]) == 1 && !seenH1 {
				seenH1 = true
				if spec.Title == "" {
					spec.Title = text
				}
				continue
			}
			sections = append(sections, section{key: normalize(text)})
			continue
		}
		if len(sections) == 0 {
			preamble = append(preamble, line)
			continue
		}
		s := &sections[len(sections)-1]
		s.lines = append(s.lines, line)
	}

	// A leading "<id> — <title>" heading carries the ID in the heading text.
	if spec.Title != "" {
		if id, title := splitID(spec.Title); id != "" {
			spec.ID, spec.Title = id, title
		}
	}

	spec.Description = strings.TrimSpace(strings.Join(preamble, "\n"))

	for _, sec := range sections {
		switch sec.key {
		case "id":
			if v := firstText(sec.lines); v != "" {
				spec.ID = v
			}
		case "title":
			if v := firstText(sec.lines); v != "" {
				spec.Title = v
			}
		case "objective", "description", "summary":
			if v := strings.TrimSpace(strings.Join(sec.lines, "\n")); v != "" {
				spec.Description = v
			}
		case "requirements", "requirement":
			spec.Requirements = append(spec.Requirements, listItems(sec.lines)...)
		case "deliverables", "deliverable":
			spec.Deliverables = append(spec.Deliverables, listItems(sec.lines)...)
		case "acceptance criteria", "acceptance", "acceptance criterion":
			spec.AcceptanceCriteria = append(spec.AcceptanceCriteria, listItems(sec.lines)...)
		case "constraints", "constraint", "rules", "rule":
			spec.Constraints = append(spec.Constraints, listItems(sec.lines)...)
		case "dependencies", "dependency", "depends on":
			spec.Dependencies = append(spec.Dependencies, listItems(sec.lines)...)
		case "execution", "execution mode":
			if items := listItems(sec.lines); len(items) > 0 {
				if mode, ok := domain.ParseExecutionMode(items[0]); ok {
					spec.ExecutionMode = mode
				}
			}
		}
	}

	if spec.Title == "" {
		spec.Title = firstHeadingOrLine(raw)
	}

	return &spec, nil
}

// Load reads and parses a task file.
func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("taskfile: read %s: %w", path, err)
	}
	spec, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("taskfile: %s: %w", path, err)
	}
	return spec, nil
}

// Render turns a Spec back into normalized Markdown, the input the planner
// reasons over.
func (s *Spec) Render() string {
	var b strings.Builder
	if heading := joinHeading(s.ID, s.Title); heading != "" {
		fmt.Fprintf(&b, "# %s\n", heading)
	}
	if desc := strings.TrimSpace(s.Description); desc != "" {
		fmt.Fprintf(&b, "\n%s\n", desc)
	}
	writeList(&b, "Requirements", s.Requirements)
	writeList(&b, "Deliverables", s.Deliverables)
	writeList(&b, "Acceptance criteria", s.AcceptanceCriteria)
	writeList(&b, "Constraints", s.Constraints)
	writeList(&b, "Dependencies", s.Dependencies)
	if s.ExecutionMode != "" {
		fmt.Fprintf(&b, "\n## Execution\n\n- %s\n", s.ExecutionMode)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// normalize lowercases and collapses a heading into a lookup key.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// splitID separates a leading task ID token from the rest of a heading. An id may
// carry one hierarchical sub-id segment (for example "PREJEV012-S6").
func splitID(s string) (id, title string) {
	m := idTokenRE.FindStringSubmatch(s)
	if m == nil {
		return "", s
	}
	rest := strings.TrimLeft(strings.TrimSpace(s[len(m[1]):]), "-–—: \t")
	return m[1], strings.TrimSpace(rest)
}

// listItems strips bullets and checkboxes from a section's lines, yielding the
// individual requirements/criteria/constraints.
func listItems(lines []string) []string {
	var out []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || t == "---" {
			continue
		}
		t = bulletRE.ReplaceAllString(t, "")
		t = checkboxRE.ReplaceAllString(t, "")
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// firstText returns the first non-empty line, for single-value sections.
func firstText(lines []string) string {
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// firstHeadingOrLine finds a fallback title from the raw text.
func firstHeadingOrLine(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if m := headingRE.FindStringSubmatch(t); m != nil {
			t = strings.TrimSpace(m[2])
		}
		return t
	}
	return ""
}

// joinHeading combines an ID and title into a heading, tolerating a missing one.
func joinHeading(id, title string) string {
	switch {
	case id != "" && title != "":
		return id + " — " + title
	case title != "":
		return title
	default:
		return id
	}
}

// writeList renders a named list section, skipping it when empty.
func writeList(b *strings.Builder, name string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", name)
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", item)
	}
}
