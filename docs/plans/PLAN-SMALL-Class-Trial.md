# PLAN — SMALL class trial

**Type:** Operator trial plan (proposed; not started).
**Decision it informs:** whether automatic routing should be allowed to select `small`, or
whether `medium` stays the effective default. The current decision is recorded in
[MODEL-ROUTING.md § Default class decision](../specs/MODEL-ROUTING.md#default-class-decision-small-reachability).

## Why a trial

Automatic `small` is reachable only when all of these hold:

1. Automatic routing is on (`models.routing_enabled` / `SOP_MODEL_ROUTING_ENABLED`; off by
   default).
2. An early-JEV checkpoint produced typed evidence (`early_jev.enabled` plus at least one
   `early_jev.gates.*`; all off by default). Without it the router returns `medium`.
3. The task qualifies as small on every signal (`internal/router` `qualifiesSmall`).

Adaptive routing (`internal/adaptiveroute`) never selects a class below the router's
baseline, so it cannot make `small` reachable on its own. No run has yet recorded the
`small` class: TEL-009 observed 0 router-sourced selections, and
[CLOSE-001](../reports/pre-performance-closure/CLOSE-001-baseline.md) lists `small`
(`ollama/qwen3:4b`) as not observed. The trial produces that evidence.

The steps run in order on an operator machine with the `small` model available. Each
step's output is the next step's input.

## Step 1 — explicit `small` runs

Goal: measure whether the `small` class completes small tasks.

1. Choose at least 10 tasks that meet the router's SMALL bounds: one file, at most 3
   acceptance criteria, no dependencies, and no security, destructive, credential, or
   approval-sensitive area.
2. Run each with the class pinned:

   ```bash
   sop run --model-class small --task TASK.md
   ```

3. For each run, read `.agent-sdlc/runs/<run-id>/trace.json`:
   - `execution.model_class` must be `small`;
   - `execution.fallback` shows whether the cloud fallback served the run instead of the
     local model;
   - `termination.stage` is `PASSED` for a success. Adaptive routing counts only `PASSED`
     as a success.
4. Record per task: task id, terminal stage, whether fallback was used, and duration.

**Exit:** at least 10 recorded `small` runs. Report the local-model success rate
separately from runs the cloud fallback served.

## Step 2 — routing with JEV triage

Goal: measure how often the router selects `small` on real work, and whether those runs
pass.

Enable in `config.yaml`:

```yaml
models:
  routing_enabled: true
early_jev:
  enabled: true
  gates:
    task_triage: true
```

Then run normal work without `--model-class`. For each run, read `routing.json` and
`trace.json` (`execution.routing_source` is `router` for an automatic selection) and
record the selected class and its reason. `sop report --decisions` gives the
project-wide routing summary.

**Exit:** at least 20 routed runs, or two weeks of normal use, whichever comes first.

Rollback: set `models.routing_enabled: false` (or `SOP_MODEL_ROUTING_ENABLED=false`). The
`early_jev` block can stay on independently.

## Step 3 — decision

Revisit the default class decision with the data from steps 1 and 2:

| Measure                                      | Source    | Keep `medium` if  | Allow automatic `small` if         |
| -------------------------------------------- | --------- | ----------------- | ---------------------------------- |
| `small` success rate on qualifying tasks     | Step 1    | below 0.75        | at least 0.75 (adaptive threshold) |
| Share of routed runs selecting `small`       | Step 2    | negligible        | a meaningful share of the work     |
| `small` runs that escalated or failed        | Step 2    | frequent          | rare                               |
| Runs served by the cloud fallback, not local | Steps 1–2 | most `small` runs | a minority                         |

If the evidence supports `small`, the change is configuration only: keep routing and
early-JEV triage enabled. The router's SMALL rule and the no-evidence → `medium` rule do
not change. Record the outcome in
[MODEL-ROUTING.md](../specs/MODEL-ROUTING.md#default-class-decision-small-reachability).
