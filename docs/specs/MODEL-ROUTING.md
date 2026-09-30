# Model Routing Specification

**Type:** Normative specification

## Purpose

Defines the required behavior of SOP's model-routing layer: the class abstraction,
the configuration precedence, the automatic router, and the boundary between
routing and the rest of the lifecycle. It is the authoritative home for model
routing rules; the reference and configuration documents link here rather than
restating them.

## Related Specifications

- [AGENT-PROVIDER.md](AGENT-PROVIDER.md) — the harness/provider/model boundary and
  how a resolved model reaches the agent.
- [PROVIDERS.md](PROVIDERS.md) — the provider/runtime layer beneath routing: how a
  selected model is inspected and (opt-in) validated before execution. Routing is
  unaffected by it.
- [OPENJEV.md](OPENJEV.md) — the JEV analysis boundary that produces the typed
  evidence the router consumes.
- [EXECUTION.md](EXECUTION.md) — the `sop run` lifecycle the router plugs into.
- [QUALITY.md](QUALITY.md) — the quality gate, which routing MUST NOT affect.
- [SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md) — the ownership model
  (JEV analyzes, SOP decides, the model runs bounded work).

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

## Model Classes

SOP MUST address models by class, never by a hard-coded model name.

```text
small  |  medium  |  large
```

A class names a slot in the routing table. The concrete provider, model name, and
locality for each class MUST come from configuration (the `models:` block,
`SOP_MODEL_*` environment, or the built-in defaults), never from a literal in
execution code. The built-in defaults are:

| Class    | Provider | Model                       | Locality |
| -------- | -------- | --------------------------- | -------- |
| `small`  | `ollama` | `qwen3:4b`                  | `local`  |
| `medium` | `ollama` | `glm-5.3-flash:cloud`       | `cloud`  |
| `large`  | `ollama` | `deepseek-v4.1-flash:cloud` | `cloud`  |

An unknown class, locality, or provider MUST fail with an actionable error naming
the accepted values.

## Configuration Precedence

The resolved model selection MUST follow this precedence, highest first:

```text
1. a manual CLI override (--model-class)
2. the automatic router's class (when the router is enabled and no CLI override)
3. the environment (SOP_MODEL_*, which an optional .env file may supply)
4. the project configuration (the models: block)
5. built-in defaults
```

A manual `--model-class` override MUST always win over the automatic router, so an
operator's explicit choice is never silently replaced. It is recorded as a
`manual_override` routing decision.

The routing layer MUST be a strict no-op — leaving the agent selection unchanged —
unless a `models:` block, a `SOP_MODEL_*` variable, a `--model-class` override, or
an enabled router with a selected class is present. The built-in defaults MUST NOT
activate routing on their own.

## Automatic Router

The automatic router (`internal/router`) MUST map typed evidence to a class. It is
a pure, deterministic function: the same inputs MUST always yield the same class
and the same reasons.

The router MUST be off by default. It is enabled only by
`SOP_MODEL_ROUTING_ENABLED=true` (environment), which overrides
`models.routing_enabled` (configuration).

### Inputs

The router MUST consume only typed evidence, never free-form prose:

- the SOP task's structural signals (acceptance-criteria count, dependency count,
  and, when known, file count), and
- the typed JEV evidence produced by the early checkpoints
  (`internal/jev` evidence: closed purpose, severity, status, category, and a
  bounded confidence).

The router MUST NOT perform string matching, keyword detection, or topic-word
analysis on any summary, detail, or message text.

### Decision rules

The router MUST apply this precedence:

```text
risk escalation       -> large   (a high-risk signal overrides small qualification)
complexity escalation -> large
cross-cutting scope   -> large
multi-file scope      -> medium
no JEV evidence       -> medium  (the safe default)
small qualification   -> small
otherwise             -> medium
```

- `medium` MUST be the safe default.
- A high-risk signal (a security, destructive, credential-sensitivity, or
  approval-sensitive category) MUST NOT route below `large`, even when the
  complexity signal is low.
- `small` MUST require affirmative typed evidence on every signal: available JEV
  evidence, low risk, low complexity, single-file scope, no cross-cutting change,
  no missing context, within the small file/criteria bounds, and no dependencies.
  Without JEV evidence the router MUST NOT select `small`.
- A stated confidence MUST NOT select a class on its own. A bare confidence
  threshold (for example "confidence < 0.70 → human/other class") MUST NOT be a
  routing rule. Confidence MAY be carried as evidence and recorded.

### Reasons

Every decision MUST carry one or more deterministic reasons drawn from a fixed set
of phrases (for example "high-risk evidence", "cross-cutting change", "isolated
low-risk task", "JEV evidence unavailable; defaulting to medium"). The reasons
MUST NOT be model-generated prose, and MUST NOT be parsed back to drive a decision.

## Manual Override

An operator MAY select a class explicitly with `sop run --model-class <class>`. The
override:

- MUST take precedence over the automatic router;
- MUST resolve through the same routing table (so it needs no configuration when
  the class has a built-in default);
- MUST NOT bypass any safety, validation, review, quality, or human-approval gate:
  it selects a model only.

## Failure Semantics

JEV analysis and JEV infrastructure failure are distinct:

- A provider failure (timeout, provider unavailable, malformed or invalid
  evidence, transport failure) MUST NOT be interpreted as a finding.
- When no usable JEV evidence is available, the router MUST fall back to its safe
  default (medium) and MUST record that the evidence was unavailable.
- Routing MUST NOT fail the lifecycle. A failure to build the agent for the
  selected class (for example an unknown provider) MUST be surfaced as an
  actionable error rather than a silent substitution of a different model.

## Persistence and Observability

Each run MUST persist the routing decision as non-secret diagnostic evidence:

- which class was selected,
- the source (policy or manual override),
- the deterministic reasons,
- the resolved provider/model/locality,
- which early checkpoints informed it, and
- a typed signal summary (risk, complexity, scope, cross-cutting, requires-context,
  confidence, and the structural counts).

The decision is written as `routing.json` beside the run's other artifacts, and is
surfaced by `sop report`. Persisting it performs no state transition and MUST NOT
create a second source of truth for task state. It MUST carry no credential (API
keys, tokens, authorization headers, provider credentials); such secrets MAY be
supplied through the environment or `.env` and MUST remain separate from persisted
routing evidence.

## Boundary

Routing MUST remain SOP-owned and deterministic. The router MUST NOT:

- transition task state, or instruct the scheduler to advance a task;
- approve, reject, block, or bypass validation, review, quality, or human approval;
- execute commands, modify the repository, commit, push, or merge;
- call a provider or choose a concrete model name;
- be moved into the provider, the agent harness, or the model itself.

A model MUST NOT control routing. `sop-controller` MAY display routing data but
MUST NOT own routing policy or invoke the router independently.

Routing MUST be disableable: with `SOP_MODEL_ROUTING_ENABLED` unset, the resolved
model selection MUST follow the non-router precedence above, so an existing
Phase 2.5 installation is unchanged.

## Implemented Behavior

The configuration table, the automatic router, the decision rules above, manual
overrides, persistence (`routing.json`), and report visibility are **implemented**.

## Proposed / Future Behavior

The following are **not implemented** and are recorded here as future work only:

- Provider/model availability validation before execution (confirming the
  configured model exists on the provider). Today an unavailable model fails when
  the agent is constructed or invoked; SOP does not pre-validate availability.
- Automatic escalation or downgrade between classes within a task, and
  benchmark- or cost-driven routing. Phase 3.5 is deterministic selection only.
