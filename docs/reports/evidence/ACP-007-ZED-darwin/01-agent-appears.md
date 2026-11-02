# ACP-007 scenario 1 — Agent appears in Zed (macOS)

Status: **PASS** — SOP appears in Zed's agent picker

Recorded 2026-11-02. Observed by the operator in Zed 1.26.2 on macOS 26.6.2.

## Environment

| Field        | Value                                      |
| ------------ | ------------------------------------------ |
| Date         | 2026-11-02 (operator observation)          |
| Host         | `Hoangs-MacBook-Pro-2.local`               |
| OS           | macOS 26.6.2 (25G83), Darwin 25.6.0, arm64 |
| Zed          | 1.26.2 (`/Applications/Zed.app`)           |
| SOP          | `sop dev`, `/Users/imhttran/go/bin/sop`    |
| SOP revision | `ba5fbd965902584f04be258d23b05fb7fcb5850a` |

## Configuration as accepted

`~/.config/zed/settings.json`, added verbatim — Zed accepted it with **no change**:

```json
"SOP": {
  "command": "/Users/imhttran/go/bin/sop",
  "args": ["acp"],
},
```

The absolute `command` path was used deliberately: a GUI-launched Zed may not
inherit the shell `PATH`.

## Observation

After restarting Zed, the agent picker in the Agent Panel lists:

```
Zed Agent
Terminal
External Agents
  Claude Agent
  SOP
+ Add More Agents
```

SOP appears under **External Agents** alongside the existing `claude-acp` entry.

## Schema verdict

- The `agent_servers` shape in `docs/guides/ACP-ZED.md` §2 (`command` + `args`)
  is **accepted unchanged by Zed 1.26.2**.
- The supported Zed version for this guide is now known for this host: **1.26.2**.
- No difference from the guide was observed, so no guide correction beyond
  removing the UNVERIFIED label was required.

## Evidence

Operator screenshot of the agent picker showing `External Agents → SOP`
(captured 2026-11-02; attached to the working session).

**Not yet a repository file.** Save it as
`docs/reports/evidence/ACP-007-ZED-darwin/agent-picker.png` so the evidence path
this record references points at a real artifact.

## Remaining gap

This scenario establishes reachability only. A **Zed-driven** bounded request
(scenario 2) has not been run; scenarios 2, 4 and 5 were recorded through an ACP
v1 stdio client instead. See `02-bounded-request.md`.
