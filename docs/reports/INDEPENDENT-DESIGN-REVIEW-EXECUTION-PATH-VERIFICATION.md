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

**Baseline:** `origin/main` @ `ade6864a`. The code paths read are unchanged from the
`1e8cb005` baseline the assessment used (no change under `internal/`, `cmd/` or `skills/`
across those commits).

## 1. Verdict

**Execution infrastructure: READY for controlled, per-run-isolated execution of arms A and
B; NOT READY for parallel execution inside one project (arm C and the B×3 control as
specified would lose run evidence there).** A separate project directory per concurrent call,
which the protocol already requires, removes the collision. Two behaviors need operator
decisions (§7). Two runtime questions stay UNVERIFIED without real inference (§6).

This verdict is about the execution path only. It is not corpus readiness, adjudicator
readiness, or experiment readiness, and it does not equal PASS, FAIL or INCONCLUSIVE.

## 2. Preflight findings

- PR #569 (corpus candidates) is merged into `main` (`0673da3b`).
- **PR #570 shows MERGED, but its content is not on `main`.** Its base branch was
  `docs/independent-design-review-corpus` (stacked on #569); it merged into that branch
  (`a522f908`) after #569 had already merged to `main`. `main` has neither the pilot results
  report nor the later feasibility-report edits. A local, unpushed commit in another
  worktree (`c59818fd`, "restore the merged pilot evidence…") would restore them; I did not
  touch it. This report therefore refers to the pilot report by PR and commit, not by link.
- The pilots were read from PR #570's head (`c0d6ada9`). Their results are preserved and not
  rewritten here.
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

Reproduce: `go test ./internal/cli -run TestVerify -count=1 -v`; and, for the collision
observation, `SOP_VERIFY_CONCURRENT=1 go test ./internal/cli -run
TestVerifyConcurrentPromptsObservation -count=20 -v` (add `SOP_VERIFY_CONCURRENT_STRICT=1` to
make a collision fail).

## 4. Summary

| ID    | Requirement                                                   | Result                          |
| ----- | ------------------------------------------------------------- | ------------------------------- |
| EP-01 | Provider and model selected as configured                     | VERIFIED (with a routing hazard, EP-13) |
| EP-02 | Requested versus provider-attested model identity             | PARTIAL                         |
| EP-03 | Token reporting and `unreported_calls`                        | VERIFIED                        |
| EP-04 | Harness selection and its effect                              | VERIFIED                        |
| EP-05 | Repository-tool availability and read-only inspection         | VERIFIED                        |
| EP-06 | Run-directory creation and identity                           | VERIFIED                        |
| EP-07 | Concurrent-run behavior in one project                        | VERIFIED defect; BLOCKED for same-project parallel arms |
| EP-08 | Environment and credential isolation                          | VERIFIED                        |
| EP-09 | Failure handling and retry                                    | PARTIAL                         |
| EP-10 | Evidence persistence                                          | PARTIAL                         |
| EP-11 | Output shape across harnesses                                 | VERIFIED difference             |
| EP-12 | Timing                                                        | PARTIAL                         |
| EP-13 | `--model-class` without a `models:` block                     | VERIFIED hazard                 |
| EP-14 | Cache off and JEV off by default                              | VERIFIED                        |
| EP-15 | Provider-side concurrency and parallel-batch latency          | UNVERIFIED                      |
| EP-16 | Native-agent inspection behavior of real models               | PARTIAL                         |

## 5. Requirements

**EP-01 Provider and model selection.** Code: `internal/cli/prompt.go` (`promptRouting`,
`promptSelection`), `applyModelRouting`. With a `models:` block, `--model-class` selects that
class's provider and model (`source: manual_override`). SYNTHETIC RUNTIME:
`TestVerifyMLXPromptIdentityAndUsage` and the native tests route to the configured model.
PILOT: both pilots routed as intended. Residual: EP-13. Remediation: every experiment project
defines `models.small|medium|large`.

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
already says; a production change to record the response `model` field is optional.

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

**EP-06 Run directory creation and identity.** Code: `promptRunID`
(`prompt-YYYYMMDD-HHMMSS`, unique only within one project's run root) and `runpkg.New`.
SYNTHETIC RUNTIME: `TestVerifyMLXPromptIdentityAndUsage` — one directory per run, named
`work_item_id`, with consistent `metadata.json`. Identical ids in different projects are
expected. Result VERIFIED.

**EP-07 Concurrent runs in one project.** Code: `promptRunID` checks that a directory does
not exist and returns an id; `runpkg.New` creates it with `MkdirAll` (no error if it exists)
and `sop prompt` takes no project lock. SYNTHETIC RUNTIME:
`TestVerifyPromptRunIDReservationGap` (deterministic: two ids drawn before either directory is
created are equal, both `New` calls succeed, the later writer overwrites the earlier). Observed
with concurrency (not gated in CI): 8 simultaneous in-process prompts left **1 run directory and
1 surviving result in 19 of 20 trials** (2 and 2 in the other); a real `sop` binary
(built from `main` at `1b61db64`) with **4 parallel processes in one project against a
synthetic local server left 1 run directory and the same `work_item_id` in all 4 stdout results
in 5 of 5 trials**. Each process still printed its own result on stdout; the on-disk run
artifacts are what collide. Result: defect VERIFIED. Parallel arms (C, B×3) in one project
are BLOCKED. Mitigation: one project directory per concurrent call (the protocol's rule) —
EXISTING TEST `TestPromptProjectsAreIsolated`; not exercised with real parallel inference.
Remediation (operator decision): keep the harness rule, or make id allocation atomic in SOP.

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
configured `agent` provider**, and nothing reaches the configured endpoint. The built-in
default can be a cloud model, so a project that names a class without defining `models.<class>`
can send content off the machine unnoticed. Result VERIFIED hazard. Remediation: the
experiment's config overlay must define all three classes (the pilot overlays do); consider a
guard in SOP (operator decision).

**EP-14 Cache and JEV defaults.** EXISTING TEST: `TestPromptWithoutCacheAlwaysCallsAgent`;
code: `--cache` is opt-in, JEV triage is a no-op unless enabled. Result VERIFIED.

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

1. **Same-project parallelism.** Keep the harness rule (one project per concurrent call), or
   change SOP so run ids are allocated atomically. The report assumes the harness rule.
2. **Routing default.** Whether SOP should refuse `--model-class` when no `models.<class>` is
   defined, instead of routing to a built-in default that may be a cloud model.
3. **Concurrency/latency verification (EP-15).** Authorize the minimum real-inference check
   above, or accept arm C's latency as first measured in the full run.

## 8. Verification conduct and one deviation

- No pilot was repeated and the full experiment was not run. No threshold was changed. No
  production code was modified; the only code added is the test file named above.
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
