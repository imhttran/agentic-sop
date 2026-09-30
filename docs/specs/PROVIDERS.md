# Providers

**Type:** Normative specification

## Purpose

This is the normative specification for SOP's **provider/runtime layer**: the
abstraction that reports which model server is configured, whether it is
reachable, which models it serves, and what those models can do (Phase 4). It
covers provider identity, the registry, health, model discovery, capabilities,
selection validation, failure behavior, configuration, and the read-only
inspection surface.

It does **not** define model-class selection. That remains
[`MODEL-ROUTING.md`](MODEL-ROUTING.md): Phase 3.5 owns the `small`/`medium`/`large`
choice. A provider only inspects the runtime that serves an already-selected
model.

## Related Specifications

- [`MODEL-ROUTING.md`](MODEL-ROUTING.md) — the model-class layer (`small`/`medium`/`large`) and its precedence; **authoritative for model selection**.
- [`AGENT-PROVIDER.md`](AGENT-PROVIDER.md) — the harness/provider/model boundary and how a provider becomes a coding agent.
- [`EXECUTION.md`](EXECUTION.md) — `sop run` and where opt-in validation runs.
- [`SECURITY.md`](SECURITY.md) — secrets, command policy, and state ownership.
- [`../architecture/provider-runtime.md`](../architecture/provider-runtime.md) — the non-normative implementation seam.
- [`../reference/CONFIGURATION.md`](../reference/CONFIGURATION.md), [`../reference/CLI.md`](../reference/CLI.md) — the `providers:` block and `sop providers`.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

## Implemented / Required / Proposed

This document distinguishes three kinds of statement:

- **Implemented** — behavior present in the repository today.
- **Required** — a normative rule the implementation MUST satisfy (including where it already does).
- **Proposed / future** — not implemented; see §12.

---

## 1. The Four Concepts

SOP MUST keep four concepts independent, and MUST NOT collapse them:

| Concept                | Meaning                                                           | Owner                                            |
| ---------------------- | ----------------------------------------------------------------- | ------------------------------------------------ |
| **Model class**        | `small` / `medium` / `large` — a routing decision.                | SOP router (`internal/router`, `internal/model`) |
| **Model**              | A concrete model id within a provider (`qwen3:4b`).               | Provider configuration                           |
| **Provider / runtime** | The server that serves a model (Ollama, llama.cpp, MLX, command). | `internal/provider`                              |
| **Harness**            | How SOP interacts with an agent (tool loop, command subprocess).  | `internal/agent`, `internal/ollamaagent`         |

A provider serves inference. A harness defines how SOP talks to an agent. The two
MUST NOT be merged into one abstraction.

## 2. Provider Identity

**Required.** Provider identity MUST be a closed, validated identifier. SOP
defines exactly four:

```text
ollama     a local or cloud Ollama server
llamacpp   an OpenAI-compatible llama.cpp llama-server
mlx        an Apple-Silicon MLX / oMLX runtime (OpenAI-compatible boundary)
command    an external subprocess that is already a full agent
```

Provider names MUST be parsed through `provider.ParseID`; code MUST NOT scatter
raw string comparisons. An unknown name MUST fail with an error naming the known
providers. An empty name is rejected (callers that treat "unset" specially MUST
check for it first).

## 3. Registry

**Required.** Providers MUST be held in an explicit `provider.Registry` that is
constructed and passed (the composition root builds it), **not** a package-level
mutable variable.

- `Register` MUST reject a duplicate id, a nil provider, and an unknown id.
- `Get` MUST fail clearly for an unknown or unregistered id.
- `List`/`IDs` MUST be deterministic: sorted by provider id, independent of
  registration order.

**Required.** A registry is read-only after construction. It MUST NOT select a
model class, execute a task, or mutate SOP state.

## 4. Provider Interface

**Required.** The provider interface is intentionally narrow:

```go
type Provider interface {
    ID() ID
    Health(ctx context.Context) HealthResult
    Models(ctx context.Context) ([]ModelInfo, error)
    Capabilities(ctx context.Context, model string) (Capabilities, error)
}
```

Every method is a read-only observation. **Required.** The interface MUST NOT
grow a method that executes work, transitions task state, commits, pushes,
merges, approves, or selects a class. The boundary is enforced by a test that
freezes the method set (`internal/provider/boundary_test.go`).

**Required.** The harness layer, not the provider, owns agent execution.

## 5. Health Semantics

**Implemented.** Health is one of `healthy`, `unavailable`, `degraded`,
`unknown`.

- `Health` MUST NOT return an error; an unreachable runtime reports
  `unavailable`.
- `unknown` is a valid, honest answer (for example the command provider exposes
  no endpoint to probe).
- **Required.** Health is informational. It MUST NOT reroute a model class, and
  it MUST NOT by itself change task state, approval, validation, review, or
  quality gates. Only the opt-in validation of §7 acts on a definite
  `unavailable`.

## 6. Model Discovery

**Implemented.** `Models` returns `ModelInfo{Name, Provider, Locality,
Capabilities}` plus optional metadata (`Family`, `ParameterSize`, `Quantization`,
`ContextWindow`, `SizeBytes`).

- **Required.** A provider that cannot enumerate models MUST return
  `ErrDiscoveryUnsupported`, not an empty list, so a caller never mistakes
  "unknown" for "absent".
- **Implemented.** A single-model server (llama.cpp) MAY report its
  operator-configured model as a known identity when it cannot enumerate models.
  It MUST be surfaced as the configured identity, never as a discovery result, and
  MUST NOT override a server that does answer; with no configured model the failure
  stays `ErrDiscoveryUnsupported`.
- **Required.** `Locality` MUST be populated only when the runtime determines it
  reliably. Ollama's model list does not report locality, so Ollama leaves it
  empty; the authoritative locality for a run is the routing selection's, never a
  provider's guess.
- **Implemented.** Optional metadata is populated only when the runtime reports it
  reliably (Ollama fills it from `/api/tags`; the OpenAI-compatible adapters leave
  it unset). A zero value means "not determined", never "absent"; `ModelInfo.Metadata`
  renders it deterministically for the inspection surface.
- **Required.** Capabilities MUST NOT be inferred from model names by brittle
  string matching.

## 7. Capability Semantics

**Implemented.** Capabilities are a typed, tri-state set (`chat`, `tools`,
`streaming`, `images`, `embeddings`, `reasoning`, `structured_output`), where
each field is `yes`, `no`, or `unknown`.

- **Required.** A capability a provider cannot determine MUST be `unknown`, never
  fabricated. An honest `unknown` is better than a speculative `yes`.
- **Required.** Capability data is **evidence only**. It MUST NOT independently
  change task state, approval, validation, review, quality gates, or the model
  class.

## 8. Selection Validation

**Implemented.** `provider.ValidateSelection(ctx, registry, selection)` checks
that a resolved selection can plausibly run, before SOP spends agent work:

```text
provider exists (known id + registered)
provider is reachable, when health can be determined
selected model exists, when the provider can authoritatively enumerate models
the model can chat, when the provider determined the chat capability
```

**Required** failure semantics:

- an unknown provider MUST fail clearly;
- a definitely-absent model MUST fail clearly (`ErrModelNotFound`);
- a provider that cannot enumerate models MUST NOT be reported as "absent";
- an `unknown` capability MUST NOT be treated as false;
- validation MUST NOT substitute a provider or model — it returns a verdict and
  the caller decides.

**Required.** Validation is a pure, read-only observation: it MUST NOT mutate the
selection it is given or any SOP state.

## 9. When Validation Runs

**Implemented.** Validation is **opt-in** and OFF by default:

- `sop providers` inspects runtimes on demand (read-only).
- `providers.validate: true` makes `sop run` check the resolved selection before
  it starts a task. With the flag off (the default), a run's behavior is
  unchanged.

**Required.** Enabling validation MUST NOT enable automatic fallback (§12): an
unavailable provider or an absent model stops the run with an actionable error,
and SOP never silently runs the task on a different provider or model.

## 10. Failure Behavior

**Required.** The provider layer distinguishes:

```text
a determined result (healthy/unavailable/degraded; present/absent; yes/no)
from
an undetermined result (unknown health; discovery unsupported; unknown capability)
```

Undetermined results MUST NOT be treated as negative ones. The command provider,
in particular, reports `unknown` health, cannot enumerate models, and declares no
capabilities; none of that is evidence that a model is missing.

## 11. Provider vs Harness

**Required.** A provider serves inference; a harness defines how SOP interacts
with an agent. Selecting a provider MUST NOT imply a harness, and vice versa.
`internal/provider` MUST NOT import `internal/agent` or `internal/ollamaagent`
(layering: the harness depends on the provider vocabulary, not the reverse).

## 12. Configuration

**Implemented.** The optional `providers:` block configures endpoints and the
opt-in validation flag:

```yaml
providers:
  validate: false
  ollama:
    endpoint: http://127.0.0.1:11434
  llamacpp:
    endpoint: http://127.0.0.1:8080
  mlx:
    endpoint: http://127.0.0.1:8000
```

**Required.**

- An omitted block MUST leave existing behavior unchanged.
- Endpoints MUST resolve **environment over configuration over built-in
  default**, reusing the existing `SOP_OLLAMA_BASE_URL` / `SOP_LLAMACPP_BASE_URL`
  and adding `SOP_MLX_BASE_URL` (one name per setting).
- Unknown keys MUST continue to fail clearly (`config.Parse` uses
  `KnownFields(true)`).
- Credentials MUST NOT be configured here and MUST NOT be persisted or printed
  by any provider surface. They stay in the environment.

## 13. Inspection Surface

**Implemented.** `sop providers [--models]` prints each provider's id, health, and
model count; `--models` also lists discovered models with locality and
capabilities, where determined.

**Required.** Inspection MUST be read-only: it MUST NOT create or mutate SOP
state and MUST NOT print secrets or endpoints.

## 14. Proposed / Future

The following are explicitly **not implemented**. They require SOP-owned
deterministic policy and are deferred:

```text
automatic provider fallback (mlx unavailable -> ollama -> cloud)
dynamic cheapest-provider selection
latency- or cost-based routing
benchmark-based model selection
learned routing or class escalation
provider load balancing or scoring
distributed workers / remote SOP Hub / MCP orchestration
```

**Required (invariant that already holds).** Provider failure MUST NOT cause
silent substitution. Any future fallback MUST be deterministic and SOP-owned, and
MUST NOT move routing authority into the provider.

## 15. Non-Goals

This specification does not:

- let a provider choose the model class;
- let a provider execute a task, mutate the repository, commit, push, or merge;
- let a provider approve, reject, or bypass validation, review, or human approval;
- replace SOP's scheduler, validation, review, quality gate, autonomy policy, or
  human approval;
- define `sop-controller` behavior — `sop-controller` is a consumer of SOP state
  and MUST NOT invoke the provider layer to create competing lifecycle state.

## 16. Validation Checklist

- [x] Provider identity is validated; unknown names fail.
- [x] The registry rejects duplicates and lists deterministically.
- [x] Health, discovery, and capabilities are read-only evidence.
- [x] Undetermined results are never treated as negative.
- [x] Validation is opt-in, read-only, and never substitutes a provider/model.
- [x] No credential is configured, persisted, or printed by the provider layer.
- [x] Existing runs are unchanged when the layer is off or unconfigured.
