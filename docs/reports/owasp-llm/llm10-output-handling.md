# LLM10 — Improper output handling (SOP Linux workstream)

Status: two reproduced display-sink gaps `IMPLEMENTED` and `VERIFIED` for the
SOP-owned CLI terminal and the Slack payload encoder; loopback HTTP transport and
one live Slack webhook delivery `VERIFIED`; observed Slack rendering
`NOT_OBSERVED`; manual acceptance not recorded. Shell, path, database, and protocol
boundaries were inspected and their existing controls confirmed. No owning SOP
plan state exists for this plan, so this report claims no lifecycle transition.

Baseline (original): `imhttran/agentic-sop` `main` at
`24be0687b0ac0dec5e2e69584be67f1795c0f6f1`, merged with `main` at `985472a`
before review (2026-11-08). Host: Linux `x86_64`, Go 1.27.1 (`linux/amd64`).
Branch: `security/llm10-improper-output-handling`.

Wrap-up re-verification (2026-11-09T22:58Z): `main` at
`ed9ec2dc215eaa0b31a13eb49dbe2938d253c913`, Darwin arm64, Go 1.27.1. The
deterministic sink tests and the focused acceptance checks in §Verification
record were re-run at this revision.

Live probe (2026-11-10T02:33:17Z): one authorized synthetic notification was
delivered through the configured Slack webhook at `main`
`0200ad3f77604d828acc90c3b1f9dab990e63042` (Darwin arm64, Go 1.27.1). The durable
outbox recorded `DELIVERED` on attempt 1 with no retry. The Slack sink and webhook
code are unchanged since `ed9ec2dc`; the `internal/notify` package changed only
for unrelated LLM08 event-identity and redaction work. Rendering was
**not observed**: no authenticated Slack session was available on the test host.
A 2xx response proves endpoint acceptance, not visible rendering or the bound
channel name. See [llm10-live-delivery.md](llm10-live-delivery.md) for the exact
revision, command, payload fixture, delivery rows, and the observation still
required. The prepared [acceptance request](llm10-acceptance-request.md) remains
open for the manual criteria.

The wrap-up merged in PR #328 as
`35340774c16b10236b0ca0143cc0d07c85a8e6b3`. Full CI passed on the PR head
`eaf34114a5fcfe96088986d8939309c1efc75ffa` (run
[42718751282](https://github.com/imhttran/agentic-sop/actions/runs/42718751282));
CodeQL passed on the same head (run
[42718748910](https://github.com/imhttran/agentic-sop/actions/runs/42718748910)).
The merge is not a lifecycle transition, and manual acceptance is still not
recorded.

The live-evidence reconciliation merged in PR #339 as
`cfca31075078f5ef3d53bbd48a0ae1dec4bdd79c` from head
`6ea8d68f49b7c31364db74dff34596e1fd33fc54`. Full CI and CodeQL passed on that
head (runs
[42744614438](https://github.com/imhttran/agentic-sop/actions/runs/42744614438)
and [42744611900](https://github.com/imhttran/agentic-sop/actions/runs/42744611900)).
The merge is not a lifecycle transition, records no manual acceptance, and does
not change the `NOT_OBSERVED` rendering state.

This report treats model, tool, provider, and configured-command output as
untrusted at each SOP-owned sink. It does not claim protection for a third-party
terminal, editor, IDE, Slack client, or remote review renderer that SOP cannot
control.

## Method

- Traced structured tool arguments, command construction, file paths, SQL, HTML
  templates, terminal output, Markdown artifacts, notifications, and protocol
  serialization from the current code.
- Distinguished intentional, operator-configured shell execution from
  interpolation of data into a command string.
- Reproduced each display-sink defect with a failing test before changing code.
- Used synthetic fixtures and in-process writers for the deterministic work. One
  separately authorized live Slack probe used inert content and the existing
  configured webhook; see [llm10-live-delivery.md](llm10-live-delivery.md). No
  exfiltration endpoint, production write, or downloaded artifact was used.
- Reused the existing command policy, path resolver, JSON encoders, and template
  auto-escaping. No new gate, policy layer, or permission system was added.

## Sink inventory

| Sink                                                                                                 | Input origin                                                                      | Parser / constructor                                             | Validation / encoding                                                                                                                                                                    | Effect                                  | Policy owner                                                                         |
| ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- | ---------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- | ------------------------------------------------------------------------------------ |
| `run_command` (tool harness)                                                                         | Model tool arguments                                                              | `toolharness.SplitCommand` tokenizes to `argv`; no shell         | Shell metacharacters and globs are rejected; `commandpolicy.Classify` must return `SAFE`; paths are confined to an authorized root; arguments are passed as separate `exec` argv entries | Executes one approved program           | `internal/toolharness`, `internal/commandpolicy`                                     |
| Configured agent command                                                                             | Operator environment (`SOP_AGENT_COMMAND`)                                        | `agent.CommandAgent` runs `sh -c <configured command>`           | Trusted configuration, not model text; the request is JSON on stdin                                                                                                                      | Model call                              | `internal/agent`; SECURITY.md §2                                                     |
| Configured validation command                                                                        | `.agent-sdlc/config.yaml`                                                         | `testrunner.execute` runs `sh -c <configured command>`           | Trusted configuration; no request data is interpolated into the command                                                                                                                  | Build/test/lint process                 | `internal/testrunner`; SECURITY.md §2                                                |
| External review command                                                                              | Operator environment (`SOP_REVIEW_COMMAND`)                                       | `review.OCRProvider` runs `sh -c <configured command>`           | Trusted configuration; the request is JSON on stdin                                                                                                                                      | Review process                          | `internal/review`                                                                    |
| Handoff compression command                                                                          | Operator configuration                                                            | `caveman.ShellRunner` runs `sh -c <configured command>`          | Trusted configuration; the bundle is JSON on stdin                                                                                                                                       | Compression process                     | `internal/handoff/caveman`                                                           |
| Comparison executor/verification                                                                     | Operator configuration/manifest                                                   | `hipcompare` runs `sh -c <configured command>`                   | Trusted configuration; no model text is interpolated                                                                                                                                     | Comparison arms                         | `internal/hipcompare`                                                                |
| Acceptance check                                                                                     | Plan `### Acceptance Checks`                                                      | `cli.runAcceptanceArgv` runs a validated `argv` directly         | `internal/commandpolicy` must classify it `SAFE`; no shell                                                                                                                               | Deterministic check                     | `internal/cli`, `internal/commandpolicy`                                             |
| Git / GitHub                                                                                         | SOP code and task identity                                                        | `git run` and `github execRunner` build explicit argument lists  | No shell; `GIT_TERMINAL_PROMPT=0` / `GH_PROMPT_DISABLED=1`                                                                                                                               | Repository and API operations           | `internal/git`, `internal/github`                                                    |
| File tools (`write_file`, `create_file`, `delete_file`, `restore_file`, `copy_file`, `rewrite_file`) | Model tool arguments                                                              | `toolharness.resolveIn` canonicalizes against an authorized root | NUL rejected; symlink-resolved boundary check; `.agent-sdlc/state.db` protected; read-only roots refuse writes; arguments validated before any effect                                    | File mutation inside an authorized root | `internal/toolharness`                                                               |
| SQLite state                                                                                         | SOP code and task/plan identity                                                   | `database/sql` with bound placeholders                           | Data values are parameters; the only concatenated SQL is a constant column list                                                                                                          | Persisted task/run/notification state   | `internal/store`                                                                     |
| Controller HTML                                                                                      | SOP state and persisted artifacts                                                 | `html/template`                                                  | Automatic contextual escaping; no `template.HTML`, `template.JS`, `template.URL`, or `template.CSS`; no model-authored markup is trusted                                                 | Browser dashboard                       | `internal/controller/web`                                                            |
| CLI terminal (stdout/stderr)                                                                         | Model prose, review/JEV findings, provider errors, validation output, SOP reasons | `cli.run` wraps both writers in `terminalSink`                   | C0/C1 control runes removed except tab and newline; multi-byte UTF-8 preserved across writes                                                                                             | Operator console                        | `internal/cli`                                                                       |
| Notification webhook (native)                                                                        | SOP lifecycle event                                                               | `notify.EncodeNativePayload` / `json.Marshal`                    | JSON encoding escapes control characters; the endpoint is configured, never model-selected                                                                                               | HTTP POST                               | `internal/notify`                                                                    |
| Notification webhook (Slack)                                                                         | SOP lifecycle event, including model-derived reasons                              | `notify.EncodeSlackPayload` → `slackEventText`                   | JSON encoding plus `&amp;`/`&lt;`/`&gt;` mrkdwn escaping                                                                                                                                 | Slack message                           | `internal/notify`                                                                    |
| MCP protocol                                                                                         | SOP application services                                                          | `mcp` server `json.NewEncoder(stdout)`                           | JSON encoding escapes control characters; diagnostics go to stderr                                                                                                                       | JSON-RPC tool results                   | `internal/mcp`                                                                       |
| ACP protocol                                                                                         | SOP application services                                                          | `acp` transport `json.Marshal` → framed writer                   | JSON encoding escapes control characters; diagnostics are bounded and redacted                                                                                                           | JSON-RPC session frames                 | `internal/acp`                                                                       |
| Persisted Markdown (`report.md`, `result.md`, `implementation.md`)                                   | Model prose and review/JEV findings                                               | Run artifact writer                                              | Stored as data; SOP does not execute or auto-render these artifacts                                                                                                                      | Later human/tool reading                | `internal/run`, `internal/cli`                                                       |
| Outbound HTTP (providers, webhook, decision adapters, RAG)                                           | Configured base URL                                                               | `net/http` request construction                                  | Target comes from configuration or environment, never from model output                                                                                                                  | Provider/webhook request                | `internal/provider`, `internal/agent`, `internal/notify`, `internal/decisionadapter` |

### Intentional shell execution versus interpolation

Shell execution is intentional only for **trusted operator configuration**: the
agent command, validation commands, external review command, handoff compression
command, and comparison commands. Those commands are run with `sh -c`, and request
data reaches them as JSON on stdin, not by string interpolation
(`docs/specs/SECURITY.md` §2).

The model-facing `run_command` tool does **not** use a shell. `SplitCommand`
rejects `; & | < > $ \` newline carriage-return * ? [ ] { } ( ) ~`, and the
resulting `argv`is executed directly after`commandpolicy.Classify`returns`SAFE`. Every path-like argument is resolved against the selected authorized root.
The same rule applies to acceptance checks, Git, and GitHub: structured arguments,
no shell.

No code path interpolates model output into a shell command.

### Terminal controls and resource fetching in owned renderers

- The CLI terminal is the only SOP-owned sink that emits free-form model prose to
  a device that interprets control sequences. Before this work, `sop review`,
  `sop prompt`, `sop run` (JEV findings), and `sop report --detailed` printed
  model-derived text verbatim, so ANSI CSI, OSC 8 hyperlinks, and carriage returns
  reached the terminal.
- The controller dashboard uses `html/template` auto-escaping, contains no
  `template.HTML`/`template.JS`/`template.URL`/`template.CSS`, and references only
  local `/static` assets. No model-selected remote resource is loaded by the
  dashboard.
- SOP performs no HTTP request to a URL taken from model output. Provider,
  webhook, decision-adapter, and RAG endpoints are configured values. Slack may
  unfurl a bare URL contained in a message; that behavior belongs to the external
  Slack client and is recorded as a limitation, not an SOP-owned fetch.

## Verification dimensions

Plan 10 has four distinct output-path claims. They do not imply one another.

| Dimension                 | State                                                                                                                        | Evidence                                                                                                                                                                                                                                                                                                                 | Does not prove                                                                                                                                                       |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| CLI terminal sanitization | `IMPLEMENTED` / `ENABLED` / `VERIFIED` on Linux `x86_64` at the original baseline; re-verified on Darwin arm64 at `ed9ec2dc` | `sanitizeTerminal` and `terminalSink`; `TestReviewOutputNeutralizesTerminalControls`, `TestPromptOutputNeutralizesTerminalControls`, `TestSanitizeTerminalPreservesLayout`, `TestTerminalSinkReassemblesSplitRune`, `TestTerminalWriterUnwrapsSink`                                                                      | Third-party terminal, IDE, editor, pager, or Markdown renderer behavior. Only SOP-owned stdout/stderr writes are filtered.                                           |
| Slack payload encoding    | `IMPLEMENTED` / `VERIFIED` (deterministic; live payload fixture)                                                             | `EncodeSlackPayload` and `slackEventText`; `TestEncodeSlackPayloadEscapesMrkdwn`, `TestEncodeSlackPayloadHasTextAndEventID`, `TestWebhookSlackPayloadCarriesNoURL`; the live probe payload escaped `& < >` to `&amp; &lt; &gt;` (see [llm10-live-delivery.md](llm10-live-delivery.md))                                   | That Slack received, rendered, or displayed the message; that all Slack mrkdwn grammar is neutralized; unfurl behavior.                                              |
| HTTP transport            | `VERIFIED` on loopback (in-process) and once against the configured Slack webhook at `0200ad3f`                              | `TestAcceptanceNotifyTestDeliversToLocalReceiver` (real `httptest.Server` on loopback through the `sop notify test` path); `TestWebhookSlackPayloadClassifiesResponses`; `TestWebhookSendsStableIdempotencyKey`; live `sop notify drain`: `claimed=1 delivered=1 retried=0 exhausted=0`, outbox `DELIVERED` on attempt 1 | That a 2xx came from the intended Slack channel; that a 2xx means visible rendering; that retries are duplicate-free (the receiver may not honor `Idempotency-Key`). |
| Live Slack delivery       | `VERIFIED` once at `0200ad3f` (transport)                                                                                    | Event `54795db0…ec16` recorded `DELIVERED`; pre/post `sop notify status` shows `delivered=1`, `pending=0`, `exhausted=0`; see [llm10-live-delivery.md](llm10-live-delivery.md)                                                                                                                                           | That the bound channel is `#sop-alert`; that the message is visible or rendered as intended.                                                                         |
| Observed Slack rendering  | `NOT_OBSERVED`                                                                                                               | No authenticated Slack session on the test host; `app.slack.com/client` redirected to workspace sign-in. A human observation in the channel is still required.                                                                                                                                                           | Nothing about Slack rendering, mention behavior, or the channel binding is claimed.                                                                                  |

## Reproduced gaps and fixes

### GAP-01 (fixed): model prose reached the terminal with control sequences

`sop prompt` printed `agent.Response.Content` through `renderPromptResult`;
`sop review` printed the model summary and finding titles; `sop run`/`sop report`
printed JEV finding messages. None of these paths filtered control characters, so
a model response containing `ESC [ 2 J` could clear the screen, `ESC ] 8 ; ; …`
could create a hyperlink, and `CR` could overwrite an earlier line.

Reproduced before the fix:

- `TestReviewOutputNeutralizesTerminalControls` observed `U+001B` in review
  stdout.
- `TestPromptOutputNeutralizesTerminalControls` observed `U+001B` and `U+0007`
  in prompt stdout.

Fix: `internal/cli/terminal.go` adds a display-sink writer.

- `run()` wraps stdout and stderr with `terminalSink`.
- `sanitizeTerminal` removes C0/C1 control runes except tab and newline. ESC and
  CR are removed, so ANSI/OSC sequences and line overwrites cannot execute. The
  sequence body can remain as inert visible text; it has no control effect.
- `terminalSink.Write` holds an incomplete trailing UTF-8 sequence until the next
  write, so a split multi-byte character is not corrupted.
- `isTerminal` unwraps the sink, so interactive activity streaming remains
  enabled. This was the reason a top-level writer wrapper alone was rejected.
- Persisted artifacts are untouched. JSON output is unaffected because JSON
  encoders already escape control characters; `TestPromptJSONKeepsModelResultIntact`
  proves the decoded model result is preserved exactly.

### GAP-02 (fixed): Slack mrkdwn was interpreted in model-derived event text

`slackEventText` rendered the event reason, outcome, and next action directly into
a `mrkdwn` body. A model-derived reason (for example an early-JEV finding detail
carried into a failure reason) could contain `<!channel>` or `<https://…|text>`,
which the Slack client interprets as a mention or masked link.

Fix: `internal/notify/slack.go` escapes `&`, `<`, and `>` in the rendered text.
Slack decodes these entities back to the original characters, so legitimate text
stays readable while mention and masked-link syntax cannot be injected.
`TestEncodeSlackPayloadEscapesMrkdwn` covers the payload. The separately
authorized live probe carried `& < >` in its outcome and next action and the
transmitted body showed the escaped entities (`&amp; &lt; &gt;`); delivery was
confirmed. Slack rendering of that body remains unobserved. See
[llm10-live-delivery.md](llm10-live-delivery.md).

## Verification record

Baseline `24be068` plus this change set, Linux `x86_64`, Go 1.27.1.

| Command                                                                                                                                 | Result         | Notes                                                                                                                                                                         |
| --------------------------------------------------------------------------------------------------------------------------------------- | -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go test ./internal/cli -run Terminal -count=1`                                                                                         | `PASS`         | The two control-sequence tests reproduced `FAIL` before the fix; all sink tests pass after it.                                                                                |
| `go test ./internal/cli -run TestPromptJSONKeepsModelResultIntact -count=1`                                                             | `PASS`         | Machine-readable output stays valid and keeps the decoded model result exactly.                                                                                               |
| `go test ./internal/cli -count=1`                                                                                                       | `PASS` (244 s) | Full CLI package with `TMPDIR` on `/srv/sop/tmp/llm10` (RAM-backed I/O path); the earlier disk-backed run hit Go's 10-minute package timeout in an unrelated init/store test. |
| `go test ./internal/toolharness ./internal/notify ./internal/store ./internal/review ./internal/jev ./internal/controller/... -count=1` | `PASS`         | Command policy, path boundary, store, review/JEV parse, and controller renderer packages.                                                                                     |
| `go vet ./internal/cli ./internal/notify ./internal/toolharness ./internal/store`                                                       | `PASS`         | No diagnostics.                                                                                                                                                               |

Wrap-up re-verification: `main` `ed9ec2dc`, Darwin arm64, Go 1.27.1
(2026-11-09T22:58Z). These are focused checks; the full repository CI runs on the
pull request.

| Command                                                                                                                                                    | Result | Notes                                                                                                                                                               |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go test ./internal/notify -run 'Slack\|Webhook\|Mrkdwn\|Payload' -count=1`                                                                                | `PASS` | Payload shape, mrkdwn escaping, 2xx/4xx/5xx/timeout classification, stable idempotency key, no-URL payload, and URL-free error redaction.                           |
| `go test ./internal/cli -run 'Terminal\|TestPromptJSONKeepsModelResultIntact' -count=1`                                                                    | `PASS` | Terminal sink and JSON-preservation paths. The `Terminal` pattern also matched two unrelated `...TerminalNotHuman` tests; they passed and are not Plan 10 evidence. |
| `go test ./internal/cli -run 'AcceptanceNotify' -count=1`                                                                                                  | `PASS` | Status isolation/configuration and the real loopback transport exercised through the `sop notify test` path.                                                        |
| `go test ./internal/toolharness -run 'TestSplitCommandRejectsShellMetacharacters\|TestDeniedOperationsCauseNoSideEffect\|TestReadRejectsEscapes' -count=1` | `PASS` | Shell-metacharacter rejection, no-side-effect on a denied operation, and read path-escape rejection.                                                                |
| `go test ./internal/store -count=1`                                                                                                                        | `PASS` | Parameterized persistence.                                                                                                                                          |
| `go test ./internal/notify -run Slack -count=1`                                                                                                            | `PASS` | Slack-specific encoder and transport tests.                                                                                                                         |
| `go test ./internal/acp -count=1`                                                                                                                          | `PASS` | ACP protocol serialization.                                                                                                                                         |
| `go test ./internal/mcp -count=1`                                                                                                                          | `PASS` | MCP protocol serialization.                                                                                                                                         |

This re-verification did not include a live Slack message.

Live probe (authorized once, 2026-11-10T02:33:17Z): `main` `0200ad3f`, Darwin arm64,
Go 1.27.1. Temporary `notifications.channel: webhook` and
`notifications.webhook.payload: slack` in a disposable worktree; the gitignored
`.env` supplied the existing credential without printing or committing it.
Pre-send `sop notify status --json` reported `pending: 0`, `delivered: 0`,
`webhook_configured: true`; the synthetic event carried `& < >` in its outcome and
next action; `sop notify drain` reported
`claimed=1 delivered=1 retried=0 exhausted=0`; post-send status reported
`active: true`, `delivered: 1`, `pending: 0`, `exhausted: 0`. The outbox row is
`DELIVERED` on attempt 1 with an empty `last_error`. Rendering was **not
observed**: the test host had no authenticated Slack session. The exact fixture,
event id, and the observation still required are in
[llm10-live-delivery.md](llm10-live-delivery.md).

Focused checks re-run at the live-probe revision `0200ad3f`:

| Command                                                                                                                                                    | Result | Notes                                                                                                                     |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------- |
| `go test ./internal/toolharness -run 'TestSplitCommandRejectsShellMetacharacters\|TestDeniedOperationsCauseNoSideEffect\|TestReadRejectsEscapes' -count=1` | `PASS` | Shell-metacharacter rejection, no-side-effect on a denied operation, and read path-escape rejection.                      |
| `go test ./internal/store ./internal/acp ./internal/mcp -count=1`                                                                                          | `PASS` | Parameterized persistence and protocol serialization.                                                                     |
| `go test ./internal/notify -run 'Slack\|Webhook' -count=1`                                                                                                 | `PASS` | Slack encoder and webhook transport tests.                                                                                |
| `go test ./internal/cli -run 'Terminal\|TestPromptJSONKeepsModelResultIntact' -count=1`                                                                    | `PASS` | Terminal sink and JSON-preservation paths; the `Terminal` pattern also matched two unrelated `...TerminalNotHuman` tests. |
| `git grep -q "Improper Output Handling" -- docs/reports/owasp-llm/README.md`                                                                               | `PASS` | Cumulative matrix retains the category.                                                                                   |

These checks pass at `0200ad3f`; the earlier `ed9ec2dc` and Linux `x86_64`
records remain separate historical evidence.

Full CI (PR #328): the repository ran on the PR head
`eaf34114a5fcfe96088986d8939309c1efc75ffa` and passed.

| Check                                                                                                           | Result | Run                                                                             |
| --------------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------------------------------------------- |
| `fmt, vet, build, test`; `race detector`; installer; Windows; Semgrep/Gitleaks/OSV; JEV and operator acceptance | `PASS` | [42718751282](https://github.com/imhttran/agentic-sop/actions/runs/42718751282) |
| CodeQL analyze jobs                                                                                             | `PASS` | [42718748910](https://github.com/imhttran/agentic-sop/actions/runs/42718748910) |

The earlier `NOT_RUN` for full CI was the pre-PR state; this is the verified
post-PR result. The full CI does not include a live Slack send and does not create
a manual acceptance record.

Focused evidence paths:

- `internal/cli/terminal.go` — terminal sink.
- `internal/cli/terminal_output_test.go` — reproduced violations and sink tests.
- `internal/cli/cli.go` — sink wiring in `run`.
- `internal/cli/activity.go` — terminal detection unwraps the sink.
- `internal/notify/slack.go`, `internal/notify/slack_test.go` — mrkdwn escaping.

## Criterion-to-evidence states

| Criterion (abbrev.)                                                     | State                                                                          | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ----------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Inventory lists origin, parser, validation/encoding, effect, owner      | `IMPLEMENTED`                                                                  | §Sink inventory                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Approved shell execution distinguished from interpolation               | `IMPLEMENTED`                                                                  | §Intentional shell execution versus interpolation; `internal/toolharness/harness.go`; `internal/git/command.go`; `internal/github/command.go`                                                                                                                                                                                                                                                                                                          |
| Terminal controls and owned-renderer fetching identified                | `IMPLEMENTED`                                                                  | §Terminal controls and resource fetching in owned renderers                                                                                                                                                                                                                                                                                                                                                                                            |
| Report delivered                                                        | `VERIFIED`                                                                     | This file                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Data arguments cannot become shell syntax                               | `VERIFIED` (existing)                                                          | `SplitCommand` tests; `commandpolicy` tests; no interpolation found                                                                                                                                                                                                                                                                                                                                                                                    |
| Invalid structured operations cannot partially execute                  | `VERIFIED` (existing)                                                          | File tools validate every argument before resolving or writing; tests in `internal/toolharness`                                                                                                                                                                                                                                                                                                                                                        |
| File operations retain workspace/symlink boundaries                     | `VERIFIED` (existing)                                                          | `resolveIn`/`evalExisting`; `TestReadRejectsEscapes`, root-boundary tests                                                                                                                                                                                                                                                                                                                                                                              |
| Database data stays parameterized                                       | `VERIFIED` (inspection + store tests)                                          | `internal/store` uses placeholders; no value formatting in SQL construction                                                                                                                                                                                                                                                                                                                                                                            |
| Owned display paths cannot execute terminal controls                    | `IMPLEMENTED`, `ENABLED`, `VERIFIED` on Linux CLI; re-verified on Darwin arm64 | GAP-01 fix and tests; §Verification dimensions                                                                                                                                                                                                                                                                                                                                                                                                         |
| Owned display paths cannot silently fetch attacker-selected resources   | `NOT_APPLICABLE` for SOP-owned fetches; Slack unfurl is external               | §Terminal controls…; outbound endpoints are configured                                                                                                                                                                                                                                                                                                                                                                                                 |
| Slack payload text cannot inject mrkdwn mentions or masked links        | `IMPLEMENTED`, `VERIFIED` (deterministic; live payload fixture)                | `TestEncodeSlackPayloadEscapesMrkdwn`; §Verification dimensions                                                                                                                                                                                                                                                                                                                                                                                        |
| HTTP transport delivers and classifies acknowledgement outcomes         | `VERIFIED` on loopback (in-process) and once live at `0200ad3f`                | `TestAcceptanceNotifyTestDeliversToLocalReceiver`; response-classification tests; live `sop notify drain` recorded `DELIVERED` on attempt 1                                                                                                                                                                                                                                                                                                            |
| Legitimate formatting/authorized operations remain usable               | `VERIFIED`                                                                     | Newline/tab and UTF-8 preserved; JSON result preserved; existing toolharness/store tests pass                                                                                                                                                                                                                                                                                                                                                          |
| Sink-specific fix, no destructive global escaping of persisted data     | `IMPLEMENTED`                                                                  | Terminal sink at the console only; artifacts and JSON unchanged                                                                                                                                                                                                                                                                                                                                                                                        |
| Protocol data valid; diagnostics do not corrupt machine-readable output | `VERIFIED`                                                                     | `TestPromptJSONKeepsModelResultIntact`; MCP `json.Encoder`; ACP `json.Marshal`; diagnostics on stderr                                                                                                                                                                                                                                                                                                                                                  |
| Synthetic canaries stay contained                                       | `VERIFIED` (deterministic; live payload fixture)                               | Control sequences removed from terminal and Slack bodies in tests; the live probe body escaped `& < >`; Slack rendering of the live body is `NOT_OBSERVED`                                                                                                                                                                                                                                                                                             |
| Required checks pass; PR records OS/client coverage                     | `PASS` (automated CI); live probe `VERIFIED`; manual acceptance `GAP`          | Full CI [42718751282](https://github.com/imhttran/agentic-sop/actions/runs/42718751282) and CodeQL [42718748910](https://github.com/imhttran/agentic-sop/actions/runs/42718748910) passed on `eaf34114`; focused checks pass at the live-probe revision `0200ad3f`; the PR body records Linux `x86_64`, Darwin arm64, the live probe, and the unverified external clients. Passing CI is evidence for this manual criterion, not an acceptance record. |
| Cumulative OWASP report updated                                         | `IMPLEMENTED` (this PR)                                                        | `docs/reports/owasp-llm/README.md`                                                                                                                                                                                                                                                                                                                                                                                                                     |
| No full coverage inferred from plan/PR existence                        | `VERIFIED`                                                                     | The matrix separates `IMPLEMENTED`, `VERIFIED`, `NOT_RUN`, `GAP`, and `NOT_APPLICABLE`                                                                                                                                                                                                                                                                                                                                                                 |

## Live-evidence and governance states

These are not Plan 10 acceptance criteria; they are separate evidence and lifecycle
dimensions that the matrix keeps visible.

| Item                                         | State                         | Note                                                                                                                                                                                                                                                                                                                                           |
| -------------------------------------------- | ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Live Slack delivery (transport)              | `VERIFIED` (once)             | One authorized synthetic event was delivered through the configured webhook at `0200ad3f`; the outbox recorded `DELIVERED` on attempt 1 with `delivered=1`, `pending=0`, `exhausted=0`. See [llm10-live-delivery.md](llm10-live-delivery.md).                                                                                                  |
| Observed Slack rendering                     | `NOT_OBSERVED`                | No authenticated Slack session was available on the test host. The channel name, visible text, and absence of pings remain a human observation.                                                                                                                                                                                                |
| Live send authorization                      | `RECORDED` (task instruction) | The operator commissioning this work authorized one synthetic notification to `#sop-alert`; no separate signed approval reference exists in the repository.                                                                                                                                                                                    |
| Manual acceptance by the designated approver | `GAP`                         | No owning SOP plan state exists, so there is no recorded outcome or `sop outcome --accept` record. Passing CI and the live transport probe are evidence, not acceptance. See [llm10-acceptance-request.md](llm10-acceptance-request.md).                                                                                                       |
| Manual-acceptance binding                    | `LIMITATION`                  | `sop outcome --accept` requires a run-derived `outcome.json` with a manual mapping and repository content identity. `sop task complete --external` clears `outcome.json`, so external completion cannot bind the six decisions. Plan closure does not read `acceptance_decisions.jsonl`; readiness alone is not acceptance.                    |
| SOP lifecycle completion                     | `GAP`                         | The owning main checkout and the disposable probe worktree both report no active plan: `sop plan historicalize --check` returns `Eligible: no` with reason `no active plan is recorded`; `sop continue --check` returns `RECONCILE_FAILED`, `HUMAN_REVIEW_REQUIRED`, and `0 tasks`; `sop approvals --json` is empty. No transition is claimed. |

## Residual risks and limitations

| Item                                | State                                          | Note                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ----------------------------------- | ---------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Third-party terminal/IDE rendering  | NOT_APPLICABLE / limitation                    | SOP filters its own console writes. A terminal that reinterprets persisted Markdown, or an IDE that renders `report.md`, is outside SOP control.                                                                                                                                                                                                                                                                                   |
| Persisted Markdown artifacts        | GAP / bounded                                  | `report.md`, `result.md`, and `implementation.md` retain model text as data. They are not executed by SOP, but a downstream renderer may interpret embedded HTML.                                                                                                                                                                                                                                                                  |
| Slack client behavior               | limitation                                     | `&`, `<`, and `>` are escaped, so angle-bracket mention and masked-link syntax is neutralized. Slack still interprets non-angular mrkdwn formatting (`*`, `_`, `~`, backtick, `>`) and emoji shortcodes, and may auto-unfurl a bare URL; that fetch is performed by Slack, not SOP.                                                                                                                                                |
| MCP/ACP client rendering            | NOT_APPLICABLE / limitation                    | Protocol payloads are valid JSON. How an external client renders the returned prose is outside SOP control.                                                                                                                                                                                                                                                                                                                        |
| Live notification delivery          | transport `VERIFIED`; rendering `NOT_OBSERVED` | One authorized synthetic event was accepted by the configured Slack webhook at `0200ad3f` and recorded `DELIVERED` on attempt 1; the payload fixture escaped `& < >`. Rendering, the bound channel name, and mention/fetch behavior were not observed. See [llm10-live-delivery.md](llm10-live-delivery.md). The probe used a bounded drain with an empty pre-send outbox and no retry; a retry can duplicate the visible message. |
| Notification status channel scoping | limitation                                     | `sop notify status` counts deliveries globally, not per channel. A prior confirmed delivery on another channel can make `availability` report `active` before the current channel has delivered. The live-test procedure records the snapshot as SOP state, not as channel proof.                                                                                                                                                  |
| Windows terminal behavior           | NOT_RUN                                        | The sink is platform-independent Go, but only Linux `x86_64` (original) and Darwin arm64 (wrap-up) were executed.                                                                                                                                                                                                                                                                                                                  |
| Unicode bidi/format characters      | bounded                                        | `sanitizeTerminal` removes C0/C1 control runes but does not strip Cf runes (for example U+202E). They cannot execute a terminal sequence, but they can visually reorder text.                                                                                                                                                                                                                                                      |
| Provider/tool output as model input | GAP / bounded                                  | Tool output is bounded and delivered to the model as data. SOP does not claim general content sanitization for model reasoning.                                                                                                                                                                                                                                                                                                    |

## Handoff

- Display-sink handling is implemented for the SOP-owned CLI terminal and the Slack
  payload encoder; loopback HTTP transport is verified, and one authorized live
  Slack delivery was confirmed at `0200ad3f` (outbox `DELIVERED`, attempt 1, no
  retry). The wrap-up merged in PR #328 (`35340774`) with full CI passing on
  `eaf34114`. Observed Slack rendering remains `NOT_OBSERVED`, and manual
  acceptance is not recorded.
- The merged documentation PR is not an SOP lifecycle transition: no owning plan
  state exists, and no completion or historicalization is claimed. `sop plan
historicalize --check` is ineligible (`no active plan is recorded`), and `sop
continue --check` returns `HUMAN_REVIEW_REQUIRED`.
- No shell, path, database, or protocol violation was reproduced at the current
  baseline; those controls remain owned by their existing packages.
- External client rendering and Slack auto-unfurl remain explicitly unverified.
  The live transport succeeded, but the channel binding, rendered text, and
  absence of pings still need a human observation in `#sop-alert`.
- Bounded claim: on the verified SOP-owned sinks (CLI stdout/stderr, Slack payload
  encoder, MCP/ACP JSON, loopback webhook transport, and one live webhook
  delivery), SOP removes terminal control characters from console display, escapes
  `&`/`<`/`>` in Slack mrkdwn so angle-bracket mentions and masked links cannot be
  injected, and keeps protocol output valid JSON. This is not a claim that all
  improper output handling is prevented, and it does not cover third-party
  renderers, persisted Markdown rendering, Slack auto-unfurl, observed Slack
  rendering, or unexecuted platforms.
- Plan 01 (LLM03 action authority) merged to `main` at `985472a` (PR #271) while
  this branch was in review; its report is linked in the cumulative matrix. This
  plan does not depend on its code to enforce the sink-specific controls above.
- Plan 07's evidence-claim work is still absent from this checkout, and no
  criterion here depends on it.
