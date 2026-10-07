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
	mdHeading = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	// mdIDToken matches a leading stage id. Its alphabetic prefix may carry one or
	// more hyphen/underscore-separated name segments before the numeric segment, so
	// a generic "<NAME>-<NAME>-<NNN>" id (for example "AS-CLEF-001") is read as one
	// id rather than truncated at its first numeric segment. The optional
	// sub-segment accepts both a letter-number suffix ("PREJEV012-S6") and a
	// numeric one ("P5-001"), so an umbrella stage and its sub-stages can be named
	// in one flat plan, as can the "P<n>-<nnn>" ids the phase plans use. No
	// concrete id is special-cased.
	mdIDToken  = regexp.MustCompile(`^((?:[A-Za-z]+[-_]?)+\d+(?:-[A-Za-z]*\d+)?[A-Za-z]?)\b`)
	mdBullet   = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	mdCheckbox = regexp.MustCompile(`^\[[ xX]\]\s*`)
)

// compileTaskPrompt asks the agent to normalize a human plan document into the
// machine plan. It is only used when deterministic compilation fails. It repeats
// the prerequisite-classification rule so a model normalizing an unrecognized
// document does not turn discovery or report work into a "requires" entry.
const compileTaskPrompt = `Convert the provided human PLAN.md into the machine plan JSON.
Preserve the stages, their ids, titles, objectives, dependencies, the
capabilities they require, deliverables, acceptance criteria, and execution mode,
along with any capability inventory and assumptions; do not invent or drop work.
A stage's "requires" lists only capabilities that are actual externally supplied
runtime, permission, tool, service, or artifact needed before the stage's work
can begin. Repository or package inspection, searches, call paths, types,
interfaces, shapes, configuration, defaults, policy, adapters, test discovery,
and report creation or inspection are work to describe in the stage's objective
or acceptance criteria, not prerequisites. Put an informational UNKNOWN
discovery target in the capability inventory without a "requires" entry.
Completed task artifacts are reusable dependency evidence: express them as stage
dependencies, not as UNKNOWN prerequisites for rediscovery.`

// PlanFromMarkdown deterministically compiles a human PLAN.md into a Plan. It
// understands two hand-written shapes of the same plan:
//
//   - the rendering produced by RenderMarkdown and close human variants: a
//     "## Project" and "## Summary" section, optional "## Capabilities" and
//     "## Assumptions" inventories, and one "## <id> — <title>" section per stage
//     with optional Dependencies / Requires / Deliverables / Acceptance Criteria /
//     Execution sub-sections;
//   - a plan that names its stages with level-3 headings under a "## Tasks"
//     section ("### P5-001 — ...") and describes each with inline "- **Field:**"
//     lines (Objective, Scope, Files, Depends on, Acceptance, Execution) — the
//     shape the phase plans use.
//
// Only recognized fields become plan fields: a stage's Status line, a Validation
// line, an unrecognized field, and unmatched prose stay documentation, so the
// execution mode is never inferred from them. It does not validate; callers call
// Validate.
func PlanFromMarkdown(markdown string) (*Plan, error) {
	plan, err := parseRenderedPlan(markdown)
	if err != nil || plan == nil {
		return plan, err
	}
	if len(plan.Stages) > 0 {
		return plan, nil
	}
	// No "## <id> — <title>" stage heading: try the "## Tasks" shape before giving
	// up, so a plan written that way compiles without an agent. An unreadable
	// dependency is an error rather than a guess: a dropped edge would silently
	// reorder the graph.
	stages, err := parseTaskSection(markdown)
	if err != nil {
		return nil, err
	}
	plan.Stages = stages
	// A hand-written plan may not name the plan-level fields the rendered form does, so
	// supply them from the document itself: the project is the document title, and the
	// summary is the "## Objective" section.
	if len(stages) > 0 {
		if strings.TrimSpace(plan.Project) == "" {
			plan.Project = documentTitle(markdown)
		}
		if strings.TrimSpace(plan.Summary) == "" {
			plan.Summary = sectionBody(markdown, "objective")
		}
	}
	return plan, nil
}

// documentTitle returns a document's level-1 heading text.
func documentTitle(markdown string) string {
	for _, raw := range strings.Split(markdown, "\n") {
		if m := mdHeading.FindStringSubmatch(raw); m != nil && len(m[1]) == 1 {
			return strings.TrimSpace(m[2])
		}
	}
	return ""
}

// sectionBody returns the prose of the first level-2 section named name, up to the
// next heading.
func sectionBody(markdown, name string) string {
	var (
		body    strings.Builder
		collect bool
	)
	for _, raw := range strings.Split(markdown, "\n") {
		if m := mdHeading.FindStringSubmatch(raw); m != nil {
			if len(m[1]) <= 2 {
				collect = len(m[1]) == 2 && normalizeHeading(strings.TrimSpace(m[2])) == name
			}
			continue
		}
		if collect {
			body.WriteString(raw + "\n")
		}
	}
	return strings.TrimSpace(body.String())
}

// parseRenderedPlan compiles the canonical rendered shape: one "## <id> — <title>"
// section per stage.
func parseRenderedPlan(markdown string) (*Plan, error) {
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

	// A document the parser recognizes as a plan is authoritative: a parse or
	// validation error is surfaced, not silently repaired by the agent. Only a
	// document that is not recognizable as a plan is handed to the agent to
	// normalize.
	parsed, perr := PlanFromMarkdown(markdown)
	if perr != nil {
		return nil, perr
	}
	if parsed != nil && len(parsed.Stages) > 0 {
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
	}, PlanContext{})
	if err != nil {
		return nil, err
	}
	// A model MUST NOT declare work complete. The "done" execution mode is operator
	// intent read deterministically from the plan document (PlanFromMarkdown); a plan
	// a model normalized is not that document's own structure, so any such
	// declaration it returned is ignored and the stage is executed normally. This is
	// the safe direction: no work is skipped because a model claimed it was done.
	for i := range plan.Stages {
		if plan.Stages[i].ExecutionMode.Done() {
			plan.Stages[i].ExecutionMode = ""
		}
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
// id may carry one or more alphabetic name segments followed by a numeric segment
// and an optional hierarchical sub-id (for example "PREJEV012-S6"), so an umbrella
// stage and its sub-stages can be named in one flat plan.
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
	rawKey := strings.TrimSpace(item[:i])
	value = strings.TrimSpace(item[i+1:])
	// A field written as "- **Depends on:** P5-001" closes its emphasis after the
	// colon, so the closing delimiter belongs to the key's markup, not to the value.
	for _, delim := range []string{"**", "__"} {
		if strings.HasPrefix(rawKey, delim) && strings.HasPrefix(value, delim) {
			value = strings.TrimSpace(value[len(delim):])
		}
	}
	return normalizeFieldKey(rawKey), value, true
}

// normalizeFieldKey normalizes a field's key. Markdown emphasis and backticks around
// the key ("- **Depends on:**") are not part of the name.
func normalizeFieldKey(s string) string {
	return normalizeHeading(strings.Trim(strings.TrimSpace(s), "*_`"))
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

// --- The "## Tasks" plan shape ---------------------------------------------

// taskSectionAliases are the level-2 sections whose level-3 headings are read as
// stages in a hand-written plan. The phase plans name their stages this way ("##
// Tasks" + "### P5-001 — ..."); a closed set means an unrelated level-3 heading is
// never mistaken for a stage.
var taskSectionAliases = map[string]bool{"tasks": true, "task": true}

// taskField is one inline "- **Key:** value" line of a stage. Its value may have
// wrapped onto indented continuation lines.
type taskField struct{ key, value string }

// taskStage accumulates the fields of one "## Tasks" stage before they are mapped
// onto a Stage.
type taskStage struct {
	id, title string
	fields    []taskField
	last      int // index of the field an indented continuation line extends
}

// parseTaskSection compiles the "## Tasks" shape: one "### <id> — <title>" heading
// per stage, each described by inline fields. A heading that carries no stage id, a
// level-3 heading under any other section, an unrecognized field, and unmatched
// prose are ignored, so the shape is read only where it is unambiguous. A dependency
// value that is not an explicit list of stage ids is an error: guessing at
// "P35-001..P35-005" would silently drop edges.
func parseTaskSection(markdown string) ([]Stage, error) {
	var (
		stages  []Stage
		cur     *taskStage
		inTasks bool
		err     error
	)
	flush := func() {
		if cur == nil || err != nil {
			return
		}
		var stage Stage
		if stage, err = cur.stage(); err == nil {
			stages = append(stages, stage)
		}
		cur = nil
	}

	for _, raw := range strings.Split(markdown, "\n") {
		if m := mdHeading.FindStringSubmatch(raw); m != nil {
			level := len(m[1])
			text := strings.TrimSpace(m[2])
			if level == 3 && inTasks {
				if id, title := splitStageHeading(text); id != "" {
					flush()
					cur = &taskStage{id: id, title: title, last: -1}
					continue
				}
			}
			// Another heading ends the stage and its last field, so a sub-section is
			// never folded into a field value.
			flush()
			if level <= 2 {
				inTasks = level == 2 && taskSectionAliases[normalizeHeading(text)]
			}
			continue
		}
		if cur != nil {
			cur.line(raw)
		}
	}
	flush()
	return stages, err
}

// line consumes one non-heading line of a stage.
func (t *taskStage) line(raw string) {
	if strings.TrimSpace(raw) == "" {
		t.last = -1
		return
	}
	// An indented line continues the value above it: the hand-written plans wrap a
	// long Scope or Acceptance value across several indented lines.
	if t.last >= 0 && (raw[0] == ' ' || raw[0] == '\t') {
		t.fields[t.last].value += " " + strings.TrimSpace(raw)
		return
	}
	key, value, ok := splitField(raw)
	if !ok {
		t.last = -1
		return
	}
	t.fields = append(t.fields, taskField{key: key, value: value})
	t.last = len(t.fields) - 1
}

// stage maps the accumulated fields onto a plan Stage. Only recognized fields become
// plan fields; a stage's Status and Validation lines stay documentation, and the
// execution mode is never inferred from them.
func (t *taskStage) stage() (Stage, error) {
	s := Stage{ID: t.id, Title: t.title}
	var objective, scope []string
	for _, f := range t.fields {
		switch f.key {
		case "objective", "goal":
			objective = append(objective, f.value)
		case "scope":
			scope = append(scope, f.value)
		case "files", "file", "likely files", "likely files/packages", "likely packages":
			s.Deliverables = append(s.Deliverables, f.value)
		case "depends on", "depends", "dependency", "dependencies":
			deps, err := parseDependencies(f.value)
			if err != nil {
				return Stage{}, fmt.Errorf("plan: stage %s %w", t.id, err)
			}
			s.Dependencies = append(s.Dependencies, deps...)
		case "acceptance", "acceptance criteria", "acceptance criterion", "tests", "done when":
			s.AcceptanceCriteria = append(s.AcceptanceCriteria, f.value)
		case "execution", "execution mode":
			if mode, ok := domain.ParseExecutionMode(executionModeValue(f.value)); ok {
				s.ExecutionMode = mode
			}
		}
	}
	// The objective is the stage's prose: an explicit Objective, else the Scope, which
	// is how a phase plan that omits Objective describes the work.
	s.Objective = strings.TrimSpace(strings.Join(objective, "\n\n"))
	if s.Objective == "" {
		s.Objective = strings.TrimSpace(strings.Join(scope, "\n\n"))
	}
	return s, nil
}

// parseDependencies reads a stage's "Depends on" value: an explicit, comma- or
// semicolon-separated list of stage ids, or a marker for none ("—", "none."). It is
// deliberately strict — a range ("P35-001..P35-005"), "all", or a parenthetical is an
// error rather than a guess, because a dropped edge would silently reorder the graph.
func parseDependencies(value string) ([]string, error) {
	var out []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		id := strings.TrimSuffix(part, ".")
		if id == "" || noneMarker(id) {
			continue
		}
		if !isStageID(id) {
			return nil, fmt.Errorf("has an unreadable dependency %q: list stage ids explicitly (for example \"P5-002, P5-003\")", part)
		}
		out = append(out, id)
	}
	return out, nil
}

// noneMarker reports whether a dependency value means "none".
func noneMarker(s string) bool {
	switch strings.ToLower(strings.Trim(s, "-–— \t")) {
	case "", "none", "no", "n/a", "na", "tbd":
		return true
	}
	return false
}

// isStageID reports whether s is exactly one stage id, so a stray word or a range is
// rejected rather than partly matched.
func isStageID(s string) bool {
	m := mdIDToken.FindStringSubmatch(s)
	return m != nil && m[1] == s
}

// executionModeValue takes the mode itself from an Execution field, whose value may
// carry an explanation after it ("verify-first — the implementation is already
// committed; ...").
func executionModeValue(value string) string {
	if cut := strings.IndexAny(value, "—–;,(."); cut >= 0 {
		return strings.TrimSpace(value[:cut])
	}
	return strings.TrimSpace(value)
}
