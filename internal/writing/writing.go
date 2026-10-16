// Package writing owns SOP's single shared writing rule.
//
// The rule governs the two places SOP produces prose: the technical prompts SOP
// generates for a model, and the human-facing status and report responses SOP
// prints. It is defined once here so no prompt builder or response renderer
// restates — and drifts from — it.
//
// The rule is a style target, not policy: it never overrides a task
// requirement, an acceptance criterion, a required structured output contract,
// or an approval condition.
package writing

// Rule is the shared writing rule, verbatim. Prompt builders carry it into a
// generated technical prompt (see PromptBlock); the human-facing status and
// report renderers implement its response clauses (lead with the outcome, then
// verification, blockers, and the next action; omit empty sections; do not
// repeat the work log) directly.
//
// It targets roughly 80% ASD-STE100 Simplified Technical English as a style
// goal, not a compliance claim.
const Rule = "Write short sentences in the active voice. Use one term for each " +
	"concept. Give one action per step. Aim for roughly 80% ASD-STE100 " +
	"Simplified Technical English as a style target, not a compliance claim. " +
	"Preserve task requirements, acceptance criteria, commands, paths, " +
	"configuration keys, identifiers, evidence, and approval conditions. For a " +
	"human-facing response, lead with the outcome, then state verification, " +
	"blockers, and the next action when they apply; omit empty sections and do " +
	"not repeat the work log."

// Heading is the stable heading the rule renders under in a generated prompt.
const Heading = "Writing rule"

// PromptBlock renders Rule as a labelled instruction block for a model prompt.
// It is the one place a prompt builder pulls the rule from, so every generated
// prompt shares a single definition.
func PromptBlock() string {
	return "## " + Heading + "\n" + Rule + "\n"
}
