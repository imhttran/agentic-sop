---
name: sop-fix-conflicts
description: |-
  Resolve an in-progress merge conflict when the resolution follows from the two
  sides, and stop when it does not. Use for: "/sop-fix-conflicts", "fix this merge
  conflict", "the PR has conflicts", "merge main into my branch and resolve it",
  "why is the merge blocked?". It reads both sides of every conflict, resolves only
  hunks whose intent can be preserved mechanically, verifies the merged result, and
  completes the merge locally. It never publishes, rewrites history, or decides a
  product, security, or policy question.
disable-model-invocation: true
---

# SOP Merge-Conflict Resolution

Canonical skill name: `sop-fix-conflicts`. Installed command: `/sop-fix-conflicts`.

`/sop-fix-conflicts` resolves an **in-progress merge** when, and only when, the
resolution follows from the two sides themselves. It is a thin client of the `sop`
CLI for context plus ordinary git for the merge: it never reproduces SOP policy and
never publishes.

```text
/sop-fix-conflicts
     |
     v
  PREFLIGHT -> CLASSIFY EACH CONFLICT -> decidable?
                                            | yes
                                            v
                          RESOLVE -> VERIFY -> COMPLETE MERGE -> REPORT
                                            | no
                                            v
                            STOP AND REPORT (markers left in place)
```

Safety invariant: **a conflict is never permission to pick a side.** Every
resolved hunk must keep both sides' intent. Dropping one side, guessing at a
product decision, or weakening a security control to get a green build is out of
scope, not a shortcut.

## PREFLIGHT

Check that `sop` is on PATH; if it is missing, STOP and report the installation
prerequisite. Do not install, rebuild, or implement anything.

Read SOP's state without changing it:

```bash
sop status
```

Confirm what is being merged, and that it is a merge at all:

```text
git rev-parse --show-toplevel
git rev-parse -q --verify MERGE_HEAD
git rev-parse HEAD MERGE_HEAD
git merge-base HEAD MERGE_HEAD
git status --short
git diff --name-only --diff-filter=U
```

- **No `MERGE_HEAD`**: there is no merge to resolve. STOP and report the actual
  state (a clean tree, ordinary edits, or another operation in progress).
- **`rebase-merge`, `rebase-apply`, `CHERRY_PICK_HEAD`, or `REVERT_HEAD` exists**:
  STOP. Those replay or rewrite history; resolving them is a policy decision for
  the operator, not a mechanical one.
- **The merge is already committed**: report it and stop; there is nothing to
  resolve.
- **The tree holds changes that neither side produced**: STOP and report them. A
  merge commit must not carry work that was not part of the merge.

Record the merge sides and the conflicted paths before touching anything; they are
the only paths this skill may modify.

## CLASSIFY EACH CONFLICT

For every conflicted path, read the base and both sides, and the commits that
changed it:

```text
git show :1:<path>        # merge base
git show :2:<path>        # HEAD (ours)
git show :3:<path>        # MERGE_HEAD (theirs)
git log --oneline HEAD ^MERGE_HEAD -- <path>
git log --oneline MERGE_HEAD ^HEAD -- <path>
```

A conflict is **decidable** only when the merged result can keep both sides'
intent and you can say so in one sentence naming what each side did. Typical
decidable shapes:

- **Both sides add distinct content at the same place** (documentation tables,
  changelogs, import or command lists). Keep both, ordered by the file's existing
  convention or by the canonical owner of the repeated data.
- **Only one side changed the conflicted hunk**, and the other side changed
  whitespace or formatting nearby. Keep the content change and the formatting
  intent.
- **The same logical change arrived on both sides** (a cherry-pick or a duplicated
  fix). Keep one copy.
- **The artifact is generated** (a mirror, golden file, checksum, or lockfile) and
  its canonical owner is in this tree. Regenerate it with the owning script or
  command instead of hand-merging bytes.

A conflict is **not decidable**, and the skill stops on that path, when any of
these hold:

- both sides change the same logical line, behavior, or default differently;
- the path is security, authorization, approval, redaction, policy, or lifecycle
  logic or state;
- test assertions or fixtures disagree, or a resolution would weaken or remove a
  test;
- public API, schema, migration, or configuration semantics differ;
- a generated artifact's owner is missing, or regenerating it is unsafe or
  nondeterministic;
- the conflict is binary, a file-mode change, or a symlink retarget;
- you cannot state the resolution in one sentence naming both sides' intent.

Never resolve a path by taking one side wholesale unless the other side is
unchanged in every conflicted hunk. After two failed verification attempts on a
path, stop on that path instead of guessing again.

## RESOLVE ONLY WHAT IS DECIDABLE

- Edit only the conflicted paths. Leave every other file untouched.
- Remove the conflict markers and keep both sides' intent; do not reformat code
  neither side changed.
- When repeated data has a canonical owner in the tree, change the owner first and
  regenerate its copies rather than editing them.
- Stage only the paths you resolved:

```text
git add -- <path>
```

- Leave an undecidable path conflicted and unstaged. Do not half-resolve it.

## VERIFY THE MERGE RESULT

Run the checks a merge into the target branch must pass:

```text
git diff --check
git grep -n -E '^(<<<<<<<|=======|>>>>>>>)' -- <resolved paths>
gofmt -l .
```

Then the project's fast gate and the narrowest meaningful tests. In this
repository that means `scripts/checks/pre-push.sh`, `go build ./...`,
`go vet ./...`, and the packages the merge touches. Prefer focused tests; never
run a live, network, or destructive suite to prove a merge.

If a check fails because of a resolution choice, revert only that path to the
marked-up conflict and re-classify it:

```text
git checkout -m -- <path>
```

Never weaken, delete, or skip a test, and never bypass a gate to obtain a green
result.

## COMPLETE THE MERGE

Complete the merge locally only when every conflicted path is resolved, staged,
free of markers, and the checks pass. Preserve the merge message:

```text
git commit --no-edit
```

If any path is still undecidable, do not complete the merge: leave it in progress
with the remaining markers so the operator sees exactly what is unresolved.

Do not amend, rebase, force, delete branches, or publish. Publication belongs to
the operator or to `/sop-publish`, where an authorized merge and its cleanup
continue.

## REPORT

Report the merge sides and merge base; every path resolved with its one-sentence
rule; every path stopped on, with the competing intents and the specific decision
the operator must make; the checks run and their results; and whether the merge was
completed locally, with its commit, or left in progress. End with exactly one next
action.

## What the skill must not do

- Do not resolve by dropping a side's work, deleting a hunk, or taking one side
  wholesale when both sides changed.
- Do not touch files that were not conflicted.
- Do not hand-edit a generated artifact when its owner can regenerate it.
- Do not resolve security, authorization, approval, redaction, policy, or
  lifecycle conflicts; stop and hand the decision back.
- Do not weaken, delete, or skip tests, and do not bypass checks.
- Do not amend, rebase, force, or rewrite history, and do not delete branches.
- Do not publish, and do not continue a rebase, cherry-pick, or revert.
- Do not reproduce SOP policy, select a model or provider, or call a provider
  directly; SOP resolves those.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see
  the project README / `./install.sh`). Never resolve the merge in SOP's place.
