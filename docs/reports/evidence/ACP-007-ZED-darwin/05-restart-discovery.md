# ACP-007 scenario 5 — Restart discovery (macOS)

Status: **NOT EXECUTED**

No restart was performed against a recorded job.

## Steps to run

1. After scenario 2 (or 4) has recorded a job, restart `sop acp` (or restart Zed).
2. Confirm the previously recorded job is discoverable from the persisted records
   under `.agent-sdlc/runs/mcp-work/<job-id>/`.
3. Confirm nothing was resubmitted automatically.
4. Confirm `session/load` remains unsupported (session history is not restored),
   consistent with the advertised `loadSession: false`.

## Fields to record

| Field | Value |
| --- | --- |
| Date / time (UTC) | |
| Host / OS version | |
| Zed version | |
| SOP revision | |
| Project directory | |
| Job id carried across restart | |
| Job discoverable after restart | |
| Automatic resubmission observed | |
| Evidence (transcript path) | |

## Notes

- Scenario 3 already recorded `agentCapabilities.loadSession: false` on this host.
  That is capability advertisement only; it does not substitute for observing
  restart behavior here.
