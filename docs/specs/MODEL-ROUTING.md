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

### Locality resolution and evidence

A model's locality MUST be resolved before it runs. Resolution uses this order,
highest first:

1. an explicit `models.<class>.locality` / `SOP_MODEL_<CLASS>_LOCALITY` value;
2. authoritative provider/runtime metadata for that provider and model;
3. reliable naming inference — for the `ollama` provider only, an explicit
   `:cloud` suffix on the model name means `cloud`;
4. the built-in default, and only when no configuration or environment layer
   mentions the class at all.

Every resolved locality carries a typed source — `explicit`, `runtime`,
`inferred`, or `default` — so evidence distinguishes operator configuration from
a built-in default.

A provider may report locality only from authoritative runtime evidence. Ollama's
`/api/tags` report includes a `remote_host` field: a non-empty value means Ollama
Cloud serves the model, while a listed entry without one is served by that local
server. Ollama's `/api/show` exposes the same evidence for one named model; a
missing or empty field there is undetermined, never a local or cloud guess. The
generic OpenAI-compatible adapter reports no locality at all. A provider MUST NOT
derive locality from the endpoint host, from the absence of `:cloud`, or from an
arbitrary model name (including a Claude name served through a custom provider).
Endpoint location is not model location: a localhost endpoint can serve a cloud
model and a remote endpoint can serve a local model.

An explicit value that contradicts reliable evidence MUST fail with an error
naming the class, provider, model, and the exact setting. When neither explicit
configuration nor reliable evidence resolves a locality, resolution MUST fail
with the same actionable reference (`models.<class>.locality` /
`SOP_MODEL_<CLASS>_LOCALITY`, or the `fallback` equivalents). It MUST NOT guess,
and it MUST NOT treat a built-in default as if the operator had written it.

Locality inference and runtime evidence MUST NOT create a fallback candidate,
change the selected class, or authorize a cloud fallback. The
`allow_cloud_fallback_for_local` and per-class availability fallback rules below
remain authoritative.

### Fallback configuration

A fallback field an operator does not set inherits the provider from the class's
primary model. The built-in fallback is the base layer of the merge, so naming a
local primary does not remove the built-in cloud fallback; the configuration and
environment layers override a fallback field by field. The environment variables
are `SOP_MODEL_<CLASS>_FALLBACK_PROVIDER`, `SOP_MODEL_<CLASS>_FALLBACK_NAME`, and
`SOP_MODEL_<CLASS>_FALLBACK_LOCALITY`.

The built-in-only fallback keeps its built-in `cloud` locality with source
`default`. A fallback that a configuration or environment layer configures is a
layered value: an omitted locality MUST resolve through the same explicit /
runtime-evidence / naming-inference order as a primary. It MUST NOT silently
inherit the built-in cloud locality. In particular, naming a non-`:cloud`
fallback model without a locality and without runtime evidence fails with guidance
naming `models.<class>.fallback.locality` and
`SOP_MODEL_<CLASS>_FALLBACK_LOCALITY`.

An explicitly configured fallback overrides the built-in Nano fallback, and an
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
  (`internal/jev` evidence: closed purpose, severity, status, category, a
  bounded confidence, and an optional `LOW`/`MEDIUM`/`HIGH` complexity rating).

The complexity signal is the higher of what the findings imply and the
checkpoint's own rating, so a clean `PASS` can still mark a task as hard: `HIGH`
routes to `large`, and `MEDIUM` keeps a task off `small`. Only the task-triage
and pre-execution checkpoints record a rating; an unstated or unknown rating
routes exactly as before.

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

## Supporting-Stage Classes

Routing and `--model-class` select the model for implementation and fixes. The
supporting stages choose their model separately:

| Stage | Unpinned behavior |
| --- | --- |
| `plan` (task-level plan) | the run-level class (`--model-class`, else the default class). It runs before per-task routing, so it never follows the routed class. |
| `review` | the run-level class |
| `jev` (early and quality JEV) | `SOP_OLLAMA_MODEL`, else the Ollama built-in default model, independent of the class table |

An operator MAY pin a stage to a class with `models.stages.<stage>` or
`SOP_MODEL_STAGE_<STAGE>_CLASS` (the environment wins). A pinned stage:

- MUST resolve through the same routing table as any other class;
- MUST fail at startup, before any model call, when the class is unknown or, for
  `jev`, when the class's provider is not `ollama` (JEV runs only on Ollama);
- MUST NOT change any gate: it selects a model only.

A stage MAY instead be pinned straight to a provider and model
(`SOP_MODEL_STAGE_<STAGE>_PROVIDER` and `_NAME`), outside the class table, so a
model that must never implement is never reachable by routing. Such a pin MUST
name both settings and a known provider, MUST NOT be combined with that stage's
class pin, and for `plan` or `review` MUST fail at startup when `SOP_AGENT_MODEL` or
the provider's own model variable would override it with a different model.

An unpinned stage MUST keep the behavior in the table above, so an existing
project is unchanged. The model each stage ran on MUST be recorded under
`stages` in `model-selection.json` (source `stage`, `run`, `ollama-env`, or
`review-engine`).

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
