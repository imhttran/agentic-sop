// Package skill describes and validates the shipped SOP agent skill surface: the
// canonical skill tree under skills/, and the per-agent adapters that install it.
//
// The command surface below is the single owner of "which SOP command maps to which
// capability". The shipped skills, the per-agent installers, and the Claude Code plugin
// package are all validated against it, so a command cannot be added, renamed, or
// remapped in one place and silently drift in another.
package skill

// Command is one command in the installed SOP surface.
type Command struct {
	// Name is the skill folder under skills/, and the name of the command the agent
	// exposes for it.
	Name string
	// Capability is the capability the command fixes, or "" when it fixes none: the
	// CLI's own default applies; end-to-end execution delegates to run instead.
	Capability string
	// Delegation is the exact text the command's SKILL.md must contain — the call it
	// makes into the SOP CLI. A command that fixes no capability must not embed one.
	Delegation string
	// RunsProjectTests marks a command whose shell examples are the target project's
	// own test-runner commands (for example Playwright) rather than `sop` invocations:
	// it runs and maintains a project's persistent browser suite. Every other command
	// stays a thin SOP CLI client, so the shell-example invariant is waived only here.
	RunsProjectTests bool
	// UsesRepositoryScripts marks a command that drives this repository's
	// deterministic helper scripts (for example the MCP candidate lifecycle) rather
	// than the SOP CLI. It still participates in the canonical surface, installer,
	// fail-closed, and contract checks; only the SOP-only shell-example invariant is
	// waived, because its commands are the helper scripts, Git, Go, and local checks.
	UsesRepositoryScripts bool
	// ResolvesMergeConflicts marks the only command allowed to stage a resolved path
	// and complete an in-progress merge. A conflict resolution exists only in the git
	// index, so it cannot be expressed as an SOP capability; the direct-mutation check
	// is narrowed to exactly `git add` and `git commit` for this command, and a
	// dedicated test forbids publishing and history rewrites. Every other command
	// keeps the full direct-mutation boundary.
	ResolvesMergeConflicts bool
}

// Commands is the canonical SOP command surface. Every supported agent installs
// exactly these; the canonical skill tree ships exactly one folder per entry.
var Commands = []Command{
	{Name: "sop", Capability: "", Delegation: "sop prompt --capability"},
	{Name: "sop-prompt", Capability: "", Delegation: `sop prompt "`},
	{Name: "sop-plan", Capability: "plan", Delegation: "sop prompt --capability plan"},
	{Name: "sop-review", Capability: "review", Delegation: "sop prompt --capability review"},
	{Name: "sop-diagnose", Capability: "diagnose_failure", Delegation: "sop prompt --capability diagnose_failure"},
	{Name: "sop-test", Capability: "design_tests", Delegation: "sop prompt --capability design_tests"},
	{Name: "sop-implement", Capability: "implement", Delegation: "sop prompt --capability implement"},
	{Name: "sop-end-to-end", Capability: "", Delegation: "sop run"},
	{Name: "sop-historicalize", Capability: "", Delegation: "sop plan historicalize"},
	{Name: "sop-continue", Capability: "", Delegation: "sop continue"},
	// A verification companion to /sop-end-to-end: it runs and maintains a project's
	// persistent Playwright suite and reads SOP's approval state; it fixes no capability.
	{Name: "sop-browser-test", Capability: "", Delegation: "sop approvals --json", RunsProjectTests: true},
	// A repository-level MCP candidate workflow: it prepares, verifies, deploys, and
	// rolls back an MCP fix through the deterministic candidate helper, then hands the
	// live test to an independent validator. It fixes no capability and never treats
	// local tests as independent validation. Its shell examples are repository
	// helpers, Git, Go, and local checks, not SOP CLI calls.
	{Name: "mcp-live-validation", Capability: "", Delegation: "sop status", UsesRepositoryScripts: true},
	// A report-only security companion: it detects repository content, delegates
	// scanning to SOP's configured adapters, and reports sanitized evidence. It
	// fixes no capability and never mutates the repository or scanner policy.
	{Name: "sop-security-scan", Capability: "", Delegation: "sop security scan"},
	// A Codex review-triage companion: it fetches unresolved Codex findings from a
	// GitHub pull request, verifies each one, fixes in-scope findings, and records
	// deferred ones in the repository backlog. It fixes no capability and never
	// publishes or merges. Its shell examples are repository helpers, GitHub CLI
	// calls, Git, and local checks, not SOP CLI calls.
	{Name: "codex-review-triage", Capability: "", Delegation: "sop status", UsesRepositoryScripts: true},

	// RAG engineering aliases: domain skills that add a pgvector/RAG checklist to an
	// existing read-only capability. They fix a read-only capability only, so they can
	// never enter the governed implementation lifecycle or mutate the repository.
	{Name: "rag-schema", Capability: "plan", Delegation: "sop prompt --capability plan"},
	{Name: "rag-test", Capability: "design_tests", Delegation: "sop prompt --capability design_tests"},
	{Name: "rag-ingest", Capability: "plan", Delegation: "sop prompt --capability plan"},
	// A RAG-integration advisor: it fixes the read-only plan capability and adds a
	// RAG-integration methodology (discovery, suitability, technology selection, a scoped
	// plan, verification), delegating the governed apply/verify/publish steps to
	// /sop-implement, sop run, and /sop-publish. It never edits the target repository.
	{Name: "sop-rag-integrate", Capability: "plan", Delegation: "sop prompt --capability plan"},
	{Name: "sop-cleanup", Capability: "", Delegation: "sop status"},
	// A read-only "what should I do next" advisor with prepare and execute modes:
	// it inspects SOP's own surfaces, delegates to sop run only when authorized, and
	// fixes no capability.
	{Name: "sop-next-action", Capability: "", Delegation: "sop status"},
	// The completion half of a merge/publish: it verifies the published target,
	// historicalizes eligible plans through SOP, and removes only fully-merged,
	// clean worktrees and branches. It fixes no capability; historicalization is
	// the governed SOP call it delegates.
	{Name: "sop-publish", Capability: "", Delegation: "sop plan historicalize"},
	// A merge-conflict resolver for an in-progress merge: it reads both sides, edits
	// only the conflicted paths, resolves a hunk only when both sides' intent can be
	// preserved, verifies the merged result, and completes the merge locally. It is
	// the one command allowed to stage and complete the merge because that result
	// lives in the git index, not in a capability; it never publishes or rewrites
	// history.
	{Name: "sop-fix-conflicts", Capability: "", Delegation: "sop status", ResolvesMergeConflicts: true},
}

// Capabilities is the operator-facing capability vocabulary the CLI accepts
// (internal/cli/prompt.go). A documented --capability value outside this set is a
// documentation error, not a new capability.
var Capabilities = map[string]bool{
	"plan":             true,
	"design_tests":     true,
	"diagnose_failure": true,
	"review":           true,
	"implement":        true,
}

// ReadOnlyCapabilities are the capabilities that never mutate the repository: each
// runs one bounded model call and can execute on a text-only provider. Every
// capability not listed here mutates the repository and must run the governed
// implementation lifecycle.
var ReadOnlyCapabilities = map[string]bool{
	"plan":             true,
	"design_tests":     true,
	"diagnose_failure": true,
	"review":           true,
}

// ProviderBinaries are provider runtimes a shipped adapter must never invoke: SOP
// resolves the provider and the model. A line that *names* one in prose is not an
// invocation; the validators look for one in command position.
var ProviderBinaries = []string{"ollama", "llama-server", "llama.cpp", "llamacpp", "openrouter", "mlx_lm"}

// PolicyKnobs are SOP configuration settings that belong in the reference docs, not in a
// shipped adapter: their presence would mean the adapter is restating SOP policy.
var PolicyKnobs = []string{
	"SOP_MODEL_ROUTING_ENABLED",
	"SOP_MODEL_ESCALATION_ENABLED",
	"SOP_MODEL_MAX_ESCALATIONS",
	"max_fix_cycles",
	"fail_on",
}

// ModelLiterals are model names that must never appear in a shipped adapter: an
// adapter selects a capability, never a model.
var ModelLiterals = []string{
	"qwen", "glm-", "deepseek", "mlx-community/", "gpt-", "claude-3", "llama-3",
}

// MutationCommands are direct-mutation instructions an adapter must not contain: an
// implement request goes through `sop prompt --capability implement`, not the shell.
var MutationCommands = []string{"git commit", "git push", "git add", "sed -i", "rm -rf"}

// MergeConflictIndexCommands are the only direct-mutation instructions a skill may
// contain, and only for a command whose Command.ResolvesMergeConflicts is set:
// staging a resolved path and completing an in-progress merge. They cannot be
// expressed as an SOP capability because the resolution is the merge result itself.
var MergeConflictIndexCommands = map[string]bool{
	"git add":    true,
	"git commit": true,
}

// AllowsDirectMutation reports whether the named command may contain instruction.
// Only the merge-conflict resolver is exempt, and only for the index operations that
// complete a merge; every other command keeps the full direct-mutation boundary.
func AllowsDirectMutation(name, instruction string) bool {
	if !MergeConflictIndexCommands[instruction] {
		return false
	}
	for _, c := range Commands {
		if c.Name == name {
			return c.ResolvesMergeConflicts
		}
	}
	return false
}
