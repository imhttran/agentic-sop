# Evaluation and verification

Optional reference for `/sop-rag-integrate`. Read it when a plan must prove that the
proposed retrieval actually helps. The requirement is a **small, reproducible
evaluation on genuine data**, not a demonstration that the pipeline runs.

## Evidence standard

- Use the project's real documents and a frozen set of real questions — no synthetic
  corpus chosen to flatter the result.
- Freeze the query set and the expected relevant sources **before** seeing results.
- Run the same questions through the application's **existing search** and through the
  new retrieval, and compare them on the same corpus.
- Record raw results and the exact commands so the comparison is reproducible.
- Do not count a skipped integration test as a pass, and do not treat "the service
  responded" as a quality result.

## Metrics

Report, per configuration, at minimum:

| Area | Measure |
| --- | --- |
| Retrieval quality | Recall@k and precision@k, and MRR or nDCG, against the frozen labels |
| Citation correctness | Fraction of returned evidence whose source identity is right and resolvable |
| Latency | p50 and p95 for a retrieval call, versus the existing search |
| Cost | approximate tokens or currency per query, and any provider bill |
| Context overhead | bytes of retrieved evidence admitted to the prompt, and truncation events |
| Failures | retrieval failures and fallbacks, with a bounded reason code each |

Report regressions and inconclusive results too: "no measurable difference" is a valid
and useful finding, and it is the honest default when a small corpus gives lexical
search no headroom to fail.

## Isolation and safety

- **Corpus / tenant isolation:** prove that a query scoped to one tenant or corpus never
  returns another's documents. Include a negative test that an unauthorized scope is
  rejected rather than silently widened.
- **Citation integrity:** every included evidence item must carry a stable source
  identity, and citations in the answer must resolve to it.
- **Injection boundary:** retrieved text is untrusted data; it must never become a system
  or policy instruction, and it must never alter an approval or completion decision.
- **Budgets:** enforce a byte or token ceiling on admitted evidence, and define the
  behavior when it is exceeded (truncate deterministically, or omit).

## Fallback and rollback

- An optional retrieval failure must degrade to the existing search with an explicit
  reason, never to a silent empty answer.
- Required evidence that is missing must block rather than proceed without it.
- State how the feature is turned off: the change ships disabled by default, behind a
  flag or configuration, so it can be reverted without a data migration.
- Re-indexing and rollback must be safe to repeat: additive, idempotent, and never
  destructive by default.

## Determinism

The same frozen inputs and the same retrieval response must produce identical selected
context and ordering. Record one determinism check alongside the quality numbers.
