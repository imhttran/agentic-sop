# Documentation

This is the index for everything under `docs/`. The root
[`README.md`](../README.md) is the project landing page; start there for what the
project is and how to run it.

## How the docs are organized

| Directory                                          | Owns                                                                   | Nature                    |
| -------------------------------------------------- | ---------------------------------------------------------------------- | ------------------------- |
| [`requirements/`](requirements/PRD.md)             | What we are building and why                                           | Normative (scope)         |
| [`architecture/`](architecture/OVERVIEW.md)        | How the system is built (pipeline, packages, boundaries)               | Descriptive (overview)    |
| [`reference/`](reference/commands.md)              | Commands, configuration, capabilities, structure, testing, experiments | Descriptive (facts)       |
| [`guides/`](guides/usage.md)                       | Task-oriented how-tos                                                  | Descriptive (procedures)  |
| [`operations/`](operations/embedding-dimension.md) | Operator-run procedures and contracts                                  | Normative (procedures)    |
| [`plans/`](plans/PLAN.md)                          | How work is sequenced                                                  | Plans (active + recorded) |

[`lessons.md`](lessons.md) sits at the root of `docs/` as a non-normative
retrospective of the reasoning behind the architecture.

The authority chain for a change is:

```text
requirements/  ->  architecture/  ->  operations/ (contracts)  ->  plans/  ->  execution
```

A requirement constrains the architecture, which the operator contracts
implement, which a plan delivers.

## Documents

| Document                                                               | Read it when                                                                        |
| ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| [requirements/PRD.md](requirements/PRD.md)                             | You need the canonical product requirements, goals, and non-goals.                  |
| [requirements/PRD-Phase-23b.md](requirements/PRD-Phase-23b.md)         | You need the requirements for the proposed PDF/OCR/multilingual phase.              |
| [architecture/OVERVIEW.md](architecture/OVERVIEW.md)                   | You want the pipeline, package responsibilities, and boundaries.                    |
| [reference/commands.md](reference/commands.md)                         | You want the command and `make`-target list.                                        |
| [reference/configuration.md](reference/configuration.md)               | You need to know what an environment setting does or its default.                   |
| [reference/capabilities.md](reference/capabilities.md)                 | You want the shipped gap-closure capabilities and where they live.                  |
| [reference/project-structure.md](reference/project-structure.md)       | You want the package and directory layout.                                          |
| [reference/testing.md](reference/testing.md)                           | You want to run or extend the unit and integration tests.                           |
| [reference/experiments.md](reference/experiments.md)                   | You want the recorded sweep results and the decisions they drove.                   |
| [guides/usage.md](guides/usage.md)                                     | You want to set up, ingest a document, or ask a question.                           |
| [guides/evaluation.md](guides/evaluation.md)                           | You want to score retrieval or compare settings/models.                             |
| [guides/agent-scripts.md](guides/agent-scripts.md)                     | You are working with the git hooks or the `.agents/scripts/` helpers.               |
| [operations/embedding-dimension.md](operations/embedding-dimension.md) | You are changing `EMBED_DIM` (the operator-run procedure).                          |
| [operations/embedding-models.md](operations/embedding-models.md)       | You are comparing embedding models on the evaluation harness.                       |
| [operations/page-provenance.md](operations/page-provenance.md)         | You need the page-provenance contract.                                              |
| [operations/prompt-injection.md](operations/prompt-injection.md)       | You need the prompt-injection risk model and mitigations.                           |
| [operations/defect-recovery.md](operations/defect-recovery.md)         | You are recovering from a suspected `agentic-sop` defect during a RAG task.         |
| [plans/PLAN.md](plans/PLAN.md)                                         | You want the engineering and learning plan.                                         |
| [plans/PLAN-RAG-Gap-Closure.md](plans/PLAN-RAG-Gap-Closure.md)         | You want the gap-closure governance and audit record.                               |
| [plans/PLAN-RAG-Phase-23b.md](plans/PLAN-RAG-Phase-23b.md)             | You want the phase-23b execution plan; task plans are `plans/PLAN-RAG-*.md`.        |
| [lessons.md](lessons.md)                                               | You want the lessons-learned retrospective (non-normative).                         |
| [experiments-eval-sweep.md](experiments-eval-sweep.md)                 | You want the recorded embedding-model sweep (`scripts/eval-model-sweep.sh` output). |

## Where a new document belongs

- A new requirement or product decision → `requirements/`.
- A new description of how the system is built → `architecture/`.
- A new command, setting, capability, or interface fact → `reference/`.
- A new how-to for a user task → `guides/`.
- A new operator-run procedure or contract → `operations/`.
- A new implementation plan → `plans/`.
- A retrospective of completed work → [`lessons.md`](lessons.md) (or a new `history/`
  directory once several accumulate).

## Paths kept in place (operational exceptions)

These paths are deliberately **not** reorganized, because code or scripts depend
on the exact location:

- [`operations/embedding-dimension.md`](operations/embedding-dimension.md) — the
  path is hardcoded in `internal/ingestion/schema.go` (`migrationFile`) and named
  by `ingestion.CheckEmbeddingDim`'s error message, so moving it would change
  behavior.
- [`experiments-eval-sweep.md`](experiments-eval-sweep.md) — it is the default
  `SWEEP_RESULTS` output path in `scripts/eval-model-sweep.sh`.
- [`operations/prompt-injection.md`](operations/prompt-injection.md) — named by a
  code comment in `internal/rag/answer.go` (comment only; not behavior).
