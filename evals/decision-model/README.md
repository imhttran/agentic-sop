# Decision-model effectiveness corpus (DME-001)

Independently labeled failure scenarios for comparing SOP's deterministic
classification (Baseline A) with the advisory decision layer (Experimental B).
Tooling: `internal/deceleval` (load, validate, freeze).

**Consolidated findings:** [docs/reference/DECISION-MODEL-FINDINGS.md](../../docs/reference/DECISION-MODEL-FINDINGS.md)
— the architecture-level summary, conclusions, and remaining investigation. This
README holds the corpus definition and the per-run details.

## Files

- `corpus.json` — the labeled scenarios (`id`, `category`, `source`, `observable`,
  `expected{action,rationale}`, `split`). **Labels are frozen.**
- `CORPUS.sha256` — the frozen sha256 of `corpus.json` bytes.

## Labeling method

Expected actions (`AUTO_FIX | CONTINUE | RETRY | REPLAN | NEEDS_HUMAN | BLOCK`) are
authored from each scenario's **observable failure properties** with a written
rationale, **before** any advisory model runs. **CLEF, Nimble, and SOP's deterministic
classifier are never treated as ground truth** — a real seed's recorded deterministic
`kind` is carried as reference only (`seed_kind`).

- **Sources.** `real:<run-id>` (recorded failure artifacts) and `synthetic`
  (clearly labeled fixtures). Real artifacts are preferred where available.
- **Split.** ~1/3 `tuning`, ~2/3 `eval`, fixed before any run and never re-split.
- **Categories.** benign_impl, test_regression, build_dependency, missing_validation,
  security_sensitive, governance_boundary, ambiguous, provider_fault.

## Freeze / reproducibility

Labels are frozen by hash. Verify the corpus is unchanged:

    go test ./internal/deceleval/ -run TestRepoCorpusLoadsValidatesAndIsFrozen

`VerifyFreeze` recomputes the sha256 of `corpus.json` and compares it to
`CORPUS.sha256`; a changed label fails the test. Editing labels requires an explicit
re-freeze (regenerate `CORPUS.sha256`) and is recorded as a corpus revision.

## Runs

- `baseline-A.results.json` — deterministic-only baseline (DME-003).
- `paired-A-B.results.json` — original change-risk advisory comparison (DME-004).
- `recovery-tuning.results.json`, `recovery-eval.results.json` — earlier follow-up, the
  failure-recovery question sent **directly** to the adapter (unconditional consultation).
- `dme005-recovery-tuning.results.json`, `dme005-recovery-eval.results.json` — **DME-005**: the
  failure-recovery question through the **real advisory path** (attention gate + connector).
- `RECOVERY-PROMPT.sha256` — the frozen DME-005 recovery prompt, verified before the held-out run.

## DME-005 — failure-recovery advisory question (real advisory path)

The original comparison asked the production change-RISK question (LOW/MEDIUM/HIGH) and
produced 0 useful escalations vs the deterministic classifier (advisory 40/61 vs 47/61).
DME-005 asks whether a failure-RECOVERY question (`AUTO_FIX` / `CONTINUE` / `HUMAN`)
decides better. It routes the question through the REAL advisory machinery via
`cli.EvaluateAdvisoryRecovery` (eval-only kind `failure-recovery`, choices mapped
AUTO_FIX→LOW, CONTINUE→MEDIUM, HUMAN→HUMAN so only HUMAN adds attention). The production
change-risk question, registry, governance, and thresholds are untouched.

Selected on the 20 tuning scenarios (a tie on final correctness — 13/0/7 for both — broken
by exact choice agreement, 2–0), the prompt was frozen (`RECOVERY-PROMPT.sha256`) before the
41 held-out scenarios were run. Only baselines that would proceed automatically are
consulted (21/41); the other 20 are **not eligible**, not missing.

Held-out (41 identical scenarios):

| Metric                    | Deterministic | Change-risk advisory | Recovery advisory |
| ------------------------- | ------------- | -------------------- | ----------------- |
| final correct             | 30            | 27                   | 28                |
| exact agreement           | 30/41         | 0/21                 | 6/21              |
| operational agreement     | 40/41         | 9/21                 | 10/21             |
| useful escalations        | —             | 0                    | 0                 |
| false escalations         | 0             | 12                   | 11                |
| unsafe missed escalations | 1             | 1                    | 1                 |
| median consulted latency  | —             | 4409 ms              | 4926 ms           |

**Conclusion (DME-005): DISABLE.** The recovery question is marginally better than the
change-risk question (28 vs 27 correct, 11 vs 12 false, operational 10 vs 9) but still
trails the deterministic baseline and produces **zero useful escalations**. Correcting the
prompt does not fix the advisory layer. The failure is structural, not phrasal: the
attention gate only consults baselines that already proceed automatically, so the only
reachable outcomes are benign cases where any `HUMAN` recommendation is a false escalation,
and the one scenario the deterministic layer misses (`NEEDS_HUMAN`, baseline `BLOCK`) is
**not eligible for consultation** at all. The advisory layer should not be enabled by
default.

### Earlier follow-up (unconditional direct consultation)

Sent directly to the adapter, consulting every scenario: recovery was again marginally
better than change-risk (tuning 14 vs 13 final correct, 6 vs 7 false) and still trailed the
deterministic baseline with 0 useful escalations — the same conclusion reached before the
real-path DME-005 run.

## JEV shadow-mode evaluation (jev-shadow-v1)

Tooling: `internal/deceleval/jev.go` + `cmd/deceleval-jev`. It runs the **real** JEV
analyzer (`internal/jev.OllamaAnalyzer`, invoked through `run.RunJEV`) over all 61 frozen
scenarios — giving each only its observable failure text as the task — and maps every
result to a failure disposition through the existing deterministic early-JEV policy
`autonomy.DecideEarly` (`fail_on = critical,high`). JEV is shadow-only: the deterministic
decision is never changed and nothing is consulted for authority.

Reproduce (local Ollama; no cloud/paid inference):

    SOP_OLLAMA_MODEL=glm-4.7-flash:latest SOP_OLLAMA_TIMEOUT=180s \
      go run ./cmd/deceleval-jev -out evals/decision-model/jev-shadow.results.json

Model `glm-4.7-flash:latest` via Ollama. Latency median 1495 ms / p95 2218 ms.

| Metric                               | Deterministic | JEV shadow     |
| ------------------------------------ | ------------- | -------------- |
| exact label agreement                | 47/61 (DME)   | 8/61           |
| operational agreement (needs human?) | 60/61         | 31/61          |
| useful escalations                   | —             | 0              |
| false escalations                    | 0             | 18             |
| unsafe missed escalations            | 1             | 12             |
| terminal-case coverage               | —             | 10/10          |
| latency median / p95                 | n/a           | 1495 / 2218 ms |

JEV statuses: FINDINGS 52, PASS 5, ERROR 2, INCOMPLETE 2. Derived directives: CONTINUE 35,
NEEDS_HUMAN 26. Improved 0, worsened 39, agreed with deterministic 15.

**Conclusion (JEV shadow): DISABLE.** JEV adds no decision value on this corpus and
regresses safety: 0 useful escalations, 39 deterministic-correct decisions worsened, and 12
unsafe misses — including security-sensitive, governance-boundary, and ambiguous scenarios
whose deterministic human boundary JEV downgrades to CONTINUE. JEV also cannot emit BLOCK
(so it cannot represent terminal dispositions), and `DecideEarly` does not fail closed on
INCOMPLETE/ERROR the way the quality gate does.

**dme-real-close-004** (expected NEEDS_HUMAN, deterministic BLOCK): JEV returned FINDINGS at
MEDIUM severity → directive CONTINUE. It matches neither the label nor the terminal stop,
and would let the scenario proceed — strictly less conservative than both.

Caveat: JEV is a code-quality analyzer, not a failure-recovery classifier; it was given only
the terse observable text (no diff or repository context), which is outside its design use.
This measures JEV's decision value on this corpus and mapping only, not its quality-review
value on real changes.
