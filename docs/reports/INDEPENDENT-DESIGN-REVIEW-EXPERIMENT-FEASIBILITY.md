# Independent Design Review Experiment — Feasibility

**Type:** Feasibility report (non-normative). It records readiness for the experiment
defined by
[EGM-GOLDFISH-REVIEW-ASSESSMENT.md](EGM-GOLDFISH-REVIEW-ASSESSMENT.md). It is not an
experiment result and does not change behavior.
**Status:** Workstream A reviewed; Workstream B pending; no experiment or pilot
inference was executed.
**Plan of record:**
[PLAN-Independent-Design-Review-Feasibility.md](../plans/PLAN-Independent-Design-Review-Feasibility.md).
**Conditional contract:**
[PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md](../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md).

**Evidence labels used below**

- **Verified** — directly observed in the repository at the recorded baseline.
- **Proposed** — a judgment awaiting the corpus owner's freeze or independent
  adjudication.
- **Unverified** — depends on evidence not available here (Linux report, operator
  decision, or a human adjudicator).

## 1. Protocol baseline

- Repository baseline: `origin/main` @ `64101f9e`, the merge of PR #565.
- Authoritative protocol:
  [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
  It owns the corpus rules, thresholds, statistical comparator, PASS/FAIL/INCONCLUSIVE
  outcomes, and call budget.
- Conditional contract:
  [PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md](../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md).
  It is unapproved and does not authorize implementation.
- The experiment has **not** run. No threshold was changed. No inference call was made
  for the experiment or pilot.
- The §1.2/§5.1 corrections from PR #567 are merged into the assessment, and this
  report's requested-model wording matches it.
- The feasibility verdict below is **not** the experiment's PASS/FAIL/INCONCLUSIVE
  outcome.

## 2. Corpus inventory and determinability evidence

### 2.1 Method and limits

I reviewed candidate plans and specifications and the later reports that record a defect
or gap. The inventory is taken at `64101f9e`; the last-modified commit is recorded where a
document is used as a corpus candidate.

A document is counted as **determinable** only if a reviewer with the document and its
referenced repository paths at the parent revision could identify the defect without the
later report. This is a proposed classification. No independent adjudicator has confirmed
any classification; the report therefore distinguishes **proposed determinable** from
**adjudicator-confirmed**.

A remediation plan is not used as the corpus document when its own text states the defect.
That would leak the answer. Such plans are recorded as **evidence pointers** until the
pre-fix parent document and revision are extracted.

All candidate documents were last modified under the git identity Tom Tran. This report
uses the git identity as a conservative proxy for authorship, so the operator is treated
as unable to adjudicate them; see §3.2 for the limits of that proxy.

### 2.2 Evidence-verified candidates

| Document (last-modified)                                                                                                             | Later evidence                                                                                                                                      | Defect or gap                                                                                                                                                                                                       | Proposed determinability                                                                                                                                     | Corpus status                                                                           |
| ------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| [PLAN-SOP-ACP-ZED.md](../plans/PLAN-SOP-ACP-ZED.md) (`e462deed`)                                                                     | [ACP-EXECUTION-EXCLUSION-GAP.md](../plans/ACP-EXECUTION-EXCLUSION-GAP.md)                                                                           | The acceptance matrix claims the project lock protects CLI, MCP, and ACP equally; the gap report shows the lock is metadata-only and is not held for execution, so CLI-first and ACP-first exclusion is unresolved. | **Determinable with referenced code**: the plan names the lock; the referenced source shows the execution lifetime is not covered.                           | Verified defect-bearing; proposed determinable.                                         |
| [PLAN-Code-Retrieval-Context-Seam.md](../plans/PLAN-Code-Retrieval-Context-Seam.md) (`03a7d7d9`)                                     | The plan's own post-merge status line. No independent later report was found.                                                                       | Retrieval was wired to IMPLEMENT but not FIX, despite the original implement/fix scope; step 5 also awaits model-budget approval.                                                                                   | **Proposed, not strict**: the status line is part of the document, so this is self-referential evidence; a reviewer at the parent revision would not see it. | Verified defect-bearing; self-referential evidence, not counted as strict determinable. |
| [PLAN-SOP-RUN-ACCEPT-001.md](../plans/PLAN-SOP-RUN-ACCEPT-001.md) (`56ff82ac`)                                                       | [PLAN-STATUS-AUDIT-2026-11-04.md](PLAN-STATUS-AUDIT-2026-11-04.md)                                                                                  | Six operator acceptance checks fail because the fixtures predate the later `### Acceptance Mapping` contract; the plan remains ACTIVE.                                                                              | **Partially determinable**: the failure is visible only after the later contract; the original plan's own checks passed at its revision.                     | Verified gap; proposed partially determinable.                                          |
| [PLAN-04-SOP-LLM04-Supply-Chain.md](../plans/PLAN-04-SOP-LLM04-Supply-Chain.md) (`e55527f4`)                                         | The plan's status and the LLM04 evidence report.                                                                                                    | Historical outcomes remain PARTIAL until re-execution under the newly recorded acceptance mapping.                                                                                                                  | **Partially determinable**: the acceptance-mapping requirement is a later contract.                                                                          | Verified gap; proposed partially determinable.                                          |
| [PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md](../plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md) (`8d2bd3b6`) | [llm02-disclosure.md](owasp-llm/llm02-disclosure.md) GAP-01, GAP-02, GAP-03.                                                                        | Acceptance-check logs, model-visible validation failures, and JSON-escaped Go test output exposed credential-shaped text; all three were fixed later.                                                               | **Partially determinable**: the defects are explicit in the later report and depend on the redaction/acceptance path.                                        | Verified gap; proposed partially determinable.                                          |
| [PLAN-Security-Scanner-Addon.md](../plans/PLAN-Security-Scanner-Addon.md) (`1b611711`)                                               | [SECURITY-SCANNER-RESIDUAL-FINDINGS.md](SECURITY-SCANNER-RESIDUAL-FINDINGS.md), [SECURITY-DEPENDENCY-COVERAGE.md](SECURITY-DEPENDENCY-COVERAGE.md). | Residual scanner findings and dependency-coverage gaps remain after the add-on's first acceptance.                                                                                                                  | **Partially determinable**: later scanner evidence supplies the defect; coverage limits were not fully known at plan time.                                   | Verified gap; proposed partially determinable.                                          |
| [PLAN-08B-SOP-LLM08-Hidden-Context-Closure.md](../plans/PLAN-08B-SOP-LLM08-Hidden-Context-Closure.md) (`aa3e1d36`)                   | [llm08-hidden-context.md](owasp-llm/llm08-hidden-context.md) confirmed defects and follow-ups.                                                      | Hidden-context and credential-redaction defects were confirmed after the earlier plan; the follow-up records current evidence.                                                                                      | **Partially determinable**: the report supplies the defect and the earlier plan supplied the context.                                                        | Verified gap; proposed partially determinable.                                          |
| [PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md](../plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md) (`05685308`)   | [llm09-retrieval.md](owasp-llm/llm09-retrieval.md) and the archived 09B follow-up.                                                                  | Retrieval cancellation/deadline and rebuild/invalidation evidence required a follow-up plan and PR #363.                                                                                                            | **Partially determinable**: the follow-up evidence names the gap; the original plan did not fully specify it.                                                | Verified gap; proposed partially determinable.                                          |

### 2.3 Evidence-only remediation plans

These plans record or repair defects in earlier artifacts, but the pre-fix parent
document and revision are not yet extracted. They are **not** counted as corpus
documents; using them would leak the defect.

| Document                                                                                                    | What it records                                                                                                                                                                                             | Use                                                                                            |
| ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| [PLAN-SOP-Acceptance-Verification-Hardening.md](../plans/PLAN-SOP-Acceptance-Verification-Hardening.md)     | Five observed lifecycle defects: PARTIAL never blocked LOCAL_DONE, prose criteria were unverifiable, ALREADY_SATISFIED was unreachable, missing deliverables were non-gating, and recovery was unsupported. | Evidence pointer; parent PRD/spec needed.                                                      |
| [PLAN-Stale-Approval-Invalidation.md](../plans/PLAN-Stale-Approval-Invalidation.md)                         | A PENDING approval becomes a deadlock after an accepted plan revision invalidates its evidence.                                                                                                             | Evidence pointer; parent approval/reconcile contract needed.                                   |
| [PLAN-SOP-Validation-Evidence-Reuse.md](../plans/PLAN-SOP-Validation-Evidence-Reuse.md)                     | Verified gaps in reusing validation evidence; a follow-up landed with SOP-HARD.                                                                                                                             | Evidence pointer; parent validation contract needed.                                           |
| [PLAN-Phase-5-Execution-Recovery.md](../plans/PLAN-Phase-5-Execution-Recovery.md)                           | A previously recorded recovery limitation was resolved.                                                                                                                                                     | Evidence pointer; parent recovery contract needed.                                             |
| [PLAN-Review-Scope-And-Output-Hardening.md](../history/plans/PLAN-Review-Scope-And-Output-Hardening.md)     | Review-scope and malformed-output defects.                                                                                                                                                                  | Evidence pointer; [RSH-001](review-hardening/RSH-001-review-seam-baseline.md) is the baseline. |
| [PLAN-RSH-Follow-Up-Hardening.md](../history/plans/PLAN-RSH-Follow-Up-Hardening.md)                         | Three residual review-governance findings.                                                                                                                                                                  | Evidence pointer.                                                                              |
| [PLAN-SOP-Lifecycle-Consistency-Hardening.md](../history/plans/PLAN-SOP-Lifecycle-Consistency-Hardening.md) | Lifecycle-consistency findings from the SOP skills audit.                                                                                                                                                   | Evidence pointer.                                                                              |
| [PLAN-Implementation-Convergence.md](../history/plans/PLAN-Implementation-Convergence.md)                   | Implementation-convergence findings.                                                                                                                                                                        | Evidence pointer.                                                                              |
| [PLAN-Run-Metrics-And-Observability.md](../history/plans/PLAN-Run-Metrics-And-Observability.md)             | Run-metrics and observability gaps.                                                                                                                                                                         | Evidence pointer.                                                                              |
| [PLAN-EV-Context-Run-Evaluation.md](../history/plans/PLAN-EV-Context-Run-Evaluation.md)                     | Context-engine live-path evaluation gaps.                                                                                                                                                                   | Evidence pointer.                                                                              |
| [PLAN-Post-Wave3-Production-Readiness.md](../history/plans/PLAN-Post-Wave3-Production-Readiness.md)         | Post-Wave-3 production-readiness gaps.                                                                                                                                                                      | Evidence pointer.                                                                              |
| [PLAN-SOP-End-to-End-Reliability.md](../history/plans/PLAN-SOP-End-to-End-Reliability.md)                   | End-to-end reliability gaps.                                                                                                                                                                                | Evidence pointer.                                                                              |

### 2.4 Not corpus entries

| Document                                                                                | Why it is not a corpus entry                                                                                                                                                          |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [PLAN-RAG-SOP-Integration.md](../plans/PLAN-RAG-SOP-Integration.md)                     | Excluded as a current candidate because the recorded NO-GO is an evaluation result; whether the plan's design caused the negative result is a **proposed judgment**, not established. |
| [PLAN-07-SOP-LLM07-Misinformation.md](../plans/PLAN-07-SOP-LLM07-Misinformation.md)     | The plan records live-model and Linux verification as NOT_RUN. That is an evidence gap, not a demonstrated design defect.                                                             |
| [PLAN-SOP-E2E-Reliability-Hardening.md](../plans/PLAN-SOP-E2E-Reliability-Hardening.md) | Excluded from the current inventory only because no later evidence pointer records a defect. Being PROPOSED does not by itself exclude a design defect.                               |

### 2.5 Corpus counts

Two separate floors apply: **15 plans with known defects** and **15 determinable known
defects**. A plan can count toward the first without counting toward the second;
partially determinable or self-referential candidates count toward neither.

| Measure                                                    | Count        |
| ---------------------------------------------------------- | ------------ |
| Candidate documents reviewed (8 + 12 + 3, §2.2–§2.4)      | 23           |
| Evidence-verified defect-bearing or gap-bearing candidates | 8            |
| Proposed determinable with independent evidence (strict)   | 1            |
| Proposed partially determinable or self-referential        | 7            |
| Independently adjudicator-confirmed determinable           | 0            |
| Assessment floor for determinable known defects            | 15           |
| Evidence-verified candidate plans available                | 8            |
| Assessment floor for plans with known defects              | 15           |
| Plan-count shortfall (even if all 8 count)                 | 7            |
| Determinable-defect shortfall (strict)                     | 14           |
| Seeded-defect cases constructed                            | 0 of about 8 |
| No-known-defect cases constructed                          | 0 of about 6 |

The repository has enough **evidence of defects** to support corpus assembly, but the
corpus itself is not assembled. The dominant missing work is extracting the pre-fix
parent document and parent revision for the evidence-only plans in §2.3, then having a
corpus owner freeze the determinability judgments. This report counts partially
determinable candidates toward neither floor as the conservative reading. The
assessment's wording ("plans with known defects" and "determinable known defects") does
not settle which floor such a plan counts toward, so the freeze rule must decide it
(§8, item 2).

## 3. Human adjudication feasibility

### 3.1 Protocol requirement

The assessment requires two distinct human adjudicators who are not authors of any plan
in the corpus. The corpus owner, seeder and analyst may overlap with each other and with
the operator, but none of them may be an adjudicator. A model may not be an adjudicator.
The second human re-adjudicates the larger of 20 findings or a random 20% sample. The
20-finding floor is a hard PASS requirement: with fewer than 20 findings in total, kappa
is reported and the outcome cannot PASS. This is a protocol requirement, not only a risk.
A single-adjudicator limitation is INCONCLUSIVE.

“Author” is defined in the protocol as a human who wrote or directly edited the plan text;
approving a plan does not make someone its author.

### 3.2 Repository evidence

- `git shortlog -sne --all` shows Tom Tran as the only human author. The other commit
  identities are `Claude`, `Zed`, and `Eval`; none is a human adjudicator.
- Every candidate document in §2.2 was last modified under the git identity Tom Tran.
  The protocol defines an author as a human who wrote or directly edited the plan text,
  and a git identity does not show that: commits made by an agent under the operator's
  identity would be counted as the operator's under this proxy, and co-author trailers
  were not examined. The report therefore takes the conservative reading, that the
  operator cannot adjudicate the candidates. That reading is a proposal until the
  commits are checked or the corpus owner rules otherwise.
- No approval record for the experiment exists. The primary checkout's
  `sop approvals --json` returned an empty approval list; the worktree has no SOP state
  database.

### 3.3 Availability

**UNCONFIRMED and currently insufficient.** Two independent human adjudicators outside
the operator's authorship are required for PASS. None is identified in the repository.
The operator may serve as corpus owner, seeder, or analyst, but that does not satisfy the
adjudication requirement.

This is a genuine feasibility blocker for a PASS outcome. It is actionable: the operator
can designate two external humans who did not author or directly edit the corpus plans.
For each designated adjudicator, record the identity and a statement or repository
evidence that the person did not author or directly edit any corpus plan; approval alone
is not authorship.

## 4. Linux provider and execution findings

### 4.1 Intake status

**UNAVAILABLE.** I searched the repository, `/Users/imhttran/agentic-workspace`, `/tmp`,
and `/Users/imhttran/Downloads` for report-like files, and listed open pull requests;
no Linux report file, branch, or PR was found. Workstream B remains pending. No provider,
routing, token, latency, or pilot-readiness claim is made from Linux evidence.

Static code-path readings of the §4.3 items (request composition, provider and model
resolution, the requested-model key, token reporting, concurrent runs and isolation) are
recorded in a
[comment on PR #565](https://github.com/imhttran/agentic-sop/pull/565#issuecomment-7065856929)
and in PR #567. They are not the independent Workstream B report, they are static
reading and not measured behavior, and the runtime items they list remain open.

### 4.2 Baseline verification (assessment, not Linux confirmation)

The assessment's code-path claims were re-checked at `64101f9e` and remain the baseline
for what the Linux report must confirm:

- `sop prompt` builds its agent with the selected model, while `FromConfig` resolves
  `SOP_AGENT_MODEL` > provider-specific environment > configured model;
  `EffectiveProvider` and `EffectiveHarness` read their variables first. A project
  `.env` is applied at startup for every command unless the variable is already exported
  (`loadDotEnv`), so it can inject an override.
- `models.stages.review` pins the review stage of `sop review` and `sop run`
  (`reviewProvider` calls `stageAgent`), and the plan stage of `sop run`; `sop prompt`
  never calls `stageAgent`, so the pin does not apply to it. The `modelEnvOverride`
  guard does not check the prompt's routed selection.
- Prompt `metadata.json` exposes routing and token usage by model. The `tokens` key is
  the model name SOP sent, not a server-returned identity; it detects a SOP-side
  override, not the model the server executed. No duration or sampling field exists;
  latency must be timed outside SOP.
- The project config is tracked in `.agent-sdlc/config.yaml`, so a scratch clone inherits
  the historical config; a fixed overlay and its sha256 are required. Under the Ollama
  REVIEW harness, the reviewer performs a bounded read-only repository inspection; a
  text-only provider has no repository access.

These are code-path readings at `64101f9e`, not measured provider behavior. Workstream B
must confirm them; they are not a substitute for the Linux agent's independent
investigation.

### 4.3 Required Workstream B evidence

- Exact prompt composition and the instruction/data delimiter.
- Provider and model resolution, including `.env`, environment precedence, and the
  stage-pin scope.
- Requested-model verification against the token key; provider-side model identity where
  available.
- Token-reporting behavior, including unreported-call counts.
- Parallel execution, per-call project isolation, and wall-clock latency measurement.
- Four-call pilot readiness and the exact commands/artifacts.

## 5. Estimated inference and human-review cost

### 5.1 Inference

The assessment's §7 budget is the authority:

- Target corpus: 26–29 plans. At that size, the base experiment is 550–700 calls without
  Arm D (default 7-call design: 550–613; disjoint 8-call design: 628–700) and 862–1,309
  calls with Arm D.
- Minimum corpus: 15 plans. At that floor, the base experiment is 319–364 calls without
  Arm D and 499–679 calls with Arm D (the assessment's §7 table: 499 for the default
  7-call design plus Arm D with B and C, up to 679 for the disjoint 8-call design plus
  the largest Arm D).
- The assessment's 319–700 (without Arm D) and 499–1,309 (with Arm D) ranges span both the
  15-plan floor and the 26–29-plan target.
- Pilot: 4 calls, excluded from the dataset.

No call was made. These are planned counts, not measured spend. If the provider does not
report token usage, the cost criterion is INCONCLUSIVE and the hypothesis cannot PASS.

### 5.2 Human review

Every finding must be read; the second adjudicator re-adjudicates the larger of 20
findings or a random 20%. The number of findings is unknown before the run, so no hour
estimate is asserted. Two independent human adjudicators are required, and neither is
currently identified.

## 6. Risks of an INCONCLUSIVE outcome

| Risk                                 | Evidence                                                                                      | Effect                                                     |
| ------------------------------------ | --------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| Corpus shortfall                     | 1 proposed determinable vs 15; 8 evidence-verified candidate plans vs 15                      | No PASS possible until the corpus is assembled and frozen. |
| Seeded/no-known-defect corpus absent | 0 of about 8 and 0 of about 6                                                                 | False-positive and calibration evidence is missing.        |
| Adjudicator availability             | Only the operator is a human author; no second human identified                               | No PASS; at best INCONCLUSIVE.                             |
| Reproducibility floor                | 15 contributing plans per arm; at the 15-plan floor one empty plan makes the arm INCONCLUSIVE | Small corpora are likely to fail the gate.                 |
| Kappa floor                          | Fewer than 20 double-adjudicated findings cannot PASS                                         | Low finding volume makes the result INCONCLUSIVE.          |
| Token reporting                      | No-usage provider cannot pass the requested-model check                                       | Cost and model-attribution criteria are INCONCLUSIVE.      |
| Model mismatch                       | Prompt path can serve an environment- or `.env`-overridden model                              | A single un-replaced mismatch invalidates the comparison.  |
| Linux evidence pending               | No provider/execution report available                                                        | Pilot readiness is unconfirmed.                            |
| Small sample                         | About 15 defects gives wide intervals                                                         | Only large effects are detectable.                         |

INCONCLUSIVE is a valid experimental outcome. It is not evidence of improvement, and it
does not authorize the conditional PRD.

## 7. Pilot readiness

**Status: NOT READY.** The four-call pilot is prepared but not authorized or executed.

### 7.1 Prerequisites

- Frozen pilot plans and request files with a fixed instruction/data delimiter.
- One throwaway clone or worktree per run (four for the pilot), each at the recorded
  revision with a fixed `.agent-sdlc/config.yaml` overlay (sha256 recorded), no `.env`,
  and only that run's request file.
- A cleared environment (`env -i` plus a recorded allowlist), or at minimum unset
  `SOP_AGENT_MODEL`, the provider-specific model override, `SOP_AGENT_PROVIDER`,
  `SOP_AGENT_HARNESS`, and the `SOP_MODEL_*` variables.
- An explicit `--model-class` for every call.
- No `--cache`; JEV triage disabled.
- Operator authorization for four review calls.
- Linux confirmation of routing, requested-model versus provider identity, token,
  timing, and per-call isolation behavior.

### 7.2 Prepared shape (not run)

```text
# one throwaway clone per (arm, plan) run, override variables unset,
# each containing only its own request file
cd <clone-plan1-A> && sop prompt --capability review --model-class <class> --file request.md
cd <clone-plan1-B> && sop prompt --capability review --model-class <class> --file request.md
# repeat for plan 2 in <clone-plan2-A> and <clone-plan2-B>
```

The pilot is arms A and B once on two plans: two arms × two plans = four clones and four
calls. A shared clone would let a repository-tool reviewer read the other arm's request
file, so the per-run isolation rule of the assessment's §5.1 applies to the pilot as
well. Record `metadata.json` routing and tokens,
the requested-model key against `routing.model`, provider-side model identity where
available, request/plan sha256, and wall-clock duration. A mismatch, missing token data,
`unreported_calls` greater than zero, a duplicate run directory, or invalid timing makes
the affected pilot check fail; it is not a review finding.

### 7.3 Blockers

- No operator authorization for the pilot.
- No Linux provider/execution report.
- Pilot plans are not frozen.
- No requested-model or provider-side identity evidence for the chosen provider.

Do not execute the pilot until these are resolved. Do not execute the full experiment or
Arm D.

## 8. Required operator decisions

1. **Corpus:** approve the target size, designate the corpus owner, and authorize
   extraction of the pre-fix parent documents for the §2.3 evidence-only plans.
2. **Determinability:** approve the freeze rule, including which floor a partially
   determinable plan counts toward, and the proposed classifications; do not treat them
   as verified until frozen.
3. **Adjudication:** designate a seeder and two independent human adjudicators who did not
   author or directly edit the corpus plans. The operator may be corpus owner, seeder, or
   analyst, but not an adjudicator.
4. **Pilot:** authorize the four-call pilot and its budget, and confirm the provider,
   model, and token-reporting expectation.
5. **Risk:** accept the INCONCLUSIVE risk at the current corpus size, or expand the corpus
   before freezing.
6. **Protocol:** confirm that no threshold changes are approved. Any change is a new
   protocol revision and invalidates the affected comparison.
7. **Workstream B:** decide whether to wait for the Linux report or proceed with
   Workstream B explicitly pending.

## 9. Final feasibility verdict

**NOT YET READY.**

- **Not READY:** the historical corpus has 1 proposed determinable defect against a
  floor of 15, and 8 evidence-verified candidate plans against a floor of 15; the seeded
  and no-known-defect corpus is not constructed; no two independent human adjudicators
  are identified; Workstream B is pending; and the four-call pilot is unauthorized.
- **Not BLOCKED:** the missing work is actionable. The repository contains evidence
  pointers for corpus assembly, and the operator can designate adjudicators and authorize
  a pilot.
- **Not an experiment outcome:** this verdict is not PASS, FAIL, or INCONCLUSIVE. No
  experiment was run, no inference was spent, and no threshold was changed.

The next step is corpus assembly and operator designation of two independent human
adjudicators, followed by the Linux evidence intake and the four-call pilot.
