package ollamaagent

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// systemPrompt states the harness's rules, the capability's policy, the tool
// protocol, and how to finish. It is the only place the model learns which tools
// it may call and how to conclude.
func systemPrompt(req agent.Request, policy CapabilityPolicy) string {
	var b strings.Builder
	b.WriteString(`You are the implementation agent inside an SOP-controlled repository.

SOP owns workflow state, validation, review, retries, and human gates. SOP decides
whether your work passes; your job is to do the engineering work.

You may inspect and modify repository files only through the tools below. You must
not commit, push, merge, reset, clean, or modify SOP state (.agent-sdlc). You must
never invent repository contents: inspect with tools before assuming.

The working tree may already contain changes that are not part of this task. Treat
them as user-owned: preserve them, do not revert, discard, or "fix" them merely
because they look unrelated, and do not report them as blocking. Only touch files
that the task actually requires.
`)

	b.WriteString("\n## Tool protocol\n")
	fmt.Fprintf(&b, "Available tools for %s: %s.\n", req.Capability, describeTools(policy))
	b.WriteString(toolReference(policy))
	b.WriteString(`
Reply with exactly one JSON object, no prose and no markdown. To call a tool:
{"tool": "<name>", "args": {...}}
`)

	b.WriteString("\n## This capability\n")
	b.WriteString(capabilityGuidance(req.Capability))

	fmt.Fprintf(&b, "\n\nCapability: %s\n\nOutput requirements:\n%s", req.Capability, outputContract(req.Capability))
	return b.String()
}

// toolReference documents only the tools the capability policy allows, so a
// read-only capability is not tempted by a mutation tool it could not use.
func toolReference(policy CapabilityPolicy) string {
	var b strings.Builder
	b.WriteString("Tools (paths are relative to the repository root and may not escape it):\n")
	for _, name := range policy.ToolList() {
		fmt.Fprintf(&b, "- %s: %s\n", name, toolUsage[name])
	}
	if policy.ReadOnly {
		b.WriteString("This capability is read-only: it may not create or change repository files.\n")
	}
	return b.String()
}

// toolUsage documents each tool's arguments in one line.
var toolUsage = map[string]string{
	toolharness.ToolReadFile:    `{"path": "..."}                        read a file`,
	toolharness.ToolWriteFile:   `{"path": "...", "content": "..."}   create or overwrite a file`,
	toolharness.ToolCreateFile:  `{"path": "...", "content": "..."}   fail if the file already exists`,
	toolharness.ToolListFiles:   `{"path": "..."}                     path optional, defaults to "."`,
	toolharness.ToolSearchFiles: `{"pattern": "...", "path": "..."}   path optional; substring search`,
	toolharness.ToolRunCommand:  `{"command": "go test ./..."}        allow-listed commands only`,
	toolharness.ToolGitStatus:   `{}                                  git status --short --branch`,
	toolharness.ToolGitDiff:     `{}                                  git diff`,
}

// capabilityGuidance tells the model how to finish the specific capability,
// especially that it does not own final validation: SOP does. Keeping the model
// from trying to prove every acceptance criterion itself is what lets it return
// before the iteration budget.
func capabilityGuidance(c agent.Capability) string {
	switch c {
	case agent.Implement, agent.Fix:
		return `Implement the requested change using the controlled tools.

For IMPLEMENT, work in three phases: discover enough context to make the change,
make the change, then finalize. You will be told when to stop exploring and
return your outcome; if you have gathered context but not yet made the change,
you will be told to implement and keep your tools. Do not keep working to prove
every acceptance criterion yourself.

When the implementation is complete, return the required final response immediately.

For a report deliverable, write the requested report using the current caller
observations and permitted inspections. Missing optional facts must be labelled
unavailable with a reason; never invent required evidence. Do not spend discovery
turns repeating observations already supplied by the caller, or attempt commands
outside the allow-list to rediscover facts the caller has supplied. An explicitly
requested Markdown report is an implementation mutation: create/update it through
the file tools, preserving unrelated content, rather than only describing it.

SOP performs independent validation and review after you return. Prefer small,
targeted edits, and run focused validation (for example "go test ./...").

Exception: when the task supplies an already-satisfied verification contract,
you may prove the existing implementation using its required validations and
inspected files, then explicitly return ALREADY_SATISFIED. Do not manufacture
an edit to satisfy the mutation requirement. The ordinary progress bounds still
apply, and SOP independently verifies the proof.`
	case agent.Plan:
		return `This is PLAN, not IMPLEMENT.

Explore the repository with the read-only tools only as far as you need to formulate
the plan; the task Input usually already contains the material you need. Discovery
is bounded, and when it ends you will be told to synthesize — so return the
required structured PLAN response as soon as the context is sufficient rather than
continuing optional exploration. Do not modify files. SOP performs independent
validation and review after you return.

When the work integrates with or extends an existing system, use discovery to
check that the capabilities the plan depends on actually exist before you commit
to tasks: application/service APIs, CLI commands, interfaces, persistence
boundaries, scheduler/lifecycle ownership, external adapters, provider
capabilities, authorization/approval operations, and existing read/write
operations. Record each as EXISTS, PARTIAL, MISSING, or UNKNOWN with the evidence
you saw. Never plan a task as if a missing or unknown capability already exists;
when the requested architecture already determines who owns a missing capability,
record the gap and keep the work that is possible instead of stopping.`
	case agent.Review:
		return `This is REVIEW. The change under review is summarised in the Input; inspect the
repository only if the Input is insufficient. Do not modify files. Return the
required structured review response as soon as you have enough information.`
	case agent.DesignTests:
		return `This is DESIGN_TESTS. Produce the requested test strategy from the Input, inspecting
existing code and tests only as needed. Do not modify files. Return the required
final response as soon as you have enough information.`
	case agent.DiagnoseFailure:
		return `This is DIAGNOSE_FAILURE. Reproduce and diagnose the reported failure with
read-only tools. Do not modify files. Return the required final response once the
diagnosis is clear.`
	default:
		return `Complete the requested capability, then return the required final response.`
	}
}

// outputContract describes the JSON the model must end with. SOP's own requests
// carry prose output requirements for mutating capabilities, so the harness
// supplies the structured outcome schema itself, exactly matching what SOP's
// command provider parses. PLAN and REVIEW carry their own document schema.
func outputContract(cap agent.Capability) string {
	if !wantsOutcome(cap) {
		return "the JSON document described under \"Output requirements\" in the task below"
	}
	contract := `Return exactly one of these JSON objects:
{"status": "completed", "summary": "<what you did>", "changes_expected": true}
{"status": "completed", "summary": "<why no change was needed>", "changes_expected": false}
{"status": "needs_human", "reason": "<decision a human must make>"}
{"status": "failed", "reason": "<why the task cannot be completed>"}
Use "changes_expected": true only when you actually changed repository files.`
	if cap == agent.Implement || cap == agent.Fix {
		contract += `
With a supplied already-satisfied verification contract, a completed outcome may
add "completion":"ALREADY_SATISFIED" and "evidence":{"acceptance":[{"criterion":"<exact criterion>","paths":["<inspected file>"]}]}, with changes_expected=false. This requires every supplied validation to pass and every criterion to have concrete inspected evidence.`
	}
	return contract
}

// userPrompt renders the task, input, and output requirements from the request.
func userPrompt(req agent.Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n", strings.TrimSpace(req.Task))
	if len(req.Deliverables) > 0 {
		fmt.Fprintf(&b, "\n%s", deliverableRequirement(req.Deliverables))
	}
	if s := strings.TrimSpace(req.Input); s != "" {
		fmt.Fprintf(&b, "\nInput:\n%s\n", s)
	}
	if s := strings.TrimSpace(req.OutputRequirements); s != "" {
		fmt.Fprintf(&b, "\nOutput requirements:\n%s\n", s)
	}
	if (req.Capability == agent.Implement || req.Capability == agent.Fix) && len(req.AcceptanceCriteria) > 0 && len(req.ValidationCommands) > 0 {
		fmt.Fprintf(&b, "\nAlready-satisfied verification contract:\nAcceptance criteria: %q\nRequired validation commands: %q\n", req.AcceptanceCriteria, req.ValidationCommands)
		b.WriteString(`If the implementation already exists, read the relevant files and run EVERY required validation command successfully. Then explicitly return {"status":"completed","completion":"ALREADY_SATISFIED","changes_expected":false,"evidence":{"acceptance":[{"criterion":"<exact supplied criterion>","paths":["<successfully read repository file>"]}]}}. Map EVERY criterion to concrete inspected files. SOP verifies tool results and unchanged repository state. Reads, narration, no-op writes, and model assertions alone cannot establish completion. Existing discovery, stale, and iteration bounds still apply.`)
	}
	return b.String()
}

// toolResultMessage renders a tool outcome for the next model turn. A tool error
// is reported to the model (not fatal): the model can correct the call.
func toolResultMessage(name, result string, err error) string {
	if err != nil {
		return fmt.Sprintf("Tool %q failed: %s\n\nContinue: fix the call or choose another tool.", name, err)
	}
	return fmt.Sprintf("Tool %q result:\n%s", name, result)
}
