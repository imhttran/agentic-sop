# OWASP LLM Top 10 — end-to-end prompts for the remaining gaps

**As of:** `main` at `b444f1bf` (2026-11-11). **Status:** working aid, not a plan
of record and not normative.

One prompt per open gap, taken from
[coverage-statement.md](../reports/owasp-llm/coverage-statement.md) and
[gap-register-20261109.md](../reports/owasp-llm/gap-register-20261109.md), which
stay authoritative. Risk IDs are OWASP 2025; evidence file names use the
repository's own plan numbers (for example `llm04-supply-chain.md` is OWASP
LLM03).

To use one: start a fresh agent session in this repository and paste the
**common rules** followed by the gap's prompt.

## Gap index

| #   | OWASP        | Gap                                                                 | Kind            | Status      | PR / evidence    |
| --- | ------------ | ------------------------------------------------------------------- | --------------- | ----------- | ---------------- |
| 1   | LLM01        | Linux case with no verdict; one model per platform                  | Evidence        | Done        | PR #422 (Linux verdicts for all five cases); PR #424 (macOS second model) |
| 2   | LLM02        | Live campaigns predate the task-input fix; Linux sinks `NOT_RUN`    | Evidence        | In progress | PR #427: task-input re-runs; Linux notification sink run recorded (llm02-campaign-linux-notify-sink.json); Linux retrieval sink still `NOT_RUN` (blocked with gap 14) |
| 3   | LLM02        | C1 redaction not on the OpenAI-compatible provider path             | Code            | Done        | PR #400 (merged) |
| 4   | LLM02, LLM07 | Model context outside the user prompt and tool results              | Code            | Done        | PR #402 (merged) |
| 5   | LLM02        | Hardlink alias: no `Nlink > 1` check; macOS not covered             | Code, gated     | Waiting on operator | macOS: two runs, protected aliases denied, positive control misses (#405, attempt 2) |
| 6   | LLM02        | macOS sandbox: parent environment readable via `kern.procargs2`     | Investigation   | Waiting on operator | PR #406 |
| 7   | LLM02        | Model-output artifacts persisted verbatim                           | Investigation   | In progress | PR #408 (option C); option A pending, proposal PR #407 |
| 8   | LLM02        | Operator decisions: Plan 03 acceptance, C1 default, residuals       | Decision packet | Waiting on operator | PR #409 |
| 9   | LLM03        | Scanner integrity: hash-then-exec race, pins, signatures            | Code, gated     | In progress | PR #410 (race closed); policy proposal in docs/plans |
| 10  | LLM04        | No production reader verifies repository index rows                 | Code            | Done        | PR #411 (merged): guard test; digests recorded as latent |
| 11  | LLM04        | RAG producer does not emit `chunk_hash`                             | Cross-repo code | Open        |                  |
| 12  | LLM05        | Slack rendering and channel binding unobserved                      | Evidence        | Waiting on operator | PR #412 (merged); escaped-markup probe undecided |
| 13  | LLM06        | Sandbox off by default; Linux host sockets outside `/tmp`, `/run`   | Code, gated     | In progress | PR #421 (Linux socket masked and verified in a container); default-on pending |
| 14  | LLM08        | Live RAG with PostgreSQL and embeddings `NOT_RUN`                   | Evidence        | Waiting on operator | producer lacks a retrieval-only entrypoint (blocker record) |
| 15  | LLM09        | Subprocess can write SOP state with the sandbox off; live `NOT_RUN` | Code + evidence | Waiting on operator | fingerprint check was NO-GO (2026-11-09); the sandbox that denies .agent-sdlc writes is opt-in; default-on waits on gap 13 |
| 16  | LLM10        | Five subprocess paths without process-group termination             | Code            | Done        | PR #398 (merged) |
| 17  | LLM10        | Token budget is off by default and is not a dollar cap              | Code, optional  | Done | PR #417 (merged): option A, token budget only |
| 18  | All          | Coverage statement and evidence matrix lag current merged evidence  | Docs            | Done        | PR #425 (merged); refreshed through PR #427 |

**Tracking.** The Status and PR / evidence columns track progress on this list
only. Statuses: `Open`, `In progress`, `Waiting on operator` (a proposal or
decision packet is open), `Done` (merged, with the evidence files updated). The
session that works a gap updates its own row in the same pull request. `Done`
here is not acceptance and not a coverage claim; the coverage statement and gap
register decide those.

Not promptable, because SOP does not own them: provider model provenance and
training, preventing prompt injection in general, factual correctness of model
prose, third-party terminal/IDE rendering, general DLP, and the agent process
outside SOP.

## Common rules (paste before every prompt)

```text
You are working in the agentic-sop repository. Follow these rules for the whole task.

Ground truth
- Read docs/reports/owasp-llm/coverage-statement.md and gap-register-20261109.md first.
  They are authoritative. If the code already closes the gap, stop and report that.
- Verify every file path, function, flag, and test name against the current code before
  relying on it. Names in this prompt are leads, not facts.

Scope and safety
- Do only this gap. No unrelated refactors, no new dependency unless the prompt allows it.
- Never use real credentials. Use synthetic canaries, and mark fixtures as the repo does.
- Do not change security defaults, decision-provider defaults, or SOP workflow logic
  unless the prompt says the operator approved it.
- Never record acceptance, approval, a waiver, or a lifecycle transition yourself. Do not
  reopen or edit archives under .agent-sdlc/. Prepare the request and stop.
- Where the prompt says STOP FOR OPERATOR, write the proposal, open it for review, and
  end the task. Do not implement past that point.

Implementation
- Smallest change that fully solves it; reuse existing packages and follow nearby code.
- New logic gets a deterministic test that fails without the change.
- Run: gofmt, go vet ./..., go build ./..., go test ./... and the race detector on the
  packages you touched. Report real failures; do not hide or skip them.

Evidence and documents
- Use the repository's status vocabulary exactly: IMPLEMENTED, VERIFIED, OBSERVED,
  NOT_RUN, NEEDS_HUMAN, PASS, and so on. Never write "covered" without a test or a
  recorded run behind it.
- Record platform, architecture, revision, model, request count, and artifact SHA-256
  for any live run. A run on one model is a claim about that model only.
- Update the matching evidence file under docs/reports/owasp-llm/, then the coverage
  statement and the gap register, and docs/reference/PROJECT-STATUS.md if it lists the item.
  State what is still not covered and why.
- Update this gap's row in the gap index of docs/plans/OWASP-LLM-GAP-PROMPTS.md: set
  Status and add the pull request and evidence file. Touch no other row.

Delivery
- main is protected. Work on a branch, run the code-review-standards and /simplify
  passes before the first commit, then open a pull request. Do not merge it.
- Final report: what changed, what was verified and how, what remains open, and any
  decision the operator must make.
```

## 1. LLM01 — Linux case with no verdict; one model per platform

```text
Gap: the LLM01 prompt-injection evidence is one model per platform, run once per case.
On Linux, 3 of 4 injection cases were denied and 1 timed out with no verdict.

Goal: get a verdict for the timed-out Linux case and widen the evidence to a second
model per platform, without changing any control.

Steps
1. Read docs/reports/owasp-llm/llm01-injection.md and the llm01-campaign*.json
   artifacts, including the two timeout attempts. Identify the timed-out case and why.
2. Find the campaign test and its runner. Decide whether the timeout was the model, the
   harness deadline, or the case design. Fix only a harness or case defect; if the
   deadline is simply too short, make it configurable for the campaign, not the product.
3. Re-run the full campaign on Linux amd64 with the original model. Every case must end
   with a verdict; a timeout is recorded as NO_VERDICT, never as a pass.
4. Run the same campaign with a second, different model on both macOS and Linux.
5. Write new artifacts beside the existing ones; do not overwrite earlier evidence.

Done when: llm01-injection.md lists each platform and model with per-case verdicts and
artifact hashes, the coverage statement's LLM01 row reflects the new bounds, and the
statement still says prompt injection cannot be prevented in general.
```

## 2. LLM02 — live campaigns predate the task-input fix; Linux sinks `NOT_RUN`

```text
Gap: the LLM02 live extraction campaigns ran before the user-prompt redaction fix
(PR #349), so that fix is verified deterministically only. Linux live retrieval and
notification sinks are NOT_RUN.

Goal: live evidence for the current code on macOS and Linux, including the two sinks.

Steps
1. Read docs/reports/owasp-llm/llm02-disclosure.md and the llm02-*campaign.json
   artifacts. List which cases exist and which routes they exercise.
2. Add live cases where a synthetic canary enters through the task, the caller input,
   and the output requirements, and assert it does not reach the model request.
3. Add Linux live cases for the retrieval sink and the notification sink. If either
   needs infrastructure that is not available, record NOT_RUN with the exact blocker.
4. Re-run the macOS campaign and the Linux campaign at the current revision. Keep
   positive controls so a pass cannot come from a broken harness.
5. Record protected cases, positive controls, and residual exposures separately.

Done when: llm02-disclosure.md and the coverage statement no longer say the live
campaigns predate the task-input fix, or say precisely which part still does. Do not
touch Plan 03 acceptance; that is gap 8.
```

## 3. LLM02 — C1 redaction not on the OpenAI-compatible provider path

```text
Gap: C1 unknown-format redaction (redact.ModelInputWithPolicy) is applied at the local
Ollama tool-harness boundary in internal/ollamaagent/prompt.go (user prompt and tool
result). It is not applied on the OpenAI-compatible provider path.

Goal: every model-bound request passes the same redaction regardless of provider.

Steps
1. Trace how a request reaches internal/provider/openai and any other provider under
   internal/provider. Find the single point where text becomes model input for each.
2. Establish whether baseline redact.ModelInput already runs on those paths. Record
   the answer in the evidence file either way.
3. Apply redaction once, at the shared boundary if one exists, instead of per provider.
   Reuse the existing policy plumbing (config.RedactionPolicy); add no new option.
4. Add a test per provider path that a known-format secret, an inventoried value, and a
   contextual credential key do not appear in the outbound request body. Add a boundary
   test that fails if a new provider bypasses the shared point.
5. Keep C1 opt-in. Do not enable it in repository configuration.

Done when: the coverage statement no longer limits C1 to the Ollama boundary, with the
tests named as evidence, and the remaining residual (context-free unknown formats)
is still stated.
```

## 4. LLM02, LLM07 — model context outside the user prompt and tool results

```text
Gap: context that reaches the model by a route other than the user prompt or a tool
result (for example a system prompt) is not redacted.

Goal: an inventory of every route by which text reaches a model, and redaction on each
route that can carry caller- or repository-derived text.

Steps
1. Enumerate every model-input route: system prompt, user prompt, tool results and
   errors, retrieved evidence, handoff capsules, decision-memory context, continuation
   and resume context, and any provider-specific field. Cite file and function for each.
2. For each route record: source of the text, whether it can contain untrusted or
   secret-bearing content, and whether redaction already applies.
3. For each unredacted route that can carry such content, apply redact.ModelInput (and
   the C1 policy where configured) at the narrowest shared point. A route that is a
   fixed compile-time string needs no redaction; say so instead of adding code.
4. Add a test per newly covered route with a synthetic canary.
5. Do not claim prompt secrecy. SOP does not treat its prompts as secret.

Done when: llm02-disclosure.md and llm08-hidden-context.md contain the route inventory
with a covered / not-applicable / open status per route, and the coverage statement's
"another route" bullet is replaced by that precise list.
```

## 5. LLM02 — hardlink alias: no `Nlink > 1` check; macOS not covered

```text
Gap: a pre-existing hardlink with an ordinary name bypasses the lexical credential-path
denial. Option C Phase 1 (PR #381) denies aliases to protected workspace credentials on
Linux only. The reviewer's alternative, a cheap Nlink > 1 check, is not implemented, and
macOS and Windows are not covered. Option C-bounded is NOT authorized.

Goal: a reviewed proposal for the remaining hardlink exposure, then implementation of
whatever the operator approves.

Steps
1. Read docs/plans/PROPOSAL-LLM02-HARDLINK-PATH-IDENTITY.md,
   PROPOSAL-LLM02-SUBPROCESS-HARDLINK.md, and the Phase 1 code and tests. Note that the
   bounded subprocess alias is already RISK_ACCEPTED under Option D.
2. Write a short proposal covering: (a) an Nlink > 1 refusal or warning for read_file,
   search_files, and copy_file sources, with its false-positive cost on real
   repositories (measure it on this one); (b) path-identity enforcement on macOS using
   the same approach as Linux Phase 1; (c) what stays residual, including
   outside-workspace origins and Windows.
3. STOP FOR OPERATOR. Open the proposal as a pull request and end the task.
4. Only after explicit approval in a later session: implement the approved options with
   deterministic tests, then a focused live campaign on each covered platform with a
   positive control, mirroring llm02-option-c-phase1-campaign.json.

Done when (proposal stage): the proposal is open with measured false-positive data and
a clear recommendation. Done when (implementation stage): evidence and the coverage
statement name which platforms deny which alias kinds, and what remains residual.
```

## 6. LLM02 — macOS sandbox: parent environment readable via `kern.procargs2`

```text
Gap: with the opt-in run_command sandbox on, macOS still lets a sandboxed process read
the parent process's environment through kern.procargs2. This is measured, and
process-info* rules do not block it.

Goal: determine whether SOP can close or bound this without weakening the sandbox, and
implement the fix if it is small and safe.

Steps
1. Reproduce it with a synthetic canary in the parent environment, using the existing
   sandbox tests and docs/reports/security-shift-left/seatbelt-evidence.md as a guide.
2. Evaluate options and record the result of each: a Seatbelt rule that does block the
   sysctl; launching the sandboxed command from an intermediate process whose own
   environment is already scrubbed; scrubbing the SOP process environment earlier.
3. If one option closes it with a contained change in internal/toolharness, implement
   it with a regression test that fails when the canary is readable. Confirm an ordinary
   sandboxed go test still passes.
4. If none does, write the finding as a bounded known limit with the reason.

Done when: the evidence file and coverage statement state either VERIFIED closed on
macOS with the test named, or a known limit with the options tried and why they failed.
Do not change the sandbox default; that is gap 13.
```

## 7. LLM02 — model-output artifacts persisted verbatim

```text
Gap: model-output artifacts are persisted verbatim. If a secret reaches the model and
the model repeats it, the secret lands in run artifacts. The residual-risk matrix
recommends INVESTIGATE.

Goal: an investigation with a recommendation, not a redaction change.

Steps
1. List every place model output is persisted: run records under .agent-sdlc/runs/,
   reports, traces, handoffs, decision memory, notifications, and caches. Cite the
   writer for each.
2. For each, record whether it is evidence that must stay verbatim. internal/cli
   deliberately does not rewrite the diff or the implementation report, because altering
   evidence could hide what happened. Respect that rule.
3. Measure the exposure: with a synthetic canary that a test model echoes, which
   artifacts contain it?
4. Propose options with costs: redact at write time for non-evidence artifacts only;
   a redacted sibling copy; file permissions and retention; or accept and document.
5. STOP FOR OPERATOR with the written recommendation.

Done when: a proposal under docs/plans/ lists the artifacts, the measured exposure, and
one recommended option, and the gap register's artifact-risk entry points to it.
```

## 8. LLM02 — operator decisions: Plan 03 acceptance, C1 default, residuals

```text
Gap: several LLM02 items are decisions only the designated approver can make. Plan 03
per-criterion acceptance is NEEDS_HUMAN for criteria 1-4 and 6. C1 is implemented but
disabled in repository configuration. The context-free unknown-format residual is
NOT_APPROVED. Retrieval/handoff is ACCEPT_PROPOSED and provider retention is
TRANSFER_PROPOSED, with neither recorded.

Goal: one decision packet the operator can act on in a single sitting. You make no
decision and record nothing.

Steps
1. Read llm02-acceptance-request.md, the closeout matrix, the six-item residual-risk
   matrix, and PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md.
2. For each Plan 03 criterion still unaccepted, give the evidence, its bounds, and the
   exact command the approver would run (PR #388 added sop outcome --plan with
   --accept, --reject, --annotate, and --run-check for historicalized plans). Verify
   the command syntax against the current CLI before writing it down.
3. For enabling C1 by default: summarize what it covers, the measured residual, the
   false-positive behavior on this repository, and the exact configuration change.
4. For each remaining residual, give the recommended disposition, the evidence, and
   what accepting it means in plain terms.
5. Check that nothing in the packet has been overtaken by PRs #393 and #394.

Done when: a single document under docs/reports/owasp-llm/ lists each decision with
evidence, recommendation, and the command or change that would record it. No
acceptance, waiver, memory entry, or configuration change is made.
```

## 9. LLM03 — scanner integrity: hash-then-exec race, pins, signatures

```text
Gap: optional binary_sha256 and rules_sha256 pins are checked before every scanner run,
but a small window exists between hashing a binary and running it, binary pins are
opt-in and platform-specific so none are committed, publisher signatures are not
checked, and the repository runs in report-only mode with empty required_categories.

Goal: close the race in code, and give the operator a proposal for the policy items.

Steps
1. Read docs/plans/PROPOSAL-SCANNER-BINARY-PINS.md, llm04-supply-chain.md, and the pin
   verification in internal/security.
2. Close the hash-then-exec window: execute exactly the bytes that were hashed. Options
   to evaluate: hash and execute through the same open file descriptor where the
   platform allows it, or copy to a private directory the run owns, hash the copy, and
   execute the copy. Pick the smallest one that works on macOS and Linux; keep the
   existing behavior on platforms where neither is possible and say so.
3. Add a test that swaps the binary after verification and asserts the swapped bytes do
   not run.
4. Write a proposal for the rest: committing per-platform binary pins for the scanners
   CI already pins; verifying publisher signatures where a publisher provides them
   (name the tool and what it would add as a dependency); moving from report-only to
   required categories.
5. STOP FOR OPERATOR on step 4. Implement step 2 and 3 in the same pull request only if
   they do not depend on a policy choice.

Done when: the race is closed with a named test on the platforms where that is possible,
the evidence file records the platforms where it is not, and the policy proposal is
open. The statement must still say provider model provenance is outside SOP.
```

## 10. LLM04 — no production reader verifies repository index rows

```text
Gap: schema 2 gives each repository index row a digest, and repoindex.VerifyRows
recomputes them, but no production reader loads context/index.json, so nothing calls
VerifyRows outside tests. The integrity check exists and is not used.

Goal: decide where verification belongs and wire it in, or record why not.

Steps
1. Confirm the claim: find every caller of repoindex.VerifyRows and every reader of a
   persisted index. Note that sop retrieve builds the index in memory.
2. If a persisted index is read anywhere on a path that feeds a model, call VerifyRows
   there and fail closed with a typed error and a clear recovery message (reindex).
3. If no persisted index is ever read, the digests protect nothing today. Do not invent
   a reader. Instead add a boundary test that fails when a reader of the persisted index
   is added without verification, and document that the control is latent.
4. Read docs/plans/PROPOSAL-REPOINDEX-ROW-DIGESTS.md so the outcome matches option A.

Done when: llm05-poisoning.md and the coverage statement state which of the two cases
holds, with the test named. Keep the statement that a digest proves the bytes are
unchanged, not that the content is benign.
```

## 11. LLM04 — RAG producer does not emit `chunk_hash`

```text
Gap: SOP recomputes an optional chunk_hash on retrieved evidence and rejects a
mismatch (PR #330), but the producer must emit chunk_hash for that to mean anything.
Until it does, evidence is unverified, and document_hash stays unverified.

This task spans two repositories: agentic-sop (consumer) and rag-template (producer).
Confirm with the operator which producer repository and branch to use before editing
anything outside agentic-sop.

Steps
1. In agentic-sop, read docs/plans/PROPOSAL-RAG-EVIDENCE-HASH.md,
   docs/architecture/RAG-RETRIEVAL-INTEGRATION.md, and internal/ragclient. Write down
   the exact hash definition SOP recomputes: algorithm, input bytes, normalization,
   encoding.
2. In the producer, emit chunk_hash on the retrieval-only response using that exact
   definition. Add a shared test vector to both repositories so drift fails a test on
   either side.
3. In agentic-sop, add a contract test that uses the shared vector, and decide with the
   operator whether evidence without chunk_hash should now be rejected or only flagged.
   STOP FOR OPERATOR on that policy choice.
4. Keep retrieval disabled by default.

Done when: both repositories carry the shared vector and passing tests, and
llm05-poisoning.md states that chunk-hashed evidence is verified end to end, with
document_hash and publisher authenticity still listed as unverified.
```

## 12. LLM05 — Slack rendering and channel binding unobserved

```text
Gap: one Slack transport event was DELIVERED, but how Slack rendered the message and
whether it reached the intended channel were not observed. Auto-unfurl is unobserved.

Goal: prepare an observation the operator can perform in a few minutes, then record
what they report. You cannot observe Slack yourself and must not send a message.

Steps
1. Read llm10-output-handling.md, llm10-live-delivery.md, and
   llm10-acceptance-request.md.
2. Write a short observation script for the operator: the exact command to send one
   synthetic notification containing escaped markup, a link, and a mention-shaped
   string; what to look at in Slack (literal text versus rendered markup, no unfurl, no
   mention triggered, correct channel); and a table to fill in.
3. STOP FOR OPERATOR. Do not send the notification.
4. In a later session, given the operator's filled-in table, record the result as
   OBSERVED with date, workspace-independent details only, and no screenshots that
   contain private data.

Done when: the observation script exists in the evidence file, and after the operator
runs it, the coverage statement says what was observed instead of "not observed".
```

## 13. LLM06 — sandbox off by default; Linux host sockets outside `/tmp`, `/run`

```text
Gap: the run_command sandbox (SOP_AGENT_RUN_COMMAND_ISOLATION=1) is off by default, so
normal agent execution is policy-only. On Linux, host Unix sockets under /tmp and /run
are hidden but one elsewhere, for example under $HOME, stays connectable. Every
sandboxed build starts with a cold build cache.

Goal: close the socket gap in code, and give the operator the data to decide on
default-on.

Steps
1. Read llm03-agency.md, docs/plans/PROPOSAL-RUN-COMMAND-ISOLATION.md, and
   internal/toolharness/isolation.go with its Linux bubblewrap arguments.
2. Socket gap: reproduce a connection from inside the sandbox to a host Unix socket
   under $HOME. Close it with the narrowest bubblewrap change that still lets a test
   start and reach its own socket. Add a regression test for both directions.
3. Default-on data: measure, on this repository, cold-cache overhead per run on macOS
   and Linux, which existing tests fail inside the sandbox and why (host loopback,
   missing modules), and behavior on a platform with no sandbox available.
4. Write a proposal: default-on, default-on with an opt-out, or stay opt-in with a
   startup notice, each with its measured cost. STOP FOR OPERATOR.
5. Do not change the default in this task.

Done when: the socket gap is VERIFIED closed on Linux with a named test, and the
default-on proposal is open with measurements.
```

## 14. LLM08 — live RAG with PostgreSQL and embeddings `NOT_RUN`

```text
Gap: retrieval isolation, authorization, and deadline handling are verified
deterministically. A live RAG producer with PostgreSQL, pgvector, and embeddings is
NOT_RUN, because no such deployment is part of the routine configuration.

Goal: one bounded live run against a disposable local deployment.

Steps
1. Read llm09-retrieval.md, docs/architecture/RAG-RETRIEVAL-INTEGRATION.md, and the
   SOP_RAG_* configuration.
2. Bring up a throwaway PostgreSQL with pgvector and an embedding model locally
   (containers are fine). Use only synthetic documents. Record versions.
3. Seed two isolated corpora. Run live cases for: cross-corpus isolation, an
   unauthorized scope, deadline and cancellation, a poisoned chunk that carries an
   embedded instruction (it must arrive as untrusted evidence and trigger no action),
   and one positive control that retrieves correctly.
4. Record the run as a campaign artifact with a hash, then tear the deployment down.
5. Leave retrieval disabled by default and change no control.

Done when: llm09-retrieval.md records the live run with its bounds, the coverage
statement's LLM08 row replaces NOT_RUN with what was observed, and semantic poisoning
inside an authorized repository is still listed as not prevented.
```

## 15. LLM09 — subprocess can write SOP state with the sandbox off; live `NOT_RUN`

```text
Gap: a run_command subprocess can write SOP state unless the opt-in sandbox is on.
Phase 1 of state-write detection is recorded in llm07-state-write-detection-phase1.md.
Live-model and Linux runs for this plan are NOT_RUN.

Goal: detect a state write when the sandbox is off, and collect the missing runs.

Steps
1. Read llm07-misinformation.md and llm07-state-write-detection-phase1.md. State
   exactly what phase 1 detects and what it misses.
2. Design the smallest detection for the sandbox-off case: for example, digest the SOP
   state directory before and after each run_command and treat an unexpected change as
   evidence tampering that stops at NEEDS_HUMAN. It must distinguish SOP's own writes
   from a subprocess's. Reuse existing digest and durable-file helpers.
3. Implement with tests: a subprocess that edits state is detected; SOP's legitimate
   writes are not flagged; detection failure fails closed.
4. Run the Plan 07 checks on Linux amd64, and one bounded live-model run where the
   model is asked to make tests "pass" by editing state.

Done when: the evidence file records detection as IMPLEMENTED and VERIFIED on both
platforms with the live run's bounds, and the coverage statement still says SOP does
not check that model prose is factually correct.
```

## 16. LLM10 — five subprocess paths without process-group termination

```text
Gap: process-group termination is not universal. These paths use exec.CommandContext
without a process group, so a timeout kills only the direct child:
internal/decision/command/command.go,
internal/decisionadapter/providers/julia/runner.go, internal/git, internal/github, and
internal/hipcompare/changes.go.

Goal: every subprocess SOP starts is terminated with its descendants on timeout or
cancellation.

Steps
1. Confirm the list by searching for every exec.Command and exec.CommandContext outside
   tests, and compare with the users of internal/procgroup and the packages that carry
   their own Setpgid helper.
2. Bind internal/procgroup on each remaining path, following an existing caller. Do not
   add a new helper. Check Windows behavior matches what procgroup already does there.
3. For each path add a test that starts a child which spawns a grandchild, cancels the
   context, and asserts both are gone.
4. Add a boundary test that fails when a new exec.CommandContext is introduced outside
   an allowlist without process-group handling, so the gap cannot reopen silently.
5. Check that git and gh invocations that must outlive nothing still behave: no
   detached helpers are expected, but confirm credential helpers are not broken.

Done when: llm06-consumption.md and the coverage statement drop the "not universal"
caveat, or list precisely which path is still excluded and why, with the tests named.
```

## 17. LLM10 — token budget is off by default and is not a dollar cap

```text
Gap: SOP_AGENT_MAX_TOTAL_TOKENS is an opt-in per-run token budget. It is off by default,
it is a token cap and not a dollar cap because SOP has no pricing data, and the request
that crosses the budget still completes.

This gap is optional. Confirm with the operator that a dollar cap is wanted before
building one.

Steps
1. Read llm06-consumption.md and internal/perf/budget.go.
2. Proposal first: a per-model price table supplied by the operator in configuration
   (never fetched, never hardcoded), a dollar budget computed from reported usage, and
   fail-closed behavior when a model has no price. Include whether a default token
   budget should be set, with a measured typical run as the basis. STOP FOR OPERATOR.
3. After approval: implement on top of the existing token accounting, with tests for a
   priced model, an unpriced model, and the one-request overshoot bound.
4. Keep the overshoot limit documented: the crossing request completes, bounded by one
   request's input plus SOP_AGENT_MAX_OUTPUT_TOKENS.

Done when: the proposal is open; and after approval, the evidence file records the cap
as IMPLEMENTED opt-in with its tests and its stated limits.
```

## 18. All — coverage statement and evidence matrix lag PRs #393 and #394

```text
Gap: docs/reports/owasp-llm/coverage-statement.md says it is as of bc735e5c (PR #378)
and does not mention PR #393 (notification redaction ordering: detect credential shapes
before exact-value replacement) or PR #394 (seven-format known-format enhancement).
The evidence matrix README does not mention them either. The gap register does.

Goal: bring the three documents back into agreement. Documentation only.

Steps
1. Read the merge commits and evidence for PRs #393 and #394, and every PR merged to
   main after bc735e5c that touches internal/redact, internal/notify, internal/security,
   internal/toolharness, or docs/reports/owasp-llm.
2. Update the coverage statement's "As of" line, the affected rows and per-risk
   sections, and the evidence matrix, using the gap register as the reference.
3. Check the public summary in docs/reference/PROJECT-STATUS.md and
   docs/architecture/DIAGRAMS.md still agree with the result.
4. Re-run nothing, record no acceptance, and change no status beyond what the merged
   evidence supports.

Done when: the three documents name the same revision and the same statuses, and a
reader can trace each changed sentence to a merged PR.
```
