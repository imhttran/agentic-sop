# Deterministic per-task model routing (Phase 3.5)

> **This page is non-normative and subordinate.** The authoritative home for
> model-routing rules — model classes, configuration precedence, the automatic
> router, and the routing boundary — is
> [`specs/MODEL-ROUTING.md`](../specs/MODEL-ROUTING.md). This page MUST NOT restate
> or override those rules; where it and the specification disagree, the
> specification wins. It records only the implementation seam, which is not
> normative.

The router is **opt-in and off by default**: it activates only when
`SOP_MODEL_ROUTING_ENABLED=true` (environment, overriding `models.routing_enabled`
in configuration), a `SOP_MODEL_*` variable, or `--model-class` is present. A manual
`--model-class` override always wins. See
[`specs/MODEL-ROUTING.md`](../specs/MODEL-ROUTING.md) for the normative rules and
[`reference/CONFIGURATION.md`](../reference/CONFIGURATION.md) for the configuration
reference.

## The seam

This section records the lifecycle integration seam for P35-004: how SOP selects
the implementation-agent model from a routing decision, and the boundaries that
keep routing from ever acting as lifecycle authority.

- **Package/function:** `internal/cli` — `routingForTask`
  (`routingForTask(cfg config.Config, d deps, spec *taskfile.Spec, tri, pre earlyGateResult) (taskRouting, bool, error)`)
  computes the decision; `applyTaskRouting` turns it into an implementation agent
  and is called from `internal/cli/run.go` (`runStages`) immediately before the
  IMPLEMENT step, after the optional pre-execution JEV checkpoint.
- **Inputs available at the seam:** the task definition (`*taskfile.Spec`: the
  acceptance-criteria and dependency counts), the early-JEV evidence pointers
  (`task_triage`, `pre_execution` gate results), the `--model-class` value
  (`deps.modelClass`), the router feature flag (`deps.routingEnabled`, resolved by
  `applyRoutingEnabled` from `model.RoutingEnabled(cfg.Models, os.Getenv)`), the
  project config (`cfg.Models`), and the agent factory (`deps.newAgent`).
- **Where the agent model is set:** `applyTaskRouting` calls `deps.newAgent` with
  the selected provider/model; the returned agent replaces the implementation
  agent for the bounded implementation work.

## The selection contract

The normative precedence and decision rules live in
[`specs/MODEL-ROUTING.md`](../specs/MODEL-ROUTING.md) ("Configuration Precedence",
"Automatic Router", and "Manual Override"). At the seam:

1. Gate on routing: a manual `--model-class` override wins; otherwise the router
   must be enabled, else the seam is a strict no-op (`ok=false`).
2. Derive typed signals: `router.TaskSignals` from the task definition, merged
   (conservatively) with `router.SignalsFrom(ev, task)` from each available
   early-JEV checkpoint.
3. `router.Decide(signals)` selects **only a class** plus fixed-phrase reasons.
4. `model.Resolve(model.Inputs{CLIClass, RoutedClass, RoutedReason, Config,
Lookup})` alone resolves the class to a concrete provider/model
   (`Selection.Provider`, `Selection.Model`). An incomplete class fails closed
   with an actionable error; no silent default is applied. For a LOCAL class with
   a configured fallback, Resolve additionally reports the fallback as
   `Result.LocalFallback` — a candidate it never applies.
5. The local-first fallback is applied here, once: `applyLocalFallback` replaces
   the selection with `Result.LocalFallback` only when the injected read-only
   availability probe (`deps.localProbe`, the production `localRuntimeProbe`)
   reports the local runtime cannot serve the primary model. It is a no-op when no
   probe is wired, so an unwired test or process keeps the local model; it is
   never reached on a generation, validation, or gate failure.
6. The resolved provider/model builds the implementation agent.

## Boundaries (what routing must never do)

The normative boundary lives in [`specs/MODEL-ROUTING.md`](../specs/MODEL-ROUTING.md)
("Boundary"). At the seam, the implementation upholds it:

- The router is pure: it never transitions task state, mutates the repository,
  calls a provider, names a concrete model, or persists anything.
- Routing only selects a model. It never approves, blocks, or bypasses an
  approval, commit, or merge gate; the lifecycle and gate logic remain the sole
  owner of any action.
- The routing decision is recorded as write-only diagnostic evidence
  (`routing.json`) via `(*run.Run).WriteRoutingArtifact`. No code reads
  `routing.json` back to drive a decision; the in-process `router.Decide`
  output (the CLI's `taskRouting`) is the sole source of truth.
- `WriteRoutingArtifact` fails closed: an artifact with an unknown version or
  source is rejected and no file is written.

## Implemented vs. proposed

The split between implemented and proposed/future behavior is owned by
[`specs/MODEL-ROUTING.md`](../specs/MODEL-ROUTING.md) ("Implemented Behavior" and
"Proposed / Future Behavior"). The local-first fallback for a local class is
**implemented**; it is an availability switch (a read-only runtime observation),
never a failure-recovery policy. Automatic downgrade after a failure, learned or
LLM-controlled routing, and cost- or benchmark-driven class selection are **not
implemented**.
