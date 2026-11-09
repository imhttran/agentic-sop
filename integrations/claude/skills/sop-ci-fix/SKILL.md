---
name: sop-ci-fix
description: |-
  Repair the two deterministic failures a push causes that a branch-only check
  cannot see: formatting drift, and a documentation link that breaks only in the
  merge with the base branch. Stops and reports anything ambiguous instead of
  guessing, never publishes, and hands merge conflicts to /sop-fix-conflicts.
  Use for: "/sop-ci-fix", "fix the CI failure", "CI is red", "the formatter check
  failed", "the documentation link check failed", "my branch passes locally but
  fails in CI".
disable-model-invocation: true
---

# /sop-ci-fix — repair a deterministic push failure

`/sop-ci-fix` handles exactly two mechanical classes and refuses everything else:

| Class        | Symptom                                             | Fix                                                  |
| ------------ | --------------------------------------------------- | ---------------------------------------------------- |
| `FORMATTING` | the formatter gate lists files                      | run the project's formatter over exactly those files |
| `MOVED_LINK` | a documentation link fails only in the merge result | correct the link target only                         |

Two things are deliberately not this skill's work:

- **Merge conflicts** belong to `/sop-fix-conflicts`, which owns the decidable-merge
  contract. Do not resolve a conflict here; hand off and stop.
- **Everything else** — a failing test, a build error, a lint rule, a provider or
  infrastructure failure — belongs to `/sop-diagnose` for diagnosis and
  `/sop-implement` for a code change.

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
formatter gate appears there. This is also how you prove the failure is **not** one
of the two classes above: if a test or build command fails, stop and report
`NOT_THIS_CLASS`.

When the repository ships `scripts/checks/pre-push.sh`, also run it. It evaluates
the documentation link check against the merge with the base branch, which is what
CI evaluates and what a branch-only check cannot see.

```bash
scripts/checks/pre-push.sh --base origin/main
```

If that gate reports `HEAD does not merge cleanly`, the branch conflicts with its
base: STOP this skill and use `/sop-fix-conflicts`. If both gates pass, there is
nothing in scope — report `NOT_THIS_CLASS`.

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

Both classes are mechanical by construction. If a fix requires a judgement about
which of two documents is correct, it is not a `MOVED_LINK`; stop and report
`NEEDS_HUMAN`.

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
4. **Next action** — exactly one of `FIXED`, `NEEDS_HUMAN`, `NOT_THIS_CLASS`,
   `USE_SOP_FIX_CONFLICTS`.

Publication is a separate, operator-authorized step. A repair is not permission to
stage, commit, push, or merge to the remote.

## Authorization boundaries

- **Never weaken the gate.** Do not edit CI configuration, disable a check, relax a
  linter, or delete a test to obtain a green result. A green build reached by
  weakening a check is a regression, not a fix.
- **Never bypass the local gate.** A failing local gate is evidence, not an
  obstacle. If a repair cannot pass it, report `NEEDS_HUMAN`.
- **Never resolve a merge conflict here.** That contract belongs to
  `/sop-fix-conflicts`; duplicating it in a second skill would give the operator two
  different rules for the same decision.
- **Never touch lifecycle evidence.** Do not edit `.agent-sdlc`, the state database,
  run history, plan files, or approval records.
- **Never publish.** Staging, committing, pushing, and merging are the operator's
  decisions; this skill stops with a verified working tree.
- **Preserve user-owned work.** Uncommitted changes that are not part of the repair
  are never reverted, stashed, or folded into the fix.
- **One class per run.** A run that starts in `FORMATTING` does not continue into the
  link class; report and let the operator invoke it again.

## What the skill must not do

- Do not diagnose an ordinary code failure, design a code change, or reimplement the
  project's validation.
- Do not choose a model or a provider; SOP owns routing.
- Do not claim a repair succeeded without re-running the gate that failed.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI.

## Examples

Formatting drift:

```text
Findings: FORMATTING — the formatter gate listed internal/cli/run.go
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

A conflict, which is not this skill's work:

```text
Findings: CONFLICT — the branch does not merge cleanly with origin/main
Actions taken: none
Verification: not run; no repair attempted
Next action: USE_SOP_FIX_CONFLICTS
```
