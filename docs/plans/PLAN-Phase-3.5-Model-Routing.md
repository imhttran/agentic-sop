# PLAN --- Phase 3.5: JEV-Guided Model Routing & Stabilization

**Type:** Implementation plan

**Status:** Implemented and committed. The plan records the work that connects the
Phase 2.5 model-class layer with the Phase 3 early-JEV evidence, so a reviewer can
see the tasks, their scope, and how the acceptance criteria map to
[../specs/MODEL-ROUTING.md](../specs/MODEL-ROUTING.md). It MUST NOT override the
specifications; where it and a specification disagree, the specification wins.

## Summary

Add a deterministic, opt-in model-class router that selects `small`, `medium`, or
`large` per task from typed task/JEV evidence. JEV provides evidence; SOP's router
decides the class; the model layer resolves the class to a concrete model. The
router is OFF by default, so an existing installation is unchanged.

## Objective

Let SOP route bounded implementation work to the most appropriate configured model
without letting JEV, or any model, choose the model or control lifecycle state, and
without bypassing validation, review, quality, or human approval.

## Architecture

```text
Task
  |
  v
deterministic analysis (task structure + typed early-JEV evidence)
  |
  v
SOP routing policy (internal/router.Decide)
  |
  v
small | medium | large
  |
  v
configured model (internal/model resolves the class)
  |
  v
execution -> validation -> review
```

Preserve:

```text
JEV provides evidence.
SOP owns policy and decisions.
IMPLEMENT/FIX changes code.
```

## Rollout Order

```text
3.5.1  Phase 3 typed-evidence stabilization
3.5.2  model-class abstraction (already existed from Phase 2.5)
3.5.3  deterministic router + unit tests
3.5.4  execution integration (select the task's agent model)
3.5.5  decision persistence (routing.json)
3.5.6  CLI / report visibility
3.5.7  manual --model-class override precedence
3.5.8  end-to-end tests
3.5.9  feature-flag rollout (SOP_MODEL_ROUTING_ENABLED, OFF by default)
```

## Tasks

Each task lists: objective, scope, dependencies, likely files/packages,
acceptance criteria, and validation.

### P35-001 --- Define the deterministic router

- **Objective:** Map typed evidence to a model class, deterministically.
- **Scope:** New `internal/router` package: closed `Level`/`Scope` enumerations, a
  `Signals` type derived from typed JEV evidence and task structure, a
  `Signals.Merge` that only escalates, and a pure `Decide(Signals) Decision`
  implementing the risk -> complexity -> cross-cutting -> multi-file -> default ->
  small precedence. Fixed-phrase reasons only.
- **Depends on:** none.
- **Likely files/packages:** `internal/router/router.go`.
- **Acceptance criteria:** Deterministic; consumes typed evidence only (`(category,
severity)`, scope/level values, counts); never string-matches; a bare confidence
  threshold is never a rule; `medium` is the safe default; security-family evidence
  never routes below `large`.
- **Validation:** `go test ./internal/router/...`; `go vet ./internal/router/...`.

### P35-002 --- Add the router feature flag and routed-class resolution

- **Objective:** Enable per-task routing without changing existing behavior.
- **Scope:** Add `models.routing_enabled` and `SOP_MODEL_ROUTING_ENABLED` (default
  false) to the model-routing layer, and a `RoutedClass`/`RoutedReason` input to
  `model.Resolve` with precedence `CLI > router > env default > config default >
built-in`. Add `model.SourceRouter`.
- **Depends on:** P35-001.
- **Likely files/packages:** `internal/model/model.go`, `internal/config/config.go`.
- **Acceptance criteria:** Routing is inactive by default; a routed class activates
  the layer and resolves through the built-in defaults; a manual `--model-class`
  override wins over the router.
- **Validation:** `go test ./internal/model/... ./internal/config/...`.

### P35-003 --- Persist the routing decision

- **Objective:** Make the routing decision auditable after the process exits.
- **Scope:** New `internal/run` routing artifact (`routing.json`): versioned,
  validated, diagnostic-only, no credential, no state transition. Carries the class,
  source, reasons, resolved provider/model/locality, informing checkpoints, and a
  typed signal summary.
- **Depends on:** P35-001.
- **Likely files/packages:** `internal/run/routing_artifact.go`.
- **Acceptance criteria:** Unknown version/source fails closed; the artifact is
  never read back to drive a decision; it never creates a second source of truth.
- **Validation:** `go test ./internal/run/...`.

### P35-004 --- Integrate routing into the lifecycle

- **Objective:** Select the task's implementation-agent model from the routing
  decision.
- **Scope:** A CLI seam that computes the decision immediately before implementation
  (after the pre-execution checkpoint), using the triage and pre-execution evidence
  when present, and rebuilds the agent for the selected class. It only selects a
  model; it never blocks, approves, or bypasses a gate. Manual override is recorded
  as `manual_override`.
- **Depends on:** P35-002, P35-003.
- **Likely files/packages:** `internal/cli/routing.go`, `internal/cli/run.go`,
  `internal/cli/drive.go`, `internal/cli/modelroute.go`.
- **Acceptance criteria:** A disabled router is a strict no-op; a manual override
  wins; the selected class resolves to a concrete model; no gate is bypassed.
- **Validation:** `go test ./internal/cli/...`.

### P35-005 --- Add CLI and report visibility

- **Objective:** Let an operator see why a task ran on the model it did.
- **Scope:** A concise `Task routing:` line, and `routing` in `report.json` /
  `report.md` / `sop report` (`Model routing:` section).
- **Depends on:** P35-004.
- **Likely files/packages:** `internal/cli/report.go`, `internal/cli/run.go`.
- **Acceptance criteria:** The class, resolved model, source, reasons, and evidence
  are shown; nothing is shown when routing did not apply.
- **Validation:** `go test ./internal/cli/...`.

### P35-006 --- Unit and end-to-end tests

- **Objective:** Prove routing behavior deterministically with no live provider.
- **Scope:** Table-driven router tests (all three classes, conflicting evidence,
  unavailable/malformed evidence, reason phrases, confidence-is-not-a-rule), and
  CLI integration tests (disabled = unchanged; clear -> small; cross-cutting ->
  large; no evidence -> medium; manual override; no credential in the artifact).
- **Depends on:** P35-001, P35-002, P35-003, P35-004, P35-005.
- **Likely files/packages:** `internal/router/router_test.go`,
  `internal/cli/routing_test.go`, `internal/model/model_test.go`.
- **Acceptance criteria:** Deterministic, fast, offline tests cover the compatibility,
  escalation, fallback, override, and secrecy requirements.
- **Validation:** `go test ./...`, `go test -race ./internal/cli/ ./internal/router/`.

### P35-007 --- Document the routing layer

- **Objective:** Make the routing rules authoritative and discoverable.
- **Scope:** New `../specs/MODEL-ROUTING.md` (normative); update
  `../reference/CONFIGURATION.md`, `../README.md`, and `.env.example`. Separate
  implemented behavior from proposed/future behavior.
- **Depends on:** P35-001, P35-002, P35-003, P35-004, P35-005, P35-006.
- **Likely files/packages:** `docs/specs/MODEL-ROUTING.md`,
  `docs/reference/CONFIGURATION.md`, `docs/README.md`, `.env.example`.
- **Acceptance criteria:** Links resolve; precedence, classes, and the boundary are
  documented; the router is described as opt-in and off by default.
- **Validation:** link check (0 broken links).

### P35-008 --- Phase 3.5 acceptance validation

- **Objective:** Confirm the whole repository is green and documentation is
  consistent.
- **Scope:** Run the repository's canonical checks and confirm no behavior changed
  for a disabled router.
- **Depends on:** P35-001, P35-002, P35-003, P35-004, P35-005, P35-006, P35-007.
- **Acceptance criteria:** `make check` (fmt, vet, test, build) passes; race tests
  pass for the affected packages; no live provider is required.
- **Validation:** `make check`; `go test -race ./internal/cli/ ./internal/router/
./internal/model/ ./internal/config/ ./internal/run/`.

## Dependency Graph

```text
P35-001 --+--> P35-002 --+--> P35-004 --> P35-005 --+
          |              |                            |
          +--> P35-003 --+                            +--> P35-006 --> P35-007 --> P35-008
```

## Overall Validation

```bash
make check
go test -race ./internal/cli/ ./internal/router/ ./internal/model/ ./internal/config/ ./internal/run/
```

## Safety Invariants

- Routing never transitions task state, approves, rejects, blocks, or bypasses
  validation, review, quality, or human approval.
- Routing never executes commands, modifies the repository, commits, pushes, or
  merges.
- JEV never chooses the model; SOP's deterministic router does.
- A model never controls routing.
- No second workflow engine, state store, or provider stack is introduced.
- `.agent-sdlc/state.db` is never edited by hand.

## Definition of Done

Phase 3.5 is complete when SOP has a first-class `small`/`medium`/`large`
abstraction; concrete models remain configurable; JEV provides typed routing
evidence but never chooses the model; routing is deterministic and unit tested;
`medium` is the safe default; high-risk or clearly cross-cutting tasks escalate to
`large`; clearly isolated low-risk tasks can route to `small`; JEV failure does not
block SOP; operators can override the class and cannot bypass safety policy; the
decision and reasons are persisted and shown in `sop report`; and automatic routing
can be disabled with a feature flag, leaving existing workflows unchanged.

## Out of Scope

Dynamic cost optimization, automatic provider switching, benchmark- or
performance-driven model selection, learned routing, LLM-controlled routing, remote
SOP Hub, MCP orchestration, distributed execution, and automatic downgrade/escalation
loops. See [../specs/MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) for what remains
proposed.
