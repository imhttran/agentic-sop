# ACP-007 scenario 1 — Agent appears in Zed (macOS)

Status: **NOT EXECUTED** — the agent picker has not been observed.

No result may be recorded here without an actual Zed session.

## Preconditions already met

- `sop` installed from `ba5fbd965902584f04be258d23b05fb7fcb5850a`; `sop --help`
  lists `acp`.
- `~/.config/zed/settings.json` contains:

```json
"SOP": {
  "command": "/Users/imhttran/go/bin/sop",
  "args": ["acp"],
},
```

- Zed 1.26.2 installed at `/Applications/Zed.app`.

## Steps to run

1. Restart Zed (or reload settings) so `agent_servers` is read.
2. Open the Agent Panel and the agent picker.
3. Confirm SOP is listed.
4. Record the accepted `agent_servers` schema as the installed Zed interprets it
   (screenshot, or the settings file path plus the entry as saved).
5. If the accepted schema differs from guide §2, fix the guide and the schema
   section of the discovery note as part of this run.

## Fields to record

| Field | Value |
| --- | --- |
| Date / time (UTC) | |
| Host | |
| OS version (`sw_vers`) | |
| Zed version (`zed --version`) | |
| SOP version / revision | |
| SOP visible in picker | |
| `agent_servers` schema accepted | |
| Differences from guide §2 | |
| Evidence (screenshot path) | |

## Notes

- This scenario is also the verification the guide currently lacks: the
  `agent_servers` example is labelled UNVERIFIED until this is recorded.
