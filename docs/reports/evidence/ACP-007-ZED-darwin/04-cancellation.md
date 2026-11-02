# ACP-007 scenario 4 — Cancellation (macOS, ACP client)

Status: **PASS** (ACP v1 client over stdio — **not** a Zed session)

Recorded 2026-11-02T21:15:03Z.

## Environment

| Field        | Value                                                            |
| ------------ | ---------------------------------------------------------------- |
| Host         | `Hoangs-MacBook-Pro-2.local`                                     |
| OS           | macOS 26.6.2 (25G83), Darwin 25.6.0, arm64                       |
| SOP          | `sop dev`, `/Users/imhttran/go/bin/sop`                          |
| SOP revision | `ba5fbd965902584f04be258d23b05fb7fcb5850a`                       |
| Client       | the same NDJSON JSON-RPC client as scenario 2, ACP v1 over stdio |
| Project      | `/tmp/acp-accept-proj` (disposable)                              |

## Steps

1. `initialize`, then `session/new` on the disposable project.
2. `session/prompt` with `capability: "implement"` and one text block:
   `Add a CODE_OF_CONDUCT.md describing expected contributor behaviour.`
3. `session/cancel` immediately after the start acknowledgement.

## Recorded transcript

```json
{"id":2,"jsonrpc":"2.0","result":{"sessionId":"sess_13c292250ff566f7"}}
{"id":3,"jsonrpc":"2.0","result":{"jobId":"work-20261102-211503","jobState":"running","message":"Started. Call sop_work_status with this job_id (or sop_status) to follow it.","outcome":"started","stopReason":"end_turn"}}
{"id":4,"jsonrpc":"2.0","result":{"jobId":"work-20261102-211503","jobState":"cancelled","message":"the session's foreground governed work was cancelled; its persisted record is authoritative","outcome":"cancelled","sessionId":"sess_13c292250ff566f7","stopReason":"cancelled"}}
```

stderr: empty.

## Persisted record

Path: `/tmp/acp-accept-proj/.agent-sdlc/runs/mcp-work/work-20261102-211503/`

```
cancelled.json  exit.json  job.json  output.log  prompt.md
```

```json
job.json:      {"version":1,"id":"work-20261102-211503","kind":"prompt","capability":"implement","fingerprint":"cb1fd560ac39b4937d70bbc5bcd37fe04df6e7b72d9afd417ef86e3a2e2dc455","pid":16083,"started_at":"2026-11-02T21:15:03.848708Z"}
cancelled.json: {"cancelled_at":"2026-11-02T21:15:03.899811Z","by":"sop"}
exit.json:     {"exit_code":-1,"ended_at":"2026-11-02T21:15:03.901465Z"}
```

## Observations

- Outcome `cancelled` with stop reason `cancelled`, and a distinct
  `cancelled.json` record — not relabelled as completion or failure.
- Observed stop bound: started 21:15:03.848708Z, cancelled 21:15:03.899811Z,
  about **51 ms**.
- `exit.json` records `exit_code: -1`, and the cancellation record is the
  authoritative result for that job identity.
- The cancelled job wrote no files: `CODE_OF_CONDUCT.md` does not exist in the
  project.
- A terminal `session/update` was received for the start (`state: running`,
  `verified: false`), and the cancel response carried the terminal outcome.

## Finding recorded during this scenario (not part of the acceptance result)

An earlier probe sent `session/cancel` with a `jobId` field and was rejected by
strict decoding:

```json
{
  "error": {
    "code": -32602,
    "message": "invalid session/cancel params: json: unknown field \"jobId\"; expected field is sessionId"
  },
  "id": 4,
  "jsonrpc": "2.0"
}
```

`internal/acp/contract.go` describes the params as "{sessionId} plus one
optional jobId or submissionKey identity", while `sessionCancelRequest` in
`internal/acp/cancel.go` accepts only `sessionId`. The documentation and the
decoder disagree. The acceptance result above was produced with the accepted
shape, `{"sessionId": "..."}`.

That probe's job (`work-20261102-211437`) was not cancelled and ended with exit
code `1` (`PARTIAL`: "deterministic checks did not pass"). It is recorded here
only so the extra job in the project is accounted for.
