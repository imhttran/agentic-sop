# Plan 10 — acceptance and live-delivery request

Status: **REQUEST — no acceptance is recorded, one authorized live transport probe
has executed, its rendering is unobserved, and no SOP lifecycle transition is
claimed.** This document prepares the exact decisions a designated approver must
make. It does not manufacture an approver decision, an acceptance record, or a
lifecycle transition.

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
| OUTPUT-001 | 1 — sink inventory lists origin, parser, validation/encoding, effect, owner | manual | `IMPLEMENTED`                                | `llm10-output-handling.md` §Sink inventory                                                                                                                                                                                                                                                                                                                |
| OUTPUT-001 | 2 — intentional shell execution separated from interpolation                | manual | `IMPLEMENTED`                                | §Intentional shell execution versus interpolation; `internal/toolharness`, `internal/git`, `internal/github`                                                                                                                                                                                                                                              |
| OUTPUT-001 | 3 — terminal controls and owned-renderer fetching identified                | manual | `IMPLEMENTED`                                | §Terminal controls and resource fetching in owned renderers                                                                                                                                                                                                                                                                                               |
| OUTPUT-001 | 4 — report delivered                                                        | check  | `VERIFIED`                                   | The report is in this repository                                                                                                                                                                                                                                                                                                                          |
| OUTPUT-002 | 1 — data arguments cannot become shell syntax                               | check  | `VERIFIED`                                   | `TestSplitCommandRejectsShellMetacharacters`                                                                                                                                                                                                                                                                                                              |
| OUTPUT-002 | 2 — invalid structured operations cannot partially execute                  | check  | `VERIFIED`                                   | `TestDeniedOperationsCauseNoSideEffect`                                                                                                                                                                                                                                                                                                                   |
| OUTPUT-002 | 3 — file operations retain workspace/symlink boundaries                     | check  | `VERIFIED`                                   | `TestReadRejectsEscapes` and root-boundary tests                                                                                                                                                                                                                                                                                                          |
| OUTPUT-002 | 4 — database data stays parameterized                                       | check  | `VERIFIED`                                   | `go test ./internal/store`                                                                                                                                                                                                                                                                                                                                |
| OUTPUT-002 | 5 — owned display paths cannot execute terminal controls                    | check  | `VERIFIED`                                   | `go test ./internal/cli -run Terminal`                                                                                                                                                                                                                                                                                                                    |
| OUTPUT-002 | 6 — protocol data remains valid; JSON result intact                         | check  | `VERIFIED`                                   | `TestPromptJSONKeepsModelResultIntact`                                                                                                                                                                                                                                                                                                                    |
| OUTPUT-002 | 7 — sink-specific fixes, no destructive global escaping                     | manual | `IMPLEMENTED`                                | §Reproduced gaps and fixes; `TestPromptJSONKeepsModelResultIntact`                                                                                                                                                                                                                                                                                        |
| OUTPUT-003 | 1 — protocol data valid; diagnostics do not corrupt output                  | check  | `VERIFIED`                                   | `TestPromptJSONKeepsModelResultIntact`; ACP/MCP tests                                                                                                                                                                                                                                                                                                     |
| OUTPUT-003 | 2 — synthetic canaries contained in tested paths                            | check  | `VERIFIED`                                   | Terminal and Slack control tests                                                                                                                                                                                                                                                                                                                          |
| OUTPUT-003 | 3 — required checks pass; PR records OS/client coverage                     | manual | automated CI `PASS`; manual acceptance `GAP` | Full CI [42718751282](https://github.com/imhttran/agentic-sop/actions/runs/42718751282) and CodeQL [42718748910](https://github.com/imhttran/agentic-sop/actions/runs/42718748910) passed on `eaf34114`; the PR body records Linux `x86_64` and Darwin arm64 scope and the unverified external clients. Passing CI is evidence, not an acceptance record. |
| OUTPUT-003 | 4 — cumulative matrix updated                                               | check  | `VERIFIED`                                   | `git grep "Improper Output Handling" docs/reports/owasp-llm/README.md`                                                                                                                                                                                                                                                                                    |
| OUTPUT-003 | 5 — no full coverage inferred from plan/PR existence                        | manual | `IMPLEMENTED`                                | README §What this matrix does not claim                                                                                                                                                                                                                                                                                                                   |

## What is not complete

1. **Observed Slack rendering — `NOT_OBSERVED`.** One authorized synthetic event
   was delivered through the configured Slack webhook at `0200ad3f` (transport
   `VERIFIED`, outbox `DELIVERED`, no retry). No authenticated Slack session was
   available, so the rendered message, the bound channel name, and the absence of
   pings remain unobserved. The HTTP 2xx distinction and the exact observation
   needed are recorded in [llm10-live-delivery.md](llm10-live-delivery.md).
2. **Manual acceptance — not recorded.** The six manual criteria above have no
   designated-approver decision record. The full CI passed for `OUTPUT-003`
   criterion 3; that is automated evidence, not an acceptance record.
3. **Owning SOP lifecycle state — absent.** No checkout holds Plan 10 state. A
   merged PR is not lifecycle completion.

## Exact evidence identity

| Item                             | Value                                                                                                                                                                                                                                    |
| -------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Wrap-up re-verification revision | `ed9ec2dc215eaa0b31a13eb49dbe2938d253c913` (2026-11-09)                                                                                                                                                                                  |
| Merged as                        | PR #328, merge `35340774c16b10236b0ca0143cc0d07c85a8e6b3` (2026-11-09T23:20:02Z)                                                                                                                                                         |
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

Read-only checks at the live-probe revision `0200ad3f` (disposable worktree) and
in the owning main checkout:

```text
sop status                       -> exit 0, no tasks; no active plan
sop plan historicalize --check   -> Eligible: no; reason: no active plan is recorded
sop continue --check             -> RECONCILE_FAILED; HUMAN_REVIEW_REQUIRED; 0 tasks
sop approvals --json             -> { "version": 1, "approvals": [] }
```

The main checkout's `plan.meta.json` has no plan identity; the initialized SOP
worktrees in this workspace own other plans
(`plan-08b-sop-llm08-hidden-context-closure`,
`plan-09b-sop-llm09-retrieval-coverage-closure`). No Plan 10 task, run, outcome, or
approval record exists.

Consequently:

- `sop outcome <task> --accept <criterion> --evidence <text> --by <approver>`
  cannot be used: it requires a recorded outcome and a mapped manual criterion.
- `sop plan historicalize` cannot pass readiness: it requires an active plan with
  satisfied tasks and verification evidence.

The supported path, if the operator wants a SOP acceptance record, is operator-owned:

1. Decide whether to establish an owning SOP state for this plan (for example by
   running it through `sop run` against the existing merged evidence). This may
   re-execute verify-first tasks; it is a separate governed decision.
2. Record the task outcomes through the lifecycle against the existing evidence.
3. Record one `sop outcome --accept` per manual criterion, naming the report section
   and revision.
4. Historicalize only when SOP readiness and the acceptance decisions are both
   satisfied.

This request does not perform any of those steps.

## What acceptance would and would not mean

- It would bind the named report sections and revision to the named manual criteria
  for the recorded attempt.
- It would **not** convert the passing CI run into an acceptance record; the run is
  evidence for `OUTPUT-003` criterion 3.
- It would **not** turn deterministic tests into live Slack rendering evidence; the
  separate transport probe is recorded in
  [llm10-live-delivery.md](llm10-live-delivery.md).
- It would **not** prove third-party terminal, IDE, editor, or Markdown rendering.
- It would **not** prove live Slack rendering, the channel binding, mention
  behavior, or auto-unfurl. The transport delivery is already recorded separately
  in [llm10-live-delivery.md](llm10-live-delivery.md).
- It would **not** prove Windows behavior or a general content-sanitization/DLP
  claim.

## Bounded security claim for the verified sinks

On the verified SOP-owned sinks, SOP removes terminal control characters from
console display, escapes `&`/`<`/`>` in Slack mrkdwn so angle-bracket mentions and
masked links cannot be injected, keeps protocol output valid JSON, and retains the
existing shell, path, SQL, and authorization boundaries. This is not a claim that
all improper output handling is prevented.

## Must not

- Do not record an acceptance decision, an approver's signature, or an acceptance
  timestamp on the operator's behalf.
- Do not enable the webhook channel or send another notification without a new
  explicit authorization.
- Do not create or edit `state.db`, `outcome.json`, or the acceptance audit files
  outside the supported command.
- Do not mark the plan complete or historicalized from this document.
- Do not treat a merged PR, a green local test, or a 2xx response as lifecycle
  completion or observed Slack rendering.

## Exact next action

Decision A's transport step is complete. The designated approver (`Tom Tran`, per
`.agent-sdlc/config.yaml`) or another operator with channel access must observe the
message in `#sop-alert` and record the channel name, visible text, absence of pings,
and a permalink or screenshot. The approver must also answer Decision B (accept or
decline the six manual criteria with the evidence text above), and separately decide
whether to establish an owning SOP plan state via `sop run`. Until those records
exist, Plan 10 keeps rendering `NOT_OBSERVED`, manual acceptance `GAP`, and lifecycle
`GAP`.
