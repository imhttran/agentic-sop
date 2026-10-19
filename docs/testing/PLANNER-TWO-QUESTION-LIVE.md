# Planner two-question live demo with a real executor

**Type:** Operator runbook. **Run 2026-10-18: GO** (attempt 3; see the attempt log and
[`planner-live-test-findings.md`](../plans/planner-live-test-findings.md)).
**Closes:** the evidence gap in
[SPIKE-006 §9](../plans/SPIKE-006-feasibility-report.md): a model-driven executor across
two planner questions in one task and run epoch, with SOP delivering the first decision to
the final round.

The earlier demos left two gaps. `planner-2q` ran two questions with a **scripted**
executor that kept the first decision in its own scratch file. `planner-chatgpt`
([findings](../plans/planner-live-test-findings.md)) used a **real** executor for one
question only. This run combines them: a real executor, two questions, and no scratch
state.

## What the run must show

1. The executor raises the ordering question, the run parks, ChatGPT answers, and SOP
   applies it.
2. The resumed executor raises the duplicates question. **Neither deliverable exists yet**,
   so the executor has stored the first decision nowhere.
3. ChatGPT answers; the final resume completes the task in the **same run epoch**, and
   `choices.txt` records **both** answers.

Because the executor keeps no state between rounds and wrote no file before step 3, the
first decision can reach the final round only through SOP's input. That is the
decision-history fix
([imhttran/agentic-sop#68](https://github.com/imhttran/agentic-sop/pull/68)).

## Prerequisites

- Ollama running locally with access to the executor model (default
  `deepseek-v4.1-flash:cloud`; override with `PLANNER2Q_MODEL`).
- `go` and `git` on `PATH`.
- The ChatGPT Secure MCP Tunnel set up as in
  [SPIKE-006 §1](../plans/SPIKE-006-feasibility-report.md#1-client-transport-and-setup).
- A dedicated SOP binary that **includes #68**. Build it from `main` once #68 is merged, or
  from the `claude/planner-decision-history` branch before then:

  ```bash
  go build -o "$TMPDIR/sop-2q" ./cmd/sop
  ```

  Do not use the installed `sop` unless it was built with #68.

## Agent-led candidate workflow (preferred)

The operator should not copy commands, apply patches, build binaries, or restart
the tunnel by hand. The agent owns preparation, deployment, and evidence
collection; the operator approves the two boundaries that matter (starting the
candidate and promoting a verified build).

1. **Isolate.** Create a branch or worktree for the fix. Do not edit the active
   SOP working tree or the live tunnel state.
2. **Implement and test locally.** Run the relevant Go tests and shell checks.
   Local tests prove the source and wrapper; they do not prove the live ChatGPT
   integration.
3. **Provision the candidate.** The helper discovers an existing checkout or
   cache first, otherwise fetches the pinned source, verifies and applies the
   pinned patch, builds into an isolated directory, and records the candidate
   path, version, SHA-256, source revision, patch SHA-256, baseline identity,
   profile, and launch arguments. It does not start or stop anything.

   ```sh
   scripts/planner-tunnel-candidate.sh provision \
     --profile sop-status-spike \
     --log-file "$PWD/.run/tunnel/logs/candidate-$(date +%Y%m%d-%H%M%S).jsonl"
   ```

   Defaults: `openai/tunnel-client` at
   `5f99daabd4aa4a77049e6d81d54a0d8c18335397`, with the issue #90 patch from
   [`docs/reports/planner/issue-90-tunnel-client-patch/`](../reports/planner/issue-90-tunnel-client-patch/README.md).
   Pass `--source PATH` to reuse a local checkout without modifying it, or
   `--build-dir PATH` / `--output PATH` to keep artifacts elsewhere. Relative
   paths are resolved against the checkout.

4. **Preflight.** Verify the exact candidate binary, checksum, version, wrapper,
   baseline, and process set before changing process state.

   ```sh
   scripts/planner-tunnel-candidate.sh preflight
   ```

5. **Preserve rollback.** Provision records the baseline binary, profile, launch
   arguments, wrapper checksum, and a wrapper backup. Nothing replaces the
   installed binary. The rollback command is printed by the helper and repeated
   in the handoff report.
6. **Stop only the verified baseline.** `stop` signals only the PID whose exact
   command line and binary identity match the manifest. It refuses when more
   than one tunnel process matches or when the process identity changed.

   ```sh
   scripts/planner-tunnel-candidate.sh stop
   ```

7. **After approval, start the candidate.** The helper launches it with the
   preserved profile and equivalent launch arguments, confirms startup, and
   records PID, version, checksum, and log path. It refuses to start a second
   tunnel. Building a candidate never restarts the tunnel by itself.

   ```sh
   scripts/planner-tunnel-candidate.sh start
   ```

8. **ChatGPT runs the live read-only test.** Use the exact request in
   [`MCP-CANDIDATE-HANDOFF.md`](MCP-CANDIDATE-HANDOFF.md): 10 rounds of
   concurrent `sop_status` and `sop_question_get`, 20 calls total, no retries,
   no state changes. The agent cannot run this layer.
9. **Collect evidence and prepare the PR.** `status` shows the recorded
   identity and running process; `handoff` prints the report with the debug-log
   path and rollback command. Promote the build only after ChatGPT reports its
   actual results and the operator approves. A clean 20/20 is supporting
   evidence, not proof the intermittent defect is gone.

### Local tests vs live validation

| Check                                                     | Where                                  | What it establishes                                                                                |
| --------------------------------------------------------- | -------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `go test`, the local wrapper MCP check, `preflight`       | Agent, local machine                   | The source builds, the binary identity matches, and the wrapper answers a read-only local MCP call |
| 10 concurrent rounds of `sop_status` + `sop_question_get` | ChatGPT, through the Secure MCP Tunnel | The real client/tunnel/server path works under the documented concurrent load                      |

The local wrapper check in `scripts/planner-tunnel-point.sh` is deliberately
labelled a local check. Do not cite it as a ChatGPT integration result.

### Download and provisioning guardrails

- Existing source checkouts, build directories, and Go caches are reused when
  the pinned revision and patch match; an existing directory is never overwritten.
- Clone/fetch retries are bounded. A failure reports the source, revision, and
  whether the failure was network, credential, or revision resolution.
- A patch is checked against the pinned revision before application. A failed
  `git am` is aborted, restoring the source checkout.
- The candidate binary is built to a temporary path and moved into place only
  after the build succeeds; an existing candidate is reused only when its
  version contains the expected source revision, otherwise `--replace` is
  required.
- The installed baseline binary is recorded, never replaced. No candidate is
  installed as the production `sop` or `tunnel-client` binary.
- The isolated regression suite exercises the full lifecycle without a live
  tunnel (`make test-planner-tunnel`). It uses mock processes and a temporary
  git checkout; it never reads or writes the live `.run/tunnel` state.

## Setup

```bash
scripts/planner-2q-fixture-setup.sh "$TMPDIR/planner-2q-live" "$TMPDIR/sop-2q"
```

The script creates a disposable project under `$TMPDIR` (never inside this checkout) with:

- planner enabled (`round_limit: 3`, no deadline), the `ollama` tool harness, and the
  human commit gate on;
- `input.txt` with repeated, unsorted lines;
- one task, `T001`, whose ordering and duplicate handling must be clarified with the
  planner, one question at a time;
- an operator-owned acceptance test (`go test ./...`) that accepts any of the four answer
  combinations but checks `normalized.txt` against the choices in `choices.txt`.

It refuses an existing root unless `PLANNER2Q_FORCE_RECREATE=1`, and it checks that the
acceptance test fails before the task runs.

Point the tunnel's MCP command at the new binary and fixture. The quickest way is
`scripts/planner-tunnel-point.sh --binary "$TMPDIR/sop-2q" "$TMPDIR/planner-2q-live"`: it
backs up and rewrites the wrapper, then checks it locally (`--build` builds a fresh binary
from `origin/main` instead). Add `--dry-run` to validate the project, binary, and wrapper
without writing anything. The wrapper script only changes what the tunnel will serve; it
does not restart the tunnel. Use `scripts/planner-tunnel-candidate.sh` for the verified
stop/start/rollback lifecycle. To write the wrapper by hand: the tunnel profile runs
`.run/tunnel/sop-mcp-planner.sh`; write it with absolute paths, because the tunnel does not
see your shell's `$TMPDIR`:

```sh
#!/bin/sh
FIXTURE="/var/folders/<...>/T/planner-2q-live"   # the same spelling as $TMPDIR
cd "$FIXTURE" || exit 1
export PWD="$FIXTURE"
exec "/var/folders/<...>/T/sop-2q" mcp --planner
```

- **With a binary older than the repository-identity fix, use the `$TMPDIR` spelling
  (`/var/...`) and export `PWD`.** Older binaries bind a question to the repository path
  without resolving symlinks. On macOS `/var` is a symlink to `/private/var`, and a process
  started without `PWD` sees `/private/var/...`, so SOP refuses the answer as `STALE`
  ("repository changed"). Current binaries resolve symlinks on both sides; the workaround
  is harmless with them.
- **Restart the tunnel client after every fixture recreate.** The running planner server
  keeps its working directory; after a recreate it no longer serves the new fixture. Use the
  candidate helper's verified `stop`/`start` rather than a broad process kill; do not start a
  second tunnel next to the running one.
- Test the wrapper locally before restarting the tunnel: pipe an MCP `initialize` and a
  `tools/call` for `sop_question_list` into it, and expect the pending `q-1`. This is a
  local check, not a ChatGPT-to-MCP validation.
- Keep all logs **outside** the fixture root; a log inside it enters the task's change set
  (this broke the earlier `planner-clarify` attempt).

## Run

Run every `sop` command from the fixture root with the dedicated binary. Capture output to
a log directory outside the fixture, for example `$TMPDIR/planner-2q-logs/`.

### Round 1: ordering

```bash
"$TMPDIR/sop-2q" run PLAN.md
```

Expected: `T001 PAUSED: WAITING_FOR_PLANNER`. Record:

- `"$TMPDIR/sop-2q" question list` shows one `PENDING` question about **ordering**.
- `ls choices.txt normalized.txt` fails: neither file exists.
- The question's run epoch (`"$TMPDIR/sop-2q" question <id>`).

In ChatGPT, have the planner read the question and submit an answer with an idempotency key
and the expected revision. Record the answer exactly.

### Round 2: duplicates

```bash
"$TMPDIR/sop-2q" run PLAN.md
```

Expected: SOP applies the first answer, resumes the same run, and parks again. Record:

- The first question is `APPLIED`; a second `PENDING` question asks about **duplicates**.
- **`ls choices.txt normalized.txt` still fails.** This is the key check: the executor has
  not written the first decision anywhere.
- `git status --short` in the fixture shows no new files besides SOP's own state.
- The run epoch is unchanged.

Answer the second question in ChatGPT and record the answer exactly.

### Round 3: completion

```bash
"$TMPDIR/sop-2q" run PLAN.md
```

Expected: the task completes. Record:

- `T001` is `LOCAL_DONE`, the run stage is `PASSED`, and the run epoch is unchanged.
- `choices.txt` matches **both** ChatGPT answers (`sort=...`, then `dedupe=...`).
- `.agent-sdlc/runs/T001/outcome.json` reports `status: "VERIFIED"` with the
  `go test ./...` acceptance check met.
- In `.agent-sdlc/questions.json`, the first question's continuation is `SUPERSEDED` and
  the second's is `COMPLETED`; none is `CLAIMED` or `PENDING`.
- `.agent-sdlc/runs/T001/implementation.md`, for any mention of the ordering decision.
  The run trace does not record prompt text, so this is supporting evidence only.

## Outcomes

| Result                                                                     | Classification                                                                                                                 |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| All round 3 checks pass, and no deliverable existed before round 3         | **GO** for the two-question loop with a real executor. Update SPIKE-006 §0, §8.1, and §9.                                      |
| `choices.txt` has the wrong ordering value, or the executor re-asks it     | **Fail**: the first decision did not reach the final round. Check the binary includes #68.                                     |
| The executor completes round 1 without asking (it chose the values itself) | **Fixture or model result, not an SOP failure.** Record it; adjust the task wording and rerun.                                 |
| The executor asks both questions at once, or returns `needs_human`         | **Fixture or model result, not an SOP failure.** Record it; adjust the task wording and rerun.                                 |
| A deliverable exists before round 3                                        | **Inconclusive**: the executor may have stored the first decision itself. Record it; the run does not prove decision delivery. |
| The task fails review or validation for reasons unrelated to the decisions | Record and diagnose as an ordinary run failure.                                                                                |

Record the result as a dated section in
[`planner-live-test-findings.md`](../plans/planner-live-test-findings.md) with the binary
fingerprint (`shasum -a 256 "$TMPDIR/sop-2q"`), the source commit, the exact questions and
answers, and the checks above. Keep the transcript sanitized: no credentials or environment
dumps.

## Attempt log

- **2026-10-18, attempt 1:** the executor completed round 1 without asking
  (`sort=true`, `dedupe=true`, outcome PARTIAL). The planning step had turned "clarify
  with the planner" into separate plan stages, and the implementation call chose both
  values itself. The planner contract did reach the IMPLEMENT request. The fixture now
  names the `needs_planner` outcome and when to use it, and adds the missing
  `### Acceptance Mapping`.
- **2026-10-18, attempt 2: inconclusive.** The fixture was built from the earlier script
  (no `needs_planner` wording). Round 1 asked about ordering; the resumed executor then
  chose `dedupe=true` itself and wrote both files. Review flagged it, the fix cycle changed
  nothing, and the gate failed. SOP did not complete a task that guessed a planner decision.
  The first answer submissions were refused as `STALE` because the tunnel wrapper used the
  `/private/var` spelling (see Setup).
- **2026-10-18, attempt 3: GO.** Real executor `deepseek-v4.1-flash:cloud`; binary built
  from `59e67ec` (#68). Round 1 raised `q-1` (ordering); ChatGPT answered `sort=true`.
  Round 2 resumed the same run epoch, raised `q-5` (duplicates), and superseded the first
  continuation; neither deliverable existed. ChatGPT answered `dedupe=false`. Round 3 wrote
  `choices.txt` = `sort=true` / `dedupe=false` and `normalized.txt` = `apple, apple,
banana, banana, cherry`; outcome `VERIFIED`, `LOCAL_DONE`, final gate `PASS`;
  continuations `q-1` `SUPERSEDED`, `q-5` `COMPLETED`. Full record in
  [`planner-live-test-findings.md`](../plans/planner-live-test-findings.md).

## Troubleshooting: HTTP 409 `mcp_initialization_required`

Tracks [#90](https://github.com/imhttran/agentic-sop/issues/90). The detailed evidence
(findings, filtered debug log) is attached there; do not copy raw logs into the repository.
**Status: intermittent, root cause unknown.** Observed and hypothesized facts are kept
apart below.

### Symptom

During concurrent read-only calls through the ChatGPT Secure MCP Tunnel, one call fails and
its sibling succeeds:

- message: `MCP server is not initialized; send initialize and notifications/initialized
before operational requests`
- connector error `CONFLICT`, HTTP `409`, JSON-RPC code `-32002`, error type
  `mcp_initialization_required`, origin `tunnel-client`.

A sequential retry succeeded, but retrying hides the failure; do not use it in the tests
below.

### Observed (from issue #90)

| Run                                                                  | Calls | Result                                                                                             |
| -------------------------------------------------------------------- | ----- | -------------------------------------------------------------------------------------------------- |
| Five rounds of `sop_question_get(q-1)` + `sop_status()` concurrently | 10    | **9 succeeded, 1 failed** (HTTP 409, round 2). Also reproduced with a single tunnel-client process |
| The same test repeated, debug logging on                             | 10    | **10 succeeded**, no retries                                                                       |

- Environment: tunnel-client `0.0.16`, revision `5f99daabd4aa4a77049e6d81d54a0d8c18335397`;
  SOP planner binary at clean revision `dc5fcbe00a281bf4de9bfd8e3bb51c9bf8f62c73`;
  ChatGPT Secure MCP Tunnel, then local tunnel-client, then `sop mcp --planner` over stdio.
- The debug log in the issue's filtered-log comment is from the **10/10** run, not the failing
  one: its ten `tools/call` responses (plus one earlier `tools/call`) all returned HTTP
  `200` with no JSON-RPC error, and the requests overlapped in time. The 10/10 run was made
  without any change, so one clean run does not show the problem is gone.
- Initialization trace of one successful session in that log: `server/discover` answered
  with JSON-RPC `-32601`, then `initialize` succeeded, then a notification was forwarded to
  the MCP server (the write returned no error), then `tools/call` succeeded. The
  `server/discover` error did not stop the tool call. The log does not record the
  notification's method, so it is **not** established that it was
  `notifications/initialized`.
- SOP side: the 409 message and `-32002` do not exist in the SOP source, and
  `TestMCPPlannerServerHasNoInitializationGate` shows `sop mcp --planner` answers
  `tools/call` with or without a prior `initialize`. The error is raised by tunnel-client.

### Observed candidate run (2026-10-19)

A patched tunnel-client candidate was built from the pinned revision plus the proposed
patch, run from an isolated path, and tested through the same ChatGPT Secure MCP Tunnel.
The installed baseline was not replaced.

| Run                                                             | Calls | Result                                                                            |
| --------------------------------------------------------------- | ----- | --------------------------------------------------------------------------------- |
| Baseline, 20-call acceptance run                                | 20    | **18/20**; two calls returned HTTP `409`, `-32002`, `mcp_initialization_required` |
| Candidate, 20-call acceptance run                               | 20    | **20/20**, no retries, no state changes                                           |
| Candidate, second 20-call round preserved in the same debug log | 20    | **20/20**, no retries                                                             |

- Candidate identity (local, not committed):
  `~/.local/.../diagnostics/candidates/tunnel-client-issue90`,
  version `0.0.16+5f99daabd4aa4a77049e6d81d54a0d8c18335397.issue90-c9349dc`,
  SHA-256 `58c88e509c5aab426fc3f996a0f4adae9bd41728240dd6131424ed2ad9ed7b38`.
- Baseline identity: `~/.local/bin/tunnel-client`, version
  `0.0.16+5f99daabd4aa4a77049e6d81d54a0d8c18335397`,
  SHA-256 `6f1e8d5494d6c4d6c45cd4f6e07b5291459c0b23f4b8cd8266c7a64be3d9a2a0`.
- The candidate debug log contains 40 `tools/call` and 40 `initialize` entries across
  two 20-call rounds, and no structured entry with HTTP `409`, JSON-RPC `-32002`,
  `mcp_initialization_required`, or a dispatcher error. A raw substring search is not
  sufficient: digits inside timestamps and limits can match `409` or `-32002`, so use the
  structured filter below.
- **Not conclusive.** The defect is intermittent and the root cause is not confirmed by a
  failing candidate round. A clean candidate run is supporting evidence; it does not prove
  the defect can never recur. Do not close #90 on this result alone.

### Hypothesis status after the candidate run

The patch addresses readiness loss across re-initialization and late `Close()` calls in
the stdio transport. The candidate result is consistent with that hypothesis, but the
baseline also had clean runs without a fix, so this is not proof of causality.

### Hypotheses (not confirmed)

Initialization-state handling, session reuse across concurrent calls, or a tunnel-client
lifecycle issue. In the 10/10 log, the ten calls each carry their own per-call request
identifier suffix and no `initialize` precedes them; whether that matters is untested.

### Run the concurrent read-only check

This is the **default live acceptance test** in
[`MCP-CANDIDATE-HANDOFF.md`](MCP-CANDIDATE-HANDOFF.md). Use a fixture with a pending
`q-1`. Do not change SOP state, answer a question, or retry a failed call.

1. Run **10 rounds**. In each, call `sop_question_get({"question_id":"q-1"})` and
   `sop_status({})` concurrently from ChatGPT, and wait for both before the next round.
2. Record, per call, success or the exact error fields above, and the wall-clock time.
3. Record every initialization failure, including HTTP `409` and
   `mcp_initialization_required`.
4. **Pass:** all **20** calls succeed with no retries and no state change.

The earlier five-round/ten-call procedure was the diagnostic run recorded in issue #90;
the ten-round/20-call form is the acceptance test. A clean 20/20 is supporting evidence,
not proof that an intermittent defect cannot recur.

### Capture a debug log

The candidate helper records the log path and starts the candidate with it. Provision with
`--log-file PATH`, then use the verified lifecycle:

```sh
scripts/planner-tunnel-candidate.sh preflight
scripts/planner-tunnel-candidate.sh stop       # only the verified baseline PID
scripts/planner-tunnel-candidate.sh start      # candidate, recorded log path
```

For a manual fallback, this command is for a **controlled restart**, not for starting a
second tunnel next to a running one. Stop the existing `tunnel-client` through the helper
first, then:

```sh
mkdir -p "$PWD/.run/tunnel/logs"
tunnel-client run \
  --profile sop-status-spike \
  --log.level debug \
  --log.format json \
  --log.file "$PWD/.run/tunnel/logs/issue-90-debug.jsonl"
```

Run it from the agentic-sop checkout. `.run/tunnel/` is git-ignored, so the log stays local.
Do not enable raw HTTP or payload capture options (for example `--log.http-raw-unsafe`):
they can record credentials and tool payloads. Keep any log outside the fixture root.

### Read the log

Each line is JSON. Reduce it to the correlation fields:

```sh
jq -c 'select(.component=="controlplane" or .component=="dispatcher")
  | {time,msg,request_id,cmd_request_id,rpc_method,rpc_request_id,has_error,rpc_error_code,status_code}
  | with_entries(select(.value != null))' .run/tunnel/logs/issue-90-debug.jsonl
```

To find failures, filter structurally rather than with a raw substring search (the digits
`409` and `-32002` also occur inside timestamps and limits):

```sh
jq -c 'select(.status_code >= 400 or .rpc_error_code != null)
  | {time,msg,request_id,rpc_method,status_code,rpc_error_code}' \
  .run/tunnel/logs/issue-90-debug.jsonl
```

- `request_id` ties one command through `control-plane command received`, `dispatcher
received response from MCP server`, `dispatcher delivered response to control plane`, and
  `dispatcher forwarded command to MCP server`. Use it with `time` to see overlap and order.
- `cmd_request_id` appears to group commands from one client session (inferred from the
  log, not documented); compare it across a failing call and its siblings, and note which
  call is the first to reach the MCP server.
- `rpc_method` shows `initialize` and `tools/call`; a notification is logged without a
  method, so it cannot be identified from these fields.
- **HTTP success is not JSON-RPC success.** `status_code` is the HTTP result delivered to the
  control plane; `has_error` and `rpc_error_code` are the JSON-RPC result. In the historical
  10/10 log `server/discover` has `status_code` 200 and `rpc_error_code` -32601: the transport
  worked and the application reported an error. The reproduced failure is the reverse case:
  HTTP 409 with JSON-RPC `-32002`.

### Preserve evidence safely

Logs contain local usernames and paths, tunnel and client identifiers, and request IDs.
Before sharing, reduce with the filter above, replace local paths and identifiers, and attach
the result to the issue, not the repository. Do not include tool arguments or results.

### Keep #90 open until

A failing round has a debug log showing why initialization was refused, a change addresses
that cause, and the 20-call test then passes (all 20 calls, no retries) **repeatedly**,
across more than one candidate process and more than one ChatGPT session. A clean 20/20 is
supporting evidence, not proof. Change `scripts/planner-tunnel-point.sh` only if the
evidence shows a wrapper or startup problem there; use
`scripts/planner-tunnel-candidate.sh` for the candidate lifecycle.

## Cleanup

The fixture is disposable. Remove `$TMPDIR/planner-2q-live` once its evidence is recorded,
and point the tunnel wrapper back at its previous working directory.
