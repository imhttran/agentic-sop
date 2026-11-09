# LLM10 — Improper output handling (SOP Linux workstream)

Status: two reproduced display-sink gaps `IMPLEMENTED` and `VERIFIED` on the Linux
CLI; live notification delivery `NOT_RUN`. Shell, path, database, and protocol
boundaries were inspected and their existing controls confirmed.

Baseline: `imhttran/agentic-sop` `main` at
`24be0687b0ac0dec5e2e69584be67f1795c0f6f1`, merged with `main` at `985472a`
before review (2026-11-08). Host: Linux `x86_64`, Go 1.27.1 (`linux/amd64`).
Branch: `security/llm10-improper-output-handling`.

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
- Used synthetic fixtures and in-process writers only. No live Slack message,
  exfiltration endpoint, production write, or downloaded artifact was used.
- Reused the existing command policy, path resolver, JSON encoders, and template
  auto-escaping. No new gate, policy layer, or permission system was added.

## Sink inventory

| Sink | Input origin | Parser / constructor | Validation / encoding | Effect | Policy owner |
| --- | --- | --- | --- | --- | --- |
| `run_command` (tool harness) | Model tool arguments | `toolharness.SplitCommand` tokenizes to `argv`; no shell | Shell metacharacters and globs are rejected; `commandpolicy.Classify` must return `SAFE`; paths are confined to an authorized root; arguments are passed as separate `exec` argv entries | Executes one approved program | `internal/toolharness`, `internal/commandpolicy` |
| Configured agent command | Operator environment (`SOP_AGENT_COMMAND`) | `agent.CommandAgent` runs `sh -c <configured command>` | Trusted configuration, not model text; the request is JSON on stdin | Model call | `internal/agent`; SECURITY.md §2 |
| Configured validation command | `.agent-sdlc/config.yaml` | `testrunner.execute` runs `sh -c <configured command>` | Trusted configuration; no request data is interpolated into the command | Build/test/lint process | `internal/testrunner`; SECURITY.md §2 |
| External review command | Operator environment (`SOP_REVIEW_COMMAND`) | `review.OCRProvider` runs `sh -c <configured command>` | Trusted configuration; the request is JSON on stdin | Review process | `internal/review` |
| Handoff compression command | Operator configuration | `caveman.ShellRunner` runs `sh -c <configured command>` | Trusted configuration; the bundle is JSON on stdin | Compression process | `internal/handoff/caveman` |
| Comparison executor/verification | Operator configuration/manifest | `hipcompare` runs `sh -c <configured command>` | Trusted configuration; no model text is interpolated | Comparison arms | `internal/hipcompare` |
| Acceptance check | Plan `### Acceptance Checks` | `cli.runAcceptanceArgv` runs a validated `argv` directly | `internal/commandpolicy` must classify it `SAFE`; no shell | Deterministic check | `internal/cli`, `internal/commandpolicy` |
| Git / GitHub | SOP code and task identity | `git run` and `github execRunner` build explicit argument lists | No shell; `GIT_TERMINAL_PROMPT=0` / `GH_PROMPT_DISABLED=1` | Repository and API operations | `internal/git`, `internal/github` |
| File tools (`write_file`, `create_file`, `delete_file`, `restore_file`, `copy_file`, `rewrite_file`) | Model tool arguments | `toolharness.resolveIn` canonicalizes against an authorized root | NUL rejected; symlink-resolved boundary check; `.agent-sdlc/state.db` protected; read-only roots refuse writes; arguments validated before any effect | File mutation inside an authorized root | `internal/toolharness` |
| SQLite state | SOP code and task/plan identity | `database/sql` with bound placeholders | Data values are parameters; the only concatenated SQL is a constant column list | Persisted task/run/notification state | `internal/store` |
| Controller HTML | SOP state and persisted artifacts | `html/template` | Automatic contextual escaping; no `template.HTML`, `template.JS`, `template.URL`, or `template.CSS`; no model-authored markup is trusted | Browser dashboard | `internal/controller/web` |
| CLI terminal (stdout/stderr) | Model prose, review/JEV findings, provider errors, validation output, SOP reasons | `cli.run` wraps both writers in `terminalSink` | C0/C1 control runes removed except tab and newline; multi-byte UTF-8 preserved across writes | Operator console | `internal/cli` |
| Notification webhook (native) | SOP lifecycle event | `notify.EncodeNativePayload` / `json.Marshal` | JSON encoding escapes control characters; the endpoint is configured, never model-selected | HTTP POST | `internal/notify` |
| Notification webhook (Slack) | SOP lifecycle event, including model-derived reasons | `notify.EncodeSlackPayload` → `slackEventText` | JSON encoding plus `&amp;`/`&lt;`/`&gt;` mrkdwn escaping | Slack message | `internal/notify` |
| MCP protocol | SOP application services | `mcp` server `json.NewEncoder(stdout)` | JSON encoding escapes control characters; diagnostics go to stderr | JSON-RPC tool results | `internal/mcp` |
| ACP protocol | SOP application services | `acp` transport `json.Marshal` → framed writer | JSON encoding escapes control characters; diagnostics are bounded and redacted | JSON-RPC session frames | `internal/acp` |
| Persisted Markdown (`report.md`, `result.md`, `implementation.md`) | Model prose and review/JEV findings | Run artifact writer | Stored as data; SOP does not execute or auto-render these artifacts | Later human/tool reading | `internal/run`, `internal/cli` |
| Outbound HTTP (providers, webhook, decision adapters, RAG) | Configured base URL | `net/http` request construction | Target comes from configuration or environment, never from model output | Provider/webhook request | `internal/provider`, `internal/agent`, `internal/notify`, `internal/decisionadapter` |

### Intentional shell execution versus interpolation

Shell execution is intentional only for **trusted operator configuration**: the
agent command, validation commands, external review command, handoff compression
command, and comparison commands. Those commands are run with `sh -c`, and request
data reaches them as JSON on stdin, not by string interpolation
(`docs/specs/SECURITY.md` §2).

The model-facing `run_command` tool does **not** use a shell. `SplitCommand`
rejects `; & | < > $ \` newline carriage-return * ? [ ] { } ( ) ~`, and the
resulting `argv` is executed directly after `commandpolicy.Classify` returns
`SAFE`. Every path-like argument is resolved against the selected authorized root.
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
`TestEncodeSlackPayloadEscapesMrkdwn` covers the payload; no live Slack message is
sent.

## Verification record

Baseline `24be068` plus this change set, Linux `x86_64`, Go 1.27.1.

| Command | Result | Notes |
| --- | --- | --- |
| `go test ./internal/cli -run Terminal -count=1` | `PASS` | The two control-sequence tests reproduced `FAIL` before the fix; all sink tests pass after it. |
| `go test ./internal/cli -run TestPromptJSONKeepsModelResultIntact -count=1` | `PASS` | Machine-readable output stays valid and keeps the decoded model result exactly. |
| `go test ./internal/cli -count=1` | `PASS` (244 s) | Full CLI package with `TMPDIR` on `/srv/sop/tmp/llm10` (RAM-backed I/O path); the earlier disk-backed run hit Go's 10-minute package timeout in an unrelated init/store test. |
| `go test ./internal/toolharness ./internal/notify ./internal/store ./internal/review ./internal/jev ./internal/controller/... -count=1` | `PASS` | Command policy, path boundary, store, review/JEV parse, and controller renderer packages. |
| `go vet ./internal/cli ./internal/notify ./internal/toolharness ./internal/store` | `PASS` | No diagnostics. |

Focused evidence paths:

- `internal/cli/terminal.go` — terminal sink.
- `internal/cli/terminal_output_test.go` — reproduced violations and sink tests.
- `internal/cli/cli.go` — sink wiring in `run`.
- `internal/cli/activity.go` — terminal detection unwraps the sink.
- `internal/notify/slack.go`, `internal/notify/slack_test.go` — mrkdwn escaping.

## Criterion-to-evidence states

| Criterion (abbrev.) | State | Evidence |
| --- | --- | --- |
| Inventory lists origin, parser, validation/encoding, effect, owner | `IMPLEMENTED` | §Sink inventory |
| Approved shell execution distinguished from interpolation | `IMPLEMENTED` | §Intentional shell execution versus interpolation; `internal/toolharness/harness.go`; `internal/git/command.go`; `internal/github/command.go` |
| Terminal controls and owned-renderer fetching identified | `IMPLEMENTED` | §Terminal controls and resource fetching in owned renderers |
| Report delivered | `VERIFIED` | This file |
| Data arguments cannot become shell syntax | `VERIFIED` (existing) | `SplitCommand` tests; `commandpolicy` tests; no interpolation found |
| Invalid structured operations cannot partially execute | `VERIFIED` (existing) | File tools validate every argument before resolving or writing; tests in `internal/toolharness` |
| File operations retain workspace/symlink boundaries | `VERIFIED` (existing) | `resolveIn`/`evalExisting`; `TestReadRejectsEscapes`, root-boundary tests |
| Database data stays parameterized | `VERIFIED` (inspection + store tests) | `internal/store` uses placeholders; no value formatting in SQL construction |
| Owned display paths cannot execute terminal controls | `IMPLEMENTED`, `ENABLED`, `VERIFIED` on Linux CLI | GAP-01 fix and tests |
| Owned display paths cannot silently fetch attacker-selected resources | `NOT_APPLICABLE` for SOP-owned fetches; Slack unfurl is external | §Terminal controls…; outbound endpoints are configured |
| Legitimate formatting/authorized operations remain usable | `VERIFIED` | Newline/tab and UTF-8 preserved; JSON result preserved; existing toolharness/store tests pass |
| Sink-specific fix, no destructive global escaping of persisted data | `IMPLEMENTED` | Terminal sink at the console only; artifacts and JSON unchanged |
| Protocol data valid; diagnostics do not corrupt machine-readable output | `VERIFIED` | `TestPromptJSONKeepsModelResultIntact`; MCP `json.Encoder`; ACP `json.Marshal`; diagnostics on stderr |
| Synthetic canaries stay contained | `VERIFIED` | Control sequences removed from terminal and Slack bodies in tests |
| Required checks pass; PR records OS/client coverage | `NOT_RUN` until the PR is opened | See the PR body |
| Cumulative OWASP report updated | `IMPLEMENTED` | `docs/reports/owasp-llm/README.md` |
| No full coverage inferred from plan/PR existence | `IMPLEMENTED` | The matrix separates `IMPLEMENTED`, `VERIFIED`, `NOT_RUN`, `GAP`, and `NOT_APPLICABLE` |

## Residual risks and limitations

| Item | State | Note |
| --- | --- | --- |
| Third-party terminal/IDE rendering | NOT_APPLICABLE / limitation | SOP filters its own console writes. A terminal that reinterprets persisted Markdown, or an IDE that renders `report.md`, is outside SOP control. |
| Persisted Markdown artifacts | GAP / bounded | `report.md`, `result.md`, and `implementation.md` retain model text as data. They are not executed by SOP, but a downstream renderer may interpret embedded HTML. |
| Slack client behavior | limitation | Mrkdwn mention/link syntax is neutralized. Slack may still auto-unfurl a bare URL; that fetch is performed by Slack, not SOP. |
| MCP/ACP client rendering | NOT_APPLICABLE / limitation | Protocol payloads are valid JSON. How an external client renders the returned prose is outside SOP control. |
| Live notification delivery | NOT_RUN | No live Slack message was sent. The encoder is unit-tested only. |
| Windows terminal behavior | NOT_RUN | The sink is platform-independent Go, but only the Linux CLI was executed here. |
| Unicode bidi/format characters | bounded | `sanitizeTerminal` removes C0/C1 control runes but does not strip Cf runes (for example U+202E). They cannot execute a terminal sequence, but they can visually reorder text. |
| Provider/tool output as model input | GAP / bounded | Tool output is bounded and delivered to the model as data. SOP does not claim general content sanitization for model reasoning. |

## Handoff

- Display-sink handling is implemented for the Linux CLI terminal and the Slack
  payload encoder.
- No shell, path, database, or protocol violation was reproduced at the current
  baseline; those controls remain owned by their existing packages.
- External client rendering and live Slack delivery remain explicitly unverified.
- Plan 01 (LLM03 action authority) merged to `main` at `985472a` (PR #271) while
  this branch was in review; its report is linked in the cumulative matrix. This
  plan does not depend on its code to enforce the sink-specific controls above.
- Plan 07's evidence-claim work is still absent from this checkout, and no
  criterion here depends on it.
