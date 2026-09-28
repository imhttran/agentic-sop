# T070 --- Run REVIEW Through the Phased (Discovery → Synthesis) Loop

> A capability that owes a structured document needs a bounded discovery and a
> forced, tool-free synthesis — the fix PLAN already got.

## Status

DONE

## Objective

Retrying AHV2010 advanced one stage further: its IMPLEMENT completed, and then
**REVIEW** exhausted its 12-turn budget reading `internal/agent/*` and never
returned a verdict, which errored the whole run:

```text
AHV2010: review: review agent: agent REVIEW failed: exit status 1
  Ollama agent REVIEW did not complete after 12 iterations
  (termination=iteration_limit, last_action="read_file path=internal/agent/ollama.go")
```

REVIEW was still on the generic `executeLoop`, which has no completion path. PLAN
had already been given a two-phase loop (bounded read-only discovery, then a
tool-free synthesis) for exactly this reason; REVIEW needs the same.

## Scope

- `internal/ollamaagent/harness.go`: the two-phase loop is generalised —
  `executePlan` → `executeTwoPhase`, parameterised by a per-capability `twoPhase`
  (discovery/synthesis bounds and synthesis wording); `Execute` routes `PLAN` and
  `REVIEW` to it; REVIEW's synthesis instructions added; the phase helpers renamed
  to drop the PLAN-specific names.
- `internal/ollamaagent/policy.go`: `reviewDiscoveryTurns`/`reviewSynthesisTurns`
  (6 + 2); `PolicyFor(REVIEW)` reports the two-phase total.
- `internal/ollamaagent/harness_test.go`: a REVIEW forced-synthesis test; the phase
  identifier and the REVIEW budget updated.
- `README.md`, `docs/PLAN.md`.

## Rules

- **One loop, two documents.** PLAN and REVIEW share `executeTwoPhase`; they differ
  only in discovery/synthesis bounds and the synthesis wording.
- **Discovery is bounded, synthesis is tool-free.** After the discovery budget the
  model is told exploration is over and may not use tools; it must return the
  document. An early final response still completes immediately.
- **Nothing else changes.** IMPLEMENT/FIX keep their phased loop; DESIGN_TESTS and
  DIAGNOSE_FAILURE keep the generic loop; the tool policies are unchanged.

## Tests

`TestReviewForcedSynthesisAfterDiscoveryLimit`: REVIEW discovers for six turns,
enters synthesis, has a tool request refused, and returns the verdict.
`TestCapabilityBudgets` updated for REVIEW's two-phase total (8). The PLAN tests
are unchanged in behaviour (only the phase identifier was renamed).

## Acceptance Criteria

- [x] REVIEW runs a bounded discovery then a tool-free synthesis and returns its
      verdict instead of erroring the run.
- [x] PLAN keeps its two-phase behaviour.
- [x] IMPLEMENT/FIX, DESIGN_TESTS, DIAGNOSE_FAILURE keep their existing loops.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T070): run REVIEW through the phased discovery → synthesis loop`

## Out of Scope

Changing REVIEW's tool policy or the review engine; making a review failure
non-fatal; changing PLAN's bounds; committing AHV2010's partial work.
