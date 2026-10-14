---
name: sop-publish
description: |-
  Complete an authorized merge and publish of SOP-managed work, including safe
  cleanup. Use for: "/sop-publish", "finish the merge", "publish this", "clean up
  after publishing", "close out the merged feature" once a merge and publish are
  already authorized. It verifies the published target contains the intended
  commits, historicalizes eligible plans through SOP, preserves lifecycle
  archives, and removes only fully-merged worktrees and branches. SOP owns the
  lifecycle; this skill orchestrates ordinary git cleanup and delegates
  historicalization.
disable-model-invocation: true
---

# SOP Merge & Publish Completion

Canonical skill name: `sop-publish`. Installed command: `/sop-publish`.

`/sop-publish` runs the completion half of a merge and publish. After an operator
has **authorized** the merge and publish of a feature, it verifies the
publication, closes eligible plans through SOP, preserves evidence, and performs
only safe cleanup. It is a thin client of the `sop` CLI plus ordinary git: it
never reproduces SOP policy, never edits SOP state, and never rewrites history.

```text
/sop-publish
     |
     v
  VERIFY PUBLICATION -> INSPECT WORKTREE -> HISTORICALIZE (governed) ->
  PRESERVE EVIDENCE -> REMOVE MERGED WORKTREES -> DELETE MERGED BRANCHES -> REPORT
```

Cleanup proceeds **without a further confirmation** for routine, safe steps
*inside the authorized merge/publish scope*. Stop and report only when evidence
shows unresolved work, potential loss, failed publication, or an actual policy
boundary.

Safety invariant: **authorization to merge and publish is the scope, not a
licence to destroy.** It never means "force-remove anything dirty", "delete a
remote branch", "mark a plan complete", or "rewrite history". SOP remains the
sole lifecycle authority.

## PREFLIGHT

Check that `sop` is on PATH; if missing, STOP and report the installation
prerequisite. Do not install, rebuild, or implement the work yourself. Confirm the
repository root and that this is an SOP-managed repository (an `.agent-sdlc/`
directory exists). Collect SOP state read-only:

```bash
sop status
sop continue --check --json
```

## 1. VERIFY PUBLICATION

Confirm the published target contains the intended commits and report local/remote
synchronization. Never assume a merge reached the remote.

```text
git fetch <remote> --prune
git rev-parse <published-target>          # e.g. origin/main
git rev-parse <feature-branch>
git merge-base --is-ancestor <feature-head> <published-target>
git rev-list --left-right --count <published-target>...<feature-branch>
```

If the intended commits are **not** contained in the published target, STOP:
publication failed. Do not clean up, remove worktrees, or delete branches until
publication is confirmed. Report local/remote divergence.

## 2. INSPECT THE FEATURE WORKTREE BEFORE REMOVAL

For every candidate worktree, enumerate tracked, untracked, and gitignored SOP
state *before* removing anything, so nothing unexplained is lost:

```text
git -C <worktree> rev-parse HEAD
git -C <worktree> status --short
git -C <worktree> status -sb
git -C <worktree> status --short --ignored     # includes gitignored SOP state
git -C <worktree> diff --check
```

Classify each item: tracked work, untracked files, and gitignored SOP state
(`.agent-sdlc/runs/`, `.agent-sdlc/archive/`, plan files, reports, traces). A
worktree with unexplained tracked or untracked changes, or with SOP state not
preserved elsewhere, is **not** removable — preserve it and report.

## 3. CLOSE ELIGIBLE PLANS THROUGH SOP

Close plans only through SOP's governed historicalization, and only when SOP
reports the plan is ready. Never infer `COMPLETE` from a merge.

```bash
sop plan historicalize --check
sop plan historicalize
```

- `--check` first. Historicalize only when readiness passes (every task
  satisfied, no pending approval, verification evidence exists). SOP's
  `ALREADY_HISTORICALIZED` is a no-op, not an error.
- **Preserve unfinished plans.** A merge does not complete a plan; do not
  historicalize a plan with unresolved tasks.
- **Never mark a plan `SUPERSEDED` automatically.** Disposal is an explicit
  operator decision, never a cleanup default.
- Never edit the state database, `plan.json`, `plan.meta.json`, or the archive to
  force an outcome.

## 4. PRESERVE EVIDENCE OUTSIDE THE WORKTREE

Before removing any worktree, copy the lifecycle archive and other required
evidence it holds to a durable location outside every worktree being removed, and
verify the copies (checksums or a diff). If preserved copies conflict with the
worktree's contents, STOP and report the conflict; do not overwrite either side.
Lifecycle archives and required evidence are protected: never delete or rewrite
them to obtain a clean tree.

## 5. REMOVE ONLY FULLY-MERGED CLEAN WORKTREES

Remove a worktree only when it is clean and its branch commits are fully contained
in the published target:

```text
git -C <worktree> status --short          # must be empty (tracked and untracked)
git merge-base --is-ancestor <worktree-head> <published-target>
git worktree remove <path>                # never --force
```

Never use force removal. A dirty worktree, an unmerged branch, or an ambiguous
result is retained and reported with its reason.

## 6. DELETE MERGED LOCAL BRANCHES

Delete a local branch only with the safe form, which refuses an unmerged branch:

```text
git branch -d <branch>
```

If deletion is refused because a stale upstream no longer matches, first verify
the branch's commits are contained in the published target, then unset that stale
upstream and retry the safe deletion:

```text
git rev-list --left-right --count <published-target>...<branch>
git branch --unset-upstream <branch>
git branch -d <branch>
```

Never force-delete a branch. If ancestry against the published target cannot be
established, retain the branch and report.

## 7. REMOTE BRANCHES AND UNRELATED WORK

Keep remote branches unless the operator explicitly authorizes their deletion;
never delete a remote branch as a cleanup default. Leave unrelated worktrees and
branches untouched — only the worktrees and branches in the authorized
merge/publish scope are eligible.

## 8. REPORT

Report: the merge/publish result; the plan disposition (historicalized,
preserved, or already closed); the retained-evidence location; the worktrees and
local branches removed; the remote branches kept; and anything retained with its
reason. End with exactly one next action.

## PROTECTED EVIDENCE

Never delete or rewrite, merely to obtain a clean tree:

```text
.agent-sdlc/runs/
.agent-sdlc/archive/
task reports
classification records
traces
lifecycle records
not-required evidence
```

Respect `.gitignore` boundaries: a gitignored path is not junk, and an unclassified
path is never removed.

## What the skill must not do

- Do not infer `COMPLETE` from a merge, and do not edit lifecycle state to force an
  outcome.
- Do not automatically mark a plan `SUPERSEDED`.
- Do not force-remove a worktree, force-delete a branch, or rewrite history.
- Do not delete a remote branch without explicit operator authorization.
- Do not delete lifecycle archives, required evidence, or an unclassified file.
- Do not touch unrelated worktrees or branches.
- Do not reproduce SOP policy, pick a model or provider, or call a provider
  directly; SOP resolves those.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see
  the project README / `./install.sh`). Never perform the cleanup in SOP's place.
