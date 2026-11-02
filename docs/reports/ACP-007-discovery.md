# ACP-007 — Discovery: ACP boundaries, Zed contract and governance

Task: ACP-007 of [PLAN-SOP-ACP-ZED.md](../plans/PLAN-SOP-ACP-ZED.md).

This note records what ACP-007 inspected before writing the guide. Facts are labeled: OBSERVED (read from repository files in this invocation), UNVERIFIED (file present, contract not re-read in this invocation), or UNKNOWN with a reason.

## 1. ACP lifecycle seams, with files and symbols

All entries in this table are OBSERVED at the revision tested in [ACP-007-validation.md](ACP-007-validation.md), unless marked otherwise.

| Seam | Files and symbols |
| --- | --- |
| Subcommand and dispatch | `internal/cli/cli.go` `case "acp"`; `internal/cli/acp.go` `runACP`, usage line `usage: sop acp`, no flags |
| Server construction | `internal/cli/acp.go` `runACP` builds `acp.NewServer(Version, stdout, stderr, acp.WithPromptSubmission(...), acp.WithProgress(...))` |
| Protocol contract | `internal/acp/contract.go` — pinned `ProtocolVersion = "v1"`, methods `initialize`, `session/new`, `session/prompt`, `session/cancel`; `session/load` rejected with `CodeCapabilityUnsupported` because `loadSession` is advertised false; `MaxFrameSize` 1 MiB; `EventBufferBound` 64; stop reasons `end_turn` and `cancelled` emitted, `max_tokens`, `max_turn_requests`, `refusal` unused vocabulary |
| Initialize | `internal/acp/initialize.go`; negotiation accepts unset/null, string `"v1"`, number `1`; anything else rejected with `CodeVersionUnsupported` |
| Session creation | `internal/acp/session.go`; `session/new` params `{cwd, mcpServers}` decoded strictly; the session record is in memory only |
| Prompt submission | `internal/acp/prompt.go` (`PromptSubmission`, `Submission`, `SubmissionResult`); `internal/cli/acp.go` `acpSubmission` wires `LockTaker`, `StateReader`, `Starter`, `Cancel` |
| Progress | `internal/acp/progress.go` (`ProgressSource`, `ProgressSnapshot`); `internal/cli/acp.go` `acpProgress` reads persisted job records; `internal/acp/delivery.go` buffers and coalesces within `EventBufferBound` and preserves terminal outcomes |
| Cancellation | `internal/acp/cancel.go`; response outcome `cancelled` with stop reason `StopReasonCancelled`; `CancelStopBound` is the documented stop bound (symbol cited; numeric value read in `internal/acp/cancel.go`) |
| Approval | `internal/acp/approval.go`; approval blockers and planner questions ride `session/update` payload fields; an unchanged blocker stops without consuming retries |
| Diagnostics | `internal/acp/diagnostics.go` — every diagnostic goes to stderr, prefixed `acp: `, with credential redaction; stdout stays protocol-only through `internal/acp/writer.go` |
| Project lock probe | `internal/cli/acp.go` `LockTaker` calls `projectlock.AcquireWait(ctx, projectDir, acpBusyProbeTimeout)` with `acpBusyProbeTimeout = 250ms`; a contended probe maps to `acp.ErrProjectBusy`; the probe is released at once, it is not the start exclusion |
| Start exclusion | `internal/cli/mcp_start_work.go` `startWork` takes the cross-process start lock through `acquireWorkLock` (`workLockFile` named `.start.lock` under `workRoot` = `.agent-sdlc/runs/mcp-work/`, stale after `workLockStale` = 2 minutes); duplicate and competing running jobs are refused while it is held |
| Persisted job records | `internal/cli/mcp_start_work.go` `listWorkJobs`, `workJob`, `workExit`; states `running`, `completed`, `failed`, `ended`, `cancelled`; job id shape `work-YYYYMMDD-HHMMSS[-N]`; job files `job.json`, `exit.json`, `output.log`, `prompt.md` under `.agent-sdlc/runs/mcp-work/<job-id>/` |
| Cancellation recording | `internal/cli/work_cancel.go` records the cancellation in the job's own record; `internal/cli/mcp_start_work.go` `listWorkJobs` treats a confirmed cancellation as authoritative over a later `exit.json` |
| Outcomes | `internal/cli/acp.go` `acpStartWorkResult` maps `started`, `already_running`, `busy`, `task_in_progress`, `plan_blocked`→`busy`, `already_ran`→`reconciled`, `in_memory`→`reconciled`, `not_initialized` |

## 2. Provider environment source (S2)

The provider environment is resolved at the CLI layer, not inside `internal/acp`:

- `internal/cli/acp.go` `runACP` calls `defaultWorkEnv()` (or the injected `d.acpWorkEnv()` in tests) and passes the resulting `workEnv` into both `acpSubmission` and `acpProgress`.
- `internal/cli/mcp_start_work.go` `launchWorkProcess` starts the governed work child with `os.Environ()` plus `JobEnvDir`, `JobEnvID`, and `JobEnvProject` (values defined in the CLI job-env constants). The child therefore inherits the whole environment of the `sop acp` process — including whatever Zed passed to it — plus the three job variables.
- Provider endpoint resolution happens when the governed work child builds its provider registry: `internal/cli/providers.go` `newProviderRegistry` and `endpointOr` apply the documented precedence environment over configuration over built-in default.

Provider environment variables OBSERVED in this invocation:

| Variable, read where | Source |
| --- | --- |
| `SOP_OLLAMA_BASE_URL` | `internal/provider/ollama/provider.go` `EnvBaseURL` via `endpointOr(cfg.Providers.Ollama.Endpoint, ...)` |
| `SOP_LLAMACPP_BASE_URL` | `internal/provider/llamacpp/provider.go` `EnvBaseURL` via `endpointOr(cfg.Providers.LlamaCpp.Endpoint, ...)` |
| `SOP_MLX_BASE_URL` | `internal/provider/mlx/provider.go` `EnvBaseURL` via `endpointOr(cfg.Providers.MLX.Endpoint, ...)` |
| llama.cpp model override | read in `internal/cli/providers.go` `llamacppConfiguredModel` from the agent layer's llama.cpp model environment constant, then `agent.model` config when llama.cpp is the configured provider |

Environment variable names listed in `internal/cli/locality.go` were not re-read in this invocation and stay UNVERIFIED here; the guide lists only the three `*_BASE_URL` names verified above and the config keys `providers.ollama.endpoint`, `providers.llamacpp.endpoint`, `providers.mlx.endpoint`, `agent.provider`, `agent.model` from `internal/cli/providers.go`.

## 3. Project selection and the `not_initialized` outcome (S2)

- `session/new` binds the session to the client's `cwd`, canonicalized and validated in `internal/acp/project.go`. The session record keeps the project directory; prompts, progress reads, and cancellation all operate on that directory. Cross-project sessions stay isolated.
- `internal/cli/acp.go` `acpSubmission` then reads persisted started-work records with `listWorkJobs(projectDir, env)` (`StateReader`) and starts exactly once with `startWork(sub.ProjectDir, env, startWorkArgs{Prompt, Capability})` (`Starter`).
- When the project has no SOP state (`statePath(dir)` missing), `internal/cli/mcp_start_work.go` `startWork` returns the outcome `not_initialized` with the `notInitializedMsg` message, mapped by `internal/cli/acp.go` to `acp.WorkOutcomeNotInitialized`. No work starts. The operator initializes the project's SOP state first (`sop init`; see `docs/guides/PROJECT-SETUP.md`), then submits again. The exact message text was not re-quoted in this note.

## 4. Zed `agent_servers` schema and supported Zed version

UNKNOWN, with reason and consequence:

- No Zed application, Zed version pin, or `agent_servers` evidence exists in this repository (searched `docs/` in this invocation; confirmed in the plan's inspection record).
- The installed Zed could not be probed in this task invocation; network checks are not available to it.
- Consequence, per the plan: no configuration example is published as verified in `docs/guides/ACP-ZED.md`. The guide carries a provisional example labeled UNVERIFIED, and the supported Zed version is recorded as UNKNOWN pending a real-Zed acceptance run. Acceptance stays outstanding until executed against Zed.
- Designated verification source: https://zed.dev/docs/ai/external-agents and the installed Zed release on the acceptance hosts (see [ACP-007-acceptance.md](ACP-007-acceptance.md)).

## 5. PR, merge and release governance steps (S1)

OBSERVED governance facts:

- `.github/` and `docs/tasks/`, `docs/plans/`, `docs/reports/` exist; the repository runs a plan-driven workflow with SOP-controlled validation, review, retries, and human gates.
- The implementation stage does not commit, push, merge, or reset. The release PR is opened through the established PR workflow by the maintainer/operator; review and verification belong to SOP's review stage and human gates.

The release PR must follow these steps, in order:

1. Complete the change in the working tree: `docs/guides/ACP-ZED.md`, the three ACP-007 reports, with `internal/acp/` unchanged in its final documented state.
2. Run the acceptance checks exactly once for the final change: `go test ./internal/acp/...` and `go build ./...`, plus the required repository checks `go test ./...` and `go vet ./...` once each; record results and the revision in [ACP-007-validation.md](ACP-007-validation.md).
3. Open one PR whose description states scope, the declared repository paths touched, CLI/ACP compatibility results, lifecycle decisions (cancellation, blockers, recovery), links to the validation and acceptance reports, and known limitations (including unverified Zed schema and macOS acceptance as outstanding). A description draft is in [ACP-007-PR-CHECKLIST.md](ACP-007-PR-CHECKLIST.md).
4. Reviewer approval and merge follow the repository's established review gates; `.github/` files were not read in this invocation, so concrete reviewer/check requirements are recorded as a gap for the release gate, not invented here.
5. Release happens after merge under the same governance; no release action is taken by the ACP-007 implementation stage.

## 6. Open questions carried to the guide and acceptance

- Supported Zed version and verified `agent_servers` schema: UNKNOWN (section 4); the guide labels its example unverified.
- Real-Zed acceptance on Linux: not executed in this task invocation; recipe and evidence paths recorded in [ACP-007-acceptance.md](ACP-007-acceptance.md).
- Real-Zed acceptance on macOS: outstanding; must not be claimed anywhere.
- Differences found by acceptance must be fixed in `docs/guides/ACP-ZED.md` by the operator running acceptance.

No production code change was made in this stage: this note adds documentation only.
