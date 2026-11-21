# Independent Design Review — Four-Call Pilot Results

**Type:** Point-in-time evidence report (non-normative). It records what the four-call
pilot of the protocol in [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](EGM-GOLDFISH-REVIEW-ASSESSMENT.md)
did and did not show. It is **not** an experiment result: there is no PASS, FAIL or
INCONCLUSIVE outcome, nothing was adjudicated, and no claim is made that any approach
improves review quality. It changes no behavior, threshold or protocol rule.
**Date:** 2026-11-21
**Related:** the readiness assessment is
[INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md](INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md);
the conditional contract is
[PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md](../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md).

## 1. Scope and authorization

- The pilot is the assessment's §6.4 pilot: arms A and B once on two plans, four calls in
  all, excluded from the dataset. It tests plumbing: model attribution, token reporting,
  per-run isolation, and evidence capture.
- The operator authorized the four calls in chat, in reply to "Authorize the four calls
  with these draft instructions?". The budget was the four calls only: no retries, no
  Arm D, no full run.
- **Not recorded:** approval of the thresholds, freezing of the instruction blocks for the
  full experiment, and role holders (corpus owner, seeder, analyst, adjudicators). The
  pilot does not evaluate thresholds and involved no adjudication. Nothing here is an
  SOP approval record.

## 2. Setup

| Item                       | Value                                                                                                                                                      |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SOP binary                 | built from `main` at `1b61db64` (`vcs.modified=false`), sha256 `c097c5ce6a9901c77a232d240bf1eace2278bf317a97f7d050b1c077bf88cdb8`                           |
| Provider and model         | oMLX at `127.0.0.1:8000`, provider `mlx`, model `Qwen3-4B-4bit` (type llm, context 32,768), selected by `--model-class medium` (all three classes set local) |
| Config overlay             | sha256 `679194c1d5ebb923e61e0331c8401f7f3d92250f8b885857687ee1ab6fc55682`, replacing the committed `.agent-sdlc/config.yaml` in each clone                  |
| Harness                    | text-only MLX provider (`SOP_MLX_TOOL_LOOP` unset): the reviewer has no repository access                                                                  |
| Environment                | `env -i` with `PATH`, `HOME` and `SOP_MLX_BASE_URL=http://127.0.0.1:8000`; no override variables; no API key (see §6)                                       |
| Other                      | no `--cache`; JEV triage off; one throwaway clone per run containing only that run's request file and no `.env`                                            |
| Provider-side identity     | the oMLX model list returns the model id only, with no digest, so attribution is client-side only                                                           |

## 3. Inputs

| Plan    | Pinned revision | Plan text sha256                                                   |
| ------- | --------------- | ------------------------------------------------------------------ |
| ACP-ZED | `79acbfc1`      | `c3723af8b19e2a0aabea19b3cf6ca4e7b0b696779aed8d67b661a11df84ddd1e` |
| LLM04   | `e55527f4`      | `028e1377e7c86f60f74c9ca1c912ffe7e9ab2be9ee2975c552cab52889e0070a` |

The operator chose these two plans for the pilot. Neither is a clean corpus entry:

- **ACP-ZED** (`docs/plans/PLAN-SOP-ACP-ZED.md`): the plan and its gap report
  (`docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md`) were added in the same commit,
  `79acbfc1`, so no earlier revision of the plan exists in git. The plan text does not
  state the gap. The feasibility report's classification of this candidate as the one
  strict determinable defect relies on later evidence that is not later in git history,
  and should be rechecked (§9).
- **LLM04** (`docs/plans/PLAN-04-SOP-LLM04-Supply-Chain.md`): the plan has one revision
  and its own status line states the gap (outcomes remain PARTIAL until re-execution
  under the newly recorded acceptance mapping), so a reviewer reads the defect in the
  plan itself.

Both were acceptable for a plumbing pilot whose results are excluded.

**Instruction blocks** (draft, not frozen). Arm A is one generic paragraph asking for
problems with a severity and a reason. Arm B asks five questions (unstated assumptions,
missing failure modes, undefined references with unverifiable ones reported as missing
evidence, unverifiable acceptance criteria, scope and reuse). Each request is the block,
a fixed `<<<PLAN BEGIN>>>`/`<<<PLAN END>>>` delimiter, and the plan text. sha256: A
`d7a44b63ece22e64ba81a210c0d16ff186c98963c1f6045cc7de6a60b5d875fc`, B
`840bd0117bd9e1abe35e9a49285dfd6cb37d4fc2d373d00bdcd6306de1482d24`.

## 4. Runs

Run order (python `random`, seed 20261121): ACP-A, LLM04-A, LLM04-B, ACP-B. Each run was
`sop prompt --capability review --model-class medium --file request.md --json` in its own
project. All four exited 0 with status `completed`.

| Run     | Request sha256 (prefix) | Tokens in / out (calls) | Approx. seconds\* | `result.md` sha256 (prefix) |
| ------- | ----------------------- | ----------------------- | ----------------- | --------------------------- |
| ACP-A   | `4a804289482d`          | 3,854 / 1,944 (1)       | 30.1              | `8f2fb71c3815e4a0`          |
| LLM04-A | `a5a5c097cab7`          | 2,875 / 2,793 (1)       | 40.0              | `4c4a1339bcf255da`          |
| LLM04-B | `687167582f08`          | 3,008 / 1,052 (1)       | 17.3              | `5e3c3e2b9999d7b7`          |
| ACP-B   | `8dd449e68a61`          | 3,987 / 2,064 (1)       | 32.9              | `4b383a35ac5be829`          |

\*Approximate and SOP-side: the interval between the run's `prompt.md` and
`metadata.json` file times. This is **not** the external wall-clock the protocol requires
(§5).

## 5. Checklist outcome

| Check                                                                                      | Result                                                                                                                         |
| ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| `routing`: class `medium`, provider `mlx`, model `Qwen3-4B-4bit`, source `manual_override` | Passed on all four                                                                                                             |
| `tokens` key equals `routing.model`                                                        | Passed on all four; this shows the name SOP sent, not a server-side identity                                                   |
| `unreported_calls` is zero                                                                 | Passed on all four; the server reported usage on every call                                                                    |
| One run directory per project; `metadata.json` matches the stdout result                   | Passed on all four                                                                                                             |
| No thinking tags in the visible result                                                     | Passed on all four                                                                                                             |
| Cache off, cleared environment, no `.env`, one request file per clone                      | Confirmed before the runs                                                                                                      |
| **External wall-clock timing**                                                             | **Failed; the timing record is invalid.** The script compared `perf_counter` values from two separate processes, so `timing.txt` shows 0.000–0.002 s. A one-process timer was written and self-tested (a 1 s sleep measured 1.010 s) but not used in a real run. |
| Provider-side model identity                                                               | Weak: model id only                                                                                                            |
| Failure handling                                                                           | Not exercised; no run failed                                                                                                   |

## 6. Observations (not findings)

- Output tokens exceed the length of the visible result on every run, so reasoning tokens
  are included in the reported output count and therefore in any cost figure.
- Arm A on ACP-ZED returned a result stating no issues were found; arm B returned
  structured findings with severity, section and reason. These outputs were not
  adjudicated and say nothing about detection.
- The oMLX server answered `/v1/models` without a key, and a chat-completions request
  with an empty body returned HTTP 422 with and without a key, so no key was used. The
  calls succeeded without one.
- SOP's MLX provider reads `SOP_MLX_BASE_URL` and `SOP_MLX_API_KEY`, and appends
  `/v1/chat/completions` itself, so the base URL carries no `/v1` suffix. `SOP_OMLX_*`
  variables are not read by the production code.

## 7. Not tested

- Concurrent runs, the run-directory race, and parallel-batch latency.
- Arm D and any second model.
- Provider-side model identity beyond the id.
- Reproducibility, adjudication, kappa, or anything about detection or false positives.

## 8. Evidence

The experiment directory holds the manifest, request files, per-run `result.md`,
`metadata.json`, `prompt.md`, `routing.json` and `state.json`, the stdout and stderr
captures, the draft instruction blocks, the config overlay and the scripts. It is kept
outside the SOP checkout and is not committed. The sha256 values above identify the
files.

## 9. Follow-ups

These are proposals only; nothing was changed.

1. Use a one-process timer for latency in any later run, and record it in the manifest.
2. Recheck the ACP-ZED classification and the strict-determinable count in the
   feasibility report, given that the plan and its gap report share a commit.
3. If these two plans are reused in the corpus, the corpus owner should rule on the
   missing pre-defect revision (ACP-ZED) and the self-referential status line (LLM04).
4. The full experiment remains **NOT YET READY**: the corpus is not frozen, no two
   independent human adjudicators are named, thresholds are not approved, and the
   concurrent-run and parallel-latency checks remain.
