package planflow

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// This file holds the conservative canonical comparisons the reconciliation path
// uses to decide whether a requested plan differs from the recorded one. The
// intended limitation is deliberate and load-bearing: compilation and comparison
// here are deterministic over recognized structured Markdown, and arbitrary model
// prose is NOT inferred equivalent. A full natural-language paraphrase (synonyms,
// filler removal, reworded sentences) is conservatively left as a real change and
// must be reviewed by a human. Only cosmetic variance that cannot change meaning is
// folded away:
//
//   - whitespace and line-wrap differences in literal-free prose only; a field that
//     contains quotes, backticks, or an indented/fenced literal is compared
//     byte-for-byte, so code, quoted values and wrapped paths are never folded;
//   - the order of independent acceptance items, compared as a conjunction;
//   - an explicit "implement" execution mode versus the default empty mode.
//
// Everything else is preserved: all tokens, scope, acceptance meaning, dependency
// sets, execution mode, negation, numbers, and the case of tokens (including
// paths, which are never lowercased). Ambiguity - whitespace inside a quoted string
// or a line-order change inside one multiline literal - stays a real change and a
// human-review decision. Nothing here approves a change: the AcceptChanged boundary
// is unaffected.

// sameTaskDefinition reports whether two tasks carry the same executable definition.
// Dependencies are compared as multisets: order does not matter, but repetition
// does, so [X,Y] and [X,X] are never equal and no dependency can be silently
// dropped. Text fields are compared by canonicalProse and acceptance criteria as
// independent-conjunction sets. A task's AcceptanceCriteria is stored as a single
// string (one criterion per line), so it is split into independent items before
// comparison.
func sameTaskDefinition(a, b *domain.Task) bool {
	return canonicalProse(a.Title) == canonicalProse(b.Title) &&
		canonicalProse(a.Objective) == canonicalProse(b.Objective) &&
		sameAcceptanceCriteria(splitCriteria(a.AcceptanceCriteria), splitCriteria(b.AcceptanceCriteria)) &&
		sameExecutionMode(a.ExecutionMode, b.ExecutionMode) &&
		sameStringSet(a.DependencyIDs, b.DependencyIDs)
}

// equivalentExecutedChange reports whether an executed task's definition changed only
// in descriptive text: its executable semantics - the acceptance criteria, the
// execution mode, and the dependencies - are unchanged under the conservative
// comparisons. A real acceptance/negation/number/scope/dependency/mode change is not
// equivalent.
func equivalentExecutedChange(cur, desired *domain.Task) bool {
	return sameAcceptanceCriteria(splitCriteria(cur.AcceptanceCriteria), splitCriteria(desired.AcceptanceCriteria)) &&
		sameExecutionMode(cur.ExecutionMode, desired.ExecutionMode) &&
		sameStringSet(cur.DependencyIDs, desired.DependencyIDs)
}

// splitCriteria preserves ambiguous multiline literals as one exact unit. Plain
// stored criteria are newline-delimited, as serialized by taskbuilder.
func splitCriteria(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if hasMultilineLiteral(s) {
		return []string{s}
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// hasMultilineLiteral detects content whose newline boundaries cannot safely be
// treated as independent criteria. Unclosed delimiters are conservative too.
func hasMultilineLiteral(s string) bool {
	if strings.Contains(s, "```") || strings.Contains(s, "~~~") {
		return true
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			return true
		}
		var quote byte
		run := 0
		for i := 0; i < len(line); i++ {
			ch := line[i]
			if quote != 0 {
				if ch == '\\' && quote != '`' {
					i++
					continue
				}
				if ch != quote {
					continue
				}
				if quote == '`' {
					n := 1
					for i+n < len(line) && line[i+n] == '`' {
						n++
					}
					i += n - 1
					if n != run {
						continue
					}
				}
				quote = 0
			} else if ch == '"' || ch == '\'' || ch == '`' {
				quote = ch
				if ch == '`' {
					run = 1
					for i+run < len(line) && line[i+run] == '`' {
						run++
					}
					i += run - 1
				}
			}
		}
		if quote != 0 {
			return true
		}
	}
	return false
}

// sameExecutionMode reports whether two execution modes are equivalent. The default
// empty mode and an explicit "implement" mode are the same behavior (run the
// implementation agent first); anything else must match exactly, so verify-first
// versus implement stays a real change.
func sameExecutionMode(a, b domain.ExecutionMode) bool {
	return effectiveExecutionMode(a) == effectiveExecutionMode(b)
}

func effectiveExecutionMode(m domain.ExecutionMode) domain.ExecutionMode {
	if m == "" {
		return domain.ExecutionImplement
	}
	return m
}

// sameStringSet reports whether a and b contain the same values as a multiset:
// order does not matter, repetition does. The previous membership-only check could
// report [X,Y] equal to [X,X], silently dropping a dependency; this compares counts
// so that can never happen.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}

// sameAcceptanceCriteria reports whether two acceptance-criteria lists express the
// same conjunction of criteria. Independent criteria are compared as a set so a
// reordering is cosmetic; sequencing within one criterion is preserved, because a
// reordered multi-step criterion can change meaning. A criterion whose prose differs
// at all (synonyms, negation, numbers, or whitespace inside code/quotes) is a real
// change.
func sameAcceptanceCriteria(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if hasMultilineLiteral(strings.Join(a, "\n")) || hasMultilineLiteral(strings.Join(b, "\n")) {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	// Match each canonicalized criterion from a to a distinct canonicalized criterion
	// in b. Duplicates are handled by consuming matches.
	remaining := make(map[string]int, len(b))
	for _, item := range b {
		remaining[canonicalProse(item)]++
	}
	for _, item := range a {
		key := canonicalProse(item)
		if remaining[key] == 0 {
			return false
		}
		remaining[key]--
	}
	return true
}

// canonicalProse collapses whitespace only in literal-free prose: when a field
// contains no quotes, backticks, or indented/fenced literals, its runs of
// whitespace are folded to a single space. A field that contains any quote,
// backtick, or indented/fenced literal is conservatively compared byte-for-byte,
// so a whitespace or line-order change inside a quoted value, path, or code
// example stays a real change rather than being guessed away as formatting. The
// byte-for-byte decision here is intentionally conservative: quote characters and
// "~~~" fences and indented lines are detected directly, while any other multiline
// literal is left to splitCriteria/hasMultilineLiteral so the comparison never
// silently normalizes code.
func canonicalProse(s string) string {
	if strings.ContainsAny(s, "`\"'") || strings.Contains(s, "~~~") {
		return s
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			return s
		}
	}
	return strings.Join(strings.Fields(s), " ")
}

// plansEquivalent reports whether a requested plan is identical to the recorded
// machine plan under the conservative comparisons: same project and summary, same
// stage set (order aside), and the same capability inventory and assumptions. A real
// Requires or inventory change is reported as changed.
func plansEquivalent(a, b *planner.Plan) bool {
	if a == nil || b == nil {
		return a == b
	}
	if canonicalProse(a.Project) != canonicalProse(b.Project) ||
		canonicalProse(a.Summary) != canonicalProse(b.Summary) ||
		len(a.Stages) != len(b.Stages) ||
		len(a.Capabilities) != len(b.Capabilities) ||
		len(a.Assumptions) != len(b.Assumptions) {
		return false
	}
	stages := make(map[string]planner.Stage, len(a.Stages))
	for _, s := range a.Stages {
		stages[s.ID] = s
	}
	for _, s := range b.Stages {
		other, ok := stages[s.ID]
		if !ok || !sameStage(other, s) {
			return false
		}
	}
	if !sameCapabilities(a.Capabilities, b.Capabilities) {
		return false
	}
	return sameAssumptions(a.Assumptions, b.Assumptions)
}

// sameStage reports whether two plan stages carry the same definition under the
// conservative comparisons, including the capabilities the stage requires. A real
// Requires change is a change; only cosmetic variance is equivalent.
func sameStage(a, b planner.Stage) bool {
	return canonicalProse(a.Title) == canonicalProse(b.Title) &&
		canonicalProse(a.Objective) == canonicalProse(b.Objective) &&
		sameStageKind(a.Kind, b.Kind) &&
		sameExecutionMode(a.ExecutionMode, b.ExecutionMode) &&
		sameAcceptanceCriteria(a.AcceptanceCriteria, b.AcceptanceCriteria) &&
		sameStringSet(a.Dependencies, b.Dependencies) &&
		sameStringSet(a.Requires, b.Requires) &&
		sameDeliverables(a.Deliverables, b.Deliverables)
}

// sameDeliverables reports whether two deliverable lists are the same multiset of
// canonically-formatted items. Deliverables are a set of independent outputs, so
// their order is not significant, but repetition and content are.
func sameDeliverables(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	remaining := make(map[string]int, len(b))
	for _, item := range b {
		remaining[canonicalProse(item)]++
	}
	for _, item := range a {
		key := canonicalProse(item)
		if remaining[key] == 0 {
			return false
		}
		remaining[key]--
	}
	return true
}

// sameCapabilities reports whether two capability inventories declare the same
// capabilities with the same status, owner, evidence, location, gap and resolution.
// Any field change is a real change; a capability that is added or dropped is a
// change too, so a missing inventory entry can never be silently ignored. Names are
// indexed exactly; a purely cosmetic rename (case/whitespace) of a declared
// capability is a real inventory change here even though validateCapabilities and
// CapabilityGaps treat the two spellings as the same requirement name, because a
// rename is a visible edit to the plan and is surfaced for review rather than
// silently normalized away.
func sameCapabilities(a, b []planner.Capability) bool {
	if len(a) != len(b) {
		return false
	}
	index := make(map[string]planner.Capability, len(a))
	for _, c := range a {
		index[c.Name] = c
	}
	for _, c := range b {
		other, ok := index[c.Name]
		if !ok {
			return false
		}
		if other.Status != c.Status ||
			canonicalProse(other.Owner) != canonicalProse(c.Owner) ||
			canonicalProse(other.Evidence) != canonicalProse(c.Evidence) ||
			canonicalProse(other.Location) != canonicalProse(c.Location) ||
			canonicalProse(other.Gap) != canonicalProse(c.Gap) ||
			canonicalProse(other.Resolution) != canonicalProse(c.Resolution) {
			return false
		}
	}
	return true
}

// sameAssumptions reports whether two assumption inventories carry the same
// assumptions with the same evidence and consequence, order aside.
func sameAssumptions(a, b []planner.Assumption) bool {
	if len(a) != len(b) {
		return false
	}
	// Assumptions carry no id: match pairs by canonical form, consuming matches so a
	// duplicate is not double-counted.
	used := make([]bool, len(b))
	for _, want := range a {
		found := false
		for j, got := range b {
			if used[j] {
				continue
			}
			if canonicalProse(want.Assumption) == canonicalProse(got.Assumption) &&
				canonicalProse(want.Evidence) == canonicalProse(got.Evidence) &&
				canonicalProse(want.Consequence) == canonicalProse(got.Consequence) {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// sameStageKind recognizes the existing default feature kind, preserving the
// meaningful distinction between feature and environment/bootstrap stages.
func sameStageKind(a, b string) bool {
	if a == "" {
		a = planner.KindFeature
	}
	if b == "" {
		b = planner.KindFeature
	}
	return a == b
}
