# ACP-007 scenario 4 — Cancellation (macOS)

Status: **NOT EXECUTED**

No cancellation was attempted. A cancellation result may not be recorded from
expectation.

## Steps to run

1. In the disposable project, start one bounded `implement`-capability prompt.
2. Before it completes, cancel it with `session/cancel` (Zed's stop control).
3. Record the outcome (`cancelled`), the job id, and the stop reason.
4. Confirm `.agent-sdlc/runs/mcp-work/<job-id>/` shows a distinct cancellation
   record, not a completion and not a failure.
5. Confirm the stop happened within the documented `CancelStopBound`.

## Fields to record

| Field | Value |
| --- | --- |
| Date / time (UTC) | |
| Host / OS version | |
| Zed version | |
| SOP revision | |
| Project directory | |
| Job id | |
| Outcome | |
| Stop reason | |
| Cancellation record path | |
| Cancellation recorded distinctly from failure/completion | |
| Observed stop latency | |
| Evidence (transcript path) | |

## Notes

- Cancellation must touch nothing the adapter did not start: CLI-run or MCP-run
  work, approval gates, verification, and retries stay SOP-owned.
