# Phase 9 — Multi-Agent Adoption Decision (ORCH-012)

**Type:** decision record (report) · **Status:** point-in-time · **Phase:** 9 · **Task:** ORCH-012

## Decision

``` text
PHASE_9_ACCEPTANCE        = PASS
MULTI_AGENT_IMPLEMENTATION = VERIFIED
DEFAULT_EXECUTION         = SINGLE
MULTI_AGENT               = OPT-IN
```

Multi-agent orchestration is implemented and verified, but default execution
remains single-agent. The deterministic, model-free adoption gate does not
demonstrate a meaningful benefit that would justify changing the default for any task
class, so multi-agent execution stays explicitly opt-in and disabled by default.

## Gate

`sop gate orchestration [--json]` runs the deterministic, model-free Phase 9
adoption gate (`internal/orchadopt`). It compares the single-agent baseline against
the multi-agent candidate over the built-in corpus of genuinely-parallelizable tasks
(independent, non-mutating analysis units) and decides whether multi-agent execution
has earned default enablement. Nothing here runs a live model; a dimension the gate
cannot observe deterministically is reported as *not measured* rather than invented.

The gate measures: task success, verification success, worker count, tool calls,
discovery work, repeated discovery, repository mutations, conflicts, integration
attempts, replans, model escalations, and context bytes/items. Wall-clock time and
provider cost/token counts are reported as not measured.

On the built-in corpus the candidate does strictly more work (one worker per unit plus
integration) with no deterministic improvement in success, so the gate returns
`REJECT`: default execution remains single-agent. Re-run `sop gate orchestration` to
reproduce the result.

## Status vocabulary

- **IMPLEMENTED:** the orchestration domain, contract, decomposition, coordinator,
  ownership, conflict, integration, budgets, trace/evaluation, context/routing, and the
  opt-in end-to-end path (ORCH-001–ORCH-011).
- **VERIFIED:** the full deterministic gate passes (`gofmt`, `go vet`, `go test`,
  `go test -race`, `go build`, `git diff --check`), and the focused orchestration tests
  pass (determinism, bounded fan-out, context routing, failure propagation, provider
  neutrality, default-off).
- **EXPERIMENTALLY PROVEN:** not established. No representative end-to-end multi-agent
  run on real parallelizable tasks has demonstrated a latency, reliability, or success
  benefit over the single-agent baseline.
- **DEFAULT-ENABLED:** NO — `orchestration.enabled` remains `false`.
- **DEFERRED:** default adoption for any task class; multi-agent adoption awaits
  representative end-to-end evidence.
- **BLOCKED:** none.

## Enablement

Multi-agent execution is reachable only by explicit opt-in: set
`orchestration.enabled: true` and run `sop orchestrate TASK.md`. With orchestration
disabled (the default), single-agent execution is unchanged and no orchestration path
is invoked. Introducing Phase 9 changed no default model-routing or provider behavior.

## Invariants preserved

Provider/model neutrality, single-lifecycle authority, bounded execution, deterministic
aggregation, context isolation, failure propagation, and existing human-boundary
semantics are preserved by the Phase 9 implementation and exercised by the deterministic
test suite.
