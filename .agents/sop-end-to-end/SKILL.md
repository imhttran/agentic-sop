---
name: sop-end-to-end
description: |-
  /sop end-to-end
  /sop end-end

  "Review this plan and take it from end to end."
  "Take this plan end to end."
  "Run this through SOP."
  "Dogfood this with SOP."
  "Take it from here."
disable-model-invocation: false
---

# SOP End-to-End

> Template shipped by `agentic-sop`. `scripts/install.sh` links this directory into
> `~/.agents/skills/sop-end-to-end`, so the same instructions are available in
> every project.

Use this skill when the user asks to execute a software project or implementation
plan end-to-end using the Agentic SOP workflow.

Typical trigger phrases:

* "Review this plan and take it from end to end."
* "Take this plan end to end."
* "Run this through SOP."
* "Dogfood this with SOP."
* "Implement this plan."
* "Run the project plan."
* "Finish this using our SOP."
* "Take it from here."
* "Execute the plan."

Do not require the user to use an exact trigger phrase when their intent is clear.

## Objective

Provide a low-touch interface over Agentic SOP.

The user should normally be able to point at a repository containing:

```text
docs/PLAN.md
```

and say:

> Review this plan and take it from end to end.

The skill reviews the plan for obvious structural or safety problems, then uses:

```bash
sop run
```

as the primary execution mechanism.

Do not reproduce the SOP state machine inside the skill. Agentic SOP remains the
authoritative workflow engine.

---

## Step 1 — Identify Project

Determine the project/repository the user is referring to.

Confirm the repository contains an Agentic SOP configuration when applicable:

```text
.agent-sdlc/config.yaml
```

Do not ask the user for information that can be determined safely from the
repository.

---

## Step 2 — Discover Planning Source

Use this precedence:

```text
.agent-sdlc/plan.json   (reused only when its source is unchanged)
docs/PLAN.md
PLAN.md
docs/PRD.md
PRD.md
```

Prefer PLAN over PRD. If a PLAN exists, treat it as the primary execution intent;
the PRD is supporting context only. Do not regenerate a PLAN from a PRD when a
usable PLAN already exists. A project may hold several plans under `docs/`
(`PLAN.md`, `PLAN-Hardening.md`, …); a specific one is selected with
`sop run docs/PLAN-Hardening.md`.

---

## Step 3 — Review the PLAN

Before execution, perform a lightweight engineering review. Check for:

* clear objective
* reasonable phases/tasks
* acceptance criteria
* dependencies
* ordering
* build/test expectations
* security implications
* destructive operations
* migration risks
* missing prerequisites
* contradictions
* obviously underspecified implementation steps

Do not rewrite a good PLAN merely to make it different. Minor implementation
details should not block execution. Only stop when the problem materially affects
correctness, safety, or the ability to execute the project.

---

## Step 4 — Decide Whether Human Input Is Required

Use `READY` when the PLAN is sufficiently actionable.

Use `NEEDS_HUMAN` only when there is a consequential ambiguity or safety issue
that cannot reasonably be resolved from repository context. Examples:

```text
production database destruction
unknown credentials
irreversible migration
unclear target repository
contradictory requirements
missing architectural decision that substantially changes implementation
```

Avoid unnecessary clarification questions; prefer sensible repository-derived
defaults for ordinary engineering decisions.

---

## Step 5 — Execute

When READY, execute:

```bash
sop run
```

Or a specific plan end to end:

```bash
sop run docs/PLAN-Hardening.md
```

Do not normally require:

```bash
sop init
sop plan
sop tasks
```

`sop run` is responsible for bootstrap, plan compilation, task generation,
resume, execution, validation, review, bounded fixes, and persisted state.

---

## Step 6 — Let SOP Control Execution

Once execution begins, Agentic SOP owns the deterministic lifecycle:

```text
PLAN
  ↓
TASK
  ↓
IMPLEMENT
  ↓
BUILD
  ↓
TEST
  ↓
LINT
  ↓
REVIEW
  ↓
QUALITY GATE
  ↓
FIX
  ↓
REVALIDATE
```

The skill supervises and interprets this process rather than replacing it. Do not
bypass failed SOP quality gates, and do not mark failed work complete.

---

## Step 7 — Continue Automatically

When a task succeeds, allow SOP to proceed to the next READY task. Do not ask
"Should I continue?" after every task: the request to take the project "end to
end" is permission to continue through ordinary reversible development work.

Continue until one of these terminal conditions occurs:

```text
DONE
BLOCKED
NEEDS_HUMAN
APPROVAL_REQUIRED
```

---

## Step 8 — Respect Approval Boundaries

Respect `.agent-sdlc/config.yaml`. For example:

```yaml
human:
  approval_before_commit: true
```

means the workflow may autonomously edit, build, test, lint, review, fix, and
retest, but must stop before commit when configured to do so. Never interpret
"end to end" as permission to bypass explicit approval gates.

---

## Step 9 — Recovery

If execution fails because of an implementation, build, test, lint, or review
problem, let SOP's configured bounded fix loop attempt recovery. Do not
immediately ask the user to fix ordinary engineering failures.

A `needs_human` outcome is not terminal: SOP requeues the task to `PLANNED`, so
once the boundary is resolved the same `sop run …` retries it. Escalate only when
SOP reaches `BLOCKED` or `NEEDS_HUMAN`, and provide:

```text
what failed
what SOP tried
why automatic recovery stopped
what decision or action is required
```

---

## Step 10 — Completion

When execution completes, provide a concise summary containing:

```text
tasks completed
important changes
validation results
review result
remaining issues
current Git state
approval/action required, if any
```

Do not dump the complete execution transcript unless requested. Detailed run
artifacts remain under `.agent-sdlc/runs/`.

---

## Guiding Principle

The intended user experience is:

> "Review this plan and take it from end to end."

The system should then:

```text
understand project
→ review PLAN
→ run SOP
→ implement
→ validate
→ review
→ repair
→ continue
→ stop only at completion or a meaningful human boundary
```

Keep the natural-language skill thin. Keep deterministic execution, state
management, validation, quality gates, recovery, and resume behavior inside
Agentic SOP.
