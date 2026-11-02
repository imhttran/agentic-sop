# ACP-007 scenario 2 — One bounded request (macOS, ACP client)

Status: **PASS** (ACP v1 client over stdio — **not** a Zed session)

Recorded 2026-11-02T21:14:30Z. Zed was running but did not drive this run; see
`01-agent-appears.md` for the Zed-side scenario that remains unexecuted.

## Environment

| Field        | Value                                                                                                           |
| ------------ | --------------------------------------------------------------------------------------------------------------- |
| Host         | `Hoangs-MacBook-Pro-2.local`                                                                                    |
| OS           | macOS 26.6.2 (25G83), Darwin 25.6.0, arm64                                                                      |
| SOP          | `sop dev`, `/Users/imhttran/go/bin/sop`                                                                         |
| SOP revision | `ba5fbd965902584f04be258d23b05fb7fcb5850a`                                                                      |
| Client       | custom NDJSON JSON-RPC client written for this run, speaking ACP v1 over the server's stdio                     |
| Project      | `/tmp/acp-accept-proj` (disposable: `git init`, `sop init`; no project manifest, so validation is unconfigured) |

## Steps

1. `initialize` with `protocolVersion` `v1`.
2. `session/new` with `{"cwd":"/tmp/acp-accept-proj","mcpServers":[]}`.
3. `session/prompt` with `capability: "plan"` and one text block:
   `Summarize this project's repository layout in one sentence.`
4. Followed the job through `.agent-sdlc/runs/mcp-work/<job-id>/`.

## Recorded transcript

```json
{"id":1,"jsonrpc":"2.0","result":{"agentCapabilities":{"loadSession":false},"agentInfo":{"name":"sop","version":"dev"},"protocolVersion":"v1"}}
{"id":2,"jsonrpc":"2.0","result":{"sessionId":"sess_511962c0fdcfbae7"}}
{"id":3,"jsonrpc":"2.0","result":{"jobId":"work-20261102-211430","jobState":"running","message":"Started. Call sop_work_status with this job_id (or sop_status) to follow it.","outcome":"started","stopReason":"end_turn"}}
{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_511962c0fdcfbae7","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"SOP job work-20261102-211430 is running; progress only, work is not complete."}},"sop":{"jobId":"work-20261102-211430","state":"running","verified":false}}}
```

stderr: empty.

## Persisted record (agrees with the reported outcome)

Path: `/tmp/acp-accept-proj/.agent-sdlc/runs/mcp-work/work-20261102-211430/`

```
exit.json  job.json  output.log  prompt.md
```

```json
job.json:  {"version":1,"id":"work-20261102-211430","kind":"prompt","capability":"plan","fingerprint":"7fc6a129b7f659396d98d64016e78ac756b09fe74d9b9265fed876f22e67218c","args":["prompt","--capability","plan","--file",".../prompt.md"],"pid":16057,"started_at":"2026-11-02T21:14:30.482284Z"}
exit.json: {"exit_code":0,"ended_at":"2026-11-02T21:14:31.594075Z"}
```

- Outcome `started`, then completed with exit code `0` about 1.1 s later.
- `output.log` records a read-only plan (`steps` name `list_files` and `read_file`
  only; 745 in / 144 out tokens across 1 model call).
- The `session/update` frame is explicitly progress-only (`verified: false`).

## Provenance note

This run did not modify the project. The untracked `CONTRIBUTING.md` that exists
in the disposable project was written by the later `implement` attempt in
scenario 4: its mtime (2026-11-02T21:14:55Z) falls inside job
`work-20261102-211437` (21:14:37–21:14:58Z), not inside this plan job.
