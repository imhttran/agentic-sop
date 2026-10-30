# PLAN — Locality Configuration Cleanup

**Status:** PROPOSED
**Rule:** Locality is resolved, not repeated. A user configures the provider and model; SOP resolves locality from explicit configuration or reliable evidence, keeps it internal for routing, fallback, and reporting, and fails with actionable guidance when the evidence is insufficient. Locality inference never authorizes a new cloud fallback.

## Project

Locality Configuration Cleanup

## Summary

Today a routed class is usable only when the operator supplies `models.<class>.locality` (or `SOP_MODEL_<CLASS>_LOCALITY`) in addition to provider and model. The built-in defaults set locality, but a partially configured class does not inherit a default locality, so a user who names a model must repeat locality even when it is obvious from reliable evidence. The current code also contradicts its own specification: the generic OpenAI-compatible provider claims every discovered model is local, even though such an endpoint may be remote.

This plan centralizes locality resolution for primary and fallback models, infers only from reliable evidence, records where the resolved locality came from, and keeps every existing fallback and cloud-safety rule intact.

The behavior contract:

- An explicit locality always wins, and an explicit value that contradicts reliable evidence fails clearly.
- Ollama models with the explicit `:cloud` suffix are cloud. This is the only model-name inference permitted.
- Authoritative provider/runtime metadata may resolve locality when available. Ollama's `/api/tags` and `/api/show` report `remote_host` for cloud models; a listed model without an upstream host is a local model.
- A localhost endpoint, the mere absence of `:cloud`, and arbitrary model names (including Claude names served through custom providers) are never locality evidence.
- When locality remains unknown, resolution fails and names the affected class/model and the exact setting to set.
- Inference never creates a fallback candidate, never changes the selected class, and never bypasses `allow_cloud_fallback_for_local`.

## Objective

Users configure the provider and model without repeating locality when SOP can determine it reliably, while locality remains available internally for routing, fallback rules, and reporting.

## LOC-001 — Central locality resolution and reliable inference

Priority: P1. Add one pure resolver that every primary and fallback locality decision uses, with typed provenance and clear conflict/unknown errors.

### Dependencies

None

### Deliverables

- `internal/model/locality.go` (new) — locality provenance, inference, evidence, resolution, and unknown/conflict errors.
- `internal/model/locality_test.go` (new) — focused table tests for every resolution branch.
- `internal/model/model.go` — route primary/fallback locality through the resolver; add the resolved source to `Selection`.
- `internal/model/execution.go` — carry the resolved locality source onto `ExecutionTarget`.
- `internal/model/model_test.go`, `internal/model/execution_test.go` — update and extend the existing tests.

### Acceptance Criteria

- `model.LocalitySource` is a closed typed set with `explicit`, `inferred`, `runtime`, and `default`; `Selection.LocalitySource` and `ExecutionTarget.LocalitySource` carry it, and both JSON-encode it as `locality_source` with an omitted-when-empty guarantee for older evidence.
- `model.InferLocality(provider, name)` returns `cloud` only for the `ollama` provider and a model name with the explicit `:cloud` suffix. It returns not-inferred for every other provider and for every name without that suffix, including names that merely contain `cloud`, Claude names, and arbitrary custom-provider models.
- `model.ResolveLocality` resolves in this order: explicit configuration, authoritative runtime evidence, reliable naming inference, built-in default. An explicit value that equals the evidence keeps source `explicit`; an explicit value that contradicts non-empty runtime evidence or `:cloud` inference returns an error naming the class, provider, model, and exact setting. Runtime evidence and inference that disagree also fail closed.
- A runtime evidence value is accepted only from an authoritative source supplied by the caller; the model package performs no network I/O and never consults an endpoint or the process environment beyond the existing injected lookup.
- `model.Inputs` accepts locality evidence keyed by provider and model. `model.LocalityUnknownError` lists every unresolved reference with its class, role (`primary` or `fallback`), provider, model, and the exact settings to set: `models.<class>.locality` / `SOP_MODEL_<CLASS>_LOCALITY` for a primary, and `models.<class>.fallback.locality` / `SOP_MODEL_<CLASS>_FALLBACK_LOCALITY` for a fallback.
- Explicit locality precedence remains environment over YAML. An explicit YAML value is not replaced by a default, and an environment value overrides the YAML value.
- The built-in default locality applies only when neither the configuration nor the environment mentions the class at all. A class that either layer partially configures never receives a default locality that masquerades as operator configuration; when inference and runtime evidence are unavailable, resolution fails with the unknown-locality error instead.
- A class's fallback uses the same resolver. It inherits the primary provider. The built-in fallback keeps its built-in cloud locality with source `default`; a fallback explicitly configured by a layer without a locality does not silently receive the cloud default and must resolve from reliable evidence or an explicit value.
- Locality inference or a resolved locality never creates a fallback candidate, never changes the selected class, and never bypasses the existing `local` primary to `cloud` fallback guard. The `allow_cloud_fallback_for_local` behavior is unchanged.
- Existing behavior for a fully specified class (provider, name, and explicit locality) is byte-compatible in resolution outcome and source.

### Acceptance Checks

- `go test ./internal/model/...`
- `go vet ./internal/model/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1
- criterion 7: check 1
- criterion 8: check 1
- criterion 9: check 1
- criterion 10: check 1

### Failure Behavior

If a configuration cannot be resolved to a locality, resolution fails closed with the affected reference and exact setting rather than guessing, defaulting, or executing a model whose locality is unknown. If evidence contradicts an explicit value, resolution fails rather than silently preferring either side.

## LOC-002 — Authoritative runtime locality metadata

Priority: P1. Make provider/runtime locality evidence honest and usable: Ollama reports what it actually knows, and the generic OpenAI-compatible provider stops guessing.

### Dependencies

- LOC-001

### Deliverables

- `internal/provider/provider.go` — document that the ModelInfo.Locality field is authoritative runtime evidence and that an empty value means undetermined.
- `internal/provider/ollama/provider.go` — read `remote_host` from `/api/tags` and `/api/show`; set cloud or local only from that authoritative data.
- `internal/provider/openai/provider.go` — stop claiming `local` for discovered or configured models; report locality undetermined.
- `internal/provider/locality.go` (new) — one read-only lookup that returns a provider's reported locality for a named model, or not-determined.
- `internal/provider/ollama/provider_test.go`, `internal/provider/openai/provider_test.go`, `internal/provider/locality_test.go` — update and add tests.

### Acceptance Criteria

- For an Ollama `/api/tags` entry with a non-empty `remote_host`, `ModelInfo.Locality` is `cloud`; for an entry with no `remote_host`, it is `local`. An absent model, an unsupported discovery, or a failed request is not-determined rather than an assumption.
- Ollama `/api/show` uses the same `remote_host` evidence when a caller asks for a model's locality, and a missing/empty field remains undetermined rather than being read as cloud or local from the model name.
- The generic OpenAI-compatible provider never reports `local` or `cloud` from the endpoint host or the model name; both local and remote endpoints are legal and both report undetermined.
- The locality lookup returns only a non-empty provider-reported value. It returns not-determined for an unknown provider, an unregistered provider, unsupported discovery, a failed request, or an absent model; it never substitutes a guess.
- The endpoint host (localhost or remote), the absence of `:cloud`, and model names such as Claude are never used as locality evidence in the provider layer.
- Locality discovery remains read-only: it issues no generation request, changes no SOP state, and does not print endpoints or credentials.
- `sop providers --models` shows a locality line only when the runtime reports one; it no longer prints a fabricated `locality: local` for OpenAI-compatible servers.

### Acceptance Checks

- `go test ./internal/provider/...`
- `go vet ./internal/provider/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1
- criterion 7: check 1

### Failure Behavior

A provider that cannot report locality returns not-determined. It never falls back to an endpoint heuristic or a model-name heuristic, so a missing observation can only lead to an actionable unknown-locality error, not to a wrong cloud/local decision.

## LOC-003 — Resolve the effective target and wire locality into every execution seam

Priority: P1. Resolve the effective provider, endpoint, and model before locality is inferred; use runtime evidence only when explicit/inferred locality is inconclusive; record the resolved locality and its source everywhere routing evidence is already recorded.

### Dependencies

- LOC-001
- LOC-002

### Deliverables

- `internal/cli/locality.go` (new) — the single CLI resolution helper and the bounded runtime-evidence loop.
- `internal/cli/locality_test.go` (new) — tests for inference, runtime evidence, explicit bypass, unknown guidance, env precedence, primary/fallback parity, and fallback restrictions.
- `internal/cli/modelroute.go`, `internal/cli/routing.go`, `internal/cli/escalation.go`, `internal/cli/prompt.go`, `internal/cli/cli.go` — route every `model.Resolve` call through the helper.
- `internal/cli/stack.go` — resolve and expose the effective endpoint alongside provider and model; use it for locality observation, never as locality evidence.
- `internal/cli/run.go`, `internal/cli/report.go`, `internal/cli/trace.go` — show and record the resolved locality source.
- `internal/run/model_selection.go`, `internal/run/routing_artifact.go`, `internal/run/attempt.go` — persist `locality_source` as optional evidence.
- `internal/runtrace/trace.go`, `internal/runtrace/orchestration.go`, `internal/runtrace/orchestration_compose.go` — carry the source on execution evidence.
- Corresponding `_test.go` files.

### Acceptance Criteria

- One CLI helper performs locality resolution for the run-level/default selection, per-task routing, prompt routing, and escalation. No call site calls `model.Resolve` directly.
- The helper first resolves with explicit configuration and reliable naming inference. It performs a runtime observation only when resolution reports unknown locality, and only for the affected provider/model references. An explicit or `:cloud` inference never triggers a probe.
- The observation uses the effective execution target's provider, endpoint, and model, resolved with the documented precedence (environment over configuration over built-in default, with the agent path's existing defaults for the provider). The endpoint is used only to choose which runtime to ask. It is never used as locality evidence.
- Runtime locality evidence is supplied to `model.Resolve` by provider/model key. The helper is bounded: it retries at most until every reported reference has been observed once, and if no new evidence appears it returns the original actionable unknown-locality error.
- A run-level configuration with an Ollama `:cloud` model and no explicit locality resolves and executes without a runtime probe. A local Ollama model with no explicit locality resolves from a positive runtime observation. An ambiguous custom-provider model with no explicit locality fails with guidance naming the class, model, and exact setting.
- An explicit locality that conflicts with reliable evidence fails at resolution, before any agent work, with the same actionable detail as LOC-001.
- Environment locality overrides YAML locality at every seam, including primary and fallback, and the recorded source remains `explicit`.
- Model-class selection, escalation, and the `local` primary to `cloud` fallback guards are unchanged. Inference and runtime evidence never create a fallback candidate and never authorize a cloud fallback.
- The startup/status summary shows the resolved locality and its source. `model-selection.json`, `routing.json`, attempt records, the report JSON/Markdown, and the run trace record `locality_source` when known; older artifacts without the field remain readable and round-trip unchanged.
- No endpoint or credential is printed or persisted in routing evidence.

### Acceptance Checks

- `go test ./internal/cli/... ./internal/run/... ./internal/runtrace/...`
- `go vet ./internal/cli/... ./internal/run/... ./internal/runtrace/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1
- criterion 7: check 1
- criterion 8: check 1
- criterion 9: check 1
- criterion 10: check 1

### Failure Behavior

If runtime observation is unavailable, the helper returns the actionable unknown-locality error and does not execute the task. It never substitutes a provider, model, locality, or class, and it never turns an observation failure into a cloud fallback.

## LOC-004 — Documentation, examples, and migration

Priority: P2. Make the new contract discoverable and remove redundant locality from examples where resolution is reliable.

### Dependencies

- LOC-001
- LOC-002
- LOC-003

### Deliverables

- `docs/specs/MODEL-ROUTING.md` — replace "Locality is configuration, never inferred" with the resolution contract, evidence rules, conflict behavior, and fallback defaults.
- `docs/reference/CONFIGURATION.md` — mark locality optional where resolvable, explain when explicit locality is required, distinguish endpoint location from model location, and add migration guidance.
- `docs/specs/PROVIDERS.md` — document Ollama `remote_host` evidence and the generic provider's undetermined locality.
- `.env.example` — remove redundant locality values for reliably inferred `:cloud` models, keep an annotated explicit local value where runtime evidence is the only reliable source, and explain the fallback.
- `internal/config/config.go` — update the generated configuration template comments to match.

### Acceptance Criteria

- The model-routing specification states the resolution order (explicit, authoritative runtime evidence, reliable naming inference, built-in default), the exact Ollama `:cloud` rule, the runtime `remote_host` evidence, and the prohibition on localhost-endpoint, absence-of-`:cloud`, and arbitrary-name inference.
- The specification and configuration reference state that endpoint location is not model location: a localhost endpoint can serve a cloud model and a remote endpoint can serve a local model.
- The configuration reference marks `models.<class>.locality` and `models.<class>.fallback.locality` as optional when reliable evidence exists, explains that a partially configured class no longer receives a default locality that could be mistaken for operator configuration, and names the exact settings in the unknown-locality guidance.
- The examples remove redundant `locality: cloud` values for Ollama `:cloud` models, keep or add explicit `locality: local` where only an operator can know, and retain a complete example showing explicit values still work.
- A migration section explains that existing explicit configurations need no change, that cloud Ollama models may drop their redundant locality, that ambiguous models (custom OpenAI-compatible endpoints, Claude names, local models not installed in the runtime) must set explicit locality once, and that a contradictory explicit value now fails instead of being silently ignored.
- Documentation links remain valid.

### Acceptance Checks

- `bash scripts/checks/check-doc-links.sh`
- `go test ./internal/config/...`
- `go vet ./internal/config/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1

### Failure Behavior

If documentation and code disagree, the code and its focused tests are authoritative and the document is corrected in the same change. No example is changed to omit locality unless the implementation and tests prove the omission resolves.
