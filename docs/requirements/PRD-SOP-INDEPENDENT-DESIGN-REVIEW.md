# SOP Independent Design Review

Status: Proposed, not approved • Version: 0.1 • Date: 2026-11-21

**Type:** Product requirements for a proposed capability. This document changes no
behavior. The normative behavior of the existing review pipeline is owned by
[REVIEW.md](../specs/REVIEW.md); the prompt path by
[PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md); the human boundary by
[HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md); and the quality gate by
[QUALITY.md](../specs/QUALITY.md). Where this document and a specification differ,
the specification owns current behavior.

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
   the `sop run` review path and `sop prompt --capability review` are stateless, and
   the code-review input is bounded to the task and change set. Isolation is already
   implemented and is not the gap.
2. **Design-stage review.** Does anything challenge the design before
   implementation? In SOP, no. The review capability is scoped to a change set, and
   planning has no semantic critic.

This PRD addresses the second question only. It proposes the smallest enhancement
that reuses the existing review boundary, adds no lifecycle stage, and makes no
claim of improved quality until the evaluation in §12 has been run.

## 2. Current behavior and evidence

### 2.1 What exists

| Capability                                             | State                                        | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------------------------------------ | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Stateless reviewer input                               | **Implemented**                              | `internal/review/review.go` defines `Request` and `Request.text()`; `internal/review/provider.go` makes one bounded `agent.Generate` call and retries only malformed output. `sop prompt` also makes one bounded call (`internal/cli/prompt_cache.go`).                                                                                                                                                                             |
| Structured findings and deterministic severity         | **Implemented**                              | `review.Finding`, `review.Report`, and `Report.Blocking` (`internal/review/review.go`); `reviewOutputSchema` and `parseReport` (`internal/review/provider.go`); `quality.BlockingFindings` and `quality.Evaluate` (`internal/quality`).                                                                                                                                                                                             |
| Findings persisted for code review                     | **Implemented**                              | `sop run` writes `review.json` with the full report (`internal/cli/run.go`), and `report.json` carries the findings (`runReportDoc.Findings`). [COMPONENTS.md](../architecture/COMPONENTS.md) documents the artifact set.                                                                                                                                                                                                           |
| Malformed-output and infrastructure-failure boundaries | **Implemented**                              | Bounded retry and `ErrMalformedOutput` (`internal/review/provider.go`); malformed output maps to the human boundary, never a pass (`internal/cli/run.go`); [REVIEW.md](../specs/REVIEW.md) §6.                                                                                                                                                                                                                                      |
| Implementation-review rubric                           | **Implemented**                              | `reviewTask` (`internal/review/provider.go`) drives goal fit, correctness, complexity, tests, and change justification; [REVIEW.md](../specs/REVIEW.md) §7.                                                                                                                                                                                                                                                                         |
| Review model/provider pinning                          | **Implemented**                              | `models.stages.review` or `SOP_MODEL_STAGE_REVIEW_PROVIDER` and `_NAME` (`internal/cli/stage_models.go`; [MODEL-ROUTING.md](../specs/MODEL-ROUTING.md)); a pin selects a model and never a gate.                                                                                                                                                                                                                                    |
| Plan schema and deterministic checks                   | **Implemented**                              | `planner.Plan.Validate`, capability inventory with evidence, assumptions with evidence, acceptance criteria and checks (`internal/planner/plan.go`); `planner.Lint` warns on deliverables without acceptance checks (`internal/planner/lint.go`).                                                                                                                                                                                   |
| Design-stage semantic review                           | **Missing**                                  | `sop plan` performs one `planner.Generate` plus schema repair and writes `PLAN.md` and `plan.json` (`internal/cli/plan.go`); no review provider is invoked. `sop plan activate` validates structure only (`internal/cli/plan.go`).                                                                                                                                                                                                  |
| Ad-hoc design critique                                 | **Partial**                                  | `sop prompt --capability review --file <path>` runs a read-only prompt, but it bypasses `review.Provider`: `promptOutputRequirements` returns prose guidance (`internal/cli/prompt.go`), not the JSON findings schema. It writes `result.md` and `metadata.json` ([PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) §9), so the raw text is durable but is never parsed, severity-classified, or recorded as structured findings. |
| Per-finding disposition                                | **Partial**                                  | The code-review fix loop consumes blocking findings, and the human approval gate records the decision; no per-finding `accepted`/`contested`/`declined` record exists.                                                                                                                                                                                                                                                              |
| Fan-out and de-duplication                             | **Missing; unnecessary for a first version** | SOP can issue several review calls, but no merge step exists. A single reviewer needs no de-duplication.                                                                                                                                                                                                                                                                                                                            |
| Decision-adapter integration                           | **Not applicable**                           | The CLEF, Nimble, and Julia adapters answer structured JEV decisions and are not review providers ([JEV-DECISION-CONTRACT.md](../architecture/JEV-DECISION-CONTRACT.md) §1). Wiring them into review would confuse two different boundaries.                                                                                                                                                                                        |

### 2.2 Evidence method

Every current-state claim below was traced in this working tree at baseline commit
`1e8cb005` by reading the named file and function. The review input for the
proposal itself was documentation-only, so the review could not open those files;
the citations are for the reader, not a substitute for the source. No behavior was
run to produce these claims; they describe code paths and call sites as written.

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
result for design review. The evaluation in §12 exists to replace these hypotheses
with evidence or to reject the capability.

## 3. Prior art (verified primary sources)

The comparison below was taken from the repositories at these commits. The source
files are named so a reader can check each claim without installing the tool. Each
claim is limited to the files read at that commit; the tools were not installed or
run, and no behavior beyond those files is asserted.

- Elephant-Goldfish Model (EGM): `google/elephant-goldfish-model` @
  `34ff1179bca663c61571d6ae189e02a1ac63d652`. Sources: `README.md`,
  `docs/designs/review.md`, `skills/egm-review-loop/SKILL.md`, and `TESTING.md`.
- Superpowers: `obra/superpowers` @
  `22f0b463e7a5e579904dd6211a520321035526f7`. Sources: `README.md`,
  `skills/writing-plans/SKILL.md`, and `skills/requesting-code-review/SKILL.md`.
- OpenSpec: `Fission-AI/OpenSpec` @
  `0c737dc1b543249b38b0f4716816fcf493d894dd`. Sources: `README.md` and
  `skills/openspec-verify-change/SKILL.md`.

| Capability                                      | SOP                                                                                                                                                | EGM                                                                                  | Superpowers                                                  | OpenSpec                                                  |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------------ | --------------------------------------------------------- |
| Reviewer without the author's conversation      | Implemented                                                                                                                                        | Implemented; a harness without fresh subagents stops the review                      | Implemented for code review; the plan check is a self-review | No fresh-context requirement in the verify skill          |
| Design/plan critique rubric                     | Missing                                                                                                                                            | Implemented; personas plus document standards                                        | Partial; a plan self-review checklist and a builder check    | Partial; templates and conformance checks, not critique   |
| Challenge to architectural assumptions          | Missing                                                                                                                                            | Implemented; `zoom_out` and `design_critic`                                          | Partial; intent discovery and builder check                  | Missing                                                   |
| Structured findings with deterministic severity | Implemented; JSON schema and `fail_on` policy                                                                                                      | Partial; structured prose with emoji severity                                        | Partial; prose by severity                                   | Partial; markdown report with CRITICAL/WARNING/SUGGESTION |
| Durable finding evidence                        | Implemented; `review.json`                                                                                                                         | Implemented; `goldfish_review_report.md`                                             | Missing as a required artifact                               | Partial; report plus archive                              |
| Per-finding disposition                         | Partial; the bounded fix loop consumes blocking findings and the human gate records the decision, but no per-finding contest/decline record exists | Implemented; `PROPOSE FIX` / `CONTEST` / `ESCALATE` / `DECLINE` and settled findings | Partial; prose push-back                                     | Partial; fix before archive                               |
| Bounded review rounds                           | Implemented                                                                                                                                        | Implemented; two clean rounds, cap five, churn check                                 | Partial; per-task reviews                                    | Not applicable                                            |
| Fan-out and de-duplication                      | Missing; unnecessary for one reviewer                                                                                                              | Implemented; synthesizer de-duplicates and assigns stable IDs                        | Missing                                                      | Missing                                                   |
| Review model/provider independence              | Partial; configurable stage pin, same run model by default                                                                                         | Harness-dependent; no model-diversity requirement                                    | Partial; the final reviewer can use a stronger model         | Not applicable                                            |
| Failure handling                                | Implemented                                                                                                                                        | Implemented; a failed reviewer is recorded as not run, never as zero findings        | Partial                                                      | Partial; explicit not-verified states                     |
| Specification/change artifact lifecycle         | Implemented                                                                                                                                        | Partial; design documents                                                            | Partial; plan documents                                      | Core                                                      |
| Deterministic lifecycle control                 | Implemented                                                                                                                                        | Missing; agent-driven                                                                | Missing; agent-driven                                        | Partial; CLI-validated                                    |

### 3.1 What the comparison means

- **EGM's isolation is not new to SOP.** A stateless `review.Request` already
  prevents the reviewer from seeing the author's context. Adopting EGM's subagent
  protocol would duplicate an existing property.
- **EGM's distinctive value is the rubric, the fan-out, and the resolution
  engine.** SOP has no design rubric. It has no merge step, but it also has no
  fan-out, so a merge step would be speculative. SOP has a bounded fix loop and a
  human gate, but no per-finding contest record.
- **Superpowers reviews code with a fresh subagent, not designs with one.** Its
  plan check is explicitly self-review. Its plan-quality leverage comes from the
  plan's structure and the `Review Focus` section, not from an independent design
  critic.
- **OpenSpec is an artifact lifecycle, not a reviewer.** `openspec-verify-change`
  checks completeness, correctness, and coherence after implementation, in the
  authoring context. Adopting its artifact model would create a second source of
  truth beside SOP's plans and specifications.
- **None of the three documents read here publishes a measured design-review
  effectiveness result.** EGM's testing playbook records installation, isolation,
  stage order, and reviewer completion; it does not publish defect-detection
  precision or recall in the files read. Superpowers records evaluations for
  several skills, but not for design-review detection in the files read. OpenSpec's
  verification is conformance, not defect detection.

## 4. Goals and non-goals

**Recommendation: B — minimal enhancement.** The design-stage gap is real, but it
does not justify a new capability, engine, or lifecycle stage. A narrow read-only
review that reuses the existing review provider, report, severity, and parser
closes the observed gap. A new capability (C) would duplicate `review`; no change
(A) would leave the design stage without structured, recorded review evidence. The
evaluation in §12 gates any claim that the enhancement improves quality.

### 4.1 Goals

- **G1** Let an operator obtain an independent, design-stage review of a plan,
  PRD, or specification before implementation.
- **G2** Ask design questions that the implementation review does not ask:
  assumptions, failure modes, security, testability, readiness, and scope.
- **G3** Return findings in the existing structured report so severity policy and
  downstream reporting are not duplicated.
- **G4** Record the review and the operator's disposition as durable evidence.
- **G5** Preserve independence: the reviewer reads the artifact and explicit
  references, not the authoring conversation.
- **G6** Keep the capability advisory and outside the lifecycle: no new gate, no
  new state, no new approval system.

### 4.2 Non-goals

- **N1** Replacing `sop review`, the fix loop, the quality gate, or the human
  approval gate.
- **N2** Blocking plan activation or approval. A blocking design gate needs a
  separate decision and a separate PRD.
- **N3** Automatically rewriting plans or applying fixes from design findings.
- **N4** A multi-persona reviewer swarm, a de-duplication engine, or a round-based
  convergence protocol in the first version.
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
- **FR-3 — Independent input.** The model input MUST be the artifact text plus only
  explicitly declared reference material. It MUST NOT include the authoring
  conversation, session-only planner answers, decision memory, retrieval context,
  or prior review findings. If a planner decision is material, it belongs in the
  artifact or in the declared references.
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
  provider that declares `REVIEW`. Operator pinning MUST use the existing
  stage/model configuration; no new configuration key is required.
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

## 8. Review independence requirements

- **IR-1** The reviewer MUST NOT receive the authoring conversation, the model's
  tool transcript, or hidden reasoning.
- **IR-2** The input MUST be reproducible from repository content: the artifact
  plus declared references. Session-only material MUST NOT be injected silently.
- **IR-3** References MUST be explicit. Automatic repository-wide retrieval is NOT
  part of the first version.
- **IR-4** Prior review findings, dispositions, and fix history MUST NOT be part of
  the input, so a later round is not anchored by an earlier one.
- **IR-5** A different model or provider MAY be pinned through the existing stage
  configuration. It is not required, and SOP MUST NOT claim that a same-model review
  is independent in the statistical sense.
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

- **AC-1** A deterministic test asserts that the model input contains only the
  artifact and declared references, and contains no conversation, prior findings,
  or session-only planner answer.
- **AC-2** Deterministic tests cover structured parsing, the malformed-output
  boundary, and the difference between an empty findings list and a failed review.
- **AC-3** A test asserts that a design review changes no task-run artifact, no
  plan artifact, no gate decision, and no approval record.
- **AC-4** A test asserts that the evidence artifact records the path, digest,
  provider, model, rubric version, findings, and timestamp, and contains no
  credential.
- **AC-5** Tests cover path confinement, symlink rejection, and redaction.
- **AC-6** A test covers provider/model pinning through the existing stage
  configuration, with no new configuration key.
- **AC-7** An operator can run the review on a real plan and adjudicate the
  findings from the printed output and the evidence artifact.
- **AC-8** The command, artifact, rubric version, and non-goals are documented in the
  existing reference and specification set.
- **AC-9** The evaluation in §12 is complete and reported. No quality-improvement
  claim may be made before then.

## 12. Evaluation plan

### 12.1 Question

Does a design-stage review find material design defects that the current SOP path
does not find, at an acceptable cost? Because isolation is already present in the
baseline, the experiment holds isolation constant and varies the rubric, the
number of scoped reviewers, and model diversity.

### 12.2 Arms

- **A — baseline.** The existing ad-hoc path: one `sop prompt --capability review`
  call on the document with a generic request.
- **B1 — rubric only.** One fresh-context call with the design rubric from FR-4.
- **B2 — rubric plus scoped reviewers.** The same rubric split across three
  reviewers: assumptions and design; failure, security, and reliability;
  testability and implementation readiness. Findings are merged by the operator
  for the experiment; no merge code is built.
- **B3 — optional diversity.** B1 with a different provider or model class pinned
  through the existing stage configuration. Run only if pinning is available
  without configuration changes.

### 12.3 Inputs and ground truth

- About 12–15 historical plans, PRDs, or specifications from the repository.
  Candidate inputs must be verified before use; a later correction does not by
  itself prove that a review would have found the defect.
- Five documents with one seeded defect each, analyzed separately as a calibration
  set: an unstated assumption, a referenced symbol that does not exist, a missing
  failure mode, an untestable acceptance criterion, and a scope violation.
- Ground truth is written down before any run: for each document, the material
  issues that a competent reviewer should find. A material issue is one that would
  change scope, correctness, safety, testability, or the implementation approach.
- The author of a document does not adjudicate that document.

### 12.4 Metrics

| Metric                   | Definition                                                                |
| ------------------------ | ------------------------------------------------------------------------- |
| Confirmed valid findings | Findings a blind adjudicator marks valid and material.                    |
| False positives          | Findings marked invalid, immaterial, or duplicates of another finding.    |
| Material issues missed   | Ground-truth material issues not raised by the arm.                       |
| Finding overlap          | Share of confirmed findings raised by more than one reviewer or arm.      |
| Token/inference cost     | Provider-reported tokens per document; spend where a provider reports it. |
| Review latency           | Wall-clock time per document, with parallel calls noted.                  |
| Operational complexity   | Number of commands, artifacts, and manual merge steps per document.       |
| Reproducibility          | Share of confirmed findings recurring across three repeats.               |

### 12.5 Adjudication

- Arm labels are removed and documents are presented in random order. The
  adjudicator does not know which arm produced a finding.
- A finding is judged against the pre-registered ground truth and against the
  document itself, not against the reviewer's confidence.
- Where only one human adjudicator is available, that limitation is recorded and a
  second person reviews a sample. A model MUST NOT be the sole adjudicator.
- Disagreements are resolved by discussion or a third adjudicator and are reported
  as disagreements, not averaged away.

### 12.6 Decision rule

Proposed thresholds, fixed before the run:

- **Rubric:** adopt B1 only if it finds at least 25% more confirmed material issues
  than A (and at least two more in absolute terms), its false-positive rate is no
  more than 10 percentage points above A, and its cost is no more than 2× A.
- **Fan-out:** adopt B2 only if it adds at least 15% more confirmed material issues
  than B1 at no more than 3× A. Otherwise a single reviewer remains the default and
  de-duplication is not built.
- **Diversity:** attribute a B3 gain to diversity only if B1 and B3 use the same
  rubric and the only difference is the model or provider.
- **Rejection:** a tie, a loss, or a cost above the thresholds means no capability
  is adopted. A pass is evidence for this input set, not proof of general
  effectiveness.

### 12.7 Threats to validity

- Small sample: with about 15 documents, only a large effect is detectable.
- Ground-truth bias: the adjudicator's judgement is the main measurement
  instrument; blinding reduces but does not remove bias.
- Model-capability confound: a stronger reviewer model can explain a gain that the
  rubric did not cause.
- Historical-defect bias: later corrections may be unrelated to the reviewed
  document or may have been found by tests rather than review.
- Cost comparability: token counts across providers are not directly comparable;
  spend is reported where available and the limitation is stated.
- Seeded defects are artificial and are reported separately from historical inputs.

## 13. Dependencies and risks

| Item                                          | Type       | Mitigation                                                                                                  |
| --------------------------------------------- | ---------- | ----------------------------------------------------------------------------------------------------------- |
| `internal/review` report and parser           | Dependency | Reuse them; do not fork. If a document subject needs a new request shape, keep `Report` and parsing shared. |
| Provider availability and `REVIEW` capability | Dependency | Fail as "review not completed"; do not fall back to a simulated review.                                     |
| Existing redaction and path-confinement rules | Dependency | Reuse the `sop prompt --file` rules.                                                                        |
| Rubric drift                                  | Risk       | Keep one rubric text with a version; do not copy it into multiple documents or packages.                    |
| Cost and latency                              | Risk       | Advisory, operator-invoked; measure before enabling by default.                                             |
| False confidence                              | Risk       | Advisory only; no quality claim without §12; empty findings is not proof of a sound design.                 |
| Anchoring across rounds                       | Risk       | Exclude prior findings from input.                                                                          |
| Same-model correlation                        | Risk       | Report it as a limitation; allow pinning but do not claim statistical independence.                         |
| Duplicating `sop prompt`                      | Risk       | Add the structured path to the existing prompt/review surface; do not build a second prompt engine.         |
| Duplicating `sop review`                      | Risk       | Share provider, report, severity, and parser; differ only in subject and rubric.                            |
| Over-gating                                   | Risk       | No lifecycle, gate, or approval changes in this PRD.                                                        |

## 14. Open decisions

Only **OD-1** and **OD-2** are needed before implementation can be planned. The
remaining decisions are resolved during implementation or after the evaluation and
do not need a separate approval now.

- **OD-1 — Command shape.** Extend `sop review` with a document mode, extend
  `sop prompt` with a structured design-review mode, or add a separate command.
  Prefer the smallest change that reuses an existing artifact layout.
- **OD-2 — Request shape.** Add an explicit subject kind to `review.Request`, or
  compose a new request type that shares `review.Report`. Do not overload `Diff`.
- **OD-3 — Single or fan-out.** Start with one reviewer. Fan-out is justified only
  by the §12 result.
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
