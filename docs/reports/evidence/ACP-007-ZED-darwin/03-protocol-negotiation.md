# ACP-007 scenario 3 — Protocol negotiation (macOS)

Status: **PASS** (recorded 2026-11-02T21:05:39Z)

Scenario source: [ACP-007-acceptance.md](../../ACP-007-acceptance.md) scenario 3.

## Environment

| Field | Value |
| --- | --- |
| Date | 2026-11-02T21:05:39Z |
| Host | `Hoangs-MacBook-Pro-2.local` |
| OS | macOS 26.6.2 (25G83), Darwin 25.6.0, arm64 |
| Zed | 1.26.2 (not involved in this scenario) |
| SOP | `sop dev`, `/Users/imhttran/go/bin/sop` |
| SOP revision | `ba5fbd965902584f04be258d23b05fb7fcb5850a` |

## Method

The acceptance recipe allows "an ACP v1 client (or the recorded transcript)". No
standalone ACP client was used; the transcript below is the client side, written
directly to the server's stdio as newline-delimited JSON-RPC frames.

## Commands

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"v1"}}\n{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/Users/imhttran/agentic-workspace/agentic-sop","mcpServers":[]}}\n' | sop acp 2>/tmp/acp-stderr.txt; echo "exit: $?"; cat /tmp/acp-stderr.txt
```

Working directory: `/Users/imhttran/agentic-workspace/agentic-sop`.

## Recorded transcript

stdout:

```json
{"id":1,"jsonrpc":"2.0","result":{"agentCapabilities":{"loadSession":false},"agentInfo":{"name":"sop","version":"dev"},"protocolVersion":"v1"}}
{"id":2,"jsonrpc":"2.0","result":{"sessionId":"sess_7bbe473c392d4aa9"}}
```

stderr: empty.

Process exit: `0`.

## Observations

- `initialize` succeeded and negotiated `protocolVersion` `v1`, matching the
  server's pinned version.
- `agentInfo` reported `{"name":"sop","version":"dev"}`.
- `agentCapabilities.loadSession` is `false`, corresponding to the documented
  "no session resumption" limitation.
- `session/new` succeeded for the canonical project directory and returned a
  `sessionId` (`sess_7bbe473c392d4aa9`).
- stdout carried protocol frames only; diagnostics were absent from stdout.
- `session/new` started no governed work, consistent with the documented
  behavior that governance begins at `session/prompt`.

## Limits of this record

- This covers protocol negotiation only. It does not cover a Zed-launched
  session, the agent picker, a bounded prompt, cancellation, or restart discovery.
- The `sessionId` is per-process and was not reused; nothing was submitted.
