# Provider runtime abstraction (Phase 4)

> **This page is non-normative and subordinate.** The authoritative home for the
> provider/runtime rules — identity, the registry, health, discovery,
> capabilities, validation, and failure behavior — is
> [`specs/PROVIDERS.md`](../specs/PROVIDERS.md). This page MUST NOT restate or
> override those rules; where it and the specification disagree, the
> specification wins. It records only the implementation seam, which is not
> normative.

The provider layer is **additive and off by default**: building the registry
starts no execution, and the only behavior it can change — pre-execution model
validation — is opt-in via `providers.validate: true`. Model-class selection is
untouched: the provider layer sits _underneath_ Phase 3.5 routing and never
chooses a class.

## The seam

- **Packages:** `internal/provider` (domain types, registry, validation),
  `internal/provider/{ollama,llamacpp,mlx,command}` (adapters),
  `internal/provider/openai` (shared OpenAI-compatible adapter),
  `internal/provider/httpx` (bounded GET/POST helpers).
- **Composition root:** `internal/cli` — `newProviderRegistry(cfg)` builds the
  registry from `cfg.Providers` and the environment (`endpointOr` applies
  environment-over-configuration-over-default). It mirrors how the agent layer is
  assembled.
- **Inspection:** `internal/cli/providers.go` — `runProviders` (the
  `sop providers` command) is read-only: it never creates SOP state.
- **Validation seam:** `internal/cli/providers.go` — `validateSelection` is the
  single opt-in check. When the automatic per-task router is OFF, it runs on the
  run-level/default selection from `internal/cli/run.go` (`runSingleTask`) and
  `internal/cli/drive.go` (`runGraph`) immediately after the up-front capability
  guard. When the router is ON, each task's final routed selection is validated in
  `internal/cli/routing.go` (`applyTaskRouting`) immediately before implementation,
  so the selection that actually executes is the one validated. Either way it is a
  strict no-op unless `providers.validate` is on.
- **Execution transport:** `internal/agent` — `OpenAICompatible` is the shared
  `POST /v1/chat/completions` transport behind both the `llamacpp` and `mlx`
  identities; identity and endpoint are configuration, not protocol. It performs
  model inference only and holds no routing, lifecycle, or approval logic.

## Data flow

```text
cfg.Providers + env endpoints
        │
        ▼
provider.Registry  ── Register(ollama|llamacpp|mlx|command)
        │
        ▼
sop providers                 (read-only inspection)
        │
        ▼
provider.ValidateSelection    (opt-in, read-only)
        │
        ├── provider exists / registered
        ├── reachable        (when health is determinable)
        ├── model present    (when discovery is authoritative)
        └── can chat         (when the capability is determined)
        │
        ▼
ok  →  run continues          error →  run stops with an actionable message
```

The selection validated is the **final** selection for what will run: with the
automatic router off, the effective execution stack's provider and model
(`selectionForValidation`); with it on, the task's routed selection
(`applyTaskRouting`). It is never rewritten.

## Execution support

The provider layer is inspection only. Actual agent execution lives in
`internal/agent`, which now covers every provider:

```text
internal/agent
  CommandAgent          command (subprocess)
  Ollama                ollama (/api/chat)
  OpenAICompatible      llamacpp + mlx + openai_compatible (POST /v1/chat/completions)
```

`LlamaCpp`, `MLX`, and `openai_compatible` are three identities of the one
`OpenAICompatible` transport, so they share the execution plumbing rather than
duplicating it; identity, endpoint, and configuration differ. `openai_compatible`
is the generic identity for any OpenAI-compatible server (oMLX, vLLM, LM Studio,
LocalAI, ...), while `mlx` and `llamacpp` keep their own identities and default
endpoints. The generic provider owns its endpoint with the precedence
`environment (SOP_OPENAI_COMPATIBLE_BASE_URL) > configuration
(providers.openai_compatible.endpoint) > default (http://127.0.0.1:8000)`; the
composition root adds the configuration tier (the agent layer cannot import
`internal/config`). Text-only providers declare no `IMPLEMENT`/`FIX`; the tool
harness remains the layer that turns a provider into a coding agent.

## Boundaries kept explicit

- `internal/provider` does **not** import `internal/agent` or
  `internal/ollamaagent`; the dependency runs the other way (harness → provider
  vocabulary).
- No provider method executes work, transitions state, or selects a class; the
  method set is frozen by `internal/provider/boundary_test.go`.
- Health, discovery, and capability results are evidence only; validation is the
  single place that acts on them, and only when opted in.
- No credential is configured here, and none is printed by `sop providers`.

## Related

- [`specs/PROVIDERS.md`](../specs/PROVIDERS.md) — the normative rules.
- [`architecture/model-routing.md`](model-routing.md) — the Phase 3.5 routing seam.
- [`reference/CONFIGURATION.md`](../reference/CONFIGURATION.md) — the `providers:` block.
- [`reference/CLI.md`](../reference/CLI.md) — `sop providers`.
