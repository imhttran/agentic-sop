# POST8-001 — Phase 8 Live-Path Integration Governance & Acceptance

> **Document class:** plan · **Lifecycle:** complete · **Authority:** historical — a record of completed governance work, not current planning authority.

**Status:** Complete. POST8-001 validated the already-merged Phase 8 live-path integration
and was archived by SOP (`sop plan complete`,
`.agent-sdlc/archive/post8-001-phase-8-live-path-integration-governance/`). Phase 8 was not
reopened.

## Project

agentic-sop

## Summary

Govern the already-merged Phase 8 live-path integration. The integration exists on
`main`; this plan does not reimplement it. It validates the merged implementation
against its stated invariants (feature-gated behavior, preserved lifecycle, preserved
provider/model neutrality), records the evidence through SOP's governed lifecycle, and
establishes a clean baseline before Phase 9.

Phase 8 is closed and is not reopened by this plan.

## POST8-001 — Phase 8 Live-Path Integration Acceptance

Validate the already-merged live-path integration of the Phase 8 facilities without
reimplementing or modifying it. Confirm that the opt-in gate (`context.decision_memory`)
preserves pre-integration behavior when off and is genuinely reachable when on; that
provider/model neutrality is intact; that lifecycle, approval, retry, replan, budget,
verification, and external-completion semantics are unchanged; and record the evidence
through SOP's external-completion operation.

The persistent cross-run verification cache is explicitly deferred: it requires a
separate evidence-gated design, so it is recorded as a follow-up item rather than wired
here.

### Acceptance Criteria

- The already-merged integration is identified from repository evidence (main HEAD,
  implementation commit(s), affected packages) and validated, not reimplemented.
- With `context.decision_memory` off (the default), `sop run` behavior is unchanged from
  the pre-integration baseline: no lifecycle, approval, retry, replan, budget, provider,
  model-class, verification, or termination semantics differ.
- With `context.decision_memory` on, the facility is reachable and exercised: applicable
  decisions appear in the implementation context.
- Decision memory never overrides current repository evidence; a decision recorded under
  another repository state is excluded, and provenance is retained.
- Provider/model neutrality holds: the provider-neutrality architecture test passes and
  the integration introduces no direct provider/model implementation dependency.
- The Phase 8 facility inventory is recorded (verifcache, promptcache, decisionmemory,
  adaptiveroute), each classified by its actual production call path.
- The persistent cross-run verification cache is recorded as deferred with a follow-up
  item; it is not implemented.
- The IMPLEMENT_NO_PROGRESS -> BLOCK -> nonretryable -> no human boundary is unchanged,
  and the approval, retry, replan, budget, external-completion, and plan-lifecycle
  regressions pass.
- `gofmt`, `go vet`, `go build`, `go test`, `go test -race`, `git diff --check`, and the
  documentation link check all pass.
