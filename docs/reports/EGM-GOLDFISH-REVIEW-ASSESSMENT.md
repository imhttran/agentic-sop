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

**Basis:** SOP `origin/main` @ `ad8a9c6c`, re-read at `64101f9e` for the
feasibility notes in §1.2 and §5.1. The code paths are unchanged since the
`1e8cb005` baseline; `git log 1e8cb005..ad8a9c6c` lists only two documentation merges
and their two documentation commits, and `git diff 1e8cb005..64101f9e -- internal cmd
skills` is empty. References were read as shallow clones outside the checkout: EGM
`34ff1179`, Superpowers `22f0b46`, OpenSpec `0c737dc`. Nothing was installed. The
feasibility notes are static reading of the code; no model call or pilot has been run.

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
  (`internal/agent/provider.go`). The prompt path builds the agent with the selected
  model, but `FromConfig` resolves `SOP_AGENT_MODEL` > provider-specific environment >
  configured model, and the `modelEnvOverride` guard in `applyStageModels` checks only
  pinned stages and the sensitive model; it does not check the prompt's routed `sel`
  (`internal/cli/prompt.go`, `internal/cli/stage_models.go`). `routing.model` can
  therefore record the selected model while a different model name is sent to the
  provider. The experiment MUST unset the override variables and cross-check the model
  key in `tokens` against `routing.model` (§5.1, §6.1); the pilot confirms the check
  works but does not defer the requirement. `models.stages.review` pins the review
  stage of `sop review` and `sop run` (`reviewProvider` calls `stageAgent` in
  `internal/cli/review.go`) and the plan stage of `sop run` (`internal/cli/run.go`);
  `sop prompt` never calls `stageAgent`, so the pin does not apply to it.
- Observability: `metadata.json` carries `routing` (class, provider, model) and
  `tokens` by model (`prompt.go:775-788`). The `tokens` key is the model name the
  client put in its own request (`chatRequest.Model`, recorded through
  `perf.AddUsageWithReport` in `internal/agent/openai.go`, `internal/agent/ollama.go`,
  `internal/ollamaagent/ollama.go` and `internal/ollamaagent/omlxclient.go`). SOP
  does not record a server-returned model name, so the key is evidence of the name SOP
  sent, not of the model the server executed. The counts come from the server's own
  report. `tokens` is omitted only when no call was recorded at all, as with the
  `command` provider, which records no usage and is the default provider when none is
  configured (`EffectiveProvider`). When a server omits usage, the model key is still
  present with zero tokens and `unreported_calls` greater than zero
  (`internal/perf/usage.go`; `TestUnreportedUsageIsNotZero`). No duration field was
  found; latency has to be timed outside SOP.
- The stored `result.md`, `metadata.json` and stdout carry the model reply after
  `redact.ModelInput`, so they are the redacted reply, not the raw one. The experiment
  hashes `result.md` as the output of record.
- Sampling controls: a search of `internal/agent` and `internal/ollamaagent` for
  temperature, seed and top_p found none. Sampling is whatever the provider defaults
  to, so reproducibility must be measured, not assumed.
- Repository access is decided by the (harness, provider) pair. Harness `tool` with
  provider `ollama` is the native in-process loop: REVIEW runs a bounded read-only
  inspection of the project directory before synthesis, and the harness adds its own
  system prompt (`internal/cli/cli.go` agent factory, `internal/ollamaagent/review.go`,
  `orchestrate.go:61`). Text-only providers have no repository access. The default
  harness is `tool` and the default provider is `command`
  (`EffectiveHarness`, `EffectiveProvider`).
- The project configuration is part of the cloned repository. The repository tracks
  `.agent-sdlc/config.yaml`, which at `64101f9e` sets harness `tool`, provider `ollama`
  and a `:cloud` model. A scratch clone at a historical parent commit therefore
  inherits the configuration of that commit, which can differ across plans and can send
  plan text to a cloud model. The project `.env` is also applied at startup for every
  command, including `sop prompt`, unless a variable is already exported (`loadDotEnv`
  in `internal/cli/cli.go`), so a `.env` in the project can inject an override.
- Concurrent invocations in one project can collide. `promptRunID`
  (`internal/cli/prompt.go`) checks that a run directory does not exist and
  `runpkg.New` (`internal/run/run.go`) creates it later with `MkdirAll`, which does not
  fail on an existing directory. `sop prompt` takes no project lock, and no test covers
  concurrent same-second prompts. Two parallel calls in one project can therefore share
  a run directory and overwrite each other's `prompt.md`, `result.md` and
  `metadata.json`. This is a static finding; it has not been reproduced.

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

**Gap that survives these limits:** no SOP path was found that asks a model to challenge
the design itself. Code review takes the task as given; the ad-hoc path has no design
rubric. This is a search-based finding: no path was found in `docs`, `skills` and
`internal`, which is not proof of absence.

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

EGM's own quality claims are unproven here. At `34ff1179`, all 12 scenarios in
`skills/egm-review/test_scenarios.yaml` are `eval_type: deterministic` pattern checks
on flow and wording, not measured defect detection. The phrase "precision and recall"
appears once, in the file-table description in `docs/designs/review.md`; no measured
result is reported. This table is the authoritative capability comparison; the PRD
summarizes it and links here rather than maintaining a second matrix.

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
repetition, not the union of the three repetitions. For H2, the control's recall for a
plan and repetition is the **union** of the three independent B runs' confirmed
defects, matching C's union of three scoped reviewers. The mean of the three
individual B runs is reported separately as a diagnostic; across repetitions, recall
is the mean per §5.5. For the control, latency is the parallel-batch wall-clock of the
three B runs and token cost is the sum of the three runs, matching C's three-call
aggregation. The H1 B run MAY serve as one of the three control runs (7 calls
per plan per repetition) or be separate (8 calls). H1 uses the single designated B run;
it does not use the best of the three control runs. The designation is fixed in the
manifest before the run by a random or pre-agreed rule, not selected from the three
control runs after the results are seen. Post-hoc selection makes the H1 comparison
INCONCLUSIVE. This is a design clarification, not a threshold change;
nothing has been run.

**Rubric provenance.** Rubrics are written for this experiment from the published
method, frozen, and hashed before any run. EGM persona files are not installed or
vendored. If their text is quoted, it is credited (Apache-2.0).

## 5. Protocol

### 5.1 Identical inputs and controlled settings

- Every arm receives byte-identical plan content (sha256 recorded). Request files
  differ only in the instruction block, which is delimited from the plan.
- Plan text is untrusted data passed as the request; the rubric is the operator's. The
  request file separates the instruction block from the plan with a fixed delimiter, and
  the rubric instructs the reviewer to treat the plan as data. SOP does not enforce an
  instruction/data boundary on the ad-hoc path: the whole request is passed as
  `agent.Request.Task`, and the agent wrapper adds its own prompt block. This is a
  prompt-engineering control, not enforced isolation. The pilot tests the delimiter. A
  suspected injection is recorded as a validity threat, not used to exclude a run after
  the fact; no run is removed on that basis, and the pre-registered decision rules are
  unchanged.
- **Harness.** One harness is used for the whole experiment and recorded in the
  manifest. Its repository-access behavior is named: a text-only harness has no
  repository access; the Ollama REVIEW harness performs a bounded read-only inspection
  before synthesis. The same harness is used across arms so the comparison is fair, and
  the access difference from a text-only harness is recorded as a limitation.
- **Project isolation.** Each run, and in particular each concurrent call, uses its own
  project directory: a clone at the plan's parent commit (§5.2) that contains only that
  run's request file. `--file` must resolve inside the project, and a repository-tool
  harness can read everything in it, so the manifest, defect records, answer key, other
  arms' request files and other runs' outputs live outside the project. After checkout a
  fixed `.agent-sdlc/config.yaml` replaces the committed one, its sha256 is recorded in
  the manifest, and no `.env` is present, so provider, harness and model configuration
  are identical across plans. Where the provider is a cloud service, the operator
  records that decision before the first run.
- **Latency timing.** The A timer starts immediately before the single invocation and
  stops at process exit. The C timer starts immediately before the three parallel
  invocations and stops when all three exit. Request-file reads, agent construction and
  provider setup are inside the measured interval for both arms; the C interval includes
  concurrent setup, not a sequential sum. Project preparation (clone, checkout and
  config overlay, see Project isolation) happens before the timer and is excluded for
  every arm. The timing method is recorded in the manifest.
- Controlled and recorded per run: SOP binary commit; harness; provider, model and
  endpoint; `--model-class` (set explicitly in every call); provider-reported
  parameters where available. SOP's prompt metadata exposes `routing` and `tokens`, not
  sampling parameters; read sampling parameters from provider-side logs when available,
  otherwise record them as unavailable and rely on reproducibility. Because SOP exposes
  no sampling controls, repetition (§5.5) carries the variance measurement.
- **Environment.** Run every call in a cleared environment (`env -i` plus a recorded
  allowlist), or at minimum unset `SOP_AGENT_MODEL`, the provider-specific model
  override (`SOP_OLLAMA_MODEL`, `SOP_LLAMACPP_MODEL`, `SOP_MLX_MODEL`, or
  `SOP_OPENAI_COMPATIBLE_MODEL`), `SOP_AGENT_PROVIDER`, `SOP_AGENT_HARNESS` and the
  `SOP_MODEL_*` variables. The prompt path builds the agent with the selected model,
  but `FromConfig` lets the model variables override it, `EffectiveProvider` and
  `EffectiveHarness` read their variables first, and the `modelEnvOverride` guard does
  not check the prompt's routed `sel`. Record the allowlist or the unset state in the
  run manifest.
- **Requested-model check.** For every run, compare `routing.model` in `metadata.json`
  with the model key(s) in `tokens`. The key is the model name SOP sent to the provider
  (§1.2); the check therefore detects an override on the SOP side and is not evidence of
  the model the server executed. A mismatch, or more than one key, invalidates the run
  for model attribution. A run with a detected mismatch is invalidated before unblinding
  and MAY be rerun once with the environment corrected, provided the replacement is
  recorded and the choice is not based on findings. If the replacement also mismatches,
  the comparison is INCONCLUSIVE. Where the provider exposes a model identity, such as a
  digest from its model listing, the operator records it in the manifest before the
  first run and after the last; where it does not, the manifest records the attribution
  as client-side only. If `tokens` is absent, or any call in the run has
  `unreported_calls` greater than zero, the model check and the cost criterion are
  INCONCLUSIVE; the run is not counted as a confirmed model match, and the rerun rule
  does not apply. A provider that reports no usage may be run, but H1 and H2 are
  INCONCLUSIVE for it; it is admissible only for H3 (exploratory) or a labelled
  feasibility run. A run replaced under this rule is excluded from the dataset and does
  not count as a failed run; only the replacement counts.
- One model per arm for the whole experiment. A provider or model change
  mid-experiment invalidates the affected runs.
- **Concurrency.** The full run records whether the provider executes parallel requests
  concurrently. The latency cap uses the measured parallel-batch wall-clock for C, which
  already reflects any provider-side serialization; the sequential sum is diagnostic
  only. C latency is first measured in the full run; a missing or invalid C timing makes
  H2 INCONCLUSIVE. Each concurrent call runs in its own project directory (see Project
  isolation). After every parallel batch the manifest records the `work_item_id` and
  run directory of each call; a duplicate id or directory invalidates the affected runs
  before unblinding, and the same rerun rule as a model mismatch applies.
- Run order is randomized and interleaved across arms and repetitions so provider drift
  is not confounded with arm. The full-run order is generated and recorded in the run
  manifest before the first full-run call. The pilot is excluded from the dataset. A
  pilot that only confirms the existing protocol does not change the manifest; any
  change to the corpus, order, rubric, thresholds, provider or model after the pilot
  starts a new frozen manifest and invalidates the affected full runs.

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
  is frozen only after each path, parent revision and evidence pointer is checked; an
  unresolved path is replaced, not shipped. If replacements cannot reach the
  15-determinable-defect floor, the run is deferred; the floor is not lowered. A later
  correction does not by itself show
  that a review could have found the defect. At corpus freeze, the corpus owner records
  whether each defect was determinable from the document and its parent revision, with a
  one-line basis, in the defect record. The adjudicator applies the pre-registered record
  and does not change the recall denominator.
- **Seeded set (about 8):** one defect each: unstated assumption, nonexistent symbol,
  missing failure mode, unverifiable acceptance criterion, scope violation, plus three
  more of the same classes. Seeded by someone other than the adjudicator, recorded in a
  sealed manifest before any run.
- **No-known-defect set (about 6):** plans with no recorded defect, to measure false
  positives where nothing is expected. "No known defect" is not "no defect"; findings
  here are adjudicated, not assumed invalid.
- **Defect record:** id, plan path and sha256, parent commit, location, description,
  class, severity (blocks implementation / changes approach / minor), evidence pointer,
  source (historical or seeded), and a determinable judgment (yes/no, one-line basis)
  recorded at freeze. The recall denominator is the pre-registered determinable set; it
  is fixed at freeze and is not changed by adjudication. Minimum to run: 15 determinable
  known defects (the recall denominator) across at least 15 plans with known defects; the
  no-known-defect set is additional and does not count toward this floor. Below the floor,
  every recall comparison is INCONCLUSIVE.
- **Per-plan count.** The manifest records the number of known defects per plan; no hard
  cap is imposed, but the per-plan counts are reported with the recall results.
- **Corpus ownership.** The corpus owner records the frozen corpus before the first run;
  the adjudicator does not verify or select it. The corpus-owner duties are defined in
  §5.4.
- **Role separation.** The roles, their separation and their duties are defined in
  §5.4. The corpus owner records the frozen corpus; the adjudicator does not select it.

### 5.4 Blinded adjudication, deduplication, false positives

- Findings are exported with arm, repetition, model and run ids removed, shuffled per
  plan, and presented in that order.
- For each finding the adjudicator records one of: matches known defect `<id>`; valid
  and not on the list (needs a one-line justification from the document); invalid or
  immaterial; duplicate of finding `<n>`. Judgment is against the document and the
  pre-registered defect list, never the reviewer's stated confidence.
- Duplicate labeling is done by the adjudicator; the within-run merge is the only merge
  step and is applied by the analyst. The adjudicator does not see arm labels; duplicate
  labels reference finding IDs. A separate analyst holds the key and applies
  deduplication within one run (arm × plan × repetition): a duplicate is removed only
  when its original is in the same run. The analyst's merge is a deterministic post-step
  performed before metrics; it may only remove duplicate occurrences and cannot add or
  remove non-duplicate findings. Identical findings in different runs remain distinct for
  per-run metrics. Duplicate occurrences are removed from both the numerator and the
  denominator of the per-run recall and false-positive rate, so a repeated invalid
  finding cannot change those metrics through repetition or shuffle order. Cross-
  repetition duplicate links are defined and used only in §5.5. No merge tooling is
  built.
- The adjudicator is not the author of the plan or the seeder of its defect.
- **Independence.** A human adjudicates. A second person independently adjudicates the
  larger of 20 findings or a random 20% sample, and agreement is reported (Cohen's
  kappa). If fewer than 20 findings exist in total, kappa is reported but the outcome
  cannot PASS. A model may serve only as a third check, never as the sole adjudicator.
- **Minimum staffing.** A PASS requires two distinct human adjudicators who are not
  authors of any plan in the corpus. They must be distinct from each other and from the
  seeder of any defect they adjudicate. The corpus owner, seeder and analyst may overlap
  with each other and with the operator, but none of them may be an adjudicator, and the
  adjudicators remain blind. A model may not be an adjudicator. If two independent human
  adjudicators are not available, the best attainable outcome is INCONCLUSIVE.
- **Role duties.** The corpus owner selects and freezes the corpus and records each
  determinability judgment; the seeder creates the seeded defects and the sealed
  manifest; the analyst holds the unblinding key and applies the within-run merge; the
  adjudicators label findings; the second adjudicator re-adjudicates the sample. An
  author is a human who wrote or directly edited the plan text; approving a plan does not
  make someone its author. The corpus owner and seeder are not adjudicators, and the
  seeder of a defect is not an adjudicator of that defect.
- **Adjudication is evidence work, not an approval.** It creates no SOP approval
  record and changes no lifecycle state. The protocol records findings and judgments;
  it does not record an operator decision to implement.
- **False-positive rate** is invalid plus immaterial distinct findings divided by all
  distinct findings, per run and separately on the no-known-defect set. Duplicates are
  removed before this computation and are not counted again. "Valid and not on the list"
  counts as a true positive but is reported separately and is not used in the recall
  numerator.

### 5.5 Repetition

- Three repetitions per plan per arm, random order. Deduplication is within one run
  (arm × plan × repetition). Per-repetition recall and false-positive rate use that
  run's deduplicated set; the plan-level value is the mean of the three repetitions. The
  union across repetitions is reported separately and is not the recall estimator.
- Reproducibility is the share of confirmed distinct findings that recur in at least two
  of three repetitions. A recurrence is established by duplicate links across
  repetitions of the same arm; those links are used only for this count and are then
  discarded. The analyst computes cross-repetition duplicate links from the adjudicator's
  duplicate labels after the within-run merge; they are reported as a separate count and
  never enter any per-run metric. Confirmed findings are findings that match a known
  defect or are adjudicated valid and not on the list; invalid, immaterial and duplicate
  occurrences are excluded. Both confirmed classes count in the reproducibility
  numerator and denominator. Reproducibility is computed per plan and per arm; the PASS
  gate uses the median across plans for each arm, and the pooled share is reported
  separately. A plan with zero confirmed findings for an arm has no defined per-plan
  share for that arm: it is excluded from that arm's median and reported separately with
  the count of such plans. The gate is computed per arm over plans with at least one
  confirmed finding for that arm; if fewer than 15 plans contribute for an arm, that
  arm's reproducibility gate is INCONCLUSIVE. At the 15-plan floor, one empty plan for an
  arm therefore makes that arm's gate INCONCLUSIVE; size the target corpus above the
  floor or accept that consequence.
- A failed call (timeout, transport error, unparseable result) is recorded as a failed
  run and counted, not dropped, and is not retried beyond what SOP itself does
  (`REVIEW.md` §6.4: a failure to review is not a finding).

### 5.6 Metrics

The per-run recall, precision and false-positive rate use the run's deduplicated set
defined in §5.4; the plan-level value is the mean across the three repetitions.
Reproducibility uses the cross-repetition duplicate links described in §5.5.

| Metric                 | Definition                                                                                                       |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Recall                 | known defects matched by distinct findings ÷ known defects, per run; plan value is the mean of three repetitions |
| Precision              | confirmed distinct findings ÷ all distinct findings, per run; plan value is the mean of three repetitions        |
| False-positive rate    | per §5.4, per run; plan value is the mean of three repetitions                                                   |
| Token cost             | tokens from `metadata.json` per plan (all calls of the arm, summed); cost per confirmed distinct defect          |
| Latency                | parallel-batch wall-clock for C; the sequential sum is diagnostic only                                           |
| Reproducibility        | per §5.5                                                                                                         |
| Adjudication agreement | kappa on the double-adjudicated sample                                                                           |

If a provider does not report tokens, cost for that arm is recorded as unavailable and
the cost criterion is INCONCLUSIVE. The hypothesis cannot return PASS without a
measured cost; the recall result may be reported separately. Output size in characters
may be reported as a labelled proxy, not as cost.

### 5.7 Statistics

- The unit of analysis is the plan. Differences are paired by plan.
- Confidence intervals are 90% cluster-bootstrap intervals over plans (at least 10,000
  resamples). With about 15 defects the intervals will be wide; that is expected and
  drives the INCONCLUSIVE outcome.
- Primary hypothesis H1: B has higher recall than A. Secondary H2: C has higher recall
  than the B×3 control. H3 (D) is exploratory and cannot by itself change a
  recommendation.

### 5.8 Run manifest

The run manifest is a single experiment record stored in the experiment directory outside
the SOP checkout, at a path the operator records before the first call. It contains at
least:

- the corpus file hashes and the frozen defect records;
- the rubric hashes;
- the approved threshold text and its sha256;
- the arm configuration, including the 7-call or 8-call choice and the designated H1 B
  run;
- the full-run order;
- the environment state (the cleared-environment allowlist or the unset variables), the
  resolved provider and model for each arm, the sha256 of the config overlay, and the
  provider-side model identity or the statement that attribution is client-side only;
- the `work_item_id` and run directory of every concurrent call;
- the harness and its repository-access behavior;
- the latency timing method and start/stop points;
- the per-plan known-defect count;
- the assessment file's sha256 (the frozen protocol text);
- the named holder of each role (corpus owner, seeder, analyst, adjudicator, second
  adjudicator).

The manifest is an experiment record, not a SOP approval record. It does not create an
approval, change a lifecycle state, or authorize implementation.

## 6. Acceptance thresholds and outcomes

Thresholds are fixed before the first run. The values below are proposals to be
approved before it.

**Pre-run revision note (2026-11-21).** Relative to the earlier PRD draft merged in
#563:

- H1 keeps ≥25% relative recall but raises the absolute floor from 2 to 3 confirmed
  known defects, and its cost cap falls from 2× A to 1.5× A.
- H2 uses the same recall rule as H1 (≥25% relative and ≥3 more confirmed known
  defects) against the B×3 control, replacing the earlier ≥15%-over-B1 fan-out rule;
  its cost cap is 3.5× A, replacing the earlier 3× A cap.

No run has started, so the changes are made before the protocol is frozen; they are
recorded here rather than applied silently. The revised thresholds remain unapproved
until the operator approves them before the first run; the approval is recorded in the
run manifest (§5.8), which also records the approved threshold text and its sha256.
This note also records the pre-run clarifications made in the same revision: the
model-mismatch rerun rule (§5.1), the 20-finding kappa floor (§5.4, §6.2), the H1
designated-run rule (§4), the run-manifest definition (§5.8), the minimum staffing rule
(§5.4), the within-run deduplication scope (§5.4–§5.6), the 15-contributing-plan
reproducibility floor (§5.5, §6.2), and the cost/latency precedence rule (§6.2).

A second round of clarifications, made on 2026-11-21 after the merge of #565 and before
any run, comes from a static feasibility read of the code at `64101f9e`: the
requested-model check and the unreported-usage rule (§1.2, §5.1, §6.1), the environment
and project-isolation rules (§5.1), and the work-item-id check for parallel batches
(§5.1). They state how the existing validity conditions are measured. No threshold or
outcome rule changed.

### 6.1 Validity conditions (checked first)

The experiment is **INCONCLUSIVE** for the affected comparison if any of these holds:

- fewer than 15 determinable known defects (the recall denominator), or fewer than 15
  plans with known defects;
- kappa below 0.6 on the double-adjudicated sample, or no second adjudicator and the
  claim would be PASS;
- more than 10% of runs failed, or arms have unequal failure rates by more than 5
  points;
- `metadata.json` routing differs from the intended provider, model or class; a model,
  provider, harness or routing override variable was set (§5.1 Environment); or the
  model key in `tokens` does not match `routing.model`, in a run that was not replaced
  under the §5.1 rerun rule, for any run in the comparison;
- token counts unavailable, including a provider that reports no token usage or any
  call with `unreported_calls` greater than zero: the requested-model check cannot pass
  and the hypothesis is INCONCLUSIVE; recall may be reported separately but cannot
  yield PASS;
- a provider-reported sampling parameter changed within an arm; where the provider
  reports no parameters, the drift is recorded as a limitation and reproducibility is
  the only guard;
- the order, corpus, rubric or thresholds changed after the first run.

Between-arm parameter differences are recorded as a limitation and are not, by
themselves, a validity failure; run order is randomized.

### 6.2 Outcome rules, applied in this order

The validity conditions in §6.1 are checked first, in the order listed, before the
outcome rules below. A failed validity condition makes the affected comparison
INCONCLUSIVE; the recall rules are not applied. A PASS additionally requires at least
20 double-adjudicated findings; below that, the outcome is INCONCLUSIVE even if the
recall rules would otherwise pass.

1. **FAIL (hard cap).** Any of these at the point estimate: false-positive rate more
   than 10 points above A, or above 50% absolute; token cost above 1.5× A (B) or 3.5×
   A (C); median latency above 1.5× A (B) or 2× A (C).
2. **PASS.** Recall gain at least 25% relative and at least 3 more confirmed known
   defects than the comparator, the lower bound of the 90% interval above zero, and a
   median-across-plans reproducibility of at least 70%.
3. **FAIL (no useful effect).** The upper bound of the 90% interval on the recall gain
   is below 10% relative.
4. **INCONCLUSIVE.** Everything else, including a positive point estimate whose interval
   includes zero.

The hard caps in step 1 are point-estimate stop rules, not interval tests; the PASS rule
in step 2 uses the interval lower bound. The asymmetry is deliberate: a cost or
false-positive breach stops the arm, while a recall claim requires interval evidence.
If the cost criterion is INCONCLUSIVE because `tokens` is unavailable, the cost hard cap
is not applied and the hypothesis is INCONCLUSIVE; it is not a PASS. A hard-cap breach in
step 1 takes precedence over a recall PASS in step 2. The latency cap uses the
parallel-batch wall-clock for C; the sequential sum is diagnostic only.

H1 and H2 are judged independently with their own comparator (A for B; the B×3 control
for C). The comparator and the cost/latency cap for each hypothesis are:

| Hypothesis                        | Primary comparison | Comparator                                        | False-positive cap              | Cost cap    | Latency cap       |
| --------------------------------- | ------------------ | ------------------------------------------------- | ------------------------------- | ----------- | ----------------- |
| H1 (rubric)                       | B vs A             | A, paired by plan                                 | ≤ A + 10 points; ≤ 50% absolute | B ≤ 1.5× A  | median B ≤ 1.5× A |
| H2 (specialized)                  | C vs B×3           | union of three independent B runs, paired by plan | ≤ A + 10 points; ≤ 50% absolute | C ≤ 3.5× A  | median C ≤ 2× A   |
| H3 (alternate model, exploratory) | B_D / C_D vs B / C | same-request B and C under the other model        | report only                     | report only | report only       |

The cost and latency caps are relative to A in both H1 and H2; the B×3 control's own
cost is part of the experiment, not the denominator. The latency cap compares the median
of the paired A runs with the median of the arm's runs; A is not a single invocation when
the comparison has multiple runs. The C cost cap is relative to A, not
B×3, because A is the operational baseline; report C/A and C/B×3 both, so an arm cheaper
than the control is visible even when it exceeds the cap. A hard-cap FAIL is recorded per
hypothesis. A PASS on H1 and a FAIL on H2 means only the rubric is eligible for a
scoped decision; neither outcome authorizes the other hypothesis. The false-positive cap
is anchored to A for both hypotheses so an arm is not rewarded for a control with a
higher false-positive rate; the B×3 control's own rate is reported.

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
- Confirm the pre-run requirements of §5.1: the environment is cleared or the override
  variables are unset, `routing.model` matches the intended model, the `tokens` key
  matches `routing.model`, and `unreported_calls` is zero for every call. The pilot
  validates the checks; the requirements apply to every run.
- Confirm the project-isolation setup: the config overlay and its sha256, no `.env`, a
  project containing only its own request file, the manifest and answer material outside
  the project, and the `result.md` hash. Record the provider-side model identity, or
  record the attribution as client-side only.
- The two-plan pilot cannot exercise the 20-finding double-adjudication floor or kappa;
  it validates metadata, model attribution and token plumbing only. The kappa and
  finding-count validity conditions are evaluated in the full run.
- The pilot does not measure a C parallel batch; C latency is first measured in the full
  run. A missing or invalid C timing makes H2 INCONCLUSIVE for the latency criterion.
- Confirm Arm D is feasible without configuration changes to SOP.
- Pilot results are not part of the dataset.

## 7. Cost of the experiment

The planned call budget is the corpus size × arms × repetitions, plus the pilot and
any optional Arm D. The target corpus is 26–29 plans (12–15 historical, about 8
seeded, about 6 no-known-defect); the minimum is 15 plans with known defects and 15
determinable known defects. Three repetitions are run per plan per arm.

| Configuration                                      | Calls per plan per repetition | 15 plans × 3 | 26 plans × 3 | 29 plans × 3 |
| -------------------------------------------------- | ----------------------------- | ------------ | ------------ | ------------ |
| A only (baseline)                                  | 1                             | 45           | 78           | 87           |
| A + B (H1)                                         | 2                             | 90           | 156          | 174          |
| A + B×3 + C (default; H1 B shares one control run) | 7                             | 315          | 546          | 609          |
| A + B_H1 + B×3 + C (disjoint H1/H2 runs)           | 8                             | 360          | 624          | 696          |
| Optional D, B + C only                             | +4                            | +180         | +312         | +348         |
| Optional D, B×3 + C                                | +6                            | +270         | +468         | +522         |
| Optional D, B_H1 + B×3 + C                         | +7                            | +315         | +546         | +609         |

Combined totals including the 4-call pilot:

| Configuration (including pilot) | 15 plans | 26 plans | 29 plans |
| ------------------------------- | -------- | -------- | -------- |
| Base, default 7-call            | 319      | 550      | 613      |
| Base, disjoint 8-call           | 364      | 628      | 700      |
| Base + D (B+C), default         | 499      | 862      | 961      |
| Base + D (B×3+C), default       | 589      | 1,018    | 1,135    |
| Base + D (B_H1+B×3+C), default  | 634      | 1,096    | 1,222    |
| Base + D (B+C), disjoint        | 544      | 940      | 1,048    |
| Base + D (B×3+C), disjoint      | 634      | 1,096    | 1,222    |
| Base + D (B_H1+B×3+C), disjoint | 679      | 1,174    | 1,309    |

The pilot adds 4 calls: arms A and B once on two plans. Pilot results are excluded from
the dataset, and the combined table above includes those 4 calls. The optional-D rows
are increments to the base configuration; the pilot is already included and is not
added again. The total range without Arm D is 319–700 calls; with Arm D it is 499–1,309
calls. For example, the default 7-call design at 15 plans is 315 calls; D (B+C) adds
180; the 4-call pilot makes 499. The equal 634 / 1,096 / 1,222 rows are not a copy
error: the default 7-call design plus D (B_H1+B×3+C) and the disjoint 8-call design plus
D (B×3+C) both use 14 calls per plan per repetition. State the exact configuration in
the run manifest (§5.8) before the first call.

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
  merge or de-duplication tooling. The conditional PRD's requested-model and
  missing-evidence requirements are protocol alignment; they remain subject to the same
  authorization boundary.
- **FAIL and INCONCLUSIVE are distinct** and neither authorizes implementation. FAIL
  means a measured negative or hard-cap result; INCONCLUSIVE means the validity
  conditions or interval precision do not support a decision. Both leave SOP
  unchanged.
