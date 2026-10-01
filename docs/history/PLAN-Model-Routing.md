# SOP Model Routing Plan

> **Historical / superseded — non-normative.** This is the early, broader model-routing
> design (MODELRT001–MODELRT022). The shipped, authoritative specification is
> [`../specs/MODEL-ROUTING.md`](../specs/MODEL-ROUTING.md); the work that actually
> delivered it is [`../plans/PLAN-Phase-3.5-Model-Routing.md`](../plans/PLAN-Phase-3.5-Model-Routing.md).
> This file is kept for traceability only and does not define current behavior.

## Project

agentic-sop

## Purpose

Add deterministic model routing so smaller, lower-risk tasks can run
locally with an Ollama model such as `qwen3.6:latest`, while larger or
harder tasks can use a stronger configured model such as
`deepseek-v4.1-flash:cloud`.

Routing remains SOP-owned. The provider transports requests; it does not
own workflow state or model-routing policy.

Implement this after the Pre-JEV stabilization gate unless needed as a
prerequisite.

## Target Architecture

``` text
SOP task
  ↓
task/capability signals
  ↓
deterministic router
  ├─ local  → qwen3.6:latest
  └─ strong → deepseek-v4.1-flash:cloud
                 ↓
               Ollama
                 ↓
             Tool Harness
                 ↓
       SOP validation/review
```

## Principles

-   deterministic routing first
-   local-first when appropriate
-   explicit, observable escalation
-   provider and model remain separate
-   user overrides beat automatic routing
-   models cannot silently escalate themselves
-   validation/review/human gates are unchanged
-   normal tests require no live Ollama
-   JEV can later advise through the existing decision seam

## MODELRT001 --- Define Routing Domain Model

Introduce focused types such as `ModelRoute` and `RoutingDecision`
carrying route, model, source, and reason.

### Acceptance Criteria

-   model is distinct from provider
-   route and reason are explicit
-   explicit user selection is representable
-   router owns no workflow state
-   routing logic is not scattered through the harness

## MODELRT002 --- Add Routing Configuration

Target semantics:

``` yaml
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
  routing:
    enabled: true
    local:
      model: qwen3.6:latest
    strong:
      model: deepseek-v4.1-flash:cloud
```

### Acceptance Criteria

-   routing is initially opt-in
-   disabled routing preserves existing behavior
-   local and strong models are configurable
-   `agent.model` remains a valid fallback
-   config parsing/validation is tested

## MODELRT003 --- Add Environment Overrides

Suggested semantics, adjusted to repository conventions if needed:

``` bash
SOP_AGENT_ROUTING=true
SOP_OLLAMA_LOCAL_MODEL=qwen3.6:latest
SOP_OLLAMA_STRONG_MODEL=deepseek-v4.1-flash:cloud
```

### Acceptance Criteria

-   env overrides project routing config
-   existing `SOP_OLLAMA_MODEL` remains compatible
-   precedence is documented and tested

## MODELRT004 --- Define Deterministic Task Signals

Use authoritative inputs such as capability, task/acceptance-criteria
size, known scope, read-only status, risk markers, retry/fix status,
prior local attempt, termination reason, and no-progress state.

### Acceptance Criteria

-   signal extraction makes no model call
-   signals come from SOP/task/runtime state
-   missing signals have safe defaults
-   useful signals are traceable

## MODELRT005 --- Implement Local-Route Policy

Conservatively route suitable work such as narrow read-only tasks,
documentation, targeted tests, small isolated changes, simple config
changes, and focused diagnosis/review.

### Acceptance Criteria

-   policy is centralized and deterministic
-   capability name alone does not decide routing
-   broad/high-risk tasks do not accidentally route local
-   routing can be disabled
-   representative cases have unit tests

## MODELRT006 --- Implement Strong-Route Policy

Route broad refactors, complex recovery, security/architecture-sensitive
changes, large scope, prior escalations, and explicit strong overrides
directly to the stronger model.

### Acceptance Criteria

-   strong rules are centralized
-   explicit strong selection wins
-   an escalated attempt does not silently downgrade
-   reason is recorded
-   representative cases are tested

## MODELRT007 --- Add Automatic Escalation

Allow local → strong escalation based on objective evidence such as
no-progress threshold, route-specific budget exhaustion, repeated
invalid tool requests, repeated structured-output failure, or an
escalation-eligible failure.

### Acceptance Criteria

-   SOP/harness controls escalation
-   model cannot directly choose the stronger model
-   escalation is bounded
-   repository/task state is preserved
-   escalation reason is visible
-   deterministic tests prove transition

## MODELRT008 --- Define Escalation Context Handoff

Pass bounded authoritative context: task, criteria, capability/phase,
relevant files, successful mutations, validation observations,
escalation reason, and remaining work.

### Acceptance Criteria

-   handoff is bounded
-   repository remains source of truth
-   failed chatter is not blindly replayed
-   stronger model receives the escalation reason
-   handoff construction is tested

## MODELRT009 --- Add Explicit CLI Model Override

Target:

``` bash
sop run --model qwen3.6:latest
sop run --model deepseek-v4.1-flash:cloud
```

### Acceptance Criteria

-   explicit model beats automatic routing
-   provider remains independently configurable
-   selection is visible
-   CLI parsing/precedence is tested

## MODELRT010 --- Add Semantic Route Override

Target:

``` bash
sop run --route local
sop run --route strong
```

### Acceptance Criteria

-   routes resolve through config
-   invalid values fail clearly
-   `--model` precedence over `--route` is documented
-   override is visible
-   CLI behavior is tested

## MODELRT011 --- Add Runtime Visibility

Example:

``` text
Agent:
  Harness: tool
  Provider: ollama
  Model: qwen3.6:latest
  Route: local
  Source: automatic
  Reason: targeted low-scope task
```

Escalation should show from/to model and reason.

### Acceptance Criteria

-   model, route, source, and reason are visible
-   escalation is visible
-   secrets are not exposed
-   visibility works without controller changes

## MODELRT012 --- Add Routing Trace Data

Integrate structured metadata into existing traces/run artifacts: route,
model, provider, decision source/reason, escalation, previous model,
capability, outcome, duration, tool count, and validation result.

### Acceptance Criteria

-   no second workflow database is introduced
-   trace format is documented
-   routing can be analyzed after a run
-   sensitive config is not persisted unnecessarily

## MODELRT013 --- Add Local-Model Budget Policy

Allow local routes to use earlier exploration/no-progress escalation
thresholds while preserving the global lifecycle state machine and hard
safety ceilings.

### Acceptance Criteria

-   route thresholds are distinct from lifecycle correctness
-   thresholds trigger escalation, not false completion
-   hard ceilings remain safety rails
-   behavior is tested

## MODELRT014 --- Preserve Validation and Quality Gates

Both routes use the same configured build/test/lint/review/quality/human
approval pipeline.

### Acceptance Criteria

-   local work gets full required validation
-   strong work gets full required validation
-   escalation skips no gate
-   route cannot independently complete a task
-   human approval is unchanged

## MODELRT015 --- Deterministic Router Test Matrix

Cover:

``` text
routing disabled
small → local
large → strong
narrow read-only → local
security-sensitive → strong
--model override
--route local/strong
env/project precedence
local no-progress → strong
local success → no escalation
strong → no downgrade
missing route model config
invocation isolation
```

### Acceptance Criteria

-   no live Ollama required
-   decisions and precedence are deterministic
-   escalation uses fake agents/models
-   tests are stable and fast

## MODELRT016 --- Ollama Integration/Dogfood Path

Provide opt-in integration using configurable models, initially testing
`qwen3.6:latest` locally and `deepseek-v4.1-flash:cloud` as strong.

### Acceptance Criteria

-   unit suite does not require Ollama
-   integration verifies local, strong, and escalation paths
-   missing model diagnostics are useful
-   model names are not hard-coded into harness identity

## MODELRT017 --- Controller Visibility Without Authority

Expose route/model/escalation information through existing SOP
status/run data where practical. `sop-controller` may display it but
must not implement V1 routing policy.

### Acceptance Criteria

-   controller can display SOP-owned routing data
-   controller owns no routing policy
-   no duplicate model config logic
-   controller tolerates absent routing metadata

## MODELRT018 --- Prepare JEV Advisory Seam

Future flow:

``` text
deterministic signals
  ↓
baseline routing decision
  ↓
optional JEV advisory
  ↓
policy reconciliation
  ↓
final SOP-owned route
```

### Acceptance Criteria

-   clear advisory extension point exists
-   deterministic router remains baseline
-   JEV is optional
-   JEV cannot execute tools or mutate SOP state
-   future disagreements can be traced
-   no premature JEV implementation is added

## MODELRT019 --- Dogfood Local-First Routing

Exercise a small local task, small local implementation, larger strong
task, intentional escalation scenario, explicit local override, and
explicit strong override.

### Acceptance Criteria

-   Qwen completes at least one real SOP task
-   strong model completes at least one real SOP task
-   escalation completes one controlled scenario
-   validation/review still run
-   routing reasons are visible
-   failures remain evidence rather than being hidden

## MODELRT020 --- Measure Routing Effectiveness

Capture task type, initial/final route, model, escalation, agent
duration, tool calls, validation/review, fix cycles, and outcome.

### Acceptance Criteria

-   local and strong runs have comparable measurements
-   escalation frequency and local success rate can be derived
-   routing mistakes can be identified
-   data is usable for later JEV evaluation
-   insufficient samples do not produce unsupported quality claims

## MODELRT021 --- Documentation

Document configuration, precedence, routing, escalation, overrides,
troubleshooting, and the future JEV seam.

### Acceptance Criteria

-   architecture/configuration are documented
-   CLI examples are documented
-   escalation and precedence are documented
-   JEV is described as future/optional unless actually enabled

## MODELRT022 --- Final Routing Gate

Verify:

``` text
SOP
 ↓
deterministic router
 ├─ local Qwen ────┐
 └─ strong model ──┤
                   ↓
                 Ollama
                   ↓
               Tool Harness
                   ↓
          SOP validation/review
```

### Acceptance Criteria

-   disabled mode is backward compatible
-   automatic local and strong routes work
-   local → strong escalation works
-   model/route overrides work
-   runtime visibility works
-   controller remains non-authoritative
-   deterministic/race/build tests pass
-   real Ollama dogfood passes
-   no critical/high routing issue remains

## Configuration Precedence

Target precedence:

``` text
--model
  ↓
--route
  ↓
environment routing/model override
  ↓
project routing config
  ↓
agent.model/default
```

Keep exact behavior compatible with existing SOP conventions.

## Final Validation

``` bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Reinstall the repository-supported known-good SOP tooling before final
dogfood.

## Safety Constraints

Do not duplicate routing in `sop-controller`, put workflow authority in
the provider, let a model alter routing configuration, silently
downgrade strong routes, silently override `--model`, weaken local
validation, hard-code Qwen/DeepSeek as the only choices, require JEV for
V1, create a second state DB, manually edit `.agent-sdlc/state.db`, or
bypass existing commit/push/merge/human controls.

## Definition of Done

``` text
small / low-risk
  → qwen3.6:latest
  → success
  → SOP validation

or

qwen3.6:latest
  → objective no-progress
  → strong-model escalation
  → SOP validation

large / complex / high-risk
  → strong model directly
  → SOP validation
```

Routing is deterministic, configurable, testable, observable, and ready
for a future JEV advisory signal without moving workflow authority out
of SOP.
