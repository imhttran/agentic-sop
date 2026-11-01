# Evaluation

## Evaluate

`cmd/eval` scores retrieval against `evals/retrieval.json`: a list of questions,
each with the `source`/`section` documents that should be retrieved. Ingest the
example corpora first, then run it:

```bash
go run ./cmd/ingest examples/loan-policy.md
go run ./cmd/ingest examples/large-loan-policy.md
go run ./cmd/ingest examples/member-services-guide.md
go run ./cmd/ingest examples/commercial-servicing-manual.md
go run ./cmd/eval
```

For each case it reports recall and precision at K = 1, 2, `TOP_K` for plain
vector search and for hybrid retrieval (vector and keyword results fused with
RRF). When a reranker is enabled it runs the reranking experiment — retrieve
`TOP_K` candidates, rerank with `internal/reranking`, keep the top `FINAL_K` —
reporting the same metrics, followed by averages across all cases.

It also runs the production pipeline from `internal/retrieval`, the same one
`cmd/rag` uses, and reports evidence recall before and after section expansion so
the effect of expanding a section into all of its chunks is visible. Add an
`evidence` array of substrings to a case in `evals/retrieval.json` to opt in.

The reranker is lexical: it scores a candidate by how many distinct question
words appear in its section and content, and `retrieval.DeduplicateSections`
drops all but the top-ranked chunk per source/section before reranking. It is a
teaching baseline, not a semantic reranker.

Both rerankers are off by default. Set `EVAL_LEXICAL_RERANK=true` for the lexical
one and/or `EVAL_LLM_RERANK=true` for the chat model (`RerankLLM` in the same
package), which asks the model to order the candidates by semantic relevance.
They use the same candidate set, so their results are reported side by side.

An optional answerability gate (`EVAL_ANSWERABILITY_GATE=true`) asks the chat
model whether the expanded evidence can answer each question — the same
documents `cmd/rag` sends to its answerability gate and the model — and reports
a confusion matrix (correct accepts/rejects, false rejects/accepts).

An optional fact judge (`EVAL_FACT_JUDGE=true`) asks the chat model which of a
case's `expected_facts` the expanded evidence supports, and reports the total
supported across cases. Add an `expected_facts` array to a case in
`evals/retrieval.json` to opt in.

Query rewriting (`QUERY_REWRITE`, on by default) rewrites each question into a
search query with the chat model (`internal/rag`), then reports multi-query
retrieval — a vector and a full-text search for both the original and the
rewritten query, four rankings fused with RRF. This is what `cmd/rag` does.

Setting `EVAL_REWRITE_ONLY=true` retrieves over the rewritten query alone
instead of fusing it with the original — the experiment behind keeping both.
Averaged over the four-document example corpus, hybrid retrieval scores:

| Mode                 | Recall@1 | Precision@1 | Recall@4 | Precision@4 |
| -------------------- | -------- | ----------- | -------- | ----------- |
| original only        | 0.68     | 0.81        | 0.94     | 0.55        |
| rewritten only       | 0.69     | 0.78        | 0.91     | 0.61        |
| original + rewritten | 0.73     | 0.86        | 0.93     | 0.52        |

Replacing the original with its rewrite holds K=1 recall but loses recall at
K=4 (0.94 → 0.91); keeping both is the best at K=1 (0.73) and still competitive
at K=4. The rewrite comes from the chat model, so its wording — and these
averages — vary a little from run to run.

### Comparing settings

`scripts/sweep.sh` (or `make sweep`) runs `cmd/eval` once per configuration and
prints the Overall averages as one row per configuration, so a change is judged
by the numbers instead of by a couple of answers:

```bash
make sweep              # every axis
make sweep AXIS=chunk   # or topk | finalk | rewrite | rerank | minsim | judge
```

Each row shows hybrid retrieval's per-K recall and precision plus whichever
optional metrics that configuration produced. The `chunk` axis re-ingests
examples/*.md before each step (cmd/ingest replaces a file's chunks, so
repeating is safe). `QUERY_REWRITE` is on by default, so every axis calls the
chat model; run `QUERY_REWRITE=false make sweep AXIS=…` to sweep without it, and
every axis still needs the database up and the corpora ingested. Recorded
results live in `docs/reference/experiments.md`.

For a controlled **embedding-model** comparison, `scripts/eval-model-sweep.sh`
sweeps `MIN_SIMILARITY` per model on a **calibration split** and reports the
disjoint **held-out split**, one isolated database per model, three repeats per
model (see `docs/operations/embedding-models.md`). Its eval runs default to
`QUERY_REWRITE=false`, so the comparison is deterministic and needs no chat
model; recorded results live in `docs/experiments-eval-sweep.md`.
