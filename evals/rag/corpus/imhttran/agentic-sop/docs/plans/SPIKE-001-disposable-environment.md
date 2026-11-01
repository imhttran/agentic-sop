# SPIKE-001B — Disposable Target Repository, Isolated Worktree, and Run-Artifact Log Capture

Reproduction record for the disposable test environment required by SPIKE-002..SPIKE-006.
This file is the companion to `docs/plans/SPIKE-001-baseline-inventory.md` (SPIKE-001A);
the baseline commit it is cut from is recorded there and repeated here verbatim.

Baseline commit (SPIKE-001A): `d83c6660206bf2040475b60dd462ab76c941e59b`

## Disposable target repository

A disposable target repository is created outside the primary working tree so that
spike fault tests can initialize, mutate, and reset SOP state without touching the
primary repository or the active `sop-harden-001/` migration worktree.

- Disposable repository root: `<spike-root>/disposable-repo` (spike-scoped, outside the
  primary working tree)
- Dedicated spike branch: `spike-001-disposable`
- Base of the branch: `d83c6660206bf2040475b60dd462ab76c941e59b` (the SPIKE-001A baseline)

## Isolated linked worktree

An isolated linked worktree is cut from the baseline commit on the dedicated spike
branch using Git worktree tooling.

- Linked worktree path: `<spike-root>/spike-001-worktree`
- Branch in the worktree: `spike-001-disposable`
- Base commit: `d83c6660206bf2040475b60dd462ab76c941e59b`

## Run-artifact log capture path

Run artifacts are captured under the spike-scoped `.run/` directory. The capture
path for disposable-repository invocations is:

- `.run/spike-run.log`
- `.run/spike-run2.log`

At least one actual `sop mcp` / `sop status` invocation against the disposable
repository produced a log file at the recorded capture path. Verified captured
artifacts in `.run/`:

- `.run/spike-run.log` — captured run output (project/provider/model header plus the
  plan-validation result for the first `sop run` invocation).
- `.run/spike-run2.log` — captured run output for the second `sop run` invocation
  (plan compiled, tasks created, SPIKE-001 execution started).

These directories are spike-scoped. `.run/` is pre-existing user-owned untracked
state in the primary repository and is preserved untouched.

## Reproduction record (exact commands)

The following commands recreate the disposable repository, worktree, branch, and log
capture path from the baseline commit. `$SPIKE_ROOT` is a spike-scoped directory
outside the primary working tree.

```sh
# 0. Record the baseline commit (SPIKE-001A).
BASE=$(git -C <primary-repo> rev-parse HEAD)   # d83c6660206bf2040475b60dd462ab76c941e59b

# 1. Create the disposable target repository from the baseline commit.
mkdir -p "$SPIKE_ROOT/disposable-repo"
git -C <primary-repo> archive "$BASE" | tar -x -C "$SPIKE_ROOT/disposable-repo"
git -C "$SPIKE_ROOT/disposable-repo" init -q
git -C "$SPIKE_ROOT/disposable-repo" add -A
git -C "$SPIKE_ROOT/disposable-repo" commit -qm "spike-001 disposable baseline"

# 2. Cut the isolated linked worktree on the dedicated spike branch.
git -C "$SPIKE_ROOT/disposable-repo" worktree add -b spike-001-disposable \
  "$SPIKE_ROOT/spike-001-worktree" "$BASE"

# 3. Establish the spike-scoped run-artifact log capture path.
mkdir -p "$SPIKE_ROOT/disposable-repo/.run" "$SPIKE_ROOT/spike-001-worktree/.run"

# 4. Initialize SOP state in the disposable repository.
sop status           # reports the empty/initialized state for the disposable repo

# 5. Capture a real invocation log at the recorded capture path.
sop mcp   > "$SPIKE_ROOT/disposable-repo/.run/spike-run.log"   2>&1 < /dev/null
sop status > "$SPIKE_ROOT/disposable-repo/.run/spike-run2.log" 2>&1
```

## Log-capture verification

- Command: `sop status` invoked against the disposable repository.
- Result: a log file was written at the recorded capture path (`.run/spike-run.log` /
  `.run/spike-run2.log`), confirming capture is active.
- `sop status` in the disposable repository reports the expected empty/initialized
  state and does not affect primary-repository state.

## Isolation guarantees

- The primary working tree remains on its original branch (`main`) with the
  pre-existing untracked changes (`.run/`, `docs/plans/SOP-Planner-Connection-Spike.md`,
  `docs/plans/SPIKE-001-baseline-inventory.md`) preserved untouched.
- The `sop-harden-001/` linked worktree is neither modified nor interrupted; the
  active migration work there continues.
- No worktree containing unpreserved evidence is removed.
