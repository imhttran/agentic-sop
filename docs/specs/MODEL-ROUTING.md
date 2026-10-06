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

| Class    | Provider | Model                       | Locality | Fallback (local-first)      |
| -------- | -------- | --------------------------- | -------- | --------------------------- |
| `small`  | `ollama` | `qwen3:4b`                  | `local`  | `nemotron-3-nano:30b-cloud` |
| `medium` | `ollama` | `nemotron-3-super:cloud`    | `cloud`  | —                           |
| `large`  | `ollama` | `deepseek-v4.1-flash:cloud` | `cloud`  | —                           |

The SMALL fallback runs on the same `ollama` provider as its primary; it is a
cloud-hosted Ollama model, not a second provider or a separate cloud API.

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

## Local-First Fallback

A class MAY declare a `fallback` model, so that a LOCAL class is local-first
without being local-only. SMALL's built-in fallback is `nemotron-3-nano:30b-cloud`.

When the selected class is `local` and has a complete fallback configured, SOP
MUST run the fallback instead of the class's primary model if and only if a
read-only availability observation reports that the local runtime cannot serve the
primary:

```text
SMALL (local)
   |
   +-- local runtime serves the model -------> local model
   |
   +-- local runtime cannot serve the model -> configured fallback (cloud)
```

"Cannot serve" means, and only means:

1. the configured local model is not present in the runtime's own model list; or
2. the runtime determines the model cannot chat (a known required capability is
   unavailable).

The observation MUST be read-only and MUST NOT spend a generation request. A
provider that cannot determine reachability or enumerate models MUST NOT be read as
an outage, so the fallback triggers only on a positive observation. When no
observation is available, the local model MUST be kept.

### The Ollama runtime itself being unreachable

The availability fallback MUST NOT be applied when the local runtime (Ollama at
`http://127.0.0.1:11434`) is unreachable. A cloud-hosted Ollama model still uses
the same Ollama execution path, so changing the model name to
`nemotron-3-nano:30b-cloud` cannot repair an unreachable Ollama endpoint. This case
MUST follow the existing provider/runtime-unavailable error path. SOP MUST
distinguish:

```text
Ollama runtime unreachable            -> provider/runtime-unavailable error
Ollama runtime healthy, model absent   -> SMALL cloud availability fallback
```

No second NVIDIA or Ollama-cloud HTTP integration is introduced.

### Availability fallback vs. quality escalation

The fallback MUST NOT be triggered by a generation failure, a malformed or invalid
model response, a validation failure, a review finding, a quality-gate failure, an
application-level error, or a timeout. Those failures MUST continue through the
existing retry, review, escalation, and error handling: the runtime fallback is an
environment-availability switch, never a failure-recovery policy. Two distinct
concepts are involved and MUST NOT be combined:

```text
availability fallback     SMALL/local -> SMALL/cloud   (pre-generation, availability)
quality escalation        SMALL -> MEDIUM -> LARGE     (post-failure, Phase 5 recovery)
```

The availability fallback happens BEFORE normal generation begins. Quality
escalation is failure-driven execution recovery owned by [RECOVERY.md](RECOVERY.md)
§8, and Phase 7 strategy replanning (which keeps the class and changes only the
strategy) is owned by [RECOVERY.md](RECOVERY.md) §9. A SMALL primary that passes
availability and then fails generation MUST NOT silently become the SMALL cloud
fallback; it follows the existing retry/recovery/escalation behavior instead.

The fallback MUST keep the selected CLASS — only the concrete provider and model
change. Applying it MUST record the deterministic source `cloud-fallback` and the
reason `local runtime unavailable; cloud fallback`, so the switch is auditable. A
class whose locality is not `local` MUST NOT have a fallback candidate.

### Locality is configuration, never inferred

A model's locality MUST come from configuration. Locality MUST NOT be inferred
from a model name (a `:cloud` suffix is a naming convention, not a fact about where
a model runs) and MUST NOT be inferred from the runtime's `/api/tags` model list,
which does not establish whether a model is local or cloud. `qwen3:4b` is local and
`nemotron-3-nano:30b-cloud` is cloud solely because configuration says so.

### Fallback configuration

A fallback field an operator does not set inherits: the provider from the class's
primary model, and the locality `cloud`. The built-in fallback is the base layer of
the merge, so naming a local model does not remove the built-in cloud fallback; the
configuration and environment layers override a fallback field by field. The
environment variables are `SOP_MODEL_<CLASS>_FALLBACK_PROVIDER`,
`SOP_MODEL_<CLASS>_FALLBACK_NAME`, and `SOP_MODEL_<CLASS>_FALLBACK_LOCALITY`. An
explicitly configured fallback overrides the built-in Nano fallback, and an
explicitly configured SMALL primary remains the primary — the fallback is consulted
only when that primary cannot be served.

### Interaction with allow_cloud_fallback_for_local

Two mechanisms both concern a local-to-cloud switch, and they are deliberately
distinct:

- `allow_cloud_fallback_for_local` (`SOP_MODEL_ALLOW_CLOUD_FALLBACK_FOR_LOCAL`)
guards the **config-completeness** fallback (`fallback_class`): a local class that
has no model at all would otherwise silently use a cloud fallback class. It
defaults to `false` and MUST continue to govern only that path.
- The **availability** fallback is authorized by the class's own explicit
`fallback:` configuration (its built-in default for SMALL included). A class that
names a fallback has explicitly opted into the local-to-cloud switch for
availability reasons, so it does not additionally require
`allow_cloud_fallback_for_local`.

The two are not contradictory: one governs "this class has no model, borrow
another class's", and the other governs "this class's model exists but the runtime
cannot serve it". Neither silently weakens the other, and neither is a privacy
boundary that the other bypasses.

## Availability Resolution Boundary

Resolving which model actually executes MUST happen after the class/model routing
decision and before generation begins:

```text
model.Selection
      |
      v
execution-target resolution
      |
      +-- primary usable -------> primary
      |
      +-- primary unavailable --> configured fallback
```

The pure configuration resolver (`model.Resolve`) MUST stay pure: it reads
configuration and environment only, performs no network call, and reports the
fallback as a candidate (`Result.LocalFallback`). The caller MUST apply the
candidate only on a positive availability observation, using the existing provider
abstractions (health, model list, capabilities); it MUST NOT add direct `/api/*`
HTTP calls outside the provider layer and MUST NOT issue a generation request to
probe availability.

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
  it selects a model only;
- MUST pin the class for the task, so the automatic router and the bounded
  escalation policy (Phase 5) do not replace it: an operator's explicit class is
  never silently swapped for another.

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

The persisted evidence MUST keep the routing CLASS distinct from the actual
EXECUTION target. If the router selected `SMALL` and the SMALL availability
fallback executed, the evidence MUST record:

```text
class             = SMALL                    (the routing decision)
model             = nemotron-3-nano:30b-cloud (the actual execution target)
execution_source  = availability-fallback     (typed provenance)
```

The routing decision MUST NOT be rewritten (a SMALL decision MUST NOT become
MEDIUM because a SMALL fallback executed). `execution_source` is a typed value
(`primary` or `availability-fallback`), never model-generated prose.

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
overrides, the local-first fallback for a local class, persistence
(`routing.json`), report visibility, and Phase 4 pre-execution availability
validation are **implemented**. Bounded, deterministic escalation
after a failed attempt (Phase 5) and bounded strategy replanning (Phase 7) are
implemented too; they are owned by [RECOVERY.md](RECOVERY.md) §8/§9, not by this
specification — routing selects the class a task starts on, escalation changes that
class, replanning keeps it, and recovery decides what to do after an attempt fails.

## Proposed / Future Behavior

The following are **not implemented** and are recorded here as future work only:

- Automatic downgrade between classes, and benchmark- or cost-driven routing.
  Selection (Phase 3.5) and failure-driven escalation (Phase 5) are deterministic
  and bounded; neither chooses a class from measured model performance.
- Automatically lowering a class back after a failure, or continuing past the
  `large` class. The ladder is one-way and ends at the existing human boundary.
