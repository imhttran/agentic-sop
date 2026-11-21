# Independent Design Review — Execution-Path Verification (Workstream B)

**Type:** Point-in-time verification report (non-normative). It verifies whether SOP can
execute the review arms of the protocol in
[EGM-GOLDFISH-REVIEW-ASSESSMENT.md](EGM-GOLDFISH-REVIEW-ASSESSMENT.md) with accurate model
attribution, token accounting, isolation and evidence capture. It is **not** an experiment
result, does not authorize the experiment or any pilot, changes no threshold, and says
nothing about corpus or experiment readiness. The coordination plan is
[PLAN-Independent-Design-Review-Feasibility.md](../plans/PLAN-Independent-Design-Review-Feasibility.md)
(task FEAS-B1); the readiness assessment is
[INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md](INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md).

**Baseline:** code read at `origin/main` @ `ade6864a`; documents at `57244208` (merged into this
branch). The code paths read are unchanged from the
`1e8cb005` baseline the assessment used (no change under `internal/`, `cmd/` or `skills/`
across those commits).

**Merge status (post-merge note):** this report was merged as PR #574 (merge `a10edc28`,
branch head `b784f2dc`). The `promptRunID` fix and its regression tests are on `main`.
Everything else in this document is the point-in-time record it was when written and is
not rewritten.

## 1. Verdict

**Execution infrastructure: READY for controlled execution of arms A and B, including
concurrent calls (arm C and the B×3 control).** The verification found that concurrent
`sop prompt` runs in one project shared a run directory and overwrote each other's evidence
(EP-07); that is **fixed in this change** (`promptRunID` now reserves its directory
atomically) and covered by regression tests and a cross-process check. The protocol's rule of
one project per concurrent call stays, for request-file isolation and leakage control. Two
runtime questions cannot be fully settled without real inference: EP-15 is UNVERIFIED and
EP-16 is PARTIAL (§6).

This verdict is about the execution path only. It is not corpus readiness, adjudicator
readiness, or experiment readiness, and it does not equal PASS, FAIL or INCONCLUSIVE.

## 2. Preflight findings

- PR #569 (corpus candidates) is merged into `main` (`0673da3b`).
- **PR #570 showed MERGED while its content was not on `main`** at `ade6864a`: its base branch was
  `docs/independent-design-review-corpus` (stacked on #569), and it merged into that branch
  (`a522f908`) after #569 had already merged to `main`. PR #572 (merge `57244208`) has since
  restored the pilot results report, and this branch is merged with that `main`; the report is
  [INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md](INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md).
- I read the pilots from PR #570's head (`c0d6ada9`) before #572 landed. Their results are
  preserved and not rewritten here.
- Existing worktrees (`corpus`, `feasibility`, `readiness`), the primary checkout's branch
  and its untracked files, and ignored `.agent-sdlc/` lifecycle evidence were left alone. All
  work was done in a new worktree from `origin/main`.

## 3. Method and evidence classes

Every result below names how it was established:

- **CODE** — read in the production source at the baseline.
- **EXISTING TEST** — an existing test that passes at the baseline.
- **SYNTHETIC RUNTIME** — a new test in
  [`prompt_execution_path_test.go`](../../internal/cli/prompt_execution_path_test.go) that runs
  `sop prompt --capability review --model-class medium --file request.md --json` through the
  **production agent factory** (`defaultDeps`) against synthetic local HTTP servers, or a real
  `sop` binary against a synthetic local server. No real provider, model or credential.
- **PILOT** — recorded in the two four-call pilots (PR #570). Not repeated.

Reproduce: `go test ./internal/cli -run 'TestVerify|TestPromptRunID|TestConcurrentPrompts|TestFailedPrompt' -count=1 -v`.

## 4. Summary

| ID    | Requirement                                           | Result                                  |
| ----- | ----------------------------------------------------- | --------------------------------------- |
| EP-01 | Provider and model selected as configured             | VERIFIED (with a routing hazard, EP-13) |
| EP-02 | Requested versus provider-attested model identity     | PARTIAL                                 |
| EP-03 | Token reporting and `unreported_calls`                | VERIFIED                                |
| EP-04 | Harness selection and its effect                      | VERIFIED                                |
| EP-05 | Repository-tool availability and read-only inspection | VERIFIED                                |
| EP-06 | Run-directory creation and identity                   | VERIFIED (allocation fixed)             |
| EP-07 | Concurrent-run behavior in one project                | FIXED (defect first VERIFIED)           |
| EP-08 | Environment and credential isolation                  | VERIFIED                                |
| EP-09 | Failure handling and retry                            | PARTIAL                                 |
| EP-10 | Evidence persistence                                  | PARTIAL                                 |
| EP-11 | Output shape across harnesses                         | VERIFIED difference                     |
| EP-12 | Timing                                                | PARTIAL                                 |
| EP-13 | `--model-class` without a `models:` block             | VERIFIED, as specified                  |
| EP-14 | Cache off and JEV off by default                      | VERIFIED                                |
| EP-15 | Provider-side concurrency and parallel-batch latency  | UNVERIFIED                              |
| EP-16 | Native-agent inspection behavior of real models       | PARTIAL                                 |

## 5. Requirements

**EP-01 Provider and model selection.** Code: `internal/cli/prompt.go` (`promptRouting`,
`promptSelection`), `applyModelRouting`. With a `models:` block, `--model-class` selects that
class's provider and model (`source: manual_override`). SYNTHETIC RUNTIME:
`TestVerifyMLXPromptIdentityAndUsage` and the native tests route to the configured model.
PILOT: both pilots routed as intended. Residual: EP-13. Remediation: every experiment project
defines `models.small|medium|large`. Two further CODE-only readings (not exercised at
runtime): the `small` class is local-first and `applyLocalFallback` can swap it to its cloud
fallback when the local runtime cannot serve it, so the overlay should use `medium` or `large`
or define no fallback; and `renderPrompt` (`internal/agent/provider.go`) places the request
file verbatim under `Task:` with no fence, so any instruction/data delimiter must be inside
`request.md`.

**EP-02 Requested versus provider-attested identity.** Code: the `tokens` key is the model
name the client sent (`chatRequest.Model`, `perf.AddUsageWithReport`); no client records a
server-returned model name. SYNTHETIC RUNTIME: `TestVerifyModelOverrideChangesRequestedModel`
and `TestVerifyNativeModelOverrideChangesRequestedModel` show that `SOP_AGENT_MODEL`,
`SOP_MLX_MODEL`, `SOP_OLLAMA_MODEL` each change the name the provider receives while
`routing.model` keeps the routed name, and the `tokens` key follows the sent name. So the
protocol's requested-model check **does detect** an override. It is **not** evidence of the
model the server executed. Result PARTIAL (requested name VERIFIED; attested identity not
available). Residual: server-side substitution or aliasing is invisible. Remediation: record
a provider-side identity out of band (Ollama digest; oMLX model id only), as the protocol
already says; a production change to record the response `model` field is optional. The same
gap applies to provider and harness (CODE only, not exercised): the agent factory in
`cli.go` re-resolves them through `EffectiveProvider`/`EffectiveHarness`, where
`SOP_AGENT_PROVIDER` and `SOP_AGENT_HARNESS` win, while `routing.provider` records the
routed selection. The cleared-environment rule must therefore cover these variables, and the
harness should check that none is set; `routing.provider` alone does not prove the backend.

**EP-03 Token reporting.** Code: `internal/perf/usage.go`. SYNTHETIC RUNTIME:
`TestVerifyMLXPromptIdentityAndUsage` (usage recorded: calls 1, input 120, output 30);
`TestVerifyMLXUnreportedUsageKeepsModelKey` (a server that omits usage still yields the model
key, with zero tokens and `unreported_calls: 1`, so key presence alone proves nothing).
EXISTING TEST: `TestUnreportedUsageIsNotZero`, `TestPromptRecordsTokenUsage`. PILOT: oMLX
and Ollama cloud both reported usage on every call. Result VERIFIED for the protocol's
conditions. Residual: reasoning tokens are included in the output count; retried native turns
are not counted (EP-09).

**EP-04 Harness selection.** Code: `defaultDeps` factory in `internal/cli/cli.go`;
`EffectiveProvider`/`EffectiveHarness` (defaults: provider `command`, harness `tool`). Repository
access exists only for harness `tool` with provider `ollama` (or `mlx` with
`SOP_MLX_TOOL_LOOP`). SYNTHETIC RUNTIME: the MLX request carries no `tools`; the native request
advertises `read_file` in its REVIEW system prompt. Result VERIFIED. Remediation: record
harness and provider in the manifest, as the protocol requires.

**EP-05 Repository tools and read-only inspection.** Code: `policyFor(agent.Review)`
(read-only tools), `executeTwoPhase` (`internal/ollamaagent/orchestrate.go`). SYNTHETIC RUNTIME:
`TestVerifyNativeReviewCanInspectRepository` — a `read_file` request is executed in the
project, the file content reaches the model on the next call, the file is unchanged, and
`tokens.calls` is 2. EXISTING TEST: `TestReviewCannotMutateRepository`,
`TestReviewEarlyFinalCompletesImmediately`. Result VERIFIED. Residual: no per-run tool trace is
persisted; the call count is the only artifact-level signal of inspection.

**EP-06 Run directory creation and identity.** Code: `promptRunID` and `promptRunPath`
(`internal/cli/prompt.go`): the id is `prompt-YYYYMMDD-HHMMSS`, with a numeric suffix when taken,
and the call **reserves** the directory with `os.Mkdir` (atomic; fails if it exists). An unused
(empty) reservation is removed if the command fails before the run writes anything. Before the
fix the id was only checked, never reserved. SYNTHETIC RUNTIME:
`TestVerifyMLXPromptIdentityAndUsage` (one directory per run, named `work_item_id`, consistent
`metadata.json`), `TestPromptRunIDReservesItsDirectory`, `TestFailedPromptLeavesNoRunDirectory`.
Ids are unique within one project only; identical ids in different projects are expected.
Result VERIFIED.

**EP-07 Concurrent runs in one project.** Defect first observed, then fixed.
_Before the fix_ (code at `main` `1b61db64`): `promptRunID` only checked that a directory did
not exist, `runpkg.New` (`MkdirAll`) did not fail when it did, and `sop prompt` takes no project
lock. Eight simultaneous in-process prompts left **1 run directory and 1 surviving result in
19 of 20 trials**; **4 parallel `sop` processes in one project against a synthetic local server
left 1 run directory and the same `work_item_id` in all 4 stdout results in 5 of 5 trials**
(each process still printed its own result on stdout; the on-disk run artifacts collided).
_After the fix_ (this change): `TestPromptRunIDIsUniqueUnderConcurrency` (32 simultaneous ids,
all distinct) and `TestConcurrentPromptsInOneProjectKeepSeparateRuns` (8 simultaneous prompts,
8 run directories, 8 distinct results) pass, including repeated with the race detector; a `sop`
binary built from this change, with the same 4-parallel-process trial against the synthetic
server, left **4 run directories, 4 `metadata.json` files and 4 distinct `work_item_id`s in 5
of 5 trials** (the pre-fix binary reproduced the collision in 5 of 5 trials as a control).
Result FIXED. Residual: the protocol still requires one project per concurrent call, for
request-file isolation and leakage control, and provider-side concurrency is untested (EP-15).

**EP-08 Environment and credential isolation.** SYNTHETIC RUNTIME:
`TestVerifyModelOverrideChangesRequestedModel`, `TestVerifyNativeModelOverrideChangesRequestedModel`
(env overrides), `TestVerifyDotEnvCanInjectModelOverride` (a project `.env` is applied to
`sop prompt` and changes the model sent). Code: `loadDotEnv` runs in `run`; `SOP_OMLX_*` is not
read by production code (the MLX provider reads `SOP_MLX_*`). EXISTING TEST: the LLM08
hidden-context tests show a provider credential travels only in the Authorization header,
not the request body. Result VERIFIED. Residual: a cleared environment (`env -i`) is a
harness rule exercised by the pilots, not by a test.

**EP-09 Failure handling and retry.** SYNTHETIC RUNTIME:
`TestVerifyMLXProviderErrorIsNotRetried` (text-only provider: one attempt, then a failed
prompt); `TestVerifyNativeProviderRetryIsInvisibleInTokens` (native client retries a failed turn
up to `maxProviderAttempts`=3; the prompt succeeds and `tokens.calls` is 1 although the
provider saw 2 requests). Result PARTIAL: behavior differs by harness, and native retries are
absent from the evidence. Residual: cost is under-counted on retried native turns; the
protocol's "no retry beyond SOP's own" holds only if this is understood. Remediation:
document in the manifest; treat `calls` as successful calls.

**EP-10 Evidence persistence.** PILOT and SYNTHETIC RUNTIME: each run writes `prompt.md`,
`result.md`, `metadata.json`, `routing.json`, `state.json`. Stored output is redacted
(`redact.ModelInput`). For the native harness the stored result is the **re-serialized**
JSON object (`json.Marshal` of the parsed final), not the raw reply
(`TestVerifyNativeReviewEarlyFinalIsOneCall`; Pilot B's result keys are alphabetical).
Result PARTIAL: the raw model reply is not preserved. Remediation: hash `result.md` as the
output of record, as the pilots did.

**EP-11 Output shape across harnesses.** Code: `promptOutputRequirements` asks the
text-only providers for prose; the native REVIEW contract requires a structured JSON
document and sets `format: "json"`. PILOT: Pilot A returned prose, Pilot B returned
`{"findings":[…]}`. Result VERIFIED. Residual: arms run on different harnesses are not
comparable in shape, and the harness adds its own system prompt, so "arms differ only in the
request file" holds within one harness only. Remediation: one harness for the whole
experiment (already in the protocol).

**EP-12 Timing.** Code: no duration in `metadata.json`. PILOT: Pilot A's timing record is
invalid (cross-process timer); Pilot B's one-process timer recorded plausible durations. Result
PARTIAL: single-call timing works with the one-process timer; parallel-batch timing has not
been measured (EP-15).

**EP-13 `--model-class` without a `models:` block.** SYNTHETIC RUNTIME:
`TestVerifyModelClassWithoutModelsBlockRoutesToBuiltInDefault` — the request routes to the
built-in class default (provider `ollama`, model `nemotron-3-super:cloud`), **ignoring the
configured `agent` provider**, and nothing reaches the configured endpoint. This **conforms to
[MODEL-ROUTING.md](../specs/MODEL-ROUTING.md)**: a `--model-class` override activates routing
and the built-in class table applies (`medium` is a cloud model). It is a hazard for an
experiment, not a defect, so no SOP change is made. Result VERIFIED, as specified. Residual: a
project that names a class without defining `models.<class>` can send content off the machine.
Remediation (done): the assessment's §5.1 now requires the overlay to define every class.

**EP-14 Cache and JEV defaults.** EXISTING TEST: `TestPromptWithoutCacheAlwaysCallsAgent`;
code: `--cache` is opt-in, JEV triage is a no-op unless enabled. Result VERIFIED. Limit:
this covers SOP's own prompt-result cache only. SOP sends no `keep_alive` or cache controls,
so provider-side prefix or KV caching is outside its control and "caching disabled" must not
be claimed for the provider.

**EP-15 Provider-side concurrency and parallel-batch latency.** Not testable without real
inference. UNVERIFIED. Minimum authorization: three concurrent real requests to the chosen
provider, each in its own project directory with a synthetic request (a local provider needs
no external data), timed with the one-process timer.

**EP-16 Inspection behavior of real models.** PILOT: 0 of 4 Pilot B runs inspected the
repository. The native REVIEW prompt says to inspect only if the request is insufficient, and
the request contains the plan. PARTIAL: the mechanism is verified (EP-05); whether a given
model ever inspects is model-dependent and observed only in these four runs.

## 6. Pilot reconciliation (original results preserved)

- **Why one model call per Pilot B invocation.** The native REVIEW system prompt says the
  change is summarised in the input and to inspect the repository only if it is insufficient;
  a first-turn final answer (a JSON object without a `tool` field) ends the invocation
  immediately (`executeTwoPhase`, `turnToolCall`;
  `TestReviewEarlyFinalCompletesImmediately`). Because every successful chat turn, including a
  tool turn, adds to `tokens.calls`, `calls: 1` with `unreported_calls: 0` establishes from the
  artifacts that no inspection turn occurred.
- **Were repository tools available?** Yes by construction: the REVIEW policy allows read-only
  tools and the system prompt names them (`TestVerifyNativeReviewEarlyFinalIsOneCall`). The
  pilot's own request bodies were not captured, so availability is verified for the production
  path, not observed in the pilot.
- **Did read-only inspection occur?** Not in Pilot B (see above). Inspection works when asked
  (`TestVerifyNativeReviewCanInspectRepository`).
- **Can parallel execution corrupt or overwrite run evidence?** Yes, inside one project
  (EP-07). The pilots ran sequentially in separate projects and were not affected.
- **Do token and timing data satisfy the protocol?** Tokens: yes for both pilots (key equals
  `routing.model`, `unreported_calls` zero). Timing: Pilot B yes; Pilot A no (recorded in the
  pilot report); parallel timing not yet measured.

## 7. Outstanding decisions

1. **Concurrency/latency check (EP-15).** Authorize the minimum real-inference check above, or
   accept that arm C's latency is first measured in the full run. This remains open.
2. **Merge (resolved).** This change, including the one production fix (`promptRunID`,
   `internal/cli/prompt.go`), was merged as PR #574 (merge `a10edc28`). The merge is not
   approval of any governance decision, and it does not resolve EP-15.

## 8. Verification conduct and one deviation

- No pilot was repeated and the full experiment was not run. No threshold was changed. One
  production change was made after the verification, at the operator's instruction to fix the
  collision: `promptRunID` in `internal/cli/prompt.go` (atomic reservation, plus removal of an
  unused reservation on early failure). Everything else added is tests and documentation.
- **Deviation: one unintended real cloud call.** While checking EP-13 I ran the pinned `sop`
  binary against a scratch project with no `models:` block and a one-word synthetic request.
  It routed to the built-in default `nemotron-3-super:cloud` through the local Ollama daemon and
  completed. The request held no plan text and no repository content (an empty scratch
  repository), and I did not keep its output. This went against the instruction to avoid
  external cloud inference without separate authorization; it is the behavior EP-13 records. The
  call count is not known (the native agent can make several), and it may count against the
  operator's Ollama account.
- The other runtime observations used synthetic local servers only.
- Gates run: `gofmt`, `go vet ./...`, `go build ./...`, the full `internal/cli` test package,
  `make check-push` (formatting and documentation links), and the code-review-standards and
  simplify checks inline. The model-based `sop review` gate was **not** run: the repository's
  configured provider is an Ollama cloud model, and running it would send the diff to a cloud
  model without the separate authorization this task requires.
