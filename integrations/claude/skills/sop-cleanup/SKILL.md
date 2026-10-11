---
name: sop-cleanup
description: |-
  Safely inspect, classify, and clean an SOP-managed repository after task, phase,
  or plan execution without losing user work, historical evidence, lifecycle
  evidence, or unrelated changes. Read-only by default: it inspects, classifies,
  and proposes; a human approves every mutation.
  Use for: "/sop-cleanup", "clean the worktree", "post-task cleanup",
  "post-plan cleanup", "worktree cleanup after a run".
disable-model-invocation: true
---

# SOP Worktree Cleanup

Canonical skill name: `sop-worktree-cleanup`. Installed command: `/sop-cleanup`.

`/sop-cleanup` inspects a dirty SOP-managed repository, classifies every item
with evidence, and proposes a minimum, path-scoped cleanup. It is conservative by
default: the first invocation is **read-only**. Nothing is deleted, moved,
unstaged, committed, pushed, or lifecycle-mutated without explicit human approval.

```text
/sop-cleanup
     |
     v
  INSPECT -> CLASSIFY -> PROPOSE -> (human approval) -> MUTATE -> VERIFY
     |                                                      |
     v                                                      v
  WORKTREE CLEANUP ASSESSMENT                         CLEANUP COMPLETE
```

Safety invariant: **cleanup is never permission.** "Clean up" must not be read as
authority to delete, reset, restore, stash, stage, commit, push, or change SOP
lifecycle state automatically.

SOP state is authoritative. SOP owns planning-source resolution, the lifecycle,
approvals, the run loop, and policy. This skill **orchestrates** them; it never
reimplements them, never edits the state database, and never edits the repository
outside an approved, path-scoped action.

## PREFLIGHT

Check that `sop` is on PATH; if missing, STOP and report the installation
prerequisite. Do not install, rebuild, or implement the work yourself. Confirm the
repository root and that this is an SOP-managed repository (an `.agent-sdlc/`
directory exists).

Collect SOP state read-only. Absence of an active plan is **information, not an
error**:

```bash
sop status
sop continue --check --json
```

## INSPECT

Collect the working-tree facts without mutating anything:

```text
git status --short
git status -sb
git rev-parse HEAD
git branch --show-current
git diff
git diff --cached
git diff --check
git log --oneline <upstream>..<branch>   (when a tracking branch exists)
git log --oneline <branch>..<upstream>   (to detect divergence)
```

Also inspect, read-only and where available: the active plan and its state; run
artifacts under `.agent-sdlc/runs/`; the machine archive under
`.agent-sdlc/archive/`; tracked plan documents under `docs/plans/`; repository
document history under `docs/history/`; and the `.gitignore` boundaries. Never run
a lifecycle-changing SOP command during INSPECT.

## CLASSIFY

Assign every dirty/path-relevant item **exactly one** primary classification, with
evidence and confidence. UNKNOWN items are never deleted automatically.

| Class | Meaning | Default action |
| ----- | ------- | -------------- |
| `TASK_OUTPUT` | A deliverable or report produced by an SOP task | preserve / commit with the task |
| `ACTIVE_WORK` | Changes belonging to in-progress work | preserve |
| `USER_OWNED` | Work owned by the operator, not SOP | preserve |
| `HISTORICAL_EVIDENCE` | Prior reports/history kept for the record | preserve |
| `SOP_LIFECYCLE_EVIDENCE` | Run/archive/lifecycle records | preserve (protected) |
| `GENERATED` | Reproducible build/runtime output | preserve / ignore |
| `STALE_ARTIFACT` | Proven no-longer-required artifact | delete only after approval |
| `COMPLETED_PLAN` | A finished plan document still under `docs/plans/` | move to `docs/history/` after approval |
| `ACTIVE_PLAN` | The currently active plan document | preserve |
| `NEW_PLAN` | A planned but not-yet-active plan document | preserve |
| `UNRELATED_CHANGE` | An unrelated change outside SOP scope | preserve |
| `UNKNOWN` | Ownership cannot be established | human decision |

For each item report: `path`, git state, classification, evidence, proposed action,
and confidence. Classification is evidence-based: use SOP task deliverables, task
reports, run artifacts, plan definitions, task mutation records, imports and
references, and known generated-file conventions.

```text
path: docs/reports/example/SP-003-report.md
class: TASK_OUTPUT
evidence: SP-003 deliverable; run artifact present
proposed: preserve / commit with the task
confidence: high
```

## PROPOSE

Produce exactly one disposition:

| Disposition | When |
| ----------- | ---- |
| `CLEAN` | Nothing is dirty; the worktree is coherent |
| `CLEANUP_COMPLETE_WITH_PRESERVED_WORK` | Items are dirty but every one is explained and proposes only `preserve` |
| `CLEANUP_PROPOSAL_READY` | One or more items propose an action: commit with the task, a delete, or a move |
| `BLOCKED_UNKNOWN_OWNERSHIP` | An item cannot be classified without guessing |
| `BLOCKED_LIFECYCLE_CONFLICT` | SOP state is internally inconsistent, or cleanup would touch protected evidence |

Emit the assessment in this shape:

```text
WORKTREE CLEANUP ASSESSMENT

Repository: <root>
HEAD:       <sha>
Branch:     <name>
SOP plan:   <id | none>

ITEMS
  path: ...   class: ...   git: ...   evidence: ...   proposed: ...   confidence: ...

DISPOSITION: <one of the five>
APPROVAL REQUIRED FOR: <the exact destructive actions, or none>
```

## APPLY (explicit approval only)

Stage 2 runs only as `/sop-cleanup apply` (or the closest supported invocation) and
only after the operator approves the **exact** proposed actions. It performs only
those actions; a newly discovered mutation is never implicitly authorized.

Before any mutation, re-check the assessment baseline (HEAD, working-tree status,
relevant SOP state). If anything changed materially since the assessment: STOP and
report `CLEANUP_REASSESSMENT_REQUIRED`. Then perform only exact, path-scoped
operations and re-run VERIFY.

## PROTECTED EVIDENCE

Treat the following as protected; never delete or rewrite them merely to obtain a
clean tree:

```text
.agent-sdlc/runs/
.agent-sdlc/archive/
task reports
classification records
traces
lifecycle records
not-required evidence
```

Respect `.gitignore` boundaries. A path matched by `.gitignore` is not "cleanup
junk"; it is out of scope.

## PLAN CLEANUP SEMANTICS

Two different things must not be conflated:

```text
A. SOP MACHINE LIFECYCLE ARCHIVE     .agent-sdlc/archive/<plan-id>/
B. REPOSITORY DOCUMENT HISTORY       docs/history/
```

A completed or released SOP plan may already be machine-historicalized while its
**tracked plan document** still sits under `docs/plans/`. When the repository
convention uses `docs/history/`, classify such a document `COMPLETED_PLAN` and
propose a documentation move only after verifying all of: the lifecycle disposition
is complete (or superseded) as appropriate; no active association remains; a
repository history convention exists; references are inspected; and historical
references are **not** blindly rewritten. Never assume that a plan-lifecycle
command moves tracked documentation.

## DANGEROUS OPERATIONS

The skill must never perform these merely because cleanup was requested, and must
never run a broad, repository-wide destructive operation:

```text
git clean -fd
git reset --hard
git checkout -- .
git restore .
git restore --staged .
git stash
a recursive forced whole-path deletion of an unclassified path
```

Prefer exact, path-scoped operations after approval, and never operate on a path
classified UNKNOWN.

## LIFECYCLE SAFETY

Cleanup must not silently invoke a lifecycle transition:

```text
sop task skip
sop task complete
sop retry
sop reconcile
sop plan complete
sop plan supersede
sop plan historicalize
sop plan activate
sop run
```

A lifecycle action may be **proposed** when evidence supports it; the actual
mutation requires explicit human approval. Never reconcile, retry, complete, skip,
supersede, or historicalize automatically.

## COMMIT / PUSH BOUNDARIES

The skill may recommend coherent commit boundaries (for example: implementation,
tests, reports, historicalization, unrelated user work) and prefers the smallest
coherent boundary that leaves the repository building and testing. It must not
create commits or push without explicit human approval, and approval to commit does
**not** imply approval to push. Never split a coherent change into a task-pure
commit that leaves the repository broken.

## CLEAN DEFINITION

"CLEAN" is not merely an empty working tree. A repository is clean only when: no
unexplained dirty file remains; user-owned work is preserved; every task output has
a disposition; lifecycle evidence is preserved; completed-plan documentation has a
deliberate disposition; no known stale artifact is unresolved; and the state is
internally coherent. A dirty repository that deliberately preserves work is
reported as `CLEANUP_COMPLETE_WITH_PRESERVED_WORK`. Do not destroy legitimate work
merely to obtain an empty status.

## VERIFY

After an approved mutation, re-run the INSPECT commands and report any residual
dirty state. When source or test files changed, run the repository's established
validation gates. Do not run expensive full gates for a pure documentation move
unless repository policy requires them.

## REPORT

Report every invocation with: assessment (repository, HEAD, branch, plan, items,
disposition); approvals required; actions taken (exactly those approved); residual
state; and exactly one next action.

## Examples

Post-task cleanup:

```text
task SP-003 LOCAL_DONE; its report is present
docs/reports/... -> TASK_OUTPUT (preserve / commit with the task)
disposition: CLEANUP_PROPOSAL_READY
```

Orphan artifact removal (requires approval):

```text
internal/old-wire/ -> STALE_ARTIFACT (zero importers; not referenced by any plan)
disposition: CLEANUP_PROPOSAL_READY
action on approval: delete that exact path only
```

Completed-plan documentation archival:

```text
docs/plans/PLAN-X.md -> COMPLETED_PLAN (machine lifecycle COMPLETE; no active association)
proposal: move to docs/history/PLAN-X.md after inspecting references
never assume the lifecycle command moves tracked documentation
```

Preserving unrelated user work:

```text
internal/providers/other/config_test.go -> UNRELATED_CHANGE (not an SOP deliverable)
disposition: preserve; never stage, revert, or delete it
```

## What the skill must not do

- Do not delete, move, reset, restore, stash, or overwrite anything without
  explicit human approval of the exact action.
- Do not classify an untracked file as junk, or assume the newest SOP task owns a
  modified file.
- Do not rewrite or delete SOP lifecycle/run/archive evidence.
- Do not edit the state database, `plan.json`, `plan.meta.json`, plan files, or the
  archive to force an outcome.
- Do not run a lifecycle transition (skip, complete, retry, reconcile, plan
  complete/supersede/historicalize/activate, or run) automatically.
- Do not create commits or push without explicit approval.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see
  the project README / `./install.sh`). Never mutate the repository in SOP's place.
