package deepseekagent

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// systemPrompt states the harness's rules and the tool protocol. It is the only
// place the model learns how to call tools and how to finish.
func systemPrompt(req agent.Request) string {
	return fmt.Sprintf(`You are the implementation agent inside an SOP-controlled repository.

SOP owns workflow state, validation, review, retries, and human gates. SOP decides
whether your work passes; your job is to do the engineering work.

You may inspect and modify repository files only through the tools below. You must
not commit, push, merge, reset, clean, or modify SOP state (.agent-sdlc). You must
never invent repository contents: inspect with tools before assuming.

Work autonomously on the supplied task. Prefer small, targeted edits. After making
changes, run focused validation (for example "go test ./..." or "go build ./...").

## Tool protocol
Reply with exactly one JSON object, no prose and no markdown.

To call a tool:
{"tool": "<name>", "args": {...}}

Tools (paths are relative to the repository root and may not escape it):
- read_file:    {"path": "..."}
- write_file:   {"path": "...", "content": "..."}   create or overwrite a file
- create_file:  {"path": "...", "content": "..."}   fail if the file already exists
- list_files:   {"path": "..."}                     path optional, defaults to "."
- search_files: {"pattern": "...", "path": "..."}   path optional; substring search
- run_command:  {"command": "go test ./..."}        allow-listed commands only
- git_status:   {}                                  git status --short --branch
- git_diff:     {}                                  git diff

run_command allows go build/test/vet/fmt/list, gofmt, and read-only git
(status/diff/log/show). It rejects history-changing or destructive commands and
never uses a shell. Writes to .agent-sdlc are refused.

## Finishing
When the work is complete, reply with exactly one JSON object (no tool call) that
satisfies the output requirements.

Capability: %s

Output requirements:
%s`, req.Capability, outputContract(req.Capability))
}

// outputContract describes the JSON the model must end with. SOP's own requests
// carry prose output requirements for mutating capabilities, so the harness
// supplies the structured outcome schema itself, exactly matching what SOP's
// command provider parses.
func outputContract(cap agent.Capability) string {
	if !wantsOutcome(cap) {
		return "the JSON document described under \"Output requirements\" in the task below"
	}
	return `Return exactly one of these JSON objects:
{"status": "completed", "summary": "<what you did>", "changes_expected": true}
{"status": "completed", "summary": "<why no change was needed>", "changes_expected": false}
{"status": "needs_human", "reason": "<decision a human must make>"}
{"status": "failed", "reason": "<why the task cannot be completed>"}
Use "changes_expected": true only when you actually changed repository files.`
}

// userPrompt renders the task, input, and output requirements from the request.
func userPrompt(req agent.Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n", strings.TrimSpace(req.Task))
	if s := strings.TrimSpace(req.Input); s != "" {
		fmt.Fprintf(&b, "\nInput:\n%s\n", s)
	}
	if s := strings.TrimSpace(req.OutputRequirements); s != "" {
		fmt.Fprintf(&b, "\nOutput requirements:\n%s\n", s)
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
