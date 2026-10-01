# PLAN — Phase 4: Provider Runtime Abstraction & Capability Discovery

**Status:** Implemented. The provider/runtime layer is shipped, off by default.
**PRD:** [../requirements/PRD-Phase-4-Provider-Runtime.md](../requirements/PRD-Phase-4-Provider-Runtime.md)
**Spec:** [../specs/PROVIDERS.md](../specs/PROVIDERS.md)
**Seam:** [../architecture/provider-runtime.md](../architecture/provider-runtime.md)

## Objective

Add a provider/runtime abstraction underneath SOP's existing model-class routing
without changing Phase 3.5 routing. Providers report capabilities and
availability; they MUST NOT choose the model class or control lifecycle state.

## Rollout order

```text
P4-001  provider types + registry
P4-002  health / model metadata / capability contracts
P4-003  ollama adapter
P4-004  llama.cpp + command adapters
P4-005  MLX / oMLX adapter
P4-006  selection availability validation (opt-in)
P4-007  CLI provider inspection
P4-008  integration + boundary tests
P4-009  documentation + full repository validation
```

Each step is additive and independently reversible; the new surface defaults OFF.

## Tasks

### P4-001 — Provider types and registry

- **Status:** Done.
- **Scope:** `provider.ID` and `ParseID`; `provider.Provider`; `provider.Registry`
  (register/get/list, deterministic, no global state); sentinel errors.
- **Files:** `internal/provider/provider.go`, `registry.go`, `errors.go`,
  `provider_test.go`, `registry_test.go`.
- **Depends on:** —
- **Acceptance:** unknown/duplicate ids fail; listing is deterministic; a
  reflection test freezes the interface method set.

### P4-002 — Health, model metadata, capability contracts

- **Status:** Done.
- **Scope:** `HealthStatus`/`HealthResult`; `ModelInfo`; tri-state `Capabilities`.
- **Files:** `internal/provider/health.go`, `capability.go`,
  `internal/provider/capability_test.go`.
- **Depends on:** P4-001.
- **Acceptance:** health never errors; unknown capability is distinct from absent;
  `Capabilities.String` omits unknown fields.

### P4-003 — Ollama adapter

- **Status:** Done.
- **Scope:** `internal/provider/ollama` — `/api/version` health, `/api/tags`
  discovery, `/api/show` capabilities; locality never guessed.
- **Files:** `internal/provider/ollama/provider.go`, `provider_test.go`.
- **Depends on:** P4-001, P4-002.
- **Acceptance:** healthy/degraded/unavailable; models parsed; capabilities
  unknown when unreported; discovery-unsupported on error.

### P4-004 — llama.cpp and command adapters

- **Status:** Done.
- **Scope:** shared OpenAI-compatible adapter (`internal/provider/openai`) behind
  `internal/provider/llamacpp`; `internal/provider/command` reports honest limits
  (unknown health, no discovery, no capabilities).
- **Files:** `internal/provider/openai/provider.go`, `openai/provider_test.go`,
  `llamacpp/provider.go`, `llamacpp/provider_test.go`, `command/provider.go`,
  `command/provider_test.go`, `httpx/httpx.go`.
- **Depends on:** P4-002.
- **Acceptance:** `/v1/models` discovery, locality local, chat+streaming known;
  command cannot enumerate and does not fabricate.

### P4-005 — MLX / oMLX adapter

- **Status:** Done.
- **Scope:** `internal/provider/mlx`, an OpenAI-compatible boundary so SOP is not
  coupled to a specific MLX server.
- **Files:** `internal/provider/mlx/provider.go`, `mlx/provider_test.go`.
- **Depends on:** P4-004 (shared adapter).
- **Acceptance:** `mlx` provider id, `/v1/models` discovery, honest capabilities.

### P4-006 — Selection availability validation

- **Status:** Done.
- **Scope:** `provider.ValidateSelection`; opt-in pre-execution hook in `sop run`.
- **Files:** `internal/provider/validate.go`, `internal/provider/validate_test.go`,
  `internal/provider/boundary_test.go`, `internal/config/config.go`
  (`providers.validate`), `internal/cli/providers.go` (`validateSelectedModel`),
  `internal/cli/run.go`, `internal/cli/drive.go`.
- **Depends on:** P4-003, P4-004, P4-005.
- **Acceptance:** valid selection passes; unknown provider / unavailable provider /
  absent model fail; discovery-unsupported and unknown capability do not fail;
  validation never mutates or substitutes; OFF by default.

### P4-007 — CLI provider inspection

- **Status:** Done.
- **Scope:** `sop providers [--models]`, read-only.
- **Files:** `internal/cli/providers.go`, `internal/cli/providers_test.go`,
  `internal/cli/cli.go` (dispatch + help).
- **Depends on:** P4-003, P4-004, P4-005.
- **Acceptance:** prints id/health/model count; `--models` lists models with
  locality and capabilities; creates no state; prints no secrets.

### P4-008 — Integration and boundary tests

- **Status:** Done.
- **Scope:** registry/validation/adapters tables; the four boundary invariants
  (interface method set; health cannot select a class; failure cannot substitute;
  validation does not mutate).
- **Files:** the `*_test.go` files listed above plus
  `internal/provider/boundary_test.go`.
- **Depends on:** P4-006, P4-007.
- **Acceptance:** all tests run offline; table-driven cases pass.

### P4-009 — Documentation and full repository validation

- **Status:** Done.
- **Scope:** normative spec, seam doc, PRD, plan, configuration and CLI references,
  `.env.example`, docs index.
- **Files:** `docs/specs/PROVIDERS.md`, `docs/architecture/provider-runtime.md`,
  `docs/requirements/PRD-Phase-4-Provider-Runtime.md`,
  `docs/plans/PLAN-Phase-4-Provider-Runtime.md`, `docs/reference/CONFIGURATION.md`,
  `docs/reference/CLI.md`, `docs/README.md`, `.env.example`.
- **Depends on:** P4-008.
- **Acceptance:** links resolve; references are accurate.

## Validation

```bash
make check
go test ./internal/provider/...
go test -race ./internal/provider/... ./internal/model/... ./internal/cli/... ./internal/router/... ./internal/jev/...
scripts/check-doc-links.sh
```

## Definition of Done

Met when: provider/runtime is first-class; Phase 3.5 routing is unchanged; providers
do not control routing or lifecycle; Ollama, llama.cpp, MLX, and the command
provider sit behind the boundary; SOP can inspect health, models, and capabilities;
the selected model can be validated before execution (opt-in, read-only); absence
is reported only when discovery is authoritative; failure never causes silent
substitution; no credential is persisted or printed; all provider tests run
offline; existing Phase 3/3.5 tests remain green; with the layer off or
unconfigured, existing behavior is unchanged.

## Hardening (post-DoD)

All read-only and still OFF by default:

- `ModelInfo` gained optional metadata (`Family`, `ParameterSize`, `Quantization`,
  `ContextWindow`, `SizeBytes`), populated from Ollama's `/api/tags` and rendered by
  `ModelInfo.Metadata` in `sop providers --models`.
- The llama.cpp provider reports its configured model as a known identity when the
  server cannot enumerate models (never as a discovery result).
- `internal/provider/httpx` gained direct unit tests.

### Provider runtime / execution alignment

- **MLX execution.** `mlx` is a first-class execution provider: `internal/agent`
  gained `NewMLX`/`NewMLXFromEnv` and the `SOP_MLX_*` variables, accepted by
  `agent.provider` and the tool harness (`harness: tool, provider: mlx`).
- **Shared transport.** The llama.cpp agent was generalized to
  `agent.OpenAICompatible`; `LlamaCpp` and `MLX` are its two identities, so both
  share the `POST /v1/chat/completions` plumbing instead of duplicating it. The
  transport holds no routing/lifecycle logic.
- **Identity unification.** The agent provider constants derive from
  `provider.KnownIDs`; the config allow-list is built from it; the model-routing
  allow-list is kept equal by a test. Provider identity can no longer drift across
  the provider/config/model/agent layers.
- **Final-selection validation.** Provider validation now runs on the **final**
  per-task selection. With the automatic router on, `applyTaskRouting` validates the
  routed selection immediately before implementation; the run-level default check is
  skipped so a class the router never picks cannot fail the run. With the router off,
  the run-level/default selection is validated up front, as before.
- **Boundary tests.** The provider boundary test also freezes the `Registry` method
  set and rejects authority-verb methods on the exported provider types.
- **Tests.** Offline `httptest` coverage for MLX discover/validate/generate, shared
  transport, identity sync, and per-task validation (positive, absent-model,
  validation-off, routing-off).

## Out of scope

Automatic provider fallback, cheapest/latency/cost routing, benchmarks, learned
routing, automatic class escalation, distributed workers, remote SOP Hub, MCP
orchestration, load balancing, provider scoring.
