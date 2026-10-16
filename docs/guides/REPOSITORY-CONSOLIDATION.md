# Repository consolidation

This guide records how the SOP controller and the decision adapters were
consolidated into the single `agentic-sop` module, the compatibility limits that
follow from that, and how the **shipped RAG skills** relate to the
**proposed/implemented RAG retrieval adapter** in the SOP core.

## What moved where

| Source repository | Destination in `agentic-sop` |
| --- | --- |
| controller `internal/config`, `internal/sopclient` | `internal/controller/{config,sopclient}/` |
| controller `internal/web`, `assets.go` + `templates/` + `static/` | `internal/controller/web/`, `internal/controller/assets/` |
| controller startup wiring | `internal/controller/service.go` (`controller.Serve`) |
| controller browser (Playwright) suite | `tests/controller/` (development-only) |
| controller launch/HTTPS scripts, Caddyfile | `scripts/controller/`, `integrations/controller/` |
| adapter public `decision/` | `decision/` (module root) |
| adapter `internal/providers/{clef,nimble,julia,fallback}`, `internal/shadow` | `internal/decisionadapter/providers/…`, `internal/decisionadapter/shadow/` |
| adapter CLI | `internal/decisionadapter/cli/` (`cli.Run`) |
| adapter fixtures | `internal/tests/fixtures/` |
| Julia helper | `tools/julia/` (retained) |
| legacy executables | `cmd/sop-controller/`, `cmd/sop-decision-adapter/` (thin wrappers) |

Imports were copied byte-exact and then rewritten by the documented, idempotent
mapping in `docs/reports/consolidation/import-rewrites.md`. Core package locations,
the canonical decision seam and the profile bridge are unchanged.

## Unified commands

- `sop run` — the governed lifecycle.
- `sop controller serve` — the read-only dashboard.
- `sop decision decide|shadow` — the advisory decision adapter.

The legacy executables remain available and behave identically.

## Shipped RAG skills vs. the RAG retrieval adapter

These are **two different things** and must not be conflated:

| | Shipped RAG **skills** | RAG **retrieval adapter** (SOP core) |
| --- | --- | --- |
| What | Domain skills that fix an existing read-only capability (`plan`, `design_tests`) | An optional retrieval path wired into the SOP lifecycle alongside BM25 |
| Where | `skills/rag-schema`, `skills/rag-test`, `skills/rag-ingest` (+ mirrored agent trees) | `internal/ragclient`, `internal/cli/retrieve.go`, `internal/rag*`, `internal/retrieval` (BM25 baseline) |
| Status | **Shipped** — installed by `./install.sh --skills …` | **Proposed/implemented, disabled by default** — opt-in only |
| Mutates? | No; produce plans/test designs, never a verdict | Can alter retrieved context when explicitly enabled |
| Gate | SOP routing, provider validation, lifecycle, approval | Deterministic quality gate, required-evidence gate, retrieval gate |
| Enablement | Always available with the installer | `SOP_RAG_ENABLED` — default off; explicit operator opt-in only |

- The **shipped RAG skills** are documented in [`RAG-SKILLS.md`](RAG-SKILLS.md); they
  are provider-independent and never name an embedding runtime or connection string.
- The **retrieval adapter** is evaluated by RAG-SOP-004; its rollout decision is recorded
  in [`../specs/RAG-SOP-004-DECISION.md`](../specs/RAG-SOP-004-DECISION.md) and its
  predeclared criteria in [`../specs/RAG-EVALUATION.md`](../specs/RAG-EVALUATION.md).
- The required **PostgreSQL integration validation category** that guards
  database-sensitive changes is a deterministic file-pattern policy, documented in
  [`../specs/RAG-POSTGRES-VALIDATION-POLICY.md`](../specs/RAG-POSTGRES-VALIDATION-POLICY.md);
  it is kept strictly separate from retrieval quality.

**Default enablement is unchanged by consolidation or by RAG-SOP-004.** Nothing here
enables the retrieval adapter automatically; expansion of usage is an explicit operator
opt-in decision.

## Compatibility limits and optional requirements

- **Go module**: one module, `go 1.27.1`; the controller's `modernc.org/sqlite`
  dependency is unified.
- **Node / Playwright**: development-only for `tests/controller/`; never required to
  build or run SOP.
- **Python / Julia**: the Julia decision provider is an optional capability — the
  repository-owned helper `tools/julia/infer.py` plus external model weights. Without
  them, Julia decisions report unavailable.
- **Caddy / HTTPS**: optional; the proxy scripts and Caddyfile are namespaced under
  `scripts/controller/` and `integrations/controller/` and never overwrite a core
  Make target.
- **Ollama / PostgreSQL**: optional runtimes. The RAG evaluation explicitly tests their
  **unavailability** as an expected scenario (degrade to BM25-only or fail closed).
- **Advisory authority**: decisions can only add attention; they cannot lower risk,
  approve work or bypass verification.

## History

Full source histories are preserved outside tracked artifacts (bundles + provenance),
and the ancestry-preserving integration is prepared but **not committed** — its merge
commit needs separate authorization.
