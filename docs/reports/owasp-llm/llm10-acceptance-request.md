# Plan 10 — acceptance and live-delivery request

Status: **CLOSED — all six manual criteria are recorded and Plan 10 is
historicalized with disposition `COMPLETE`.** This document is retained as the
historical decision request and durable evidence index. The original Slack
transport probe remains at `0200ad3f`; verification and acceptance were recorded at
`afc92022`; the stage recovery used the merged binary from `3fc1b90f`. This
document does not claim full LLM10 coverage; the documented limitations remain.

Related: [llm10-output-handling.md](llm10-output-handling.md),
[llm10-live-delivery.md](llm10-live-delivery.md), the cumulative
[evidence matrix](README.md), the
[gap register](gap-register-20261109.md), and
[PLAN-10-SOP-LLM10-Improper-Output-Handling.md](../../plans/PLAN-10-SOP-LLM10-Improper-Output-Handling.md).

## What is complete

The implementation and deterministic evidence were merged before this request. The
wrap-up re-ran the focused checks at `main` `ed9ec2dc` on Darwin arm64 (Go 1.27.1)
and preserved the original Linux `x86_64` record.

| Task       | Criterion                                                                   | Type   | State                                        | Evidence                                                                                                                                                                                                                                                                                                                                                  |
| ---------- | --------------------------------------------------------------------------- | ------ | -------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OUTPUT-001 | 1 — sink inventory lists origin, parser, validation/encoding, effect, owner | manual | `ACCEPTED`                                | `llm10-output-handling.md` §Sink inventory                                                                                                                                                                                                                                                                                                                |
| OUTPUT-001 | 2 — intentional shell execution separated from interpolation                | manual | `ACCEPTED`                                | §Intentional shell execution versus interpolation; `internal/toolharness`, `internal/git`, `internal/github`                                                                                                                                                                                                                                              |
| OUTPUT-001 | 3 — terminal controls and owned-renderer fetching identified                | manual | `ACCEPTED`                                | §Terminal controls and resource fetching in owned renderers                                                                                                                                                                                                                                                                                               |
| OUTPUT-001 | 4 — report delivered                                                        | check  | `VERIFIED`                                   | The report is in this repository                                                                                                                                                                                                                                                                                                                          |
| OUTPUT-002 | 1 — data arguments cannot become shell syntax                               | check  | `VERIFIED`                                   | `TestSplitCommandRejectsShellMetacharacters`                                                                                                                                                                                                                                                                                                              |
| OUTPUT-002 | 2 — invalid structured operations cannot partially execute                  | check  | `VERIFIED`                                   | `TestDeniedOperationsCauseNoSideEffect`                                                                                                                                                                                                                                                                                                                   |
| OUTPUT-002 | 3 — file operations retain workspace/symlink boundaries                     | check  | `VERIFIED`                                   | `TestReadRejectsEscapes` and root-boundary tests                                                                                                                                                                                                                                                                                                          |
| OUTPUT-002 | 4 — database data stays parameterized                                       | check  | `VERIFIED`                                   | `go test ./internal/store`                                                                                                                                                                                                                                                                                                                                |
| OUTPUT-002 | 5 — owned display paths cannot execute terminal controls                    | check  | `VERIFIED`                                   | `go test ./internal/cli -run Terminal`                                                                                                                                                                                                                                                                                                                    |
| OUTPUT-002 | 6 — protocol data remains valid; JSON result intact                         | check  | `VERIFIED`                                   | `TestPromptJSONKeepsModelResultIntact`                                                                                                                                                                                                                                                                                                                    |
| OUTPUT-002 | 7 — sink-specific fixes, no destructive global escaping                     | manual | `ACCEPTED`                                | §Reproduced gaps and fixes; `TestPromptJSONKeepsModelResultIntact`                                                                                                                                                                                                                                                                                        |
| OUTPUT-003 | 1 — protocol data valid; diagnostics do not corrupt output                  | check  | `VERIFIED`                                   | `TestPromptJSONKeepsModelResultIntact`; ACP/MCP tests                                                                                                                                                                                                                                                                                                     |
| OUTPUT-003 | 2 — synthetic canaries contained in tested paths                            | check  | `VERIFIED`                                   | Terminal and Slack control tests                                                                                                                                                                                                                                                                                                                          |
| OUTPUT-003 | 3 — required checks pass; PR records OS/client coverage | manual | `ACCEPTED` | The six decisions were recorded at `afc92022`; the PR/CI record covers Linux `x86_64` and Darwin arm64 CLI/ACP/MCP and the live Slack transport probe. PR-head CI `8f06afa0` passed; the merge-commit CI run was partially cancelled and is not claimed as a full pass. Third-party clients and Slack rendering remain unverified. |
| OUTPUT-003 | 4 — cumulative matrix updated                                               | check  | `VERIFIED`                                   | `git grep "Improper Output Handling" docs/reports/owasp-llm/README.md`                                                                                                                                                                                                                                                                                    |
| OUTPUT-003 | 5 — no full coverage inferred from plan/PR existence                        | manual | `ACCEPTED`                                | README §What this matrix does not claim                                                                                                                                                                                                                                                                                                                   |

## Acceptance and lifecycle complete

1. **Manual acceptance — recorded.** All six manual criteria were accepted at
   `afc92022` under the designated approver (Tom Tran), bound to content identity
   `2d6a7c8f…`. The decisions are in `acceptance_decisions.jsonl`; the attempt
   0→1 recoveries are in `acceptance_recoveries.jsonl`.
2. **Lifecycle — historicalized `COMPLETE`.** The plan was archived at
   `2026-11-10T04:02:39.918545Z` after the supported stage recovery. The archive
   is `.agent-sdlc/archive/plan-10-sop-llm10-improper-output-handling/`.
3. **Remaining limitations.** Slack rendering and the `#sop-alert` binding remain
   `NOT_OBSERVED`; external renderers, unfurl, Windows terminal behavior,
   provider/tool output as model input, persisted Markdown rendering, and the
   other documented limits remain. No full LLM10 coverage is claimed.

## Exact evidence identity

| Item                             | Value                                                                                                                                                                                                                                    |
| -------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Wrap-up re-verification revision | `ed9ec2dc215eaa0b31a13eb49dbe2938d253c913` (2026-11-09)                                                                                                                                                                                  |
| Merged as                        | PR #328, merge `35340774c16b10236b0ca0143cc0d07c85a8e6b3` (2026-11-09T23:20:02Z)                                                                                                                                                         |
| Live-evidence reconciliation | PR #339, merge `cfca31075078f5ef3d53bbd48a0ae1dec4bdd79c`, head `6ea8d68f49b7c31364db74dff34596e1fd33fc54` (2026-11-10T02:48:54Z); full CI and CodeQL `PASS` on the head (runs [42744614438](https://github.com/imhttran/agentic-sop/actions/runs/42744614438), [42744611900](https://github.com/imhttran/agentic-sop/actions/runs/42744611900)) |
| Full CI                          | [42718751282](https://github.com/imhttran/agentic-sop/actions/runs/42718751282) and CodeQL [42718748910](https://github.com/imhttran/agentic-sop/actions/runs/42718748910), `PASS` on PR head `eaf34114a5fcfe96088986d8939309c1efc75ffa` |
| Wrap-up host                     | Darwin arm64, Go 1.27.1                                                                                                                                                                                                                  |
| Original verification host       | Linux `x86_64`, Go 1.27.1 (`linux/amd64`) at `24be068` + change set                                                                                                                                                                      |
| Report                           | `docs/reports/owasp-llm/llm10-output-handling.md`                                                                                                                                                                                        |
| Live-test preparation            | PR #313 (`f557c6fd`); this request supersedes its "exact next action" with the current rendering observation                                                                                                                             |
| Terminal evidence                | `internal/cli/terminal.go`, `internal/cli/terminal_output_test.go`                                                                                                                                                                       |
| Slack encoder evidence           | `internal/notify/slack.go`, `internal/notify/slack_test.go`                                                                                                                                                                              |
| Transport evidence               | `internal/cli/notify_acceptance_test.go`, `internal/notify/webhook_test.go`                                                                                                                                                              |
| Live probe                       | `0200ad3f77604d828acc90c3b1f9dab990e63042`, event `54795db078313f7f440560a5a8e2f6b28c7a1a17d3b232c25c656919ed7eec16`, `DELIVERED` on attempt 1 at `2026-11-10T02:33:17.836286Z`; see [llm10-live-delivery.md](llm10-live-delivery.md)    |
| Live payload                     | The inert `& < >` probe fixture in [llm10-live-delivery.md](llm10-live-delivery.md) §Payload fixture; the default `sop notify test` payload was not used because it does not exercise escaping                                           |
| Verification and acceptance | `afc9202290a6bb0a3a4ec09385f532c686b23b62`, content identity `2d6a7c8fc45b36a7619c9b2ed85cd705ad25cefad75a03291daaccede7a0209d`; three `VERIFIED`/`LOCAL_DONE` tasks; six acceptances by Tom Tran (2026-11-10T03:26:23Z, 03:37:21Z, 03:39:47Z) |
| Stage recovery | merged binary built from `3fc1b90f2dac461d0bac843ef198f49702a5f1e7` (PR #348); `sop task recover-stage` for OUTPUT-001..003; one audit each; task status, attempt, outcome, validation, and acceptance evidence unchanged |
| Final disposition | `HISTORICALIZED`, disposition `COMPLETE`, recorded `2026-11-10T04:02:39.918545Z`; archive `.agent-sdlc/archive/plan-10-sop-llm10-improper-output-handling/`; run evidence `.agent-sdlc/runs/OUTPUT-001..003/` |
| CI provenance | PR-head `8f06afa0` CI passed; merge-commit CI for `3fc1b90f` was partially cancelled after a newer main push; cancelled checks are not claimed as passed |
| External backup | workspace-local `llm10-closeout-backup-3fc1b90f/` with `pre-recovery/`, `pre-historicalize/`, and `post-closeout/` snapshots |

## Decision A — live Slack test and rendering observation

Decision A was answered once by the operator commissioning the follow-up work: one
synthetic notification to the `#sop-alert` webhook was authorized and executed at
`0200ad3f`. The transport evidence is recorded in
[llm10-live-delivery.md](llm10-live-delivery.md). The remaining Decision A item is
the human observation of the rendered message in the channel; no second send is
authorized. The send used the existing configured credential, an empty pre-send
outbox, and inert `& < >` content with no mention, secret, or external link.

## Decision B — accept the manual criteria

The designated approver decides whether to accept the named evidence for:

- `OUTPUT-001` criteria 1–3;
- `OUTPUT-002` criterion 7;
- `OUTPUT-003` criteria 3 and 5.

Proposed evidence text (draft, not applied):

- `OUTPUT-001` criterion 1: `llm10-output-handling.md` §Sink inventory lists every
  owned sink with input origin, parser, encoding or validation, effect, and owner.
- `OUTPUT-001` criterion 2: §Intentional shell execution versus interpolation
  identifies operator-configured `sh -c` paths and confirms no model interpolation.
- `OUTPUT-001` criterion 3: §Terminal controls and resource fetching identifies the
  CLI console as the free-form owned sink and records the dashboard and outbound
  request boundaries.
- `OUTPUT-002` criterion 7: §Reproduced gaps and fixes and
  `TestPromptJSONKeepsModelResultIntact` show the terminal filter is display-only
  and persisted/JSON data is unchanged.
- `OUTPUT-003` criterion 3: the PR records Linux `x86_64` (original) and Darwin arm64
  (wrap-up) CLI/ACP/MCP scope and the live Slack transport probe; third-party
  clients and live Slack rendering are unverified.
- `OUTPUT-003` criterion 5: README §What this matrix does not claim and the Plan
  06–09 rows keep partial and historical evidence separate.

## Lifecycle state and supported interface

Before the local verification run, read-only checks at the live-probe revision
`0200ad3f` (disposable worktree) and in the owning main checkout were:

```text
sop status                       -> exit 0, no tasks; no active plan
sop plan historicalize --check   -> Eligible: no; reason: no active plan is recorded
sop continue --check             -> RECONCILE_FAILED; HUMAN_REVIEW_REQUIRED; 0 tasks
sop approvals --json             -> { "version": 1, "approvals": [] }
```

The main checkout's `plan.meta.json` has no plan identity; the initialized SOP
worktrees in this workspace own other plans
(`plan-08b-sop-llm08-hidden-context-closure`,
`plan-09b-sop-llm09-retrieval-coverage-closure`). Before the local run, no Plan 10
task, run, outcome, or approval record existed in a repository-owned checkout.

The verification run at `afc92022` produced three `VERIFIED`/`LOCAL_DONE`
tasks. After the supported stage recovery, the plan was historicalized
`COMPLETE`:

```text
sop status                       -> no active plan
archive lifecycle.json           -> disposition COMPLETE
sop plan historicalize --check   -> no active plan (already historicalized)
sop approvals --json             -> { "version": 1, "approvals": [] }
```

The final archive is `.agent-sdlc/archive/plan-10-sop-llm10-improper-output-handling/`.
The run, acceptance, validation, and recovery evidence remains under
`.agent-sdlc/runs/OUTPUT-001..003/`.

The supported adoption and closeout limits, verified by code and tests and
observed in the local run at `afc92022`:

1. `sop plan activate docs/plans/PLAN-10-SOP-LLM10-Improper-Output-Handling.md`
   installs the plan's task graph as ACTIVE. For this well-formed plan it needs no
   model and records no task attempt. A deterministic compile produces three
   `PLANNED` tasks (`OUTPUT-001..003`), all `verify-first`, with dependencies
   `OUTPUT-001 -> OUTPUT-002 -> OUTPUT-003`. Activation binds no existing evidence.
2. `sop task complete <task-id> --external --commit <sha>` marks a task `LOCAL_DONE`
   and writes `external-completion.json` (source `external`, the implementation
   commit, repository head, validation `PASS`, recorded by explicit operator
   action). It runs the configured validation, records no model run or attempt, and
   distinguishes external work from a SOP execution. It also clears `outcome.json`.
3. `sop outcome <task-id> --accept ...` requires a run-derived `outcome.json` with a
   manual-evidence mapping and repository `content_identity`. It cannot bind to an
   `external-completion.json`; after external completion it fails with "no recorded
   outcome ... run the task first".
4. `sop run` on this verify-first plan executes a new task attempt and re-runs the
   configured validation and acceptance checks at the current tree. It can complete
   without an implementation-agent call when validation passes, and it derives
   `outcome.json` with the manual mappings and current content identity. This is the
   only supported precursor to `sop outcome --accept`, but it produces new lifecycle
   evidence, not the original merged evidence at `0200ad3f`.
5. `sop plan complete` / `sop plan historicalize` check satisfied tasks, pending
   approvals, and verification evidence (a passed run, an external-completion
   record, or a not-required disposition). They do not read
   `acceptance_decisions.jsonl`. Readiness alone therefore does not prove the six
   manual criteria were accepted; the operator must confirm the acceptance records
   separately before closure.

The one-run description is not a supported closeout path. The observed per-task
sequence at `afc9202290a6bb0a3a4ec09385f532c686b23b62` (content identity
`2d6a7c8fc45b36a7619c9b2ed85cd705ad25cefad75a03291daaccede7a0209d`) completed:

1. `sop plan activate ...` installed three `PLANNED` verify-first tasks.
2. The first `sop run` executed only `OUTPUT-001`; validation passed, no
   implementation agent was invoked, and the outcome was `PARTIAL` because
   `criterion-1..3` had no acceptance decisions. The run parked at `NEEDS_HUMAN`
   and requeued `OUTPUT-001`.
3. Three `sop outcome OUTPUT-001 --accept criterion-1|criterion-2|criterion-3`
   decisions were recorded under the designated approver. The acceptance recovery
   records bridged mapping attempt 0 to task attempt 1.
4. `sop approve OUTPUT-001` approved the gate. The next `sop run` resumed
   `OUTPUT-001` as `VERIFIED`/`LOCAL_DONE` without an implementation agent, then
   executed `OUTPUT-002` verify-first. `OUTPUT-002` parked at its own
   `NEEDS_HUMAN` gate.
5. `sop outcome OUTPUT-002 --accept criterion-7`, `sop approve OUTPUT-002`, and
   `sop run` completed `OUTPUT-002` as `VERIFIED` and advanced to `OUTPUT-003`.
6. `sop outcome OUTPUT-003 --accept criterion-3|criterion-5`,
   `sop approve OUTPUT-003`, and `sop run` completed `OUTPUT-003` as `VERIFIED`;
   all three tasks were satisfied.
7. The accepted-outcome stage defect left each run stage at
   `WAITING_FOR_HUMAN`. PR #348 fixed the transition, and the already affected
   records were repaired with the merged binary built from `3fc1b90f` using
   `sop task recover-stage` for all three tasks. Each repair wrote one
   `stage-recoveries.jsonl` audit record and preserved the task, attempt, outcome,
   validation, and acceptance evidence.
8. `sop plan historicalize` archived the plan as `COMPLETE` at
   `2026-11-10T04:02:39.918545Z`.

The closeout sequence is therefore iterative per task:

```text
run -> record that task's manual acceptances -> approve its gate -> run again
    to resume/complete it and advance to the next task -> repeat
```

A single run plus six acceptances is not supported. Each dependent task starts
only after the prior task's acceptance decisions are recorded and its gate is
approved.

Observed closure-readiness limit (resolved): after each accepted resume the run
`state.json` still recorded `stage: WAITING_FOR_HUMAN` and `attempt.txt` remained
`PARTIAL|ACCEPTANCE|NEEDS_HUMAN`, even though the task was `LOCAL_DONE` and its
outcome `VERIFIED`. `sop plan historicalize --check` therefore reported
`Unresolved verification`. PR #348 fixed the accepted-outcome transition to
persist `PASSED` before completion, and `sop task recover-stage` repaired the
already affected records. No records were manually edited.

Consequently, the original merged evidence was not adopted as lifecycle closure;
the operator authorized the iterative per-task verification and acceptance
sequence above, followed by the supported stage recovery and historicalization.
The plan is now `COMPLETE`. The remaining limitations are recorded in
§What acceptance does and does not mean and the report's residual risks.

## What acceptance does and does not mean

- The six decisions bind the named report sections and revision to the named
  manual criteria for the recorded attempt.
- The decisions do **not** convert the passing CI run into an acceptance record;
  the run is evidence for `OUTPUT-003` criterion 3.
- They do **not** turn deterministic tests into live Slack rendering evidence; the
  separate transport probe is recorded in
  [llm10-live-delivery.md](llm10-live-delivery.md).
- They do **not** prove third-party terminal, IDE, editor, or Markdown rendering.
- They do **not** prove live Slack rendering, the channel binding, mention
  behavior, or auto-unfurl. The transport delivery is recorded separately.
- They do **not** prove Windows behavior or a general content-sanitization/DLP
  claim.

## Bounded security claim for the verified sinks

**LLM10 scoped protections implemented, verified, and accepted with documented
limitations.** On the verified SOP-owned sinks, SOP removes terminal control
characters from console display, escapes `&`/`<`/`>` in Slack mrkdwn so
angle-bracket mentions and masked links cannot be injected, keeps protocol output
valid JSON, and retains the existing shell, path, SQL, and authorization
boundaries. One live Slack webhook transport delivery was confirmed. Slack
rendering and the channel binding remain `NOT_OBSERVED`; external renderers,
unfurl, Windows, provider/tool output as model input, and persisted-Markdown
rendering remain limitations. This is not full LLM10 coverage.

## Must not

These constraints governed the historical request. The decisions and
historicalization were recorded through the supported commands.

- Do not record an acceptance decision, an approver's signature, or an acceptance
  timestamp on the operator's behalf.
- Do not enable the webhook channel or send another notification without a new
  explicit authorization.
- Do not create or edit `state.db`, `outcome.json`, or the acceptance audit files
  outside the supported command.
- Do not mark the plan complete or historicalized from this document alone; the
  recorded decisions and supported stage recovery were required first.
- Do not treat `sop plan complete` / `sop plan historicalize` readiness as proof of
  manual acceptance: those readiness checks do not read `acceptance_decisions.jsonl`.
- Do not use `sop task complete --external` as a substitute for the six
  `sop outcome --accept` decisions; external completion clears `outcome.json` and
  cannot bind them.
- Do not treat a merged PR, a green local test, or a 2xx response as lifecycle
  completion or observed Slack rendering.

## Exact next action

No further acceptance or lifecycle action remains: all six decisions are recorded,
the plan is historicalized `COMPLETE`, and the archive and backup preserve the
evidence. The remaining observation is Slack rendering and the `#sop-alert`
channel binding, which remain `NOT_OBSERVED`; no notification should be resent. The
other documented coverage limitations remain in force.
