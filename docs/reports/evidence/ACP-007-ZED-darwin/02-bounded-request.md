# ACP-007 scenario 2 — One bounded request (macOS)

Status: **NOT EXECUTED**

No job was submitted. Nothing may be recorded here until a real prompt is run
from Zed.

## Steps to run

1. Use a disposable project with SOP state initialized (`.agent-sdlc/` present).
   This repository qualifies, but prefer a disposable project for the prompt.
2. From Zed's Agent Panel, select SOP and submit one bounded, read-only prompt
   (for example a `plan` capability prompt).
3. Record the job id and the final outcome.
4. Confirm the returned outcome agrees with the persisted record in
   `.agent-sdlc/runs/mcp-work/<job-id>/` (`job.json`, `output.log`, `exit.json`).
5. Note any outcome other than `started`, using guide §4's meanings
   (`already_running`, `busy`, `task_in_progress`, `reconciled`,
   `not_initialized`).

## Fields to record

| Field | Value |
| --- | --- |
| Date / time (UTC) | |
| Host / OS version | |
| Zed version | |
| SOP revision | |
| Project directory | |
| Prompt text (bounded) | |
| Job id | |
| Outcome | |
| Persisted record agrees | |
| `job.json` / `output.log` / `exit.json` paths | |
| Evidence (transcript path) | |
