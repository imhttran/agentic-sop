# Technology selection — choose for the project

Optional reference for `/sop-rag-integrate`. Read it when a suitability decision has
chosen retrieval augmentation and a stack must be proposed. Everything here is an
example, not a mandate; the project's own dependencies come first.

## Principles

1. **Reuse what the project already depends on.** An existing database, an existing
   search service, and existing application libraries beat a new runtime.
2. **Prefer native ecosystem integration.** A first-class library for the project's
   language and framework is easier to operate and review than a general framework
   bolted on.
3. **Verify support before recommending.** Check that the library is maintained, that
   it supports the project's language/framework version, that its license is acceptable,
   and that it can be pinned to a released version. Do not recommend a version you have
   not confirmed exists.
4. **Provider-neutral.** The embedding model, the generation model, and the store are
   configuration, not code. Prefer an abstraction the application already has (or a thin
   seam of your own) so a provider can be swapped without a rewrite.
5. **Minimal operational complexity.** Fewer moving parts wins: a vector extension in
   the existing database beats a new clustered service for a small corpus. A hosted
   retrieval service is acceptable when the project already runs one.
6. **Small enough to review.** Prefer a scoped change over a platform. Do not build a
   framework, an orchestration engine, or a general agent system.

## Options by ecosystem (examples)

| Ecosystem | Retrieval / orchestration | Storage options | Notes |
| --- | --- | --- | --- |
| Java / Spring Boot | Spring AI, LangChain4j, or the application's existing clients | PostgreSQL + pgvector, or a JVM-native index, or a managed search service | Prefer a starter the project can already build; keep the store in the existing database when possible. |
| Python | LlamaIndex, Haystack, or LangChain, or the application's existing stack | PostgreSQL + pgvector, a local index, or a managed vector service | Widest library choice; resist adding two overlapping frameworks at once. |
| Go | An established Go retrieval library, or a thin client to a **retrieval service** | PostgreSQL + pgvector is the common low-ops choice | Fewer batteries included; prefer explicit HTTP calls over a large dependency. |
| JavaScript / TypeScript | The application's current AI stack, or an established retrieval library | pgvector, a serverless vector service, or an existing search engine | Match the existing runtime; avoid mixing ESM/CJS stacks needlessly. |

A **retrieval service** (a small internal HTTP service that owns ingestion and search)
is a valid choice when several applications share one corpus, or when the embedding
runtime should not live inside the application process.

## Documenting the choice

For each component, record: the option chosen, the alternatives considered, the reason,
the version to pin, and the operational owner. "We used X" without the alternatives is
not a selection.

## What not to select

- A brand-new framework when the project already has an adequate library.
- A second vector store when the existing database has a vector extension.
- A vendor whose SDK the team cannot operate, or a provider whose data-handling rules
  conflict with the security model discovered earlier.
- `rag-template` as a drop-in: it is a reference for a retrieval-only contract and
  pgvector schema habits, not a component to copy into another project.
- Any model or provider chosen for the operator: that is SOP's to resolve.
