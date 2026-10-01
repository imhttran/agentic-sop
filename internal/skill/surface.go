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
	// CLI's own conservative (read-only) default then applies.
	Capability string
	// Delegation is the exact text the command's SKILL.md must contain — the call it
	// makes into the SOP CLI. A command that fixes no capability must not embed one.
	Delegation string
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
