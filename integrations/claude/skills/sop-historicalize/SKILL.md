---
name: sop-historicalize
description: |-
  Historicalize (close) a completed or explicitly disposed Agentic SOP plan.
  Use when an operator asks to finalize, close, retire, or archive a finished
  plan, move it out of the ACTIVE set, or preserve its audit trail. SOP owns the
  deterministic lifecycle transition; this skill only discovers the plan, runs
  readiness, delegates, verifies, and reports.
disable-model-invocation: true
---

# SOP Historicalize

A thin entry point over the deterministic `sop plan historicalize` lifecycle
operation. SOP state is authoritative. SOP owns the eligibility decision, the
archive, the terminal disposition, and the approval boundary; the skill
discovers, checks readiness, delegates, verifies, and reports. It never decides
that a plan is complete, and it never edits plan files, the state database, or
the archive itself.

```text
Operator
   ↓
/sop-historicalize            (this file: discover, check, delegate, verify, report)
   ↓
sop plan historicalize        (SOP's deterministic, model-free lifecycle transition)
   ↓
.agent-sdlc/archive/<plan-id>/   (task outcomes, provenance, verification, disposition)
```

## DISCOVER

Identify the relevant ACTIVE plan from the operator's request. When they name a
plan, use it; otherwise the active plan is the one SOP already recorded. Inspect
`.agent-sdlc/plan.meta.json` for the recorded source and plan id, and
`.agent-sdlc/plan.json` for the machine plan. Do not maintain a separate
planning-source precedence list and do not guess. Report the plan path, id, and
current state before acting. If the target plan is materially ambiguous, report
the ambiguity and stop.

## READINESS

Run SOP's deterministic readiness evaluation. It is read-only: it reports the
plan, lifecycle state, task counts by state, unresolved tasks, unresolved
approvals, unresolved verification, eligibility, and the reason when ineligible.

```bash
sop plan historicalize --check --json
```

If the operator named an explicit plan, pass it:

```bash
sop plan historicalize --check --json "docs/plans/PLAN-Example.md"
```

A non-zero exit means the plan is not eligible right now; that is SOP's
deterministic answer, not a failure to work around. Relay the blockers verbatim
(the unresolved task ids and states, the pending approval gate, or the missing
verification) and STOP. Make no mutation.

## DELEGATE

When readiness is eligible, invoke the single deterministic transition:

```bash
sop plan historicalize
```

With an explicit plan:

```bash
sop plan historicalize "docs/plans/PLAN-Example.md"
```

Disposal of unfinished work is a separate, explicit operator decision, recorded
faithfully rather than fabricated as completion:

```bash
sop plan historicalize --disposition SUPERSEDED
```

Do not split the operation, do not pass `--disposition` on the operator's behalf
without an explicit instruction, and do not retry indefinitely. SOP validates
again at mutation time and refuses a plan that changed since readiness.

## VERIFY

Confirm the resulting lifecycle state rather than trusting an exit code:

```bash
sop status
```

SOP reports `HISTORICALIZED` (already historicalized is a deterministic no-op)
and removes the plan from ACTIVE selection. Report exactly what changed: the
archive directory, the terminal disposition, the preserved task count, and that
the plan is no longer ACTIVE.

## REPORT

Return a concise, evidence-backed summary:

- Plan: source path and plan id.
- Before: lifecycle state and task counts by state.
- Readiness: eligibility and the blockers when ineligible (then stop).
- Change: archive path, disposition (`COMPLETE` or `SUPERSEDED`), preserved task
  count, and that the plan left ACTIVE selection.
- After: the next active/runnable plan when one exists, or that none is active.
- Human approval required: any pending gate SOP reported, or none established.

## What the skill must not do

- Do not edit plan files, `.agent-sdlc/plan.json`, `.agent-sdlc/plan.meta.json`,
  the state database, or `.agent-sdlc/archive/` directly to force the outcome.
  The transition is `sop plan historicalize`.
- Do not decide that a plan is complete, fabricate a PASS or an approval, or
  rewrite an unresolved task outcome.
- Do not approve, decline, accept changed tasks, commit, push, or merge on the
  operator's behalf.
- Do not reimplement SOP's eligibility, disposition, or approval policy.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see
  the project README / `./install.sh`). Never mutate the repository in SOP's
  place.
