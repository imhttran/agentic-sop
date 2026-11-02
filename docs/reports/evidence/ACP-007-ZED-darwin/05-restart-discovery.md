# ACP-007 scenario 5 — Restart discovery (macOS, ACP client)

Status: **PASS** (ACP v1 client over stdio — **not** a Zed session)

Recorded 2026-11-02T21:15:40Z.

## Environment

| Field        | Value                                              |
| ------------ | -------------------------------------------------- |
| Host         | `Hoangs-MacBook-Pro-2.local`                       |
| OS           | macOS 26.6.2 (25G83), Darwin 25.6.0, arm64         |
| SOP          | `sop dev`, `/Users/imhttran/go/bin/sop`            |
| SOP revision | `ba5fbd965902584f04be258d23b05fb7fcb5850a`         |
| Client       | the same NDJSON JSON-RPC client, ACP v1 over stdio |
| Project      | `/tmp/acp-accept-proj` (disposable)                |

## Steps

1. Ended the `sop acp` process used by scenario 4 (clean EOF on stdin).
2. Started a **new** `sop acp` process against the same project.
3. `initialize`, `session/new`.
4. Compared the persisted job directory list before and after the restart.
5. Resubmitted the scenario-4 prompt text, unchanged, to test discovery of the
   prior job identity across the restart.

## Recorded results

```
jobs before restart:                     ['work-20261102-211430', 'work-20261102-211437', 'work-20261102-211503']
jobs after restart (before any prompt):  ['work-20261102-211430', 'work-20261102-211437', 'work-20261102-211503']
no automatic resubmission:               True
```

Resubmission of the same text (same capability, same fingerprint):

```json
{
  "id": 3,
  "jsonrpc": "2.0",
  "result": {
    "jobId": "work-20261102-211503",
    "jobState": "cancelled",
    "message": "this work was cancelled under this job identity; a new submissionKey starts fresh work",
    "outcome": "reconciled",
    "stopReason": "end_turn"
  }
}
```

```
jobs after resubmit:                     ['work-20261102-211430', 'work-20261102-211437', 'work-20261102-211503']
no new job created:                      True
prior job still on disk:                 True ['cancelled.json', 'exit.json', 'job.json', 'output.log', 'prompt.md']
```

stderr: empty.

## Observations

- Restarting the server started no work by itself; the job set was byte-identical
  before and after.
- The prior job identity was discoverable across the restart: resubmitting the
  same text returned `outcome: reconciled` with the same `jobId` and its recorded
  state (`cancelled`) instead of starting a second work item.
- No new job directory appeared, and the prior job's records (including its
  cancellation record) survived.
- The session itself was new (`session/new` returned a new `sessionId`), which is
  consistent with `loadSession: false`: conversation history is not restored,
  but persisted work is.
