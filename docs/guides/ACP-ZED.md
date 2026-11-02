# SOP in Zed — Agent Client Protocol (ACP) setup and operations

This guide explains how to run SOP as an external agent in [Zed](https://zed.dev) over the Agent Client Protocol (ACP), and how to operate the sessions afterwards.

SOP stays the authority for execution, lifecycle state, verification, retries, and approvals. The editor is an interface. The existing `sop` CLI and MCP interfaces remain supported and unchanged.

## 1. What you install and what Zed launches

Install SOP so the `sop` executable is on the PATH (see `INSTALLATION.md`, `WINDOWS-INSTALLATION.md`). Zed launches the installed SOP binary with the argument `acp`:

```
sop acp
```

`sop acp` serves the pinned ACP v1 protocol over stdio. It takes no flags. It speaks the methods `initialize`, `session/new`, `session/prompt`, and `session/cancel`, and streams progress on `session/update`. Session resumption (`session/load`) is not supported in this release and is advertised as unsupported.

## 2. Add SOP as a custom external agent (`agent_servers`)

Zed custom external agents are configured under the `agent_servers` key in the Zed settings. Give the entry a name, set `command` to the installed SOP binary, and set `args` to `acp`:

```json
{
  "agent_servers": {
    "SOP": {
      "command": "/usr/local/bin/sop",
      "args": ["acp"]
    }
  }
}
```

> **Verified against Zed 1.26.2.** The `agent_servers` schema above was accepted unchanged by Zed 1.26.2 on macOS 26.6.2, and SOP appears under **External Agents** in the agent picker (`docs/reports/evidence/ACP-007-ZED-darwin/01-agent-appears.md`). Treat this as verified for that release; re-check it when you change Zed versions.

After configuration, SOP should appear in Zed's agent picker in the Agent Panel. Whether it appears there is part of the bounded acceptance in `docs/reports/ACP-007-acceptance.md`.

## 3. Provider environment inheritance

A Zed-launched `sop acp` process runs the governed work as a detached child of the SOP binary. The child inherits the full environment of the `sop acp` process — including the environment Zed passes to it — plus three job variables set by SOP (`JobEnvDir`, `JobEnvID`, `JobEnvProject` in `internal/cli/mcp_start_work.go`), which carry the job directory, job id, and project directory.

Provider endpoints are then resolved per call with the documented precedence: environment over configuration over built-in default (`internal/cli/providers.go`, `endpointOr`). The provider environment variables SOP reads are:

| Variable                | Provider  | Configuration fallback        |
| ----------------------- | --------- | ----------------------------- |
| `SOP_OLLAMA_BASE_URL`   | Ollama    | `providers.ollama.endpoint`   |
| `SOP_LLAMACPP_BASE_URL` | llama.cpp | `providers.llamacpp.endpoint` |
| `SOP_MLX_BASE_URL`      | MLX       | `providers.mlx.endpoint`      |

The llama.cpp model override is read from the agent layer's llama.cpp model environment variable first, then from `agent.model` when llama.cpp is the configured provider (`llamacppConfiguredModel` in `internal/cli/providers.go`).

Because precedence puts the environment first, a Zed session uses whatever endpoint the launching environment supplies. If providers are unreachable from the editor, check what Zed inherited: a GUI-launched Zed may see a different environment from a terminal-launched one. This difference is a known risk, recorded in the plan; it is not yet covered by a verified acceptance run (see Known limitations).

## 4. Project selection

Zed creates an ACP session with `session/new`, passing the workspace `cwd`. SOP canonicalizes and validates that directory (`internal/acp/project.go`) and binds the session to it. Every prompt, progress read, and cancellation for the session then acts on that directory only. Sessions cannot reach another project's state.

`session/new` starts no work and creates no SOP state. Governance starts with `session/prompt`, which reaches the same machinery as `sop prompt` and the MCP `sop_start_work` tool (`internal/cli/acp.go`, `acpSubmission`):

1. A short probe (250 ms) tests the project's writer lock (`internal/projectlock`): a contended probe returns a busy result instead of queueing.
2. The persisted started-work records are re-read, newest first.
3. Exactly one governed start happens through `startWork`, which serializes check-then-launch under a cross-process start lock (`.agent-sdlc/runs/mcp-work/.start.lock`) and refuses duplicates or competing running jobs.

Outcome meanings:

| Outcome            | Meaning                                                                                                                                                         |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `started`          | One work item started; the job identity is returned                                                                                                             |
| `already_running`  | The same work is already running                                                                                                                                |
| `busy`             | Another job runs, or a plan reconciliation gate applies; nothing started, nothing queued                                                                        |
| `task_in_progress` | A task is mid-run, probably driven by a `sop run` outside the editor; recover it from a terminal with `sop resume` and `sop run` if nothing is actually running |
| `reconciled`       | This work already ran or is the active plan; pass the rerun control to start it again                                                                           |
| `not_initialized`  | The project has no SOP state yet; nothing started. Initialize the project's SOP state first (see `PROJECT-SETUP.md`), then submit again                         |

## 5. Logs and diagnostics

Each governed job keeps its records under `.agent-sdlc/runs/mcp-work/<job-id>/` in the project:

- `job.json` — job identity, fingerprint, args, pid, start time
- `output.log` — the job subprocess's stdout and stderr
- `exit.json` — how the process ended, when observed
- `prompt.md` — the submitted prompt text

Job ids look like `work-YYYYMMDD-HHMMSS(-N)`. Inspect jobs through the MCP `sop_work_status` tool, the `sop status` CLI surface, or the files directly. `output.log` is evidence; treat it as append-only.

Protocol diagnostics for the ACP stream itself go to the stderr of the `sop acp` process, prefixed `acp: `, with credential-shaped values redacted (`internal/acp/diagnostics.go`). stdout never carries diagnostics; it is protocol-only. Where Zed exposes that stderr is a Zed-side behavior and is part of what acceptance records; until then, run `sop acp` from a terminal against the same project when you need to see diagnostics directly.

## 6. Blockers

SOP reports structured outcomes on `session/update` as payload fields; they never change the protocol stop reasons. A blocker means SOP stopped governed work at a gate — a human approval gate, a plan reconciliation gate, a verification failure, or a busy project — and persisted that state.

- Approval blockers: execution stops at the gate and stays pending. An ACP/UI permission grant is not an SOP approval. Resolve approvals through the SOP approval interface (`docs/guides/APPROVALS.md`), then submit again; the adapter re-reads authoritative state and follows the existing continuation path.
- An unchanged blocker stops without consuming retries. Repeated prompts against it cannot consume retry budget.
- Busy-style outcomes (`busy`, `task_in_progress`, `already_running`) are listed in section 4. They block a new start, not an existing execution.

A prompt response's stop reason describes the prompt turn only. `end_turn` ends a finished turn; `cancelled` marks a turn stopped by cancellation. Only persisted SOP outcomes decide whether work succeeded, and the progress frames carry those outcomes as fields.

## 7. Cancellation

Cancel a session's foreground work with `session/cancel` (`internal/acp/cancel.go`). SOP cancels exactly the named started job's child process through the supported SOP cancellation path (`internal/cli/work_cancel.go`), records the cancellation in that job's own record, and answers with outcome `cancelled`, the job id, and stop reason `cancelled`.

- Cancellation stops within the documented stop bound (`CancelStopBound` in `internal/acp/cancel.go`); partial evidence is preserved through existing lifecycle transitions.
- Cancellation touches nothing the adapter did not start: CLI-run or MCP-run work, approval gates, verification, and retries stay SOP-owned.
- A cancellation is recorded distinctly from failure and from completion. A later `exit.json` never relabels a confirmed cancellation as failed.
- Cancellation racing normal completion yields exactly one authoritative recorded result.

## 8. Recovery

- **Editor disconnect / EOF.** When the Zed side closes, `sop acp` requests safe cancellation of the session's foreground work within the stop bound, re-reads the persisted job records, and exits cleanly. No work restarts automatically, and no completion is fabricated.
- **Ambiguous start.** Before any new submission, the submission bridge reconciles against the persisted started-work records; a persisted prior attempt for the same identity is returned (`reconciled`) instead of starting duplicate work.
- **Restart.** After a `sop acp` or editor restart, existing jobs are discovered from `.agent-sdlc/runs/mcp-work/` through the same persisted records. Session history is not restored (`session/load` unsupported), and no work is resubmitted automatically. Use `sop status` and `sop_work_status` to reconcile, and the SOP CLI to resume or re-run deliberately.
- **Rollback.** Reconcile active ACP-started jobs through SOP, remove the `agent_servers` entry from the Zed settings, and return to CLI/MCP. Preserve task history and evidence; do not reset lifecycle state or delete worktrees.

## 9. Known limitations

- **Zed version and `agent_servers` schema.** Verified against Zed 1.26.2 on macOS 26.6.2: the section 2 example is accepted unchanged and SOP appears under External Agents (`docs/reports/evidence/ACP-007-ZED-darwin/01-agent-appears.md`). A Zed-driven bounded request has not yet been run, so Zed-side operational acceptance remains open.
- **macOS acceptance.** Scenario 1 is verified in Zed 1.26.2 (SOP in the agent picker) and scenarios 2–5 are recorded on macOS, with 2, 4 and 5 executed through an ACP v1 stdio client. A bounded request submitted from Zed itself is still outstanding; `docs/reports/ACP-007-acceptance.md` carries the status and evidence paths.
- **Linux acceptance.** Status and evidence paths are recorded in `docs/reports/ACP-007-acceptance.md`; any scenario not yet executed is recorded there as outstanding, never claimed.
- **No session resumption.** `session/load` is unsupported; conversation history is not restored after restart.
- **Text prompts only.** Prompt content is bounded text; attachments, model selection UI, and editor-mediated file writes or terminal execution are deferred per the plan.
- **GUI-launched Zed environment.** The provider environment a GUI-launched Zed supplies may differ from a terminal environment (section 3); this is documented but not yet verified by a real-Zed run.

## References

- Plan: `docs/plans/PLAN-SOP-ACP-ZED.md`
- Discovery note: `docs/reports/ACP-007-discovery.md`
- Validation evidence: `docs/reports/ACP-007-validation.md`
- Acceptance record: `docs/reports/ACP-007-acceptance.md`
- Approvals: `docs/guides/APPROVALS.md`; project setup: `docs/guides/PROJECT-SETUP.md`
- Zed external agents: https://zed.dev/docs/ai/external-agents; ACP overview: https://agentclientprotocol.com/protocol/v1/overview
