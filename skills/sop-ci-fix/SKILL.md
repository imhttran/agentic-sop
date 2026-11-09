---
name: sop-ci-fix
description: |-
  Repair the deterministic failures a push causes — formatting drift, and
  documentation links that break only in the merge with the base branch — and
  resolve merge conflicts confined to files the branch itself changed. Stops and
  reports anything ambiguous instead of guessing, and never publishes.
  Use for: "/sop-ci-fix", "fix the CI failure", "CI is red", "the formatter check
  failed", "fix the merge conflict", "my branch no longer merges".
disable-model-invocation: true
---

# /sop-ci-fix — repair a deterministic CI failure

`/sop-ci-fix` handles exactly three mechanical classes and refuses everything
else:

| Class | Symptom | Fix |
| --- | --- | --- |
| `FORMATTING` | the formatter check lists files | run the project's formatter over exactly those files |
| `MOVED_LINK` | a documentation link fails only in the merge result | correct the link target only |
| `CONFLICT` | the branch no longer merges with its base | resolve only hunks the branch alone changed |

Anything outside those three classes is not this skill's work. Report it and stop;
`/sop-diagnose` owns diagnosis and `/sop-implement` owns code changes.

## Preconditions

- Detect the `sop` binary before doing anything: `command -v sop`, or `where sop` on
  Windows. An empty result or non-zero exit status means "not on PATH".

If `sop` is not on PATH, STOP. Do not install or rebuild it, do not call a provider,
and do not attempt the repair yourself. Report the prerequisite and stop.

## Step 1 — confirm nothing else is running

```bash
sop status
```

If SOP reports a task in flight or an active run, STOP: a repair that edits the tree
underneath a running lifecycle destroys evidence. Report the run and let it finish.

## Step 2 — reproduce locally

```bash
sop validate
```

`sop validate` runs the project's configured build/test/lint commands, so a
formatter gate appears there. When the repository also ships
`scripts/checks/pre-push.sh`, run it: it evaluates the documentation link check
against the merge with the base branch, which is what CI evaluates and what a
branch-only check cannot see.

```bash
scripts/checks/pre-push.sh --base origin/main
```

Treat a passing reproduction as "not one of the three classes" and stop.

## Step 3 — classify and fix

### `FORMATTING`

The gate names files. Format only those files, with the project's own formatter —
`gofmt -w` for Go, the project's configured formatter otherwise. Do not reformat
files the gate did not name: unrelated reformatting buries the real diff and
violates the smallest-change boundary.

### `MOVED_LINK`

The link check prints `broken: <file> -> <target>`. Determine which is true:

- the target moved — locate it (`git log --follow -- <old path>`, or search for the
  file name) and change **only the link target**, recomputing the relative path.
  Never change the surrounding prose.
- the target was deleted with no replacement — STOP. Do not repoint the link at a
  different document and do not delete the surrounding sentence. Report the missing
  target for the operator to decide.

### `CONFLICT`

Merge the base into the branch:

```bash
git merge origin/main
```

Then resolve only a hunk that satisfies one of these:

- the branch is the only side that changed those lines;
- the base applied a move or rename the branch did not touch;
- both sides changed different, non-overlapping lines of the same hunk.

Stop and abort the merge (`git merge --abort`) when:

- both sides changed the same logic, and one resolution would silently discard a
  change;
- the conflicting file is not one the branch changed;
- the conflict is in generated output, a lockfile, or a binary.

Never rebase published history, and never reset, restore, stash, or clean the
worktree to "get past" a conflict. Those operations discard work this skill cannot
recover.

## Step 4 — verify

Re-run both gates from Step 2. When Go files changed, also run:

```bash
go build ./...
```

and the tests for the changed packages. One repair attempt per class. If the same
class still fails after one attempt, STOP — a second guess is not a fix.

## Step 5 — report

Four sections, in this order:

1. **Findings** — the class, the exact gate output that identified it, and the files
   involved.
2. **Actions taken** — every file changed and why; state "none" when the run only
   inspected.
3. **Verification** — the commands re-run and their results. Never claim a pass that
   was not observed.
4. **Next action** — exactly one of `FIXED`, `NEEDS_HUMAN`, `NOT_THIS_CLASS`.

Publication is a separate, operator-authorized step. A repair is not permission to
stage, commit, push, or merge to the remote.

## Authorization boundaries

- **Never weaken the gate.** Do not edit CI configuration, disable a check, relax a
  linter, or delete a test to obtain a green result. A green build reached by
  weakening a check is a regression, not a fix.
- **Never bypass the hook.** A failing local gate is evidence, not an obstacle. If a
  repair cannot pass it, report `NEEDS_HUMAN`.
- **Never touch lifecycle evidence.** Do not edit `.agent-sdlc`, the state database,
  run history, plan files, or approval records.
- **Never publish.** Staging, committing, pushing, and merging are the operator's
  decisions; this skill stops with a verified working tree.
- **Preserve user-owned work.** Uncommitted changes that are not part of the repair
  are never reverted, stashed, or folded into the fix.
- **One class per run.** A run that starts in `FORMATTING` does not continue into
  conflict resolution; report and let the operator invoke it again.

## What the skill must not do

- Do not diagnose an ordinary code failure, design a code change, or reimplement the
  project's validation.
- Do not choose a model or a provider; SOP owns routing.
- Do not claim a repair succeeded without re-running the gate that failed.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI.

## Examples

Formatting drift:

```text
Findings: FORMATTING — `gofmt -l .` listed internal/cli/run.go
Actions taken: formatted internal/cli/run.go
Verification: `sop validate` clean; `scripts/checks/pre-push.sh` broken links: 0
Next action: FIXED
```

A link that only breaks in the merge:

```text
Findings: MOVED_LINK — docs/reports/x.md -> ../../plans/PLAN-OLD.md; the base moved
  the file to docs/history/plans/PLAN-OLD.md
Actions taken: changed only that link target
Verification: broken links: 0 on the merge with origin/main
Next action: FIXED
```

A conflict that is not mechanical:

```text
Findings: CONFLICT — internal/retrieval/rank.go, both sides changed the same scorer
Actions taken: none; merged state aborted, tree unchanged
Verification: not run; no repair attempted
Next action: NEEDS_HUMAN
```
