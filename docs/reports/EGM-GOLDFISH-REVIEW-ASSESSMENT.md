# Assessment: independent design review (EGM) and a pre-registered evaluation

**Type:** Assessment report and the **authoritative, pre-registered experiment
protocol**. It is not a specification, defines no behavior, and adds no source of
truth. Behavior is owned by [REVIEW.md](../specs/REVIEW.md),
[PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md),
[MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) and
[HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md).

**Related proposal:**
[PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md](../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md)
is a conditional, unapproved implementation contract. It is not authorization, does
not restate this protocol, and must not be implemented unless this assessment returns
PASS on the relevant hypothesis and the operator records a separate governance
decision.

**Status:** Nothing has been run. No claim is made that any approach improves review
quality. PASS, FAIL and INCONCLUSIVE are distinct outcomes; only PASS can support a
scoped implementation decision, and PASS alone does not authorize structured
findings, disposition APIs, or new orchestration.

**Basis:** SOP `origin/main` @ `ad8a9c6c`. The code paths are unchanged since the
`1e8cb005` baseline; `git log 1e8cb005..ad8a9c6c` contains only the two documentation
merges. References were read as shallow clones outside the checkout: EGM `34ff1179`
(an earlier draft read `8e2a5ed`; the claims below were re-checked at `34ff1179`),
Superpowers `22f0b46`, OpenSpec `0c737dc`. Nothing was installed.

## 1. Verified architecture

Line references were verified at `ad8a9c6c`; symbol names are authoritative if lines
drift. The code paths are unchanged from `1e8cb005`.

### 1.1 Review of a change (`provider.Review`)

- `review.Request` has the fields `Task`, `Criteria`, `Diff`, `TestSummary`
  (`internal/review/review.go:64-70`), rendered by `Request.text()` (`:78`).
- The three production call sites set only `Task` and `Diff`:
  - `sop run`: `Task` is the rendered task spec plus the run's earlier planner
    decisions (`internal/cli/run.go:1021,1035`, `withPlannerDecisions` in
    `internal/cli/planner.go:595`);
  - `sop review`: `Task` is a fixed string (`internal/cli/review.go:64`);
  - MCP review: `Task` is a fixed string (`internal/cli/mcp.go:427`).
- `AgentProvider.Review` makes one `agent.Generate` per attempt, with the fixed
  `reviewTask` instruction (`internal/review/provider.go:25,120-139`). Attempts repeat
  only to recover malformed output (bound 3).
- The reviewer is asked whether the change satisfies the task. It is not asked to
  challenge the task (`provider.go:25-36`, `REVIEW.md` §7.1).

### 1.2 Ad-hoc review (`sop prompt --capability review`)

- Flags: `--capability`, `--model-class`, `--file`, `--json`, `--cache`,
  `--deliverable` (implement only), `--accept`; at most one of a positional prompt and
  `--file` (`internal/cli/prompt.go:51,103-177`).
- `--file` must resolve, after symlink resolution, inside the project directory
  (`prompt.go:299-337`, `PROMPT-EXECUTION.md` §12). An experiment file therefore has to
  live inside the project being run.
- Request composition on the read-only path: `agent.Request{Capability, Task:
wi.Content, OutputRequirements}` with `OutputRequirements` fixed to "Return concise
  review findings with severities. Do not modify the repository."
  (`internal/cli/prompt_cache.go:20-27`, `prompt.go:667-672`). `wi.Content` is the
  operator's text or file, verbatim. No context-engine, memory or retrieval input is
  added on this path; `prompt.Compile` is used only to compute the optional cache key.
- Read-only prompts make one bounded call, with no escalation and no quality gate
  (`PROMPT-EXECUTION.md` §6, §8).
- Model selection: the same deterministic router as a task. It does not read prompt
  text; without typed JEV evidence it resolves to MEDIUM; `--model-class` overrides it
  (`PROMPT-EXECUTION.md` §5). Provider and model come from `models.<class>.*` config
  and the `SOP_AGENT_PROVIDER` / `SOP_AGENT_MODEL` variables
  (`internal/agent/provider.go`). The precedence of those variables against
  class routing for a prompt was **not** traced and must be confirmed by the
  `routing` section of `metadata.json` in the pilot (§6.4).
- Observability: `metadata.json` carries `routing` (class, provider, model) and
  `tokens` by model "as the servers reported it", omitted when no call reported usage
  (`prompt.go:775-788`). No duration field was found; latency has to be timed outside
  SOP.
- Sampling controls: a search of `internal/agent` and `internal/ollamaagent` for
  temperature, seed and top_p found none. Sampling is whatever the provider defaults
  to, so reproducibility must be measured, not assumed.
- With the Ollama harness, REVIEW runs a bounded read-only inspection of the
  repository before synthesis, and the harness adds its own system prompt
  (`internal/ollamaagent/review.go`, `orchestrate.go:61`). A text-only provider has no
  repository access.

### 1.3 Planning and approval

- `plan` is a read-only capability; plan acceptance is deterministic evidence
  (`internal/planacceptance`); approval is the existing human gate. No model critique
  of a finished plan was found in a search of `docs`, `skills` and `internal`. A search
  is not proof of absence.

## 2. What context independence is proven

**Proven by the code above:**

1. No call site builds a conversation transcript into the review input. `agent.Request`
   has no conversation-history field (`internal/agent/agent.go:43-60`).
2. On the ad-hoc path the only content is the operator's text, plus the harness's own
   system prompt.
3. On the `sop run` path the input is the task spec, planner decisions and the diff.

**Not proven, and not claimed:**

- That reviewers lack the author's assumptions. The `sop run` review input includes
  the planner's questions and answers, so some design decisions are visible to the
  reviewer by design.
- That provider-side state is absent. A `command` provider can wrap an external agent
  that keeps its own memory; this was not examined.
- That repository tools do not leak the answer. Under the Ollama harness the reviewer
  can read the repository, so "document only" holds only for text-only providers.
- That a fresh context on the same model avoids the author model's blind spots.
  Nothing in SOP or EGM measures this.

**Gap that survives these limits:** no SOP path asks a model to challenge the design
itself. Code review takes the task as given; the ad-hoc path has no design rubric.

The linked PRD proposes a **separate design-review path**. Its independence
requirements are narrower than the existing `sop run` path: the PRD requires the
artifact and explicit references only, while `sop run` includes the task and applied
planner decisions by design. A claim about the proposed path must not be read as a
claim about the existing path, or the reverse. The PRD is conditional and does not
authorize implementation.

## 3. Capability comparison

| Capability                                | SOP                                                             | EGM                                       | Superpowers                                               | OpenSpec                                               |
| ----------------------------------------- | --------------------------------------------------------------- | ----------------------------------------- | --------------------------------------------------------- | ------------------------------------------------------ |
| Reviewer without the authoring transcript | Implemented (limits in §2)                                      | Core mechanism (`docs/designs/review.md`) | Fresh subagents (`brainstorming/builder-check-prompt.md`) | Not a review tool                                      |
| Design-level review rubric                | Missing                                                         | Persona rubrics (`agents/*.md`)           | One builder-check prompt                                  | `/opsx:verify` checks implementation against artifacts |
| Naive-reader paraphrase check             | Missing                                                         | `comprehension` persona                   | Builder check, similar                                    | Missing                                                |
| Findings never decide the verdict         | Implemented (`REVIEW.md` §3)                                    | Human triage                              | Controller                                                | n/a                                                    |
| Finding triage and contest                | Partial (fix loop, human approval)                              | `egm-resolve`                             | `receiving-code-review`                                   | n/a                                                    |
| Bounded rounds                            | Implemented                                                     | Stop after 2 clean rounds, ask at 5       | Per skill                                                 | n/a                                                    |
| Structured findings                       | Implemented for `sop run`; the ad-hoc prompt path returns prose | Prose reports                             | Prose                                                     | n/a                                                    |
| Spec and change artifacts, archive        | Implemented (`docs/specs`, `plan.json`)                         | n/a                                       | n/a                                                       | Core                                                   |

**Gaps:** G1 no design-level rubric; G2 no way to run several scoped reviewers and
merge their findings; G3 no measurement of whether any review path finds design
defects.

**Not worth adopting:** OpenSpec's artifact model (a second source of truth);
Superpowers' agent-driven process control; EGM's interactive triage modes (SOP has the
approval gate) and its style/structure personas.

EGM's own quality claims are unproven here: in the scenario file read
(`skills/egm-review/test_scenarios.yaml`, first ~50 lines, at `34ff1179`) the
assertions are pattern checks on flow and wording, not measured defect detection. The
remainder was not read. This table is the authoritative capability comparison; the
PRD summarizes it and links here rather than maintaining a second matrix.

## 4. Experiment: question and arms

**Question.** Does a design-stage review find more genuine design defects than SOP's
existing generic ad-hoc review, at acceptable cost? Isolation is the same in every arm
because the baseline already has it, so the experiment varies the rubric, the number of
scoped reviewers, and the model.

Every arm is `sop prompt --capability review --file <request>` run in the scratch
project of §5.2, with no `--cache` and JEV triage disabled. Arms differ only in the
request file.

| Arm                           | Request                                                                                                                                                                                | Calls per plan     |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------ |
| A: generic                    | Fixed one-paragraph generic instruction, then the plan                                                                                                                                 | 1                  |
| B: rubric                     | One frozen design rubric (assumptions stated; failure modes; referenced symbols exist; acceptance criteria verifiable; scope and reuse), then the plan                                 | 1                  |
| C: specialized                | Three frozen reviews that split the same rubric (assumptions and design; failure, security and reliability; testability and implementation readiness), then the plan; findings unioned | 3, run in parallel |
| D: alternate model (optional) | Arm B and Arm C on a different provider or model class                                                                                                                                 | as B, C            |

**Controls.**

- C is compared with the union of three independent B runs (B×3 control), so a gain
  from simply sampling three times is not credited to specialization.
- D is run only if a different provider or class can be pinned through existing
  `models.<class>.*` config. It is exploratory; a D gain is attributed to model
  diversity only if B and D both ran under the identical request.

**B×3 control definition.** The control is three independent B calls per plan per
repetition, not the union of the three repetitions. The H1 B run MAY serve as one of
the three control runs (7 calls per plan per repetition) or be separate (8 calls).
The choice is fixed before the pilot and recorded in the run manifest. This is a
design clarification, not a threshold change; nothing has been run.

**Rubric provenance.** Rubrics are written for this experiment from the published
method, frozen, and hashed before any run. EGM persona files are not installed or
vendored. If their text is quoted, it is credited (Apache-2.0).

## 5. Protocol

### 5.1 Identical inputs and controlled settings

- Every arm receives byte-identical plan content (sha256 recorded). Request files
  differ only in the instruction block, which is delimited from the plan.
- Plan text is untrusted data passed as the request; the rubric is the operator's.
- Controlled and recorded per run: SOP binary commit; harness; provider, model and
  endpoint; `--model-class` (set explicitly in every call); provider-reported
  parameters where available. Because SOP exposes no sampling controls, repetition (§5.5)
  carries the variance measurement.
- One model per arm for the whole experiment. A provider or model change
  mid-experiment invalidates the affected runs.
- Run order is randomized and interleaved across arms and repetitions so provider drift
  is not confounded with arm. The full-run order is generated and recorded in the run
  manifest before the first full-run call. The pilot is excluded; if the pilot changes
  the protocol, the full run starts from a new frozen manifest.

### 5.2 Isolation from the answer (leakage control)

- The experiment runs in a throwaway clone or worktree, never the primary checkout, so
  `sop prompt` run artifacts under `.agent-sdlc/` stay out of it.
- For a historical plan, the clone is checked out at the commit before the defect was
  discovered, so a later fix, erratum or history entry is not visible to a reviewer
  with repository access. If the harness has repository tools, this is mandatory;
  otherwise it is recorded as a precaution.
- Plans are 2026 documents, so models trained earlier cannot have seen them. A model
  with later training data is recorded as a contamination risk.

### 5.3 Evaluation corpus

Fixed and written down before the first run:

- **Historical set (target 12-15):** plans, specs or requirements whose defect later
  appeared in merged fixes, erratum, reverted change or recorded gap. Each defect has an
  evidence pointer (commit, PR or file). Candidates to verify, not yet verified:
  `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md`,
  `docs/reports/PLAN-STATUS-AUDIT-2026-11-04.md`,
  `docs/reports/CONV-001-convergence-baseline.md`. These three paths resolved in the
  baseline tree at `ad8a9c6c`; existence is not evidence of the defect. The final corpus
  frozen only after each path, parent revision and evidence pointer is checked; an
  unresolved path is replaced, not shipped. A later correction does not by itself show
  that a review could have found the defect; the adjudicator records whether the defect
  was determinable from the document and its parent revision.
- **Seeded set (about 8):** one defect each: unstated assumption, nonexistent symbol,
  missing failure mode, unverifiable acceptance criterion, scope violation, plus three
  more of the same classes. Seeded by someone other than the adjudicator, recorded in a
  sealed manifest before any run.
- **No-known-defect set (about 6):** plans with no recorded defect, to measure false
  positives where nothing is expected. "No known defect" is not "no defect"; findings
  here are adjudicated, not assumed invalid.
- **Defect record:** id, plan path and sha256, parent commit, location, description,
  class, severity (blocks implementation / changes approach / minor), evidence pointer,
  source (historical or seeded). Minimum to run: 15 known defects across at least 15
  plans; below that, every result is INCONCLUSIVE.

### 5.4 Blinded adjudication, deduplication, false positives

- Findings are exported with arm, repetition, model and run ids removed, shuffled per
  plan, and presented in that order.
- For each finding the adjudicator records one of: matches known defect `<id>`; valid
  and not on the list (needs a one-line justification from the document); invalid or
  immaterial; duplicate of finding `<n>`. Judgment is against the document and the
  pre-registered defect list, never the reviewer's stated confidence.
- Deduplication is therefore done by the adjudicator and is the only merge step. No
  merge tooling is built.
- The adjudicator is not the author of the plan or the seeder of its defect.
- **Independence.** A human adjudicates. A second person independently adjudicates a
  random 20% sample of findings and agreement is reported (Cohen's kappa). A model may
  serve only as a third check, never as the sole adjudicator. If only one human is
  available, that limitation is recorded and the best attainable outcome is "suggestive",
  never PASS.
- **Adjudication is evidence work, not an approval.** It creates no SOP approval
  record and changes no lifecycle state. The protocol records findings and judgments;
  it does not record an operator decision to implement.
- **False-positive rate** is invalid plus immaterial findings divided by all findings,
  per arm and separately on the no-known-defect set. "Valid and not on the list" counts
  as a true positive but is reported separately and is not used in the recall numerator.
  The adjudicator assigns one class per finding; a duplicate that repeats an invalid
  finding is recorded as a duplicate and is not counted a second time in the
  false-positive numerator.

### 5.5 Repetition

- Three repetitions per plan per arm, random order. Recall for a plan is the mean
  across repetitions; the union across repetitions is reported separately.
- Reproducibility is the share of confirmed findings that recur in at least two of
  three repetitions.
- A failed call (timeout, transport error, unparseable result) is recorded as a failed
  run and counted, not dropped, and is not retried beyond what SOP itself does
  (`REVIEW.md` §6.4: a failure to review is not a finding).

### 5.6 Metrics

| Metric                 | Definition                                                                                                         |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Recall                 | known defects matched ÷ known defects, per arm                                                                     |
| Precision              | confirmed findings ÷ all findings                                                                                  |
| False-positive rate    | per §5.4                                                                                                           |
| Token cost             | tokens from `metadata.json` per plan (all calls of the arm, summed); cost per confirmed defect                     |
| Latency                | wall-clock around the command, timed outside SOP; for C, the parallel batch, with the sequential sum also reported |
| Reproducibility        | per §5.5                                                                                                           |
| Adjudication agreement | kappa on the double-adjudicated sample                                                                             |

If a provider does not report tokens, cost for that arm is recorded as unavailable and
the cost criterion is INCONCLUSIVE. Output size in characters may be reported as a
labelled proxy, not as cost.

### 5.7 Statistics

- The unit of analysis is the plan. Differences are paired by plan.
- Confidence intervals are 90% cluster-bootstrap intervals over plans (at least 10,000
  resamples). With about 15 defects the intervals will be wide; that is expected and
  drives the INCONCLUSIVE outcome.
- Primary hypothesis H1: B has higher recall than A. Secondary H2: C has higher recall
  than the B×3 control. H3 (D) is exploratory and cannot by itself change a
  recommendation.

## 6. Acceptance thresholds and outcomes

Thresholds are fixed before the first run. The values below are proposals to be
approved before it.

### 6.1 Validity conditions (checked first)

The experiment is **INCONCLUSIVE** for the affected comparison if any of these holds:

- fewer than 15 known defects, or fewer than 15 plans;
- kappa below 0.6 on the double-adjudicated sample, or no second adjudicator and the
  claim would be PASS;
- more than 10% of runs failed, or arms have unequal failure rates by more than 5
  points;
- `metadata.json` routing differs from the intended provider, model or class for any
  run in the comparison;
- token counts unavailable (cost criterion only);
- the order, corpus, rubric or thresholds changed after the first run.

### 6.2 Outcome rules, applied in this order

1. **FAIL (hard cap).** Any of these at the point estimate: false-positive rate more
   than 10 points above A, or above 50% absolute; token cost above 1.5× A (B) or 3.5×
   A (C); median latency above 1.5× A (B) or 2× A (C).
2. **PASS.** Recall gain at least 25% relative and at least 3 more confirmed known
   defects than the comparator, the lower bound of the 90% interval above zero, and
   reproducibility of at least 70%.
3. **FAIL (no useful effect).** The upper bound of the 90% interval on the recall gain
   is below 10% relative.
4. **INCONCLUSIVE.** Everything else, including a positive point estimate whose interval
   includes zero.

The hard caps in step 1 are point-estimate stop rules, not interval tests; the PASS rule
in step 2 uses the interval lower bound. The asymmetry is deliberate: a cost or
false-positive breach stops the arm, while a recall claim requires interval evidence.

H1 and H2 are judged independently with their own comparator (A for B; the B×3 control
for C). The comparator and the cost/latency cap for each hypothesis are:

| Hypothesis                        | Primary comparison | Comparator                                        | False-positive cap              | Cost cap    | Latency cap       |
| --------------------------------- | ------------------ | ------------------------------------------------- | ------------------------------- | ----------- | ----------------- |
| H1 (rubric)                       | B vs A             | A, paired by plan                                 | ≤ A + 10 points; ≤ 50% absolute | B ≤ 1.5× A  | median B ≤ 1.5× A |
| H2 (specialized)                  | C vs B×3           | union of three independent B runs, paired by plan | ≤ A + 10 points; ≤ 50% absolute | C ≤ 3.5× A  | median C ≤ 2× A   |
| H3 (alternate model, exploratory) | B_D / C_D vs B / C | same-request B and C under the other model        | report only                     | report only | report only       |

The cost and latency caps are relative to A in both H1 and H2; the B×3 control's own
cost is part of the experiment, not the denominator. A hard-cap FAIL is recorded per
hypothesis. A PASS on H1 and a FAIL on H2 means only the rubric is eligible for a
scoped decision; neither outcome authorizes the other hypothesis.

### 6.3 What each outcome allows

| Outcome      | Consequence                                                                                                                                                                                                                                                                                |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| PASS on H1   | Evidence that a design rubric helps. The operator MAY then decide whether to authorize a scoped rubric implementation. The conditional PRD does not become authorization: structured findings, disposition APIs, evidence persistence, and orchestration each require a separate decision. |
| PASS on H2   | Evidence for multiple scoped reviewers. A separate decision is required for merge and de-duplication tooling; PASS does not authorize it.                                                                                                                                                  |
| FAIL         | No change to SOP. Record the result. The conditional PRD remains unimplemented.                                                                                                                                                                                                            |
| INCONCLUSIVE | No change to SOP. Enlarge the corpus or fix the invalid condition, and rerun. The conditional PRD remains unimplemented.                                                                                                                                                                   |

### 6.4 Pilot (before the full run)

- Run arms A and B once on two plans.
- Confirm in `metadata.json` that provider, model and class match intent for each run.
- Confirm the token fields are populated for the chosen provider.
- Confirm whether the `SOP_AGENT_MODEL` / `SOP_AGENT_PROVIDER` variables or
  `models.<class>.*` config decides the model for a prompt.
- Confirm Arm D is feasible without configuration changes to SOP.
- Pilot results are not part of the dataset.

## 7. Cost of the experiment

The planned call budget is the corpus size × arms × repetitions, plus the pilot and
any optional Arm D. The target corpus is 26–29 plans (12–15 historical, about 8
seeded, about 6 no-known-defect); the minimum is 15 plans and 15 known defects. Three
repetitions are run per plan per arm.

| Configuration                                      | Calls per plan per repetition | 15 plans × 3 | 26 plans × 3 | 29 plans × 3 |
| -------------------------------------------------- | ----------------------------- | ------------ | ------------ | ------------ |
| A only (baseline)                                  | 1                             | 45           | 78           | 87           |
| A + B (H1)                                         | 2                             | 90           | 156          | 174          |
| A + B×3 + C (default; H1 B shares one control run) | 7                             | 315          | 546          | 609          |
| A + B_H1 + B×3 + C (disjoint H1/H2 runs)           | 8                             | 360          | 624          | 696          |
| Optional D, B + C only                             | +4                            | +180         | +312         | +348         |
| Optional D, B×3 + C                                | +6                            | +270         | +468         | +522         |
| Optional D, B_H1 + B×3 + C                         | +7                            | +315         | +546         | +609         |

The pilot adds 4 calls: arms A and B once on two plans. Pilot results are excluded
from the dataset. Including the pilot, the base experiment is 319–613 calls at the
default 7-call design, or 364–700 at the disjoint 8-call design. Optional Arm D adds
180–609 calls depending on whether it runs B+C, B×3+C, or B_H1+B×3+C, so the total
range is 499–1,309 calls. State the exact configuration in the run manifest before
the first call.

These are call counts, not token costs. Token cost is measured from `metadata.json`
per plan; if the provider does not report tokens, the cost criterion is INCONCLUSIVE
and output size is not substituted for cost. Adjudication is the dominant human cost:
every finding is read, and a 20% sample is read twice. The number of findings is not
known before the run, so no finding count is invented; the actual volume and the
double-adjudicated sample are recorded.

## 8. Risks and trade-offs

- **Security.** Plan text and rubrics are untrusted prompt data and cannot change SOP
  behavior (`PROMPT-EXECUTION.md` §12). No plugin or framework is installed, so no new
  tool or permission surface. Run artifacts stay in the scratch project.
- **Reliability.** Parallel calls add provider-failure cases, handled as failed runs.
  Findings stay advisory (`REVIEW.md` §3).
- **Operational complexity.** The experiment needs no code in SOP. Adoption of arm C
  would need deduplication, which would be real code and is deferred until H2 PASSes
  and a separate decision authorizes it.
- **Lifecycle compatibility.** The experiment uses the existing read-only `review`
  capability and adds no stage, state or approval gate.
- **Validity threats.** Same-model correlation; adjudicator bias; corpus selection
  (defects that later surfaced are not a random sample of design defects); small
  samples; rubric overfitting if rubrics are tuned after seeing results (prevented by
  freezing). These are why INCONCLUSIVE is a first-class outcome.

## 9. Recommendation

- **Reject** installing or vendoring EGM, Superpowers or OpenSpec. Isolation, bounded
  loops, structured findings and approval gates already exist in SOP.
- **Reject** OpenSpec's artifact model and Superpowers' agent-driven process control.
- **Adapt, conditional:** the one candidate addition is a design-level rubric in the
  review prompt (G1). Run the §5 protocol first. Until it returns PASS there is no
  evidence of improvement, and no implementation is justified.
- **Authorization boundary.** A PASS on H1 supports a scoped rubric decision only. The
  conditional [PRD](../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md) remains
  unapproved and unimplemented; structured findings, disposition APIs, evidence
  persistence, and orchestration each require a separate governance decision. A PASS
  on H2 would support a separate decision on multiple reviewers; it does not authorize
  merge or de-duplication tooling.
- **FAIL and INCONCLUSIVE are distinct** and neither authorizes implementation. FAIL
  means a measured negative or hard-cap result; INCONCLUSIVE means the validity
  conditions or interval precision do not support a decision. Both leave SOP
  unchanged.
