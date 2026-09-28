# PREJEV012 — Reconciliation State

Captured: 2026-09-28. Scope: `agentic-sop`, the PREJEV012 umbrella milestone after
its decomposition into sub-tasks PREJEV012-S6 through PREJEV012-S12.

## Summary

The PREJEV012 decomposition is applied to the **plan** — both the committed human
source and the machine plan. The **live task graph** in `.agent-sdlc/state.db`
still holds the original 18 tasks and does not contain the S6–S12 sub-tasks. This
document records that gap, why SOP cannot close it automatically, and the options.

## Plan state (in sync)

- Human source: `docs/PLAN-Pre-JEV-Stabilization.md`. PREJEV012 is an umbrella
  that depends on `PREJEV012-S6` … `PREJEV012-S12`.
- Machine plan: `.agent-sdlc/plan.json` — 25 stages; the umbrella plus seven
  sub-stages, all `execution_mode: implement`.
- Provenance: `.agent-sdlc/plan.meta.json` `source_sha256` =
  `a254a4d91f3c9949a10bde4e4bf1ac82558233fa4eecb5168a71558c38bb9b43`, matching
  the source file exactly.
- Decomposition detail: `docs/PREJEV012-REGRESSION-DECOMPOSITION.md`. Runnable
  specs: `docs/tasks/prejev012/PREJEV012-S6.md` … `PREJEV012-S12.md`.

## Task-graph state (lagging)

`.agent-sdlc/state.db` still contains the 18 pre-decomposition tasks
(PREJEV001–PREJEV018). `PREJEV012-S6` … `S12` are plan stages, not persisted
tasks. Because `plan.meta.json` was regenerated to match the source, `sop run`
with no argument will **not** raise the usual `NEEDS_HUMAN: plan changed` prompt;
it will silently keep driving the old graph and will not create the sub-tasks.

## Why SOP does not close it automatically

- Tasks are created only when the graph is empty (`planflow.Prepare` →
  `ensureTasks`). With tasks already present, SOP reconciles state instead.
- `store.SaveTasks` rejects a batch that contains an existing task, so `sop tasks`
  cannot add the new stages to a populated graph.
- SOP refuses to mix two plans, or to silently rebuild a graph that has execution
  history — by design.

## Options

### A — Run the sub-tasks directly (non-destructive; recommended)

Execute each sub-task from its task file. No graph change is required; each run
writes its own directory `.agent-sdlc/runs/PREJEV012-S<n>/`.

```bash
sop run --task docs/tasks/prejev012/PREJEV012-S6.md
sop run --task docs/tasks/prejev012/PREJEV012-S7.md
sop run --task docs/tasks/prejev012/PREJEV012-S8.md
sop run --task docs/tasks/prejev012/PREJEV012-S9.md
sop run --task docs/tasks/prejev012/PREJEV012-S10.md
sop run --task docs/tasks/prejev012/PREJEV012-S11.md
sop run --task docs/tasks/prejev012/PREJEV012-S12.md
```

Each runs the same loop — verify existing coverage → implement only the missing
coverage → validate independently — and completes with `changes_expected=false`
when the area is already covered. Stops at the human gate; never commits, pushes,
or merges.

### B — Rebuild the graph from the new plan (deliberate; destructive)

Rebuilding materializes all 25 stages as tasks, including `PREJEV012-S6` … `S12`.
SOP builds a graph only when none exists, so this requires archiving or removing
`.agent-sdlc/state.db` first, then running `sop run`. This **discards prior task
execution history** for the pre-JEV plan and conflicts with the plan's Safety
Constraints, which forbid reset or erase as a *recovery* step. Treat it as a
one-time, human-approved migration, not as recovery, and back up `.agent-sdlc`
first.

```bash
cp -a .agent-sdlc .agent-sdlc.bak   # keep the history
rm .agent-sdlc/state.db             # explicit, human-approved
sop run                             # rebuilds 25 tasks from plan.json
```

### C — Keep the old graph, satisfy the umbrella out of band

Leave the 18-task graph as-is (PREJEV001–PREJEV011 are already complete) and treat
PREJEV012 as satisfied by the S6–S12 runs from Option A, evidenced by
`.agent-sdlc/runs/PREJEV012-S<n>/`. No `state.db` change; the umbrella is closed by
evidence rather than by a graph edge.

## Invariants (hold for every option)

- Lifecycle remains `PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`.
- Human gates are intact; nothing commits, pushes, or merges without approval.
- No fabricated CI, PR, or merge results.
- No regression task duplicates SOP orchestration.
- `.agent-sdlc/state.db` is never hand-edited.

## Recommendation

Use **Option A** now — it is reversible and needs no graph surgery. Defer **Option
B** until the pre-JEV graph is otherwise finished, and only if the plan graph
itself must reflect the decomposition before JEV. **Option C** is the natural
outcome if B is deferred and the umbrella is closed by the recorded S6–S12 runs.

## Reproduce the plan state

```bash
# The plan compiles deterministically (no agent) to 25 stages:
python3 - <<'PY'
import hashlib, json
src = open("docs/PLAN-Pre-JEV-Stabilization.md","rb").read()
meta = json.load(open(".agent-sdlc/plan.meta.json"))
print("source sha:", hashlib.sha256(src).hexdigest())
print("meta   sha:", meta["source_sha256"])
print("stages:", len(json.load(open(".agent-sdlc/plan.json"))["stages"]))
PY
```
