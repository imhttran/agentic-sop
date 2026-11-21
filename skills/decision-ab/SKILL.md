---
name: decision-ab
description: |-
  Run the frozen served-model decision A/B benchmark: discover the served model
  ids, send the production structured-decision request to one model at a time,
  score the first response against the frozen six-action corpus, and report
  quality, consequential errors, and latency. Read-only with respect to model
  defaults: it never changes a default, substitutes a model, or decides the
  replacement.
disable-model-invocation: false
---

# /decision-ab — served-model decision A/B benchmark

This is a repository-owned evaluation client, not a second decision engine. It
fixes no SOP capability and changes no model default. The benchmark sends the
production structured-decision request (`POST /v1/systemone`, through the same
request builder and response parser the decision adapter uses) and scores the
first response.

Fail closed: if `sop` is not on PATH, or Go, the endpoint, or the frozen corpus is
unavailable, stop and report that. Do not invent a benchmark, do not answer the
question from general knowledge, and do not substitute another model.

## PREFLIGHT

```bash
sop status
go run ./cmd/deceleval-ab -discover -endpoint <endpoint>
```

- `sop status` proves the SOP CLI is available. If it fails, stop.
- `-discover` lists the served model ids and endpoint metadata without an
  inference request. Record the exact ids the operator named; do not guess an id.
- The frozen inputs are `evals/decision-model/disposition-ab-v1.json`,
  `DISPOSITION-AB.sha256`, and `DISPOSITION-AB-PROMPT.sha256`. The command verifies
  both freezes before any model call. A mismatch is a stop condition; do not
  re-freeze or edit the corpus.

## RUN

Run one model at a time, with the same case order, timeout, and cache policy.

```bash
# 12 smoke cases, two per governed action
go run ./cmd/deceleval-ab -mode quick -out <artifact-dir> -models <baseline-id>,<candidate-id> -concurrency 1,4 -timeout 60s

# 36 held-out cases; refused until a completed quick run exists in <artifact-dir>
go run ./cmd/deceleval-ab -mode full -out <artifact-dir> -models <baseline-id>,<candidate-id> -concurrency 1,4 -timeout 60s

# Both in order
go run ./cmd/deceleval-ab -mode all -out <artifact-dir> -models <baseline-id>,<candidate-id> -concurrency 1,4 -timeout 60s
```

- Quick mode is the smoke check. Do not run the held-out split until the quick run
  is complete; the command enforces this gate.
- Each attempt is the first production attempt: the benchmark calls the provider
  directly, with no retry and no fallback. Do not add a retry loop or a fallback
  model; that would mask the failure under test.
- Warm-up and cold load are measured outside the timed batches. Keep them separate
  from the measured inference numbers.
- `<artifact-dir>` defaults to a new temporary directory outside the repository. Do
  not write generated artifacts into the repository unless the operator asks for
  durable evidence and the project convention calls for it.

## SCORE

Report, for each model and concurrency level:

- first-pass correct / attempted, overall and per action; a pass requires both
  schema validity and the correct action;
- invalid responses, wrong actions, timeouts, and fallback use (the benchmark's
  fallback count is always zero);
- median and slowest end-to-end latency, naming the slowest case;
- correct first-pass decisions per minute from the batch wall time;
- every disagreement whose expected action is BLOCK or NEEDS_HUMAN, with the
  selected action, so the operator can tell an unsafe automatic continuation from
  a malformed or timed-out attempt.

Report raw counts with percentages. Label an incomplete batch explicitly. Use null
for a value the endpoint does not report; never report an unavailable value as a
zero.

## MODEL AVAILABILITY

A model that is advertised but cannot load is an availability finding, not a
quality result. Report the exact model id and the endpoint's error, complete every
independent measurement, and do not substitute another model. A load failure (for
example an HTTP 409 from the runtime) is a stop for that model; it is not evidence
that the model decides well or badly.

Record the comparison as baseline only when the candidate is unavailable. Keep the
replacement decision with the operator: report the measured evidence and the
trade-off, and do not change a model default, routing policy, or configuration. If
no operator latency budget is documented, report the measured latency and leave the
threshold to the operator.

## ARTIFACTS

The command writes one JSONL record per attempted case plus a summary, run
metadata, and a Markdown report:

```text
attempts-<mode>.jsonl   summary-<mode>.json   run-<mode>.json   REPORT-<mode>.md
```

The JSONL includes timeouts and errors, with case id, model, concurrency, expected
and actual action, first-pass schema validity, correctness, elapsed time, error
category, and token counts. Keep the artifacts with the run; if the operator asks
for durable evidence, copy them to the project's established decision-model
evidence directory and record the corpus and prompt hashes.
