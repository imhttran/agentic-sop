# Workstream B Evidence Reconciliation — Independent Design Review

**Type:** Reconciliation record (non-normative). It maps the merged Workstream B
verification onto the protocol's execution-readiness requirements, records what each
result does and does not establish, and names the freeze dependencies it leaves open. It
is **not** an experiment result, does not freeze the corpus, does not authorize the
experiment, and does not approve any governance decision.

**Baseline:** `origin/main` @ `a10edc28`, the merge of PR #574. Earlier baselines quoted
below are preserved as recorded by the source documents.

**Sources of record (not re-run here):**

- [INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md](../INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md)
  — the Workstream B verification (PR #574; merge `a10edc28`, branch head `b784f2dc`).
- [INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md](../INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md)
  — the two four-call pilots.
- [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) — the
  authoritative protocol and its execution-readiness requirements.
- [READINESS.md](READINESS.md), [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md),
  [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) — the current freeze and decision records.

No pilot was repeated, no inference was executed, and no test was re-run for this record.
Where this record restates a result, the evidence class is the one the Workstream B report
recorded: **CODE**, **EXISTING TEST**, **SYNTHETIC RUNTIME**, or **PILOT**.

## 1. What the merge changes

PR #574 is merged. The Workstream B intake is therefore **complete as evidence intake**:
the execution path has been inspected, partially exercised against synthetic local
servers, and the same-project run-directory collision it found was fixed in the same
change. The fix (`promptRunID` in `internal/cli/prompt.go`) and its regression tests are
on `main`.

The merge changes the **status of the report**, not the **status of the experiment**:

- Workstream B is no longer a missing report. It is merged point-in-time evidence.
- Execution infrastructure is recorded as READY for controlled execution of arms A and B,
  including concurrent calls (arm C and the B×3 control), subject to the open items below.
- One runtime requirement remains UNVERIFIED (EP-15), and five remain PARTIAL (EP-02,
  EP-09, EP-10, EP-12, EP-16). None of them is resolved by merging the document.
- The corpus, seeded cases, adjudicators, thresholds and execution authorization are
  unchanged. The corpus remains **NOT FROZEN**; the full experiment remains **NOT YET
  READY**; H1 and H2 remain **NOT ESTABLISHED**.

Merging a report is not an approval of the decisions it documents. It does not approve an
eligibility rule, a threshold, a corpus, a seeder, an adjudicator, a freeze, or an
execution budget.

## 2. Reconciliation matrix

Each row corresponds to one execution-readiness requirement named in the Workstream B
scope. `VERIFIED` means the production path is established by code, an existing test, or
synthetic runtime; `PARTIAL` means the mechanism is established but a required observable
is missing or provider-dependent; `UNVERIFIED` means it cannot be established without
real inference; `BLOCKED` means evidence is unavailable for a reason outside this
workstream.

| ID      | Requirement                                         | Evidence class                                | Result                   | What is established                                                                                                                                                                                                                                                                                                               | Residual and freeze dependency                                                                                                                                                                                                                                                                                    |
| ------- | --------------------------------------------------- | --------------------------------------------- | ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| WS-B-01 | Provider routing and model selection                | CODE, SYNTHETIC RUNTIME, PILOT                | VERIFIED                 | `--model-class` selects the class's provider and model from a `models:` block (`source: manual_override`). Cache and JEV are off by default.                                                                                                                                                                                      | A class without a `models.<class>` entry routes to the built-in table (provider `ollama`, cloud `medium`) and ignores `agent.provider` (EP-13). The freeze overlay must define every class and record its sha256.                                                                                                 |
| WS-B-02 | Requested versus provider-attested model identity   | CODE, SYNTHETIC RUNTIME, PILOT                | PARTIAL                  | The `tokens` key is the model name SOP sent; it detects a SOP-side override. Both pilots matched `routing.model`.                                                                                                                                                                                                                 | No client records a server-returned model id. oMLX exposes an id only; the Ollama cloud digest is not proof against substitution. Attribution must be recorded as client-side only where the provider exposes no identity.                                                                                        |
| WS-B-03 | Token accounting and unreported calls               | CODE, EXISTING TEST, SYNTHETIC RUNTIME, PILOT | VERIFIED                 | Usage is recorded per model; an omitted usage block yields zero tokens with `unreported_calls: 1`. Both pilots had `unreported_calls: 0` on every call.                                                                                                                                                                           | Reasoning tokens are included in the output count. Native retried turns are not counted (WS-B-07). The protocol's rule stays: `tokens` absent or `unreported_calls > 0` makes the model check and cost criterion INCONCLUSIVE.                                                                                    |
| WS-B-04 | Native-agent execution and repository-tool evidence | CODE, EXISTING TEST, SYNTHETIC RUNTIME, PILOT | PARTIAL                  | The REVIEW policy allows read-only tools, the native request advertises `read_file`, an inspection turn reaches the model and leaves the file unchanged, and the call count rises to 2.                                                                                                                                           | 0 of 4 Pilot B runs inspected the repository, and no per-run tool trace is persisted. The call count is the only artifact-level signal. "The harness performs a bounded read-only inspection" describes availability, not observed behavior; record the limitation rather than claiming inspection.               |
| WS-B-05 | Isolation and concurrency                           | CODE, EXISTING TEST, SYNTHETIC RUNTIME        | VERIFIED                 | One run directory per run; the id is reserved atomically. Concurrent prompts in one project previously collided (1 directory in 19 of 20 trials; 4 processes in 5 of 5) and now keep separate runs (8 simultaneous prompts; 4 processes in 5 of 5). Credentials travel only in the authorization header.                          | The protocol still requires one project per concurrent call for request-file isolation and leakage control. Provider-side concurrency is untested (WS-B-06). A cleared environment is a harness rule, not a tested invariant. The batch check must record the absolute run directory and `work_item_id` per §5.1. |
| WS-B-06 | Timing and latency methodology                      | CODE, PILOT                                   | PARTIAL                  | The one-process timer measures a single call; Pilot B's durations are valid for its setup.                                                                                                                                                                                                                                        | Pilot A's cross-process timing is invalid. Parallel-batch latency has not been measured (EP-15, UNVERIFIED). C latency is first measured in the full run; a missing or invalid C timing makes H2 INCONCLUSIVE. No pilot latency may be compared with the experiment.                                              |
| WS-B-07 | Failure handling and evidence persistence           | CODE, SYNTHETIC RUNTIME, PILOT                | PARTIAL                  | A text-only provider makes one attempt and fails the prompt. Native retries are bounded at three attempts. Each run persists `prompt.md`, `result.md`, `metadata.json`, `routing.json`, `state.json`, with model input redacted.                                                                                                  | A native prompt can succeed after two provider requests while `tokens.calls` is 1, so cost is under-counted and `calls` means successful turns. The native stored result is a re-serialized JSON object, not the raw reply. Hash `result.md` as the output of record.                                             |
| WS-B-08 | Reproducibility                                     | CODE, EXISTING TEST, SYNTHETIC RUNTIME, PILOT | PARTIAL                  | Candidate commits, artifact sha256, config-overlay sha256, request sha256, instruction-block sha256 and the run-order seed were recorded for the pilots. The run-directory allocation is now race-free.                                                                                                                           | Provider identity is client-side only; sampling parameters are unavailable (SOP exposes none); native retry counts and tool use are invisible. Reproducibility is manifest-and-artifact reproducibility, not byte-identical model behavior. Record these as limitations in the freeze record.                     |
| WS-B-09 | Pilot limitations                                   | PILOT                                         | VERIFIED as a limitation | Both pilots are four-call plumbing runs, excluded from the dataset. Nothing was adjudicated; no quality, PASS, FAIL or INCONCLUSIVE claim follows. Pilot A's timing is invalid. Pilot B's ACP-ZED clones contain the gap report, so those two runs are contaminated for defect detection; LLM04's own status line states its gap. | Neither pilot plan is a clean corpus entry. The Workstream B report also records one unintended real cloud call while checking EP-13 (no plan or repository content; output not kept; call count unknown; may count against the operator's Ollama account). It is not experiment or pilot data.                   |

**BLOCKED:** no Workstream B requirement is BLOCKED by missing evidence. The freeze and
execution blockers are outside this workstream: corpus floor, seeded and no-known-defect
cases, independent seeder, two independent human adjudicators, approved thresholds, and
the freeze and execution authorizations.

## 3. Open Workstream B items

| Item  | Status     | What would close it                                                                                                                             | Effect if left open                                                                                                       |
| ----- | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| EP-15 | UNVERIFIED | Three concurrent real requests to the chosen provider, each in its own project, timed with the one-process timer, under separate authorization. | C latency is first measured in the full run; a missing or invalid C timing makes H2 INCONCLUSIVE.                         |
| EP-16 | PARTIAL    | Instrument the native path to persist a per-run tool trace, or record that inspection is unobserved.                                            | The experiment cannot claim the reviewer used repository access; the harness description stays an availability statement. |
| EP-02 | PARTIAL    | A provider that exposes an attested model identity, or a recorded client-side-only attribution.                                                 | Model attribution is client-side; provider-side substitution is invisible.                                                |
| EP-09 | PARTIAL    | A counted provider-request field, or a recorded convention that `calls` means successful turns.                                                 | Cost is under-counted on retried native turns.                                                                            |
| EP-10 | PARTIAL    | Hashing `result.md` as the output of record, as the pilots did.                                                                                 | The raw model reply is not preserved for the native harness.                                                              |
| EP-12 | PARTIAL    | The one-process timer recorded in the manifest; parallel timing measured under EP-15.                                                           | Single-call timing is usable; parallel timing is not.                                                                     |

The merge decision recorded in the Workstream B report (its §7, item 2) is resolved by
PR #574 and is retained there as history, not as an open item.

## 4. What this reconciliation does not establish

- It does not convert a synthetic-runtime check into real-provider behavior.
- It does not infer review quality, defect detection or false-positive behavior from the
  pilots; the pilots are excluded and nothing was adjudicated.
- It does not show that a reviewer would find a defect in a run.
- It does not change the corpus counts: 0 strict proposed determinable, 10 proposed
  partial, 2 verified-defect holds, 3 unverified holds, 8 excluded.
- It does not approve the eligibility amendment, a threshold, a freeze or an execution.
- It does not make the Workstream B evidence part of the experiment dataset.

## 5. Effect on the freeze and the experiment

The merged evidence removes "Workstream B report unavailable" as a reason to defer. It
does not remove any other blocker.

- **Freeze:** still **NOT READY**. The freeze record may now cite the execution-path
  evidence for configuration, isolation, timing method and evidence persistence, but it
  must also record the residual limits above. Corpus, seeded-case, adjudicator and
  approval preconditions are unchanged; see
  [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) §8.
- **Full experiment:** still **NOT YET READY**. Decisions 1–9 in
  [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) remain open, and EP-15 is
  an explicit execution dependency.
- **H1/H2:** **NOT ESTABLISHED.** No experiment was run.
