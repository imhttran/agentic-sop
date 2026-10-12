Create `docs/BACKLOG.md` in `agentic-sop` as the canonical long-term enhancement backlog.

Use a lightweight Markdown format with ID, priority, status, problem, requirements, and acceptance criteria.

Add the first item:

**SOP-LC-001 — Automatic Task-State Closure with Evidence Reuse**

- Priority: P2
- Status: PROPOSED
- Reuse valid task-specific acceptance and validation evidence.
- Automatically update persisted task states when evidence is sufficient.
- Avoid redundant model calls and validation runs.
- Preserve approval boundaries, audit history, and deterministic outcomes.
- Close fully satisfied plans automatically.
- Leave tasks with insufficient evidence unresolved.

Acceptance: A 12-task plan with sufficient evidence can close in one governed workflow without 12 redundant validation runs.

Keep the backlog separate from active SOP plans and runtime state. Do not implement the enhancement.

Commit the documentation locally. Do not push.
