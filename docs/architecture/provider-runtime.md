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
untouched: the provider layer sits *underneath* Phase 3.5 routing and never
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
- **Validation seam:** `internal/cli/providers.go` — `validateSelectedModel` runs
  in `internal/cli/run.go` (`runSingleTask`) and `internal/cli/drive.go`
  (`runGraph`) immediately after the up-front capability guard, before the
  lifecycle starts. It is a strict no-op unless `providers.validate` is on.

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

The selection validated is the routing layer's selection when routing is active,
otherwise the effective execution stack's provider and model
(`selectionForValidation`). It is never rewritten.

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
