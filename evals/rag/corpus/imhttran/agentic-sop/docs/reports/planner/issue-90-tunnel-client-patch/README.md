# tunnel-client patch for issue #90 (proposed, not installed)

A proposed fix for the intermittent HTTP 409 / `-32002` / `mcp_initialization_required`
in `openai/tunnel-client`, tracked in [#90](https://github.com/imhttran/agentic-sop/issues/90).
The patch is **not applied to the installed baseline binary**. A candidate was built from the
pinned revision plus this patch, run from an isolated path, and tested live; see
[Observed live result](#observed-live-result-2026-10-19). #90 stays open because the failure
is intermittent and the root cause is not conclusively confirmed.

- Baseline: `openai/tunnel-client` @ `5f99daabd4aa4a77049e6d81d54a0d8c18335397`
  (the installed revision).
- Patch: [`0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch`](0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch)
  (commit `c9349dc0`, 2 files, +309 / -10). Revised after review; earlier versions were
  `0372c1fc` and `560e70c3`. The last revision only adds a comment documenting the
  `childEpoch` safety invariant; no runtime behavior changed.

## What it changes

The stdio initialization guard keeps one readiness flag for the shared child, so three
paths could reject another session's calls:

1. An accepted `initialize` always cleared readiness until `notifications/initialized`
   arrived. A ready child now stays ready; an unready child still needs the full
   handshake. A server that rejects a duplicate `initialize` (go-sdk does) no longer
   leaves the child unready until restart.
2. A late `Close()` of a connection whose request already finished reset readiness and
   closed the shared pipes under a sibling request. That is now a no-op for the
   guard-enabled stdio transport.
3. (Added in review.) A late `Close()` of an abandoned connection (read or write failure)
   could reset readiness that by then described a replacement child a sibling had
   re-handshaken. The reset now applies only if no reset happened since that connection
   started (a per-transport child epoch). The destructive close of the pipes is kept, so
   possible late responses are still purged.

A `Close()` with the slot held, in-flight requests and fatal I/O still reset readiness and
close.

## Review findings (second pass)

- **Protocol / server lifecycle (re-initialize keeps readiness).** Checked in source, not in
  the specification text (it could not be fetched):
  - go-sdk v1.7.0 `ServerSession.initialize`: a duplicate `initialize` returns an error and
    the session stays initialized.
  - TypeScript SDK `Server._oninitialize` (9bc40e9): overwrites client capabilities; no
    gate on `initialized`, so requests are accepted throughout.
  - Python SDK (2af9e30) `runner.py` / `connection.initialize_accepted`: the request gate
    opens when `initialize` is recorded, and a repeat does not close it.
  - SOP's `sop mcp` server is stateless.
    None of these reverts to a state where operational requests are invalid after a
    duplicate `initialize`, so keeping readiness matches them. Older SDK versions and other
    servers were not checked.
- **Blocking gap found and fixed:** the first patch left a case where an abandoned
  connection's late `Close()` revoked a sibling's fresh readiness (new test
  `TestStdioInitializationLateCloseOfAbandonedConnectionKeepsReplacementReady` failed on the
  first patch). Item 3 above fixes it.
- **Checked, no change needed:** read errors, write errors, cancellation and concurrent
  ownership keep their prior behavior (abandoned and in-flight requests still reset and
  close; response-deadline retirement still preserves readiness); `lifecycleDone` is cleared
  on every new acquire; `go test -race` found no data race, including a 50-connection
  concurrent late-Close test.

## Verification (run in a scratch checkout)

- Against the pinned source six new tests fail; against the first patch one fails; with
  this patch all pass. Other new tests pin preserved behavior.
- `go test -race ./...` shows no failures; the initialization and serialized tests pass
  with `-count=10 -race`.
- A live end-to-end candidate run was performed on 2026-10-19; see below.

## Observed live result (2026-10-19)

The candidate was built from `openai/tunnel-client` @
`5f99daabd4aa4a77049e6d81d54a0d8c18335397` plus this patch and run from an isolated path. The
installed baseline at `~/.local/bin/tunnel-client` was not replaced.

| Run                                                   | Calls | Result                                                                                     |
| ----------------------------------------------------- | ----- | ------------------------------------------------------------------------------------------ |
| Baseline, 20-call acceptance run                      | 20    | **18/20**; two calls returned HTTP `409`, JSON-RPC `-32002`, `mcp_initialization_required` |
| Candidate, 20-call acceptance run                     | 20    | **20/20**, no retries, no state changes                                                    |
| Candidate, second 20-call round in the same debug log | 20    | **20/20**, no retries                                                                      |

Candidate identity (local; the binary is not committed):

- Path: an isolated `diagnostics/candidates/tunnel-client-issue90` path
- Version: `0.0.16+5f99daabd4aa4a77049e6d81d54a0d8c18335397.issue90-c9349dc`
- SHA-256: `58c88e509c5aab426fc3f996a0f4adae9bd41728240dd6131424ed2ad9ed7b38`

Baseline identity:

- Path: `~/.local/bin/tunnel-client`
- Version: `0.0.16+5f99daabd4aa4a77049e6d81d54a0d8c18335397`
- SHA-256: `6f1e8d5494d6c4d6c45cd4f6e07b5291459c0b23f4b8cd8266c7a64be3d9a2a0`

The candidate debug log has 40 `tools/call` and 40 `initialize` entries across two 20-call
rounds, with no structured entry showing HTTP `409`, JSON-RPC `-32002`,
`mcp_initialization_required`, or a dispatcher error. Filter the log structurally; a raw
substring search also matches digits inside timestamps and limits.

**This is not conclusive.** The baseline also produced clean runs without a fix, and no
failing candidate round with a debug log was captured. A clean 20/20 is supporting evidence,
not proof that the intermittent defect can never recur. #90 stays open.

## Agent-led workflow

The manual steps are preserved below for review, but the preferred workflow is documented in
[`docs/testing/PLANNER-TWO-QUESTION-LIVE.md`](../../../testing/PLANNER-TWO-QUESTION-LIVE.md)
and the handoff contract is
[`docs/testing/MCP-CANDIDATE-HANDOFF.md`](../../../testing/MCP-CANDIDATE-HANDOFF.md). The
agent uses `scripts/planner-tunnel-candidate.sh` to discover or fetch the pinned source,
verify the patch, build in an isolated directory, record provenance, verify the exact
candidate and process identity, and preserve rollback.

```sh
# Provision only. This does not start or stop the tunnel.
scripts/planner-tunnel-candidate.sh provision --profile sop-status-spike

# Verify the exact candidate, baseline, wrapper, and process set.
scripts/planner-tunnel-candidate.sh preflight

# Explicitly stop the verified baseline, then start the candidate after approval.
scripts/planner-tunnel-candidate.sh stop
scripts/planner-tunnel-candidate.sh start

# Collect evidence and hand off to ChatGPT.
scripts/planner-tunnel-candidate.sh status
scripts/planner-tunnel-candidate.sh handoff --tests LOCAL-TESTS.md

# Deterministic rollback: stop the candidate, restore the wrapper, start the baseline.
scripts/planner-tunnel-candidate.sh rollback --start
```

`provision` reuses an existing source checkout or build directory when the pinned revision
and patch match, retries bounded network failures, and never overwrites the installed
binary. `start` refuses to launch while another tunnel process is running and never signals a
PID whose exact command line differs from the recorded one. Building a candidate never
restarts the tunnel by itself.

### Demonstration

The isolated regression suite exercises the whole lifecycle against mock processes and a
temporary git checkout:

```sh
make test-planner-tunnel
```

It covers provision/dry-run, provenance, preflight, missing and mismatched binaries,
multiple and unexpected processes, startup failure, verified stop, wrapper restore, baseline
restart, the handoff contract, and the wrapper pointer's local-only dry-run. It never reads
or writes the live `.run/tunnel` state and never touches a real `tunnel-client`.

A separate real provision was run in an isolated checkout from the pinned local
`openai/tunnel-client` source plus this patch. It applied the patch, built the candidate
(version `0.0.16+d8ecb31`), recorded its SHA-256 and source/patch provenance, and passed
`preflight`. It was not started, so it is a build/identity demonstration, not a live MCP
validation.

## Apply (to a local clone, not the installed binary)

The preferred path is the candidate helper, which pins, patches, builds, and records
provenance automatically:

```sh
scripts/planner-tunnel-candidate.sh provision --profile sop-status-spike
```

For a manual review checkout:

```sh
git clone https://github.com/openai/tunnel-client && cd tunnel-client
git checkout 5f99daabd4aa4a77049e6d81d54a0d8c18335397
git am /path/to/0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch
go test -race ./pkg/mcpclient/...
```

## Known risks

- A re-initialize no longer revokes readiness, so a server that truly resets on one would
  answer operational calls with its own error instead of the tunnel's 409. None of the three
  reference SDKs or SOP does.
- A `Close()` of a connection that holds the slot but never wrote still resets readiness
  and closes the shared pipes (upstream pins this in `TestStdioInitializationReadyClearsOnCloseOrFatalIO`).
  Left unchanged; a cancellation between connect and write can still force a child respawn.
- The destructive close of an abandoned connection still closes the shared pipes, which can
  disturb a sibling in the same child; kept to purge late responses.
- Neither mechanism is confirmed as the cause of the original live 409.
