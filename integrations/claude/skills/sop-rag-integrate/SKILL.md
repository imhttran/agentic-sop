---
name: sop-rag-integrate
description: |-
  Evaluate, plan, implement, and verify a retrieval-augmented (RAG) capability in an
  existing application — or recommend NO-RAG when conventional SQL, full-text search,
  or an existing search service already answers the questions. Use for: "should this
  project use RAG", "add retrieval to my app", "integrate RAG", "which vector store
  fits this service", "is semantic search worth it here". Language-agnostic at the
  workflow level, framework-aware during implementation, provider-neutral, and
  governed by SOP. This skill fixes the read-only `plan` capability; it never edits
  the target repository or its database itself, and it never reproduces SOP policy.
disable-model-invocation: false
---

# /sop-rag-integrate — right-size RAG integration in an existing project

Canonical skill name: `sop-rag-integrate`. Installed command: `/sop-rag-integrate`.

Capability: `plan` (read-only). This is a RAG-domain adapter: it fixes the read-only
`plan` capability, adds a RAG-integration methodology (discovery, suitability,
technology selection, a scoped plan, and verification requirements), and delegates the
governed work of applying, verifying, and publishing a change to SOP's existing
commands and skills. It never applies a change itself.

```text
/sop-rag-integrate
      |
      v
 PREFLIGHT -> DISCOVER -> SUITABILITY (RAG | NO-RAG) -> TECHNOLOGY ->
 SCOPED PLAN -> /sop-implement or `sop run` -> VERIFY -> /sop-publish
```

This is **not** a RAG framework and it ships no retrieval engine. It produces a
right-sized plan for one existing application and hands the governed steps to SOP.

## The one command

```bash
sop prompt --capability plan "<the operator's RAG-integration request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Read-only: this capability does not modify the repository and cannot. Applying a
  change is a separate, governed step (see "Implement through SOP").
- SOP picks the model class and the provider. Do not choose either.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see the
  project README). Never answer the request yourself and never mutate the repository
  in SOP's place.

The expanded checklists live in the optional references beside this file:
[`references/discovery-and-suitability.md`](references/discovery-and-suitability.md),
[`references/technology-selection.md`](references/technology-selection.md), and
[`references/evaluation.md`](references/evaluation.md). Read only the ones the request
needs; a simple application never needs all three.

## 1. DISCOVER before proposing anything

Before recommending any retrieval component, establish what the target application
already is. Reuse SOP's own discovery and planning capabilities instead of re-deriving
architecture by hand, and ask the operator for the facts you cannot read from the
repository:

- language, framework, and runtime; database and schema; how documents arrive and change;
- the **existing search**: conventional SQL, full-text/`LIKE`, a search service, or nothing;
- the security model: authentication, authorization, tenants, and data classification;
- the deployment environment, existing dependencies, and operational appetite.

Record what you found and where. A recommendation that does not cite the project's
**current** search implementation is not a suitability assessment.

## 2. SUITABILITY — allow an explicit NO-RAG

Decide whether retrieval augmentation is *necessary*, not whether it is possible.
Consider each cheaper option first, in this order:

1. conventional SQL over the existing tables;
2. full-text / lexical search the database already supports;
3. the search service the application already runs;
4. **only then** vector or hybrid retrieval.

Recommend **NO-RAG** — and stop — when the existing search already answers the
questions in scope, the corpus is small and stable, or the operational cost is not
justified. NO-RAG is a first-class outcome, not a failure. The rubric and the NO-RAG
triggers are in
[`references/discovery-and-suitability.md`](references/discovery-and-suitability.md).

## 3. TECHNOLOGY — choose for the project, not for the SOP repo

Prefer mature libraries and native ecosystem integration; prefer what the project
already depends on; verify current support and compatibility before recommending a
version. Do not hardcode one provider or framework, and do not select a stack merely
because the SOP repository uses it. `rag-template` is a reference, not a requirement:
do not copy SOP's internal RAG infrastructure into the target project. Per-ecosystem
options and the selection principles are in
[`references/technology-selection.md`](references/technology-selection.md).

## 4. PLAN — only the components the application needs

Produce a scoped SOP plan covering only what this application needs, drawn from:
document ingestion and updates; chunking and embeddings; index/storage selection;
retrieval and optional reranking; source citations; authorization and tenant
isolation; context budgets and fallback; configuration and observability; evaluation
and rollback. Mark each component included or explicitly out of scope with a reason. A
minimal application may need only ingestion, an index, and retrieval with citations.

## 5. IMPLEMENT through SOP

The plan is applied by SOP's existing governed workflows, never by this skill:

```bash
sop run docs/plans/<your-rag-integration-plan>.md
```

- Prefer the project's existing validation commands; a small, low-risk change can use
  SOP's lightweight execution profile while keeping targeted tests and essential safety
  checks.
- Consequential architectural, database, security, or deployment changes require the
  operator's approval — SOP raises that boundary; do not resolve it here.
- Do not select a model, a provider, a database, or a connection string.

## 6. VERIFY with real, reproducible evidence

Require a small, reproducible evaluation using genuine data and the selected retrieval
implementation, and compare it against the application's existing search when one
exists. Measure quality, latency, cost, and security outcomes that matter here. Do not
claim that retrieval augmentation improves results without supporting evidence; "it
works" is not evidence, and a skipped integration test is not a pass. The metric set,
isolation checks, and rollback expectations are in
[`references/evaluation.md`](references/evaluation.md).

## 7. PUBLISH through SOP

Complete an authorized merge and publish with `/sop-publish`. Never infer completion
from a merge; SOP owns the lifecycle.

## Relationship to the SOP RAG proof of concept

The SOP RAG integration was a proof of concept. Its operator decision was **NO-GO for
enabling RAG by default**, because the limited evaluation showed no measurable
improvement over keyword retrieval (see `docs/specs/RAG-SOP-004-DECISION.md`). This
skill preserves that decision: it treats retrieval augmentation as one option among
several, requires evidence before recommending it, and never enables RAG by default
anywhere. The retrieval-only contract, the pgvector schema guidance (`/rag-schema`),
the ingestion guidance (`/rag-ingest`), and the test guidance (`/rag-test`) remain
available as optional, composable building blocks.

## What this skill must not do

- Do not invent a new RAG framework, vector database, or orchestration engine; reuse
  established libraries and SOP's existing capabilities.
- Do not hardcode a single provider, framework, model, or database.
- Do not force RAG when the existing search is sufficient; a NO-RAG recommendation is
  valid and complete.
- Do not select `rag-template`, or copy SOP's internal RAG infrastructure into the
  target repository.
- Do not edit the target repository or its database to satisfy a request; hand the
  change to `/sop-implement` (or `sop run`) and the database change to the operator.
- Do not approve, decline, accept a changed task, stage, commit, push, or merge on the
  operator's behalf, and never rewrite history.
- Do not choose a provider, a model class, or a model; SOP resolves those.
- Do not restate SOP policy or reimplement its routing, approval, or lifecycle.

## Failure behavior

`sop prompt` exits non-zero and reports an actionable error when the capability is
unsupported by the selected provider, the model cannot be validated, or the lifecycle
does not pass. Surface that error as-is; do not work around it. When reporting
BLOCK/BLOCKED, FAIL, or an execution error, link the latest relevant `sop-run.log` for
the invocation's project, as the canonical `/sop` skill describes. Never present a
different run's error as current.
