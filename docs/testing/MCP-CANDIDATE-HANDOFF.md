# MCP candidate handoff contract

This is the handoff contract for a planner MCP candidate. It exists because the
agent can build and locally test a candidate, but it cannot claim a live
ChatGPT-to-MCP integration result. The two layers are different:

| Layer | What it proves | Who runs it |
| --- | --- | --- |
| Local unit/integration tests | The source builds, the wrapper answers an MCP `tools/call` locally, and the candidate's recorded identity matches its files | The agent |
| Live ChatGPT-to-MCP test | The real ChatGPT client, Secure MCP Tunnel, `tunnel-client` candidate, and `sop mcp --planner` work together under concurrent load | ChatGPT |

A clean local run is never a substitute for the live layer. A clean live 20/20
run is supporting evidence, not proof that an intermittent defect can never
recur.

## Required handoff report

The agent produces this before asking ChatGPT to run the live test. The
`scripts/planner-tunnel-candidate.sh handoff` command prints the recorded facts
and placeholders for the two result sections.

```markdown
# Planner MCP candidate handoff

## Repository
- Branch:
- Commit:
- Working tree:

## Candidate executable
- Path:
- Version:
- SHA-256:
- Source:
- Patch (SHA-256):
- Profile:
- Baseline (unchanged):

## Active process
- Manifest state:
- PID:
- Command:
- Debug log:

## Local tests (not a live ChatGPT test)
<commands and results>

## ChatGPT live MCP results
<ChatGPT pastes its actual results here>

## Rollback
<exact command>

## Exact read-only MCP test request
<the request below>

## Acceptance
<the criteria below>
```

Do not fill the ChatGPT section yourself. Do not mark the candidate verified
until ChatGPT reports actual results.

## Exact read-only MCP test request

Use a fixture with a pending `q-1`. Do not change SOP state, answer a question,
or retry a failed call.

Run **10 rounds**. In each round, call these two tools **concurrently** and wait
for both responses before starting the next round:

- `sop_status({})`
- `sop_question_get({"question_id":"q-1"})`

That is **20 calls total**. Record, per call:

- wall-clock time;
- success, or the exact error fields: HTTP status, JSON-RPC code, error type,
  and origin;
- every initialization failure, including HTTP `409` and
  `mcp_initialization_required`.

**Pass:** all 20 calls succeed with no retries and no state change.

**Fail:** any call fails. Record the exact error and the matching lines from the
candidate debug log.

## Rollback

The handoff report contains the exact command. It is normally:

```sh
scripts/planner-tunnel-candidate.sh rollback --checkout <checkout> --start
```

Rollback stops only the verified candidate PID, restores the wrapper backup,
and starts the recorded baseline with the preserved launch arguments. Keep the
candidate binary and debug log until the evidence is recorded; do not delete
them before rollback.

## Evidence hygiene

Tunnel debug logs contain local usernames and paths, tunnel/client identifiers,
and request IDs. Before sharing outside the machine, reduce the log to the
correlation fields, replace local paths and identifiers, and attach the reduced
result to the issue rather than committing raw logs. Do not enable raw HTTP or
payload capture options.
