# Acceptance Checks — Authoring Guidance

This guide explains machine-checkable acceptance in SOP plans: what an
`Acceptance Check` is, how to author the `### Acceptance Checks` section, and why
a stage that declares deliverables should also declare at least one check.

## What an Acceptance Check is

An **Acceptance Check** is an operator-declared, deterministic check for a stage:
a shell command whose **exit status 0 means the criterion is met**. It is the
verifiable half of acceptance.

A stage carries two related lists:

- **`Acceptance Criteria`** — free-text statements of what "done" means for the
  stage. They are read by humans and by the model, but nothing executes them.
- **`Acceptance Checks`** — the executable commands that actually prove the
  criteria. Each check is a command, e.g. `go test ./internal/planner/...`.

The `Acceptance Checks` field is optional in the plan schema (`acceptance_checks`
in `plan.json`, rendered as `### Acceptance Checks` in `PLAN.md`). A stage with no
checks omits the key, so a plan without checks stays byte-identical to the earlier
format.

## How to author an Acceptance Checks section

In a rendered plan (`## <id> — <title>` stage shape), add an `### Acceptance
Checks` sub-section and list one command per bullet:

```markdown
## HARD02-001 — Expose the non-fatal authoring lint

Introduce a warning-only diagnostics surface in internal/planner.

### Dependencies

None

### Deliverables

- internal/planner/lint.go exposing the warning-only lint

### Acceptance Criteria

A stage with deliverables and no checks warns without failing.

### Acceptance Checks

- `go test ./internal/planner/...`
- `go vet ./internal/planner/...`
```

In the hand-written `## Tasks` shape, the same field is written inline:

```markdown
### RAG025-001 — Index the corpus

- **Objective:** Build the retrieval index.
- **Files:** `internal/rag/index.go`
- **Acceptance:** The index returns ranked results for a known query.
- **Acceptance Checks:** `go test ./internal/rag/...`
```

Backticks around a command are stripped on parse, so write the command plainly or
wrap it in a single pair of backticks.

## Deliverables without checks: warn, never fail

A stage that **declares deliverables but has no acceptance checks** is flagged
during authoring and migration. The warning names the offending stage, for
example:

```
stage RAG025-001: declares deliverables but no acceptance checks; add an
'### Acceptance Checks' section with a command whose exit status 0 proves each
deliverable
```

This is a **warning, never a hard failure**. Migration stays backwards compatible:

- Existing plans without checks **remain loadable** and still **pass** structural
  validation (`Plan.Validate`).
- The warning is surfaced by a separate authoring lint that is deliberately kept
  distinct from validation. No plan-loading or compilation code path turns the
  warning into an error.
- The RAG-025 plan family — stages that declare deliverables but no checks — is
  flagged as an example of this warning path rather than being rejected.

The lint reports **one warning per offending stage**, so a migrating plan is
flagged stage by stage while remaining valid.

## Required deliverables remain enforceable

Acceptance checks are the mechanism by which a declared deliverable becomes
enforceable: once gating is enabled, a check that does not pass (exit status
truthy) blocks the stage from being considered done, and the declared deliverable
is then actually required to exist. Nothing in this guidance enables gating; the
check-less warning path preserves the required-deliverable contract so it stays
enforceable when gating lands.

## Summary

| Concern | Behaviour |
| --- | --- |
| Stage with deliverables and checks | No warning; checks gate once gating is enabled |
| Stage with deliverables, no checks | **Warning** naming the stage; plan stays loadable and valid |
| Stage with no deliverables | No warning |
| Existing check-less plan (migration) | Loads and validates; flagged, never rejected |
