# SOP Independent Design Review

Status: **Conditional proposal; implementation unauthorized.** Version 0.1 • Date: 2026-11-21

**Type:** Product requirements for a proposed capability. This document changes no
behavior. The normative behavior of the existing review pipeline is owned by
[REVIEW.md](../specs/REVIEW.md); the prompt path by
[PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md); the human boundary by
[HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md); and the quality gate by
[QUALITY.md](../specs/QUALITY.md). Where this document and a specification differ,
the specification owns current behavior.

**Authoritative protocol and condition of implementation.** The experiment protocol,
corpus, thresholds, statistical comparator, and PASS/FAIL/INCONCLUSIVE outcomes are
owned by [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
This PRD does not restate them. None of its requirements is authorized for
implementation until:

1. each hypothesis the PRD claims (H1 for the rubric; H2 for multiple scoped
   reviewers) returns PASS; a FAIL or INCONCLUSIVE on a claimed hypothesis blocks the
   requirements that depend on it;
2. the operator records a separate governance decision authorizing the specific
   surface (rubric, structured findings, evidence persistence, disposition, or
   orchestration); and
3. OD-1 and OD-2 in §14 are resolved.

A PASS on the rubric experiment does not by itself authorize structured findings, a
disposition API, evidence persistence, or new orchestration.

## 1. Problem statement

SOP reviews code, not designs. A plan is authored, schema-validated, and activated;
the first adversarial reading of the change happens after implementation, when the
reviewer receives the task and the diff. By then an unstated assumption, a missing
failure mode, or a referenced symbol that does not exist has already been encoded in
the plan's stages.

Reviewing code catches execution defects. It does not ask whether the design is the
right design, whether its assumptions are stated, whether its failure modes are
covered, or whether its acceptance criteria can actually be verified.

Two separate questions are often conflated:

1. **Isolation.** Does the reviewer see the author's conversation? In SOP, no. Both
   the `sop run` review path and `sop prompt --capability review` carry no authoring
   transcript. The `sop run` path does include the rendered task and applied planner
   decisions; the ad-hoc path carries the operator's text or file. "Stateless" here
   means no conversation history, not zero design context. Isolation is already
   implemented and is not the gap.
2. **Design-stage review.** Does anything challenge the design before
   implementation? In SOP, no. The review capability is scoped to a change set, and
   planning has no semantic critic.

This PRD addresses the second question only. It proposes the smallest enhancement
that reuses the existing review boundary, adds no lifecycle stage, and makes no
claim of improved quality until the assessment's protocol has returned an outcome and
the operator has recorded an authorization decision.

## 2. Current behavior and evidence

### 2.1 What exists

| Capability                                             | State                                        | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------------ | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Reviewer input without the authoring transcript        | **Implemented**                              | `internal/review/review.go` defines `Request` and `Request.text()`; `internal/review/provider.go` makes one bounded `agent.Generate` call and retries only malformed output. `sop prompt` also makes one bounded call (`internal/cli/prompt_cache.go`). The `sop run` path includes the rendered task and applied planner decisions; see the assessment §1–§2 for the precise limit.                                                |
| Structured findings and deterministic severity         | **Implemented for `sop run`**                | `review.Finding`, `review.Report`, and `Report.Blocking` (`internal/review/review.go`); `reviewOutputSchema` and `parseReport` (`internal/review/provider.go`); `quality.BlockingFindings` and `quality.Evaluate` (`internal/quality`). The ad-hoc prompt path is prose; see §2.3 D2.                                                                                                                                               |
| Findings persisted for code review                     | **Implemented**                              | `sop run` writes `review.json` with the full report (`internal/cli/run.go`), and `report.json` carries the findings (`runReportDoc.Findings`). [COMPONENTS.md](../architecture/COMPONENTS.md) documents the artifact set.                                                                                                                                                                                                           |
| Malformed-output and infrastructure-failure boundaries | **Implemented**                              | Bounded retry and `ErrMalformedOutput` (`internal/review/provider.go`); malformed output maps to the human boundary, never a pass (`internal/cli/run.go`); [REVIEW.md](../specs/REVIEW.md) §6.                                                                                                                                                                                                                                      |
| Implementation-review rubric                           | **Implemented**                              | `reviewTask` (`internal/review/provider.go`) drives goal fit, correctness, complexity, tests, and change justification; [REVIEW.md](../specs/REVIEW.md) §7.                                                                                                                                                                                                                                                                         |
| Review-stage model/provider pinning                    | **Implemented**                              | `models.stages.review` or `SOP_MODEL_STAGE_REVIEW_PROVIDER` and `_NAME` (`internal/cli/stage_models.go`; [MODEL-ROUTING.md](../specs/MODEL-ROUTING.md)); a pin selects a model and never a gate. The ad-hoc prompt path does not use this pin; it uses the deterministic class router and `--model-class` (assessment §1.2).                                                                                                        |
| Plan schema and deterministic checks                   | **Implemented**                              | `planner.Plan.Validate`, capability inventory with evidence, assumptions with evidence, acceptance criteria and checks (`internal/planner/plan.go`); `planner.Lint` warns on deliverables without acceptance checks (`internal/planner/lint.go`).                                                                                                                                                                                   |
| Design-stage semantic review                           | **Missing**                                  | `sop plan` performs one `planner.Generate` plus schema repair and writes `PLAN.md` and `plan.json` (`internal/cli/plan.go`); no review provider is invoked. `sop plan activate` validates structure only (`internal/cli/plan.go`).                                                                                                                                                                                                  |
| Ad-hoc design critique                                 | **Partial**                                  | `sop prompt --capability review --file <path>` runs a read-only prompt, but it bypasses `review.Provider`: `promptOutputRequirements` returns prose guidance (`internal/cli/prompt.go`), not the JSON findings schema. It writes `result.md` and `metadata.json` ([PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) §9), so the raw text is durable but is never parsed, severity-classified, or recorded as structured findings. |
| Per-finding disposition                                | **Partial**                                  | The code-review fix loop consumes blocking findings, and the human approval gate records the decision; no per-finding `accepted`/`contested`/`declined` record exists.                                                                                                                                                                                                                                                              |
| Fan-out and de-duplication                             | **Missing; unnecessary for a first version** | SOP can issue several review calls, but no merge step exists. A single reviewer needs no de-duplication.                                                                                                                                                                                                                                                                                                                            |
| Decision-adapter integration                           | **Not applicable**                           | The CLEF, Nimble, and Julia adapters answer structured JEV decisions and are not review providers ([JEV-DECISION-CONTRACT.md](../architecture/JEV-DECISION-CONTRACT.md) §1). Wiring them into review would confuse two different boundaries.                                                                                                                                                                                        |

### 2.2 Evidence method

Every current-state claim below was traced in this working tree at baseline commit
`ad8a9c6c` by reading the named file and function. The code paths are unchanged from
the `1e8cb005` baseline; `git log 1e8cb005..ad8a9c6c` contains only the two
documentation merges. The review input for the proposal itself was documentation-only,
so the review could not open those files; the citations are for the reader, not a
substitute for the source. No behavior was run to produce these claims; they describe
code paths and call sites as written.

### 2.3 Observed gaps in the current code paths (static trace)

These are observed in the current tree by reading the code paths, not by running
behavior. They are static-trace gaps, not runtime reproductions:

- **D1 — The test summary is declared but not supplied.** `review.Request` has a
  `TestSummary` field, and `reviewTask` asks whether the tests prove the change, but
  no production caller populates it: `sop run` passes `Task` and `Diff`
  (`internal/cli/run.go`), `sop review` passes `Task` and `Diff`
  (`internal/cli/review.go`), and the MCP tool passes `Task` and `Diff`
  (`internal/cli/mcp.go`). The reviewer must infer test sufficiency from the diff.
  `Criteria` is also unpopulated, but the rendered task already contains the
  acceptance criteria, so that omission is partly mitigated.
- **D2 — The ad-hoc review path has no structured output.** A
  `sop prompt --capability review` call returns prose. It does not produce
  `review.Report`, does not apply `quality.fail_on`, and cannot be consumed as
  findings evidence by another SOP surface.
- **D3 — A plan can be activated without semantic review.** `sop plan activate`
  validates the plan's structure and task graph; `planner.Lint` only warns about
  deliverables without acceptance checks. Neither asks whether the plan is sound.
- **D4 — `sop review` has no document mode.** It accepts no arguments and reads the
  working-tree diff; there is no way to point the structured review boundary at a
  plan or specification.

### 2.4 Hypothetical risks, not measured

The following are plausible but unmeasured. They must not be presented as findings:

- a design review would catch material defects earlier than code review;
- several scoped reviewers would find more than one;
- the cost and latency would be acceptable for large or high-risk plans;
- structured design findings would improve approval decisions.

No SOP review path and no reviewed framework publishes a measured defect-detection
result for design review. The [assessment's protocol](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md)
exists to replace these hypotheses with evidence or to reject the capability.

## 3. Prior art

The authoritative comparison is [Assessment §3](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
It covers EGM, Superpowers, and OpenSpec at the commits it records and classifies
each capability as implemented, partial, missing, or not applicable. This PRD does
not maintain a second matrix.

The conclusions relevant to this contract:

- **EGM's isolation is not new to SOP.** A stateless `review.Request` already means
  the reviewer carries no authoring transcript. EGM's distinctive additions are the
  rubric, the fan-out, and the resolution engine.
- **Superpowers reviews code with a fresh subagent, not designs with one.** Its plan
  check is self-review.
- **OpenSpec is an artifact lifecycle, not a reviewer.** Its verification is
  post-implementation conformance in the authoring context.
- **No reviewed framework publishes measured design-review effectiveness** in the
  files the assessment read. That is why the assessment's protocol, not a framework
  claim, governs any adoption decision.
- **Do not adopt** EGM, Superpowers, or OpenSpec as dependencies; do not add a second
  artifact model or agent-driven process control.

A design-level rubric is the only candidate addition this PRD carries. The assessment
tests it; this PRD does not authorize it.

## 4. Goals and non-goals

**Recommendation: Option B — minimal enhancement.** The design-stage gap is real, but it
does not justify a new capability, engine, or lifecycle stage. A narrow read-only
review that reuses the existing review provider, report, severity, and parser
closes the observed gap. A new capability (Option C) would duplicate `review`; no
change (Option A) would leave the design stage without structured, recorded review
evidence. The [assessment's protocol](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md)
gates any claim that the enhancement improves quality.

The recommendation options A/B/C are a different axis from the experiment arms A/B/C/D
in the assessment; do not conflate them.

### 4.1 Goals

- **G1** Let an operator obtain an independent, design-stage review of a plan,
  PRD, or specification before implementation.
- **G2** Ask design questions that the implementation review does not ask:
  assumptions, failure modes, security, testability, readiness, and scope.
- **G3** Return findings in the existing structured report so severity policy and
  downstream reporting are not duplicated.
- **G4** Record the review and the operator's disposition as durable evidence.
- **G5** Preserve independence: for the proposed path, the reviewer reads the artifact
  and explicit references, not the authoring conversation. `sop run` is unchanged and
  continues to include the rendered task and applied planner decisions.
- **G6** Keep the capability advisory and outside the lifecycle: no new gate, no
  new state, no new approval system.

### 4.2 Non-goals

- **N1** Replacing `sop review`, the fix loop, the quality gate, or the human
  approval gate.
- **N2** Blocking plan activation or approval. A blocking design gate needs a
  separate decision and a separate PRD.
- **N3** Automatically rewriting plans or applying fixes from design findings.
- **N4** A production multi-persona reviewer swarm, a de-duplication engine, or a
  round-based convergence protocol. Arm C may be run as an experiment under the
  assessment; a production implementation requires a separate decision after H2.
- **N5** A new specification or change-artifact system, an OpenSpec-style archive,
  or a second source of truth.
- **N6** Integrating with the JEV decision adapters; they answer a different
  question.
- **N7** A new provider, transport, database, queue, service, or daemon.
- **N8** New configuration keys, lifecycle states, or approval states.

## 5. Functional requirements

- **FR-1 — Read-only entry point.** SOP MUST provide an operator-invoked,
  read-only design-review path that accepts a repository design artifact (a plan,
  PRD, specification, or design document) and does not modify the repository.
- **FR-2 — Reuse the review boundary.** The design review MUST use the existing
  review provider and report types. It MUST NOT introduce a second findings schema,
  severity vocabulary, parser, or verdict rule.
- **FR-3 — Independent input.** For the proposed path, the model input MUST be the
  artifact text plus only explicitly declared reference material. It MUST NOT include
  the authoring conversation, session-only planner answers, decision memory, retrieval
  context, or prior review findings. This is a requirement for the proposed path; the
  existing `sop run` review is unchanged and continues to receive the rendered task
  and applied planner decisions. If a planner decision is material to the design, it
  belongs in the artifact or in the declared references.
- **FR-4 — Design rubric.** The prompt MUST ask design questions, at minimum:
  problem and goal fit; unstated assumptions and undefined terms; failure modes and
  recovery; security and data handling; testability and acceptance evidence;
  implementation readiness, including whether referenced paths and symbols exist;
  maintainability, including unnecessary complexity, duplication, and ownership
  boundaries; reuse of existing capabilities; and scope or YAGNI violations.
- **FR-5 — Structured output.** The response MUST be parsed into the existing
  report (severity, title, detail, file, line, suggestion). A malformed response
  MUST follow the existing bounded malformed-output boundary.
- **FR-6 — Advisory only.** The design review MUST NOT change lifecycle state,
  plan acceptance, the quality gate, an approval record, or a task-run exit code.
  It MUST NOT block plan activation or approval.
- **FR-7 — Durable evidence.** A completed review MUST write a durable artifact
  under the existing `.agent-sdlc` state root. The artifact MUST record the artifact
  path, a content digest, capability, provider and model, rubric version, findings,
  and timestamp. It MUST NOT contain credentials or hidden reasoning.
- **FR-8 — Disposition.** The operator MAY record a disposition per finding
  (`accepted`, `contested`, `declined`) in the same artifact. The disposition is
  data; it MUST NOT create a lifecycle state or an approval system.
- **FR-9 — Provider and model independence.** The design review MUST run on any
  provider that declares `REVIEW`. Pinning MUST use the routing surface of the chosen
  entry point: `models.stages.review` for a review-stage path, or `--model-class` and
  the class table for a prompt path. No new configuration key is required. The
  implementation MUST record the actual provider and model; it MUST NOT claim a stage
  pin for a prompt-path implementation.
- **FR-10 — Failure is not a clean review.** A provider failure, timeout, or
  unparseable response MUST be reported as "review not completed" with a non-zero
  exit. It MUST NOT be reported as a clean review or as an empty findings list.
- **FR-11 — No silent reuse.** A reused or cached result, if caching is added,
  MUST be labelled as reused and MUST NOT be presented as a new review of changed
  content.

## 6. Architecture boundaries

- **AB-1 — One owner for review semantics.** `internal/review` remains the owner of
  `Report`, `Finding`, severity, parsing, and blocking. A design review composes
  that boundary; it does not fork it.
- **AB-2 — A document is a distinct subject.** A design document is not an
  invocation-scoped change set. The implementation MUST NOT overload `Request.Diff`
  with a document and call it a diff. It MUST introduce an explicit subject kind or
  a composing request type that shares `Report` and parsing.
- **AB-3 — No lifecycle change.** `internal/quality`, `internal/approval`,
  `internal/planflow`, the task state machine, and `sop run` MUST be unchanged.
  A future blocking policy is out of scope.
- **AB-4 — No second source of truth.** The plan, PRD, or specification remains
  authoritative. The review artifact is evidence about it, never a replacement.
- **AB-5 — No decision-adapter integration.** The JEV adapters are a separate
  boundary. A design review produces findings; it does not produce a `LOW` /
  `MEDIUM` / `HIGH` / `HUMAN` decision.
- **AB-6 — No new state root.** Evidence belongs under the existing `.agent-sdlc`
  layout, reusing the existing run or prompt artifact conventions where they fit.
- **AB-7 — No new operational surface.** No service, daemon, queue, database, or
  network protocol is introduced.

## 7. Integration with the existing SOP lifecycle

- The design review is an **operator action**, not a lifecycle stage. It runs before
  `sop plan activate` or before a human approves a plan.
- It writes evidence only. It does not write `plan.json`, `state.json`,
  `review.json`, `gate.json`, or `report.json` for a task run.
- Its findings are input to the human approval decision. The human gate remains the
  only authority that approves, blocks, or commits.
- A plan or specification MAY reference the review artifact. No existing document
  is required to change.
- The review is not scheduled, not retried by the lifecycle, and not part of the
  bounded fix loop.
- If an operator wants design findings to block, that is a separate policy decision
  with its own PRD. It is not enabled here.
- No requirement here is authorized until the assessment returns PASS on the relevant
  hypothesis and the operator records a separate governance decision (see the header).

## 8. Review independence requirements

- **IR-1** The reviewer MUST NOT receive the authoring conversation, the model's
  tool transcript, or hidden reasoning.
- **IR-2** For the proposed path, the input MUST be reproducible from repository
  content: the artifact plus declared references. Session-only material MUST NOT be
  injected silently. This does not change `sop run`, which includes the task and
  applied planner decisions.
- **IR-3** References MUST be explicit. Automatic repository-wide retrieval is NOT
  part of the first version. If the chosen harness gives the reviewer repository
  tools, the review is not document-only; the implementation MUST record that access
  and MUST NOT claim document-only isolation.
- **IR-4** Prior review findings, dispositions, and fix history MUST NOT be part of
  the input, so a later round is not anchored by an earlier one.
- **IR-5** A different model or provider MAY be pinned through the routing surface of
  the chosen entry point. It is not required, and SOP MUST NOT claim that a same-model
  review is independent in the statistical sense.
- **IR-6** A reviewer MUST NOT be given its own earlier output as context in the
  same review invocation.

## 9. Security and reliability constraints

- **SR-1** Artifact text and references are untrusted data. SOP MUST NOT interpret
  them as commands, lifecycle transitions, configuration, or approvals.
- **SR-2** Model input and evidence MUST pass through the existing credential
  redaction policy. No credential or hidden reasoning may be persisted.
- **SR-3** File arguments and references MUST be confined to the project directory
  using the same symlink-safe rule as `sop prompt --file`
  ([PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) §12).
- **SR-4** Input size MUST be bounded. When the artifact exceeds the model's
  context budget, the review MUST fail with a clear error; it MUST NOT silently
  truncate the document.
- **SR-5** Retries MUST be bounded. Provider timeouts and malformed output MUST
  use the existing bounded behavior.
- **SR-6** Evidence writes MUST be atomic. A partial artifact MUST NOT be left
  behind.
- **SR-7** The review MUST use the existing provider transport and authorization
  path. It MUST NOT add a network surface.
- **SR-8** The review MUST be read-only with respect to the repository. Writing the
  evidence artifact under `.agent-sdlc` is the only permitted write.

## 10. Failure behavior

- **FB-1** Provider timeout, transport error, or unavailable provider: report
  "review not completed", exit non-zero, and write no findings artifact that could
  be read as a completed review.
- **FB-2** Malformed output: retry within the existing bound, then report "review
  not completed", preserve the raw response for diagnosis, and never report a pass.
- **FB-3** A valid response with an empty findings list is a clean review and exits
  zero. It MUST be distinguishable from a failed review.
- **FB-4** An evidence write failure is an error. SOP MUST NOT print a success
  summary after a failed write.
- **FB-5** A design-review failure MUST NOT change a task run, a plan, a gate, or an
  approval. It also MUST NOT create a false block.

## 11. Acceptance criteria

These criteria are shape-neutral. They apply to whichever command shape and request
shape OD-1 and OD-2 select; they do not require a particular one.

- **AC-1** A deterministic test asserts that, for the proposed path, the model input
  contains only the artifact and declared references, and contains no conversation,
  prior findings, or session-only planner answer. The test also asserts that `sop run`
  is unchanged.
- **AC-2** Deterministic tests cover structured parsing, the malformed-output
  boundary, and the difference between an empty findings list and a failed review.
- **AC-3** A test asserts that a design review changes no task-run artifact, no
  plan artifact, no gate decision, and no approval record.
- **AC-4** A test asserts that the evidence artifact records the path, digest,
  provider, model, rubric version, findings, and timestamp, and contains no
  credential.
- **AC-5** Tests cover path confinement, symlink rejection, and redaction.
- **AC-6** A test covers provider/model pinning through the routing surface of the
  chosen entry point, with no new configuration key.
- **AC-7** An operator can run the review on a real plan and adjudicate the
  findings from the printed output and the evidence artifact.
- **AC-8** The command, artifact, rubric version, and non-goals are documented in the
  existing reference and specification set.
- **AC-9** The assessment's protocol has been run and its outcome recorded as PASS,
  FAIL, or INCONCLUSIVE. No quality-improvement claim may be made before then.
- **AC-10** Implementation is not authorized by this PRD alone. Authorization requires
  a PASS on the relevant hypothesis and a separate operator governance decision
  recorded under the existing approval convention; no new approval system is created.

## 12. Evaluation reference and authorization boundary

The authoritative protocol is
[EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §4–§7.
It defines the question, arms A/B/C/D, the B×3 control, the corpus, repetitions, the
pilot, blinded human adjudication, metrics, statistics, PASS/FAIL/INCONCLUSIVE
outcomes, and the call budget. This PRD does not restate those values; a second copy
would drift.

What this PRD requires:

- **Outcome vocabulary.** PASS, FAIL, and INCONCLUSIVE are distinct. A PASS on H1
  (rubric) is evidence for a scoped rubric decision only. A PASS on H2
  (specialized reviewers) is evidence for a separate multi-reviewer decision only.
  FAIL and INCONCLUSIVE leave SOP unchanged and the PRD unimplemented.
- **Human adjudication.** Findings are adjudicated by a human who did not author the
  document, blind to arm and repetition. A second human adjudicates a sample. A model
  is never the sole adjudicator. A single-adjudicator limitation makes the result
  suggestive and never PASS. No approval or evidence is invented.
- **Authorization boundary.** A successful rubric experiment does not authorize
  structured findings, a disposition API, evidence persistence, or new orchestration.
  Each requires a separate governance decision recorded under the existing approval
  convention. This PRD remains conditional after a PASS.
- **No threshold drift.** The assessment's thresholds and corpus are fixed before the
  first run. If a run has begun, they are not changed here; a change requires a new,
  labelled protocol and invalidates the affected comparison.

## 13. Dependencies and risks

| Item                                          | Type       | Mitigation                                                                                                  |
| --------------------------------------------- | ---------- | ----------------------------------------------------------------------------------------------------------- |
| `internal/review` report and parser           | Dependency | Reuse them; do not fork. If a document subject needs a new request shape, keep `Report` and parsing shared. |
| Provider availability and `REVIEW` capability | Dependency | Fail as "review not completed"; do not fall back to a simulated review.                                     |
| Existing redaction and path-confinement rules | Dependency | Reuse the `sop prompt --file` rules.                                                                        |
| Rubric drift                                  | Risk       | Keep one rubric text with a version; do not copy it into multiple documents or packages.                    |
| Cost and latency                              | Risk       | Advisory, operator-invoked; measure before enabling by default.                                             |
| False confidence                              | Risk       | Advisory only; no quality claim without the assessment; empty findings is not proof of a sound design.      |
| Anchoring across rounds                       | Risk       | Exclude prior findings from input.                                                                          |
| Same-model correlation                        | Risk       | Report it as a limitation; allow pinning but do not claim statistical independence.                         |
| Duplicating `sop prompt`                      | Risk       | Add the structured path to the existing prompt/review surface; do not build a second prompt engine.         |
| Duplicating `sop review`                      | Risk       | Share provider, report, severity, and parser; differ only in subject and rubric.                            |
| Over-gating                                   | Risk       | No lifecycle, gate, or approval changes in this PRD.                                                        |

## 14. Open decisions

OD-1 and OD-2 are the only open decisions needed before a plan can be written, and
only after the header's conditions are met. The remaining decisions are resolved
during implementation or after the assessment and do not need a separate approval
now.

- **OD-1 — Command shape.** Extend `sop review` with a document mode, extend
  `sop prompt` with a structured design-review mode, or add a separate command.
  Prefer the smallest change that reuses an existing artifact layout.
- **OD-2 — Request shape.** Add an explicit subject kind to `review.Request`, or
  compose a new request type that shares `review.Report`. Do not overload `Diff`.
- **OD-3 — Single or fan-out.** Start with one reviewer. Fan-out is justified only
  by the assessment's H2 result.
- **OD-4 — References.** Require an explicit reference list, or allow the reviewer
  to read paths named in the document. Automatic repo-wide retrieval is out of
  scope.
- **OD-5 — Gate.** Keep the review advisory in the first version. A blocking policy
  is a separate decision.
- **OD-6 — Evidence location.** Reuse the prompt-run or task-run artifact directory,
  or add a design-review subdirectory under `.agent-sdlc`. No new top-level state
  root.
- **OD-7 — Disposition ownership.** Keep dispositions in the review artifact, or
  attach them to the approval record. Do not create a second approval system.
- **OD-8 — Rubric ownership.** Keep the rubric as one versioned text owned by one
  document or one constant. Do not restate it in several places.

## 15. References

- [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md) —
  the authoritative capability comparison and pre-registered experiment protocol.
- [REVIEW.md](../specs/REVIEW.md) — the review pipeline, bounded input, malformed
  output, and self-review focus.
- [PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) — read-only prompt execution,
  artifacts, structured results, and prompt security.
- [HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) — the human gate.
- [QUALITY.md](../specs/QUALITY.md) — the verdict and blocking-finding rule.
- [MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) — stage model and provider pinning.
- [OVERVIEW.md](../architecture/OVERVIEW.md) — the review architecture.
- [COMPONENTS.md](../architecture/COMPONENTS.md) — the run artifact set.
- [PACKAGES.md](../reference/PACKAGES.md) — the `review` package contract.
- [JEV-DECISION-CONTRACT.md](../architecture/JEV-DECISION-CONTRACT.md) — why the
  decision adapters are a separate boundary.
- [CLI.md](../reference/CLI.md) — the current `sop review` and `sop prompt` surface.
- [plans/README.md](../plans/README.md) — plan status and the proposed/unapproved
  convention.
- [PRD.md](../PRD.md) — the canonical product requirements.
