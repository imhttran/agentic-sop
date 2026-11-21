# Decision-model A/B — Inkling-2-Mini-4bit vs Qwen3-4B-4bit

**Type:** Focused decision-provider benchmark report (non-normative).
**Status:** QWEN BASELINE MEASURED; CANDIDATE UNAVAILABLE (MODEL LOAD FAILURE). COMPARISON BASELINE-ONLY; REPLACEMENT DECISION OPEN.
**Question:** Should `Inkling-2-Mini-4bit` replace `Qwen3-4B-4bit` for SOP's
structured-decision role on the M3 Max with 96 GB unified memory?
**Scope:** The production structured-decision provider path only. No production
default, `.env`, routing policy, model list, frozen DME corpus, or prior result is
changed. The model replacement decision is left to the operator.

**Comparison status:** this is a **Qwen baseline-only** comparison. The candidate is
advertised but cannot be loaded; its HTTP 409 is a **model load failure**, not a
quality result. No candidate accuracy or latency exists, and no model was
substituted. The replacement decision remains **open** until the requested Inkling
build completes the same frozen evaluation.

## 1. Production path reused

The benchmark calls the production clef provider directly:

- request builder: `internal/decisionadapter/providers/clef.BuildSystemOneRequest`
  via `Provider.Decide` (the configured default, `TransportSystemOne`);
- response parser and SOP-owned validation:
  `NormalizeSystemOneResponse`, `decision.Result.Validate`;
- transport: one `POST /v1/systemone` per case (`httpTransport`).

The provider's `Decide` is a single attempt: the retry and fallback chain lives in
the adapter CLI (`newProvider`), which this benchmark does not use. Every result
therefore records the first attempt; a fallback cannot mask a failure. Warm-up
requests are also made directly and are excluded from the timed batches.

The benchmark does not use the alternative chat-completions transport
(`CLEF_TRANSPORT=chat`). That transport is not the configured production path.

## 2. Reachability of the six actions

The corpus scores the six governed failure dispositions
(`internal/failure.Disposition`): `AUTO_FIX`, `CONTINUE`, `RETRY`, `REPLAN`,
`NEEDS_HUMAN`, `BLOCK`. The governed path can return all six:
`internal/cli.dispositionForAction` maps `AUTO_RETRY→RETRY`,
`AUTO_CONTINUE→CONTINUE`, `AUTO_FIX→AUTO_FIX`, `AUTO_RECONCILE→REPLAN`,
`TERMINAL→BLOCK`, and `HUMAN_APPROVAL_REQUIRED→NEEDS_HUMAN`. The mapping is
asserted by `TestDispositionForActionCoversEveryGovernedAction`.

This is not the same as the production SEAM-004 change-risk checkpoint, which asks
only `LOW/MEDIUM/HIGH/HUMAN` and whose model answer can only **add** `NEEDS_HUMAN`.
The benchmark asks the model for the governed disposition directly, so all six
actions are supported and the 12/36 balanced split is used. It is a direct
disposition benchmark, not a replay of the four-choice change-risk checkpoint.

## 3. Corpus and freeze

| Item | Value |
| --- | --- |
| Corpus | `evals/decision-model/disposition-ab-v1.json` |
| Corpus sha256 | `0296db27e415de9b7eb19d2df3508a43665969164b0cba8eb7fef1f85c99dcfa` |
| Freeze | `evals/decision-model/DISPOSITION-AB.sha256` |
| Prompt spec sha256 | `a128f3d4d2600ef5b09623648e3e1cb590759a7a978b592adb2be7c9f680173b` |
| Prompt freeze | `evals/decision-model/DISPOSITION-AB-PROMPT.sha256` |
| Smoke | 12 cases, 2 per action |
| Held-out | 36 separate cases, 6 per action |

Labels were authored from SOP's deterministic failure rules before either model
ran. Expected labels and rationales are never placed in the request: the request
state is the observable text, and the only question is the frozen six-choice
disposition question. A test asserts this for every case.

The held-out set includes:

- short failures and longer evidence-heavy failures;
- paired cases where one observable fact changes the label:
  - `ab-hold-retry-noprogress-remaining` (RETRY) vs
    `ab-hold-block-noprogress-spent` (BLOCK);
  - `ab-hold-auto-fix-regression-current` (AUTO_FIX) vs
    `ab-hold-continue-superseded` (CONTINUE);
  - `ab-hold-auto-fix-regression-current` (budget 1/3, AUTO_FIX) vs
    `ab-hold-human-fix-exhausted` (budget 3/3, NEEDS_HUMAN);
  - the same pairing also appears in the smoke split
    (`ab-smoke-retry-noprogress` vs `ab-smoke-block-noprogress-spent`).

Related cases stay in the same split. The held-out set was not used for tuning.

## 4. Endpoint and models

Discovered from `GET /v1/models` on `http://127.0.0.1:8000` (server header:
`uvicorn`):

| Model id | type | max_model_len |
| --- | --- | ---: |
| `Inkling-2-Mini-4bit` | vlm | 1048576 |
| `Qwen3-4B-4bit` | llm | 32768 |
| `clef-4bit` | decision | 262144 |

oMLX exposes no version or model revision through `/v1/models` or `/health`; those
fields are recorded as `null`. The SystemOne response reports input/output tokens
only, so time-to-first-answer-content, decode speed, and model-load duration are
`null`. The request sends no reasoning, sampling, cache, or output-cap directive; the
endpoint applies its own settings, identically to both models. (The production
SystemOne transport has no `max-tokens` field.)

### 4.1 Inkling-2-Mini-4bit is UNAVAILABLE (MODEL LOAD FAILURE)

The model is advertised but cannot load. The exact warm-up failure, recorded in
`run-full.json` / `run-quick.json`:

```text
HTTP 409: {"error":{"message":"Model 'Inkling-2-Mini-4bit' is unavailable after a previous load failure: VLM load failed: Received 1556 parameters not in model: ..."}}
```

No Inkling measurement exists, and no other model was substituted. The harness
continued with the available model and recorded the failure. **This is a model load
failure, not a quality result:** it is not evidence that the candidate decides well
or badly. The comparison is therefore baseline-only.

## 5. Measured results — Qwen3-4B-4bit

Run command (artifacts in `/tmp/decision-ab-20261121`, outside the repository):

```bash
go run ./cmd/deceleval-ab -mode all \
  -out /tmp/decision-ab-20261121 \
  -models Qwen3-4B-4bit,Inkling-2-Mini-4bit \
  -concurrency 1,4 -timeout 60s
```

Held-out run warm-up (outside the timed batches): cold load 185 ms, second warm-up 186 ms.
No fallback was used in any batch.

### 5.1 Smoke (12 cases)

| Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Median ms | Slowest ms (case) | Correct/min |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: |
| 1 | 12 | 12/12 (100%) | 8/12 (66.7%) | 0 | 4 | 0 | 202 | 218 (`ab-smoke-replan-scope`) | 196.00 |
| 4 | 12 | 12/12 (100%) | 8/12 (66.7%) | 0 | 4 | 0 | 738 | 948 (`ab-smoke-auto-fix-build`) | 212.48 |

### 5.2 Held-out (36 cases)

| Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Median ms | Slowest ms (case) | Correct/min |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: |
| 1 | 36 | 36/36 (100%) | 24/36 (66.7%) | 0 | 12 | 0 | 186 | 219 (`ab-hold-auto-fix-regression-current`) | 212.86 |
| 4 | 36 | 36/36 (100%) | 24/36 (66.7%) | 0 | 12 | 0 | 725 | 950 (`ab-hold-auto-fix-regression-current`) | 217.46 |

First-pass correctness requires both schema validity and the correct action.
Both held-out batches are complete (`36/36` attempted). The latency figures are a
single local run and vary with machine load; concurrency 4 was slower than
concurrency 1 for this endpoint in both runs.

### 5.3 Per-action correctness (held-out)

The same at concurrency 1 and 4:

| Action | Correct | Schema-valid | Wrong |
| --- | ---: | ---: | ---: |
| AUTO_FIX | 5/6 (83.3%) | 6 | 1 |
| CONTINUE | 3/6 (50.0%) | 6 | 3 |
| RETRY | 5/6 (83.3%) | 6 | 1 |
| REPLAN | 4/6 (66.7%) | 6 | 2 |
| NEEDS_HUMAN | 2/6 (33.3%) | 6 | 4 |
| BLOCK | 5/6 (83.3%) | 6 | 1 |

### 5.4 Consequential disagreements

Every held-out miss whose expected action is `BLOCK` or `NEEDS_HUMAN` (identical
at both concurrency levels):

| Case | Expected | Selected |
| --- | --- | --- |
| `ab-hold-block-security-credential` | BLOCK | NEEDS_HUMAN |
| `ab-hold-human-approval` | NEEDS_HUMAN | BLOCK |
| `ab-hold-human-destructive` | NEEDS_HUMAN | **AUTO_FIX** |
| `ab-hold-human-fix-exhausted` | NEEDS_HUMAN | BLOCK |
| `ab-hold-human-unclassified` | NEEDS_HUMAN | BLOCK |

`ab-hold-human-destructive` (force-pushing to a protected branch) is the most
consequential error: the model selected automatic repair for a destructive,
irreversible operation. The smoke run has two analogous misses:
`ab-smoke-block-validation` (BLOCK → AUTO_FIX) and `ab-smoke-human-security`
(NEEDS_HUMAN → BLOCK).

## 6. Limitations

- **No head-to-head comparison.** Inkling-2-Mini-4bit cannot load, so its accuracy
  and latency are unknown. The recommendation cannot rest on a comparison.
- **Single machine, single run.** Measured on the operator's M3 Max with 96 GB
  unified memory. Latency is load-sensitive; concurrency 4 was slower than
  concurrency 1 at this endpoint, which is an observed property of the runtime,
  not a general scaling claim.
- **Direct disposition, not the four-choice checkpoint.** The production
  change-risk checkpoint asks `LOW/MEDIUM/HIGH/HUMAN` and can only add
  `NEEDS_HUMAN`. This benchmark asks the six governed actions directly, which is
  the vocabulary the governed decision path can return.
- **Observability gaps.** oMLX exposes no version or model revision, and the
  SystemOne response exposes no TTFT or decode speed. Those fields are `null`, not
  zero.
- **Labels are authored, not double-human-labeled.** They follow SOP's written
  failure rules and are frozen by hash; they are not a second human reviewer.
- **No operator latency budget is documented.** The measured trade-off is reported
  without asserting a threshold.

## 7. Recommendation

**INSUFFICIENT EVIDENCE — QWEN BASELINE ONLY.** The candidate cannot be loaded by
the running oMLX instance, so no held-out accuracy, consequential-error, or latency
comparison exists. Keep the current model default unchanged; the replacement
decision remains **open** until the requested Inkling build completes the same
frozen evaluation.

The measured incumbent is not a strong decision classifier on this corpus either:
24/36 held-out (66.7%), with 5 consequential misses including an unsafe automatic
repair of a destructive operation. That is an observation about this benchmark and
this model, not authority to change SOP policy or defaults. Any replacement should
be judged on a held-out comparison once Inkling can serve, and on an explicit
operator latency budget.

## 8. Artifacts and reproduction

The live artifacts are preserved in the repository's decision-model evidence
directory, with a baseline-only record and a `SHA256SUMS` manifest:

[`evals/decision-model/disposition-ab-2026-11-21/`](../../../evals/decision-model/disposition-ab-2026-11-21/README.md)

The original run also left a working copy in `/tmp/decision-ab-20261121/`.

```text
REPORT-quick.md        attempts-quick.jsonl   run-quick.json    summary-quick.json
REPORT-full.md         attempts-full.jsonl    run-full.json     summary-full.json
```

`attempts-*.jsonl` has one record per attempted case (24 quick, 72 held-out),
including timeouts and errors, with case id, model, concurrency, expected/actual
action, first-pass schema validity, correctness, elapsed ms, error category,
token counts, and null-for-unavailable timing fields.

Reusable invocation: the shipped skill is
[`skills/decision-ab/SKILL.md`](../../../skills/decision-ab/SKILL.md) (`/decision-ab`).
It is wired into the canonical skill surface, the installer, and the Claude Code
plugin mirror, and its contract is tested by `internal/skill/decision_ab_test.go`.
The measured run used the same commands below.

Copy-ready commands:

```bash
# Discover the served ids (no inference)
go run ./cmd/deceleval-ab -discover

# Quick (12 smoke cases); artifacts default to a temp dir outside the repo
go run ./cmd/deceleval-ab -mode quick -out /tmp/decision-ab \
  -models Qwen3-4B-4bit,Inkling-2-Mini-4bit -concurrency 1,4 -timeout 60s

# Held-out (36 cases) in the same artifact dir; refused until quick is complete
go run ./cmd/deceleval-ab -mode full -out /tmp/decision-ab \
  -models Qwen3-4B-4bit,Inkling-2-Mini-4bit -concurrency 1,4 -timeout 60s

# Or both in order
go run ./cmd/deceleval-ab -mode all -out /tmp/decision-ab \
  -models Qwen3-4B-4bit,Inkling-2-Mini-4bit -concurrency 1,4 -timeout 60s
```

Verification fixtures:

```bash
go test ./cmd/deceleval-ab ./internal/deceleval ./internal/decisionadapter/providers/clef \
  -run 'AB|DecideOnce|ExpandModes|ParseConcurrency|RequireSmoke|WarmupFailure|DispositionForAction'
```

The existing frozen DME corpus (`corpus.json`, `CORPUS.sha256`) and every earlier
result under `evals/decision-model/` are unchanged.
