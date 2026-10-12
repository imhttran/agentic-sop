# RAG engineering skills

Three SOP skills add a Retrieval-Augmented Generation (RAG) engineering checklist to
the existing read-only capabilities:

| Command        | Capability     | Mutates? | Use for                                                       |
| -------------- | -------------- | -------- | ------------------------------------------------------------- |
| `/rag-schema`  | `plan`         | no       | inspect a pgvector schema and plan a safe dimension change     |
| `/rag-test`    | `design_tests` | no       | design unit, race, integration, end-to-end, and mismatch tests |
| `/rag-ingest`  | `plan`         | no       | design a safe ingestion pipeline (chunking → store)            |

They are **domain skills**, not new capabilities: each fixes an existing read-only
capability and delegates to the canonical `sop` skill, which owns routing, provider
validation, the lifecycle, and approval. They never mutate the repository and never
choose a provider, a model, or a connection string. Applying a change goes through the
governed `/sop-implement` lifecycle; changing a live database is an operator action.

They are provider-independent. Nothing here names an embedding runtime, a model, a
model class, or a connection string; the examples read the project's own configuration.

## Install

The skills live in the canonical [`skills/`](../../skills) tree, so the ordinary
installer exposes them alongside every other SOP command. No extra step is needed.

Zed (global `~/.agents/skills`, or project `<project>/.agents/skills`):

```bash
./install.sh --skills zed
```

Claude Code (global `~/.claude/skills`, or project `<project>/.claude/skills`), and the
Claude Code plugin:

```bash
./install.sh --skills claude
./install.sh --plugin claude
```

See [`INSTALLATION.md`](INSTALLATION.md), [`ZED-SKILLS.md`](ZED-SKILLS.md),
[`CLAUDE-SKILLS.md`](CLAUDE-SKILLS.md), and [`CLAUDE-PLUGIN.md`](CLAUDE-PLUGIN.md). The
plugin mirrors the same tree, so the same three commands appear there (namespaced as
`/sop:rag-schema`, and so on).

## Use

Everything after the command name is prompt data, passed to `sop` as one argument —
never a shell command. SOP picks the provider and the model.

```text
/rag-schema review the pgvector schema and plan moving documents.embedding to the width our new embedding model produces

/rag-test design the verification for the ingestion and retrieval pipeline, including the dimension-mismatch cases

/rag-ingest design an ingestion pipeline that replaces a document atomically and recovers from a failed embedding without a partial write
```

Each run is recorded under the project's `.agent-sdlc/runs/prompts/<run-id>/` and can be
inspected with `sop report prompts/<run-id>`.

## What each skill covers

- **`/rag-schema`** — read the real column type from the catalog, interpret the vector
  typmod (pgvector stores the dimension directly in `atttypmod`; render the type instead
  of deriving an offset), reconcile the column width with the configured embedding
  dimension, design an **operator-controlled** migration (never a destructive automated
  one), isolate PostgreSQL test schemas, and run data-integrity checks. See
  [`../../skills/rag-schema/SKILL.md`](../../skills/rag-schema/SKILL.md).
- **`/rag-test`** — design the four-layer test matrix (unit, race, PostgreSQL
  integration, end-to-end), the dimension-mismatch cases, and the failure/acceptance
  evidence, and discover the project's **required validation gate** so the tests run
  where SOP enforces them. See
  [`../../skills/rag-test/SKILL.md`](../../skills/rag-test/SKILL.md).
- **`/rag-ingest`** — specify parsing and chunking, bounded embedding concurrency,
  retry/backoff classification, transactional replacement, provenance and idempotency,
  and recovery that never leaves a partial write. See
  [`../../skills/rag-ingest/SKILL.md`](../../skills/rag-ingest/SKILL.md).

## Safety boundaries

The skills cannot, by construction:

- **Bypass SOP approval gates** — they only call `sop prompt` with a read-only
  capability; approval and mutation stay with SOP.
- **Silently mark a task complete** — they produce a plan or a test design, never a
  completion verdict; only SOP's deterministic validation establishes a pass.
- **Perform destructive migrations** — a schema change is a plan; the database change is
  an operator action and the repository change goes through `/sop-implement`.
- **Commit or push without authorization** — no skill runs version control; commits are a
  human decision outside SOP's default lifecycle.
- **Modify SOP lifecycle state directly** — they never touch `.agent-sdlc/` state, plan
  files, or the store; SOP owns every transition.

These are reinforced by the shipped validators: a skill under `skills/` may not contain a
direct-mutation instruction, name a provider runtime in command position, or restate a
SOP policy setting (see [`../../internal/skill/surface.go`](../../internal/skill/surface.go)
and [`../../internal/skill/skill_test.go`](../../internal/skill/skill_test.go)).

## Controller changes required to enforce required integration tests

RAG failures that matter — wrong column order, a vector typmod/dimension mismatch, a
sequential scan where an index scan is expected, a non-atomic replace — only appear
against a real PostgreSQL/pgvector server. Unit tests cannot see them. SOP already
enforces that a mutating task has *some* configured validation; it does not yet
distinguish a database-backed **integration** gate from the fast unit gate. These changes
are **documented here, not applied** — they belong to the validation controller and
deserve their own review.

### Current enforcement (verified in-tree)

- [`internal/quality/quality.go`](../../internal/quality/quality.go) — `Evaluate` returns
  `FAIL` when `Input.ValidationRequired && !Input.ValidationConfigured`, so an empty suite
  never passes.
- [`internal/cli/run.go`](../../internal/cli/run.go) — sets `validationRequired` from the
  task's execution mode and observed mutation, and `validationConfigured` from
  `validate.Enabled(cfg.Validation)`.
- [`internal/failure/failure.go`](../../internal/failure/failure.go) — `Classify` returns
  `ValidationNotConfigured` (disposition `Block`).
- [`internal/autonomy/autonomy.go`](../../internal/autonomy/autonomy.go) — maps that kind
  to `ActionTerminal` at every level: an operator-intervention/configuration state, never
  a human approval and never auto-continued.
- [`internal/validate/validate.go`](../../internal/validate/validate.go) — `Enabled` is
  `len(Checks(v)) > 0`; `config.Validation` carries only `build`, `test`, and `lint`.

**Gap.** "Configured validation" means *any* command. A change to a migration or a
pgvector query would pass with only `go build`/`go test` configured, while the
integration tests that would catch the real failure never ran.

### Required changes

1. **Represent integration checks in configuration.** Add an `integration` list to
   `config.Validation` (in [`internal/config/config.go`](../../internal/config/config.go)),
   since `testrunner` already defines the `INTEGRATION_TEST` category
   ([`internal/testrunner/runner.go`](../../internal/testrunner/runner.go)):

   ```go
   type Validation struct {
       Build       []string `yaml:"build"`
       Integration []string `yaml:"integration"`
       Test        []string `yaml:"test"`
       Lint        []string `yaml:"lint"`
   }
   ```

   Order them in `validate.Checks` as build → test → integration → lint, so the cheap
   checks still fail fast before the database-bound ones.

2. **Make the requirement deterministic.** Derive `IntegrationRequired` from the task's
   execution mode and the change set (for example, when the change touches
   `migrations/**`, a `*_test.go` integration file, or a configured glob), never from
   model output. Add `IntegrationRequired`/`IntegrationConfigured` to
   `quality.Input` and mirror the existing missing-validation branch.

3. **Wire the evidence.** In `run.go`, run the integration checks and pass
   `categoryPassed(suite, testrunner.IntegrationTest)`; set `IntegrationConfigured`
   from a new `validate.EnabledIntegration(cfg.Validation)`.

4. **Name the missing category.** Reuse the `Block`/terminal disposition of
   `ValidationNotConfigured`, but include the missing category in the reason (for
   example `validation.integration is not configured`) so the operator knows exactly
   what to add. Do not introduce an approval for it.

5. **Test it** following the existing patterns: extend
   [`internal/quality/validation_gate_test.go`](../../internal/quality/validation_gate_test.go),
   [`internal/failure/validation_config_test.go`](../../internal/failure/validation_config_test.go),
   [`internal/autonomy/validation_config_test.go`](../../internal/autonomy/validation_config_test.go),
   and [`internal/cli/validation_enforcement_test.go`](../../internal/cli/validation_enforcement_test.go)
   with the integration category.

The normative behavior lives in [`../specs/VALIDATION.md`](../specs/VALIDATION.md) and
[`../specs/QUALITY.md`](../specs/QUALITY.md); update those owners alongside the code.

## Validation

The shipped validators and installers were exercised for this change:

- `go test ./...` — every package passes, including `internal/skill` (skill-body
  contract) and `internal/dist` (tree/surface parity, installers, and the generated
  Claude plugin mirror).
- `go build ./...` — clean.
- `./scripts/packaging/build-claude-plugin.sh --check` — the plugin mirror is current.
- `scripts/install/install-skills.sh {zed,claude} --project <dir>` then `--uninstall` —
  all 13 commands install as flat, name-matching skill folders and uninstall
  idempotently.

## Recommended next RAG skills

- `/rag-eval` — design retrieval evaluation: labelled cases, recall/precision at K,
  reranker and rewrite comparisons, and answerability/fact-judge gates.
- `/rag-observability` — define the run trace for retrieval and generation: candidate
  counts, scores, filter reasons, latency, and cost, without leaking prompts or secrets.
- `/rag-migrate` — a dedicated, operator-run migration runbook for pgvector stores:
  backup, backfill, dual-write, verify, and cut over, with rollback.
- `/rag-guardrails` — specify prompt-injection, PII, and citation-grounding guards at the
  retrieval/generation boundary, and how they fail closed.

## See also

- [`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md) — the canonical skill contract
  and capability map.
- [`ZED-SKILLS.md`](ZED-SKILLS.md), [`CLAUDE-SKILLS.md`](CLAUDE-SKILLS.md),
  [`CLAUDE-PLUGIN.md`](CLAUDE-PLUGIN.md) — the per-agent install guides.
- [`../specs/VALIDATION.md`](../specs/VALIDATION.md) — the deterministic validation runner
  and its evidence rules.
