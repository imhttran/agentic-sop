# ACP-007 — macOS (darwin) acceptance evidence

Evidence index for the bounded real-Zed acceptance defined in
[ACP-007-acceptance.md](../../ACP-007-acceptance.md).

This directory was created on macOS so the acceptance run has a home. Only the
scenarios with a real recorded result are marked PASS; everything else is
**NOT EXECUTED** and must not be claimed.

## Environment

Recorded 2026-11-02T21:05:39Z on the acceptance host.

| Field        | Value                                                                       |
| ------------ | --------------------------------------------------------------------------- |
| Date         | 2026-11-02T21:05:39Z                                                        |
| Host         | `Hoangs-MacBook-Pro-2.local`                                                |
| OS           | macOS 26.6.2 (build 25G83), Darwin 25.6.0, arm64                            |
| Zed          | 1.26.2 (`/Applications/Zed.app`)                                            |
| SOP          | `sop dev`, installed at `/Users/imhttran/go/bin/sop` (installed 2026-11-02) |
| SOP revision | `ba5fbd965902584f04be258d23b05fb7fcb5850a`                                  |

## Configuration applied before the run

- `sop` installed from the revision above via `make install`; `sop --help` lists
  the `acp` subcommand.
- `~/.config/zed/settings.json` gained an `agent_servers` entry:

```json
"SOP": {
  "command": "/Users/imhttran/go/bin/sop",
  "args": ["acp"],
},
```

The `agent_servers` schema is still **UNVERIFIED** against Zed 1.26.2 until
scenario 1 observes SOP in the agent picker.

## Scenario status

| #   | Scenario                  | Status                             | Evidence                     |
| --- | ------------------------- | ---------------------------------- | ---------------------------- |
| 1   | Agent appears             | NOT EXECUTED — picker not observed | `01-agent-appears.md`        |
| 2   | Bounded request completes | **PASS** — ACP v1 client           | `02-bounded-request.md`      |
| 3   | Protocol negotiation      | **PASS** — recorded transcript     | `03-protocol-negotiation.md` |
| 4   | Cancellation              | **PASS** — ACP v1 client           | `04-cancellation.md`         |
| 5   | Restart discovery         | **PASS** — ACP v1 client           | `05-restart-discovery.md`    |

Scenarios 2, 4 and 5 were executed with a small ACP v1 client written for this
run, speaking newline-delimited JSON-RPC to `sop acp` over stdio. They prove
SOP's ACP surface end to end, **not** Zed's use of it. Only scenario 1 exercises
Zed, and it remains unexecuted, so no Zed-side acceptance claim is made here.

## Completing the remaining scenarios

Each file lists the exact fields to fill from the run. Nothing here may be filled
in from expectation, from source reading, or from another platform's run.

## Client attribution

| Scenario | Client               | Zed involved               |
| -------- | -------------------- | -------------------------- |
| 1        | Zed 1.26.2 (planned) | required, not yet observed |
| 2        | ACP v1 stdio client  | no                         |
| 3        | ACP v1 stdio client  | no                         |
| 4        | ACP v1 stdio client  | no                         |
| 5        | ACP v1 stdio client  | no                         |
