# End-to-end skill assessment and evaluation

The canonical adapter is [skills/sop-end-to-end/SKILL.md](../../skills/sop-end-to-end/SKILL.md).
It delegates to `sop run`; the execution contract remains in
[EXECUTION.md](../specs/EXECUTION.md), with routing in
[MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) and human decisions in
[APPROVALS.md](../guides/APPROVALS.md).

## Baseline assessment

Inspected on 2026-10-02 at `891ce96`, before this patch. The installed entry was a
standalone `~/.agents/skills/sop-end-to-end/SKILL.md`, with frontmatter
`disable-model-invocation: false`. No corresponding tracked source, catalog entry,
installation entry, plugin copy, or end-to-end skill test existed. `/sop end-end`
was a natural-language trigger in that global skill, not a CLI subcommand; the
tracked `/sop` entry did not document the handoff.

```text
USER
  ↓
/sop end-end (assistant interprets the trigger)
  ↓
global skill: source discovery, lightweight review, run, supervision, summary
  ↓
sop run [named PLAN]
  ↓
planflow.Prepare → graph driver → scheduler/resume → governed task lifecycle
  ↓
configured agent harness → provider runtime → selected model
```

The old skill already delegated execution and instructed agents not to reproduce
the state machine. Duplication was in its instructions, not a second executable
orchestrator: a planning-source precedence list and an IMPLEMENT/validation/REVIEW/
FIX diagram restated behavior owned by SOP. Its `needs_human` requeue explanation
could encourage retries across a real human boundary. It lacked explicit FAIL and
provider-failure handling, bounded observation instructions, and routing/provenance
reporting. Neither the installed skill nor the graph driver used a fixed polling
sleep; this patch must not claim to have removed one.

Authoritative behavior found during inspection:

- `internal/planflow/planflow.go`: source resolution, source SHA/plan ID metadata,
  unresolved active-source preservation, reconciliation, and completed-plan
  handoff with archival. A skill must not choose another source over active work.
- `internal/cli/drive.go` and `internal/scheduler`: dependency scheduling,
  continuation/retry budgets, at most one automatic recovery per blocked task
  per invocation, and fail-closed failed recovery. Local completion is LOCAL_DONE;
  no commit, PR, CI run, or merge is performed by `sop run`.
- `internal/cli/resume.go` and `internal/resume`: next legal action and recovery.
  `sop resume` may persist recovered state; it is not purely an observation command.
- `internal/cli/reconcile.go`: `--list-changed` is read-only; acceptance of changed
  executed tasks is separate and policy/human governed.
- `internal/cli/approval.go`: applicable approvals come from authoritative SOP
  operations. PLANNED task status can coexist with WAITING_FOR_HUMAN run state;
  NEEDS_HUMAN without an explicit request is not an invented approvable gate.
- `internal/cli/routing.go`, `routing_visibility.go`, and `escalation.go`: configured
  class selection, availability fallback, and separately configured bounded
  escalation. SMALL availability fallback is not a generic generation-error retry.
  Routing and escalation are off by default. The skill must not set overrides.
- `internal/cli/report.go`, persisted run reports, routing artifacts, and activity
  events: concise evidence for reporting. Exit zero can mean no runnable work;
  latest report can be historical or belong to another work item.
- Recent synthesis corrections, productive discovery, verified tool mutation,
  and verified ALREADY_SATISFIED are harness/lifecycle contracts. This adapter
  must preserve them, including distinct no-change and no-progress evidence.

Existing `internal/skill` checks validated prompt adapters and scratch-directory
install safety; `internal/dist` checked the generated plugin and installers.
CLI/planflow/resume/routing/approval fixtures already covered most scenarios below.
The outer dirty-tree completion-attribution limitation remains separate; this
skill change does not repair or disguise it.

## Target contract

```text
USER → /sop end-end → tracked skill
     DISCOVER → PREFLIGHT → DELEGATE: sop run [explicit plan]
              → OBSERVE → HUMAN BOUNDARY → RESUME/RECOVER → REPORT
                         ↓
              SOP orchestration → configured providers/models
```

The skill checks operator intent, repository/document prerequisites, existing
task/run evidence, and applicable approvals. It makes one execution invocation
per request. All scheduling, lifecycle work, remediation, routing, and state
changes remain in SOP. Subsequent operator-authorized continuation uses the same
command and existing state. CONTINUE is reported as incomplete/resumable, without
an outer retry loop or a fabricated human requirement.

Foreground output/process observation is preferred. A yielded process is observed
with waits of at most ten seconds, with meaningful user updates at least once per
minute. Supplementary status reads are bounded to three per invocation, and never
launch another run. Losing observation requires a truthful last-known-state report,
not a second execution. No independent run timeout or provider retry budget is added.

## Evaluation matrix

Use an isolated temporary repository for each scenario, deterministic fake agents,
injected diff evidence, real temporary SQLite state, and configured local validation
commands. Routing uses injected availability/factory seams, never real model outages.
Those fixtures test SOP contracts, not real model quality or dirty-tree attribution.
Inputs below are commands the skill delegates or observes, not commands to execute
against the developer's current project state.

| Case | Initial state | Command/input | Expected SOP behavior | Expected skill behavior | Expected terminal state | Forbidden behavior |
| --- | --- | --- | --- | --- | --- | --- |
| 01 New repository/new plan | No state; named valid PLAN and deterministic passing checks | `/sop end-end docs/PLAN-Hardening.md` → `sop run docs/PLAN-Hardening.md` | Bootstrap, resolve plan ID, create DAG once, execute passing lifecycle | Lightweight preflight; delegate; report exact task/gate evidence | LOCAL_DONE tasks, SOP COMPLETE/PASS | Manual init/plan/tasks chain; direct implementation; invented remote completion |
| 02 Partially completed plan | Same plan with satisfied tasks and unfinished/in-flight task | `sop run <same plan>`; implicit run when no plan was specified | Resume the active task; preserve satisfied tasks/history | Observe resumed work and distinguish old completions | Remaining work completes, or actual gate remains | Regenerate DAG; select another task over active work; rerun completed tasks |
| 03 Recoverable BLOCKED | REVIEW_UNRESOLVED, budget remains, dependencies satisfied | `sop run` | Recover each eligible blocked task at most once in this invocation; execute normal gates | Let SOP recover; report recovery/result | LOCAL_DONE on successful recovery | Independent retry/recovery counter; forced retry |
| 04 Unrecoverable BLOCKED | Recovery fails again, dependency cannot complete, or retry budget spent | `sop run`; budget fixture also probes ordinary `sop retry <id>` refusal | Preserve block/history; stop failed recovery; do not execute later work | Stop with original diagnostic and next action | BLOCKED or unresolved dependency; never PASS | Raise budgets; reset state; rerun indefinitely; claim a zero exit means completion |
| 05 APPROVAL_REQUIRED | Explicit pending SOP request on current task/run | Preflight `sop approvals --json`, then `sop task <id>`/`sop report <id>` | Request remains applicable; approval cannot satisfy remaining lifecycle gates | Stop before execution; show actual request and human resolution action | Pending request, APPROVAL_REQUIRED/WAITING_FOR_HUMAN | Approve on behalf of user; infer authorization from end-to-end intent |
| 06 NEEDS_HUMAN | WAITING_FOR_HUMAN stage; may be PLANNED after requeue; request may be absent | Same plan request; inspect task/run and approval operations | Preserve genuine human boundary; no request means no synthetic approval operation | Stop; explain exact decision/manual action; distinguish approval from other human input | NEEDS_HUMAN; task status and run stage reported separately | Retry because task says PLANNED; manufacture an approval target |
| 07 False IMPLEMENT success | Change required; no actual change; green validation or model claim of no required changes | `sop run <plan>` | No PASS without verified mutation or explicit verified ALREADY_SATISFIED proof; preserve no-change/no-progress distinction | Report incomplete CONTINUE and evidence; recommend SOP continuation | Non-pass/requeued state; budgets may eventually block | Treat model assertion/no-op as mutation; fake changed files; equate CONTINUE with approval |
| 08 Validation failure and FIX | IMPLEMENT change; check fails until first FIX; alternate fixture never repairs | `sop run <plan>` | Run bounded FIX/revalidation; PASS on repair, NEEDS_HUMAN/non-pass on exhaustion | Observe checks/cycles; report actual result | PASS or genuine bounded failure/human boundary | Separate skill FIX commands; weaken checks; extra cycles |
| 09 SMALL availability fallback | Routing enabled, SMALL selected, local probe unavailable, configured fallback | `sop run <plan>` with existing configuration | Resolve configured cloud fallback and record selection/reason | Report actual class/provider/model and reason; change nothing | Normal lifecycle result with explicit fallback evidence | Pick a model/tier; overwrite mappings; apply availability fallback to arbitrary generation failure |
| 10 Completed plan repeated | Same plan fingerprint; all tasks satisfied | `sop run <same plan>` | Reuse graph; no implementation or task recreation; no new archive | Report already completed, zero newly executed tasks; old checks are historical | SOP COMPLETE; existing satisfaction preserved | Claim fresh tests ran; duplicate DAG/history; fake new work |
| 11 Resume/reconcile state | Interrupted task or materially changed executed task definition | Same/implicit `sop run`; inspect `sop reconcile <plan> --list-changed --json` when gated | Resume same task; reconcile safe changes; gate unapproved executed changes; preserve history and tree | Surface authoritative source/fingerprint and required resolution; inspect only | Resumed completion, or reconciliation decision required | Restart, delete state, accept changed executed tasks without authorization, overwrite dirty user work |
| 12 Provider/model failure | Selected provider fails generation or selected factory unavailable | `sop run <plan>` under unchanged configuration | Fail/requeue according to existing policy, or explicitly configured bounded escalation; no silent substitution | Report diagnostic and actual recorded selection; stop at SOP terminal result | Non-pass or configured recovery result; never fabricated PASS | Swap provider/model; bypass validation/capability checks; use SMALL availability fallback after generation failure |

## Executable checks

`TestEndToEndSkillEvaluation` in `internal/cli/end_to_end_skill_test.go` groups the
existing command fixtures under the twelve case IDs. It calls their assertions
unchanged rather than creating another orchestration harness. Important examples:

- 01/02/10: named-plan bootstrap, active-plan resume, active-task preference,
  same-plan idempotency, and satisfied-task skipping.
- 03/04: recovery continuation, once-per-task bound, failed-recovery stop,
  dependency protection, and exhausted retry refusal.
- 05/06: active approval and WAITING_FOR_HUMAN preservation, approval never bypasses
  gates, and absence of an explicit request never invents an approval.
- 07: no-change false completion and verified ALREADY_SATISFIED.
- 08: actual deterministic failed-check repair and FIX exhaustion.
- 09/12: configured unavailable-local fallback, generation failure without
  availability substitution, and selected-model factory fail-closed.
- 11: interrupted task resume, executed-change gate, read-only listing, and
  preservation of dirty files and history.

`internal/skill/end_to_end_test.go` checks the tracked adapter contract, alias
handoff, observation/human/completion/reporting instructions, and the constrained
command examples. Existing catalog/install/plugin tests cover distribution.
Run:

```bash
go test ./internal/skill/... ./internal/dist/...
go test ./internal/cli/... -run '^TestEndToEndSkillEvaluation$' -count=1 -v
go test ./internal/planflow/... ./internal/resume/... ./internal/scheduler/... ./internal/ollamaagent/... ./internal/toolharness/...
go test ./...
git diff --check
```

The full suite already had twelve CLI/JEV failures at this baseline, listed in
[PROJECT-STATUS.md](../reference/PROJECT-STATUS.md#current-checkout-verification-2026-10-02).
Report actual fresh failures rather than changing their expectations in this patch.

### Results for this patch (2026-10-02)

- Skill contracts, installer safety, and plugin distribution tests: PASS.
- All twelve grouped deterministic evaluation scenarios: PASS.
- Planflow, resume, scheduler, Ollama harness, and toolharness regressions: PASS.
- `go vet ./...`, `go build ./...`, plugin mirror check, documentation links,
  modified-Go formatting, and `git diff --check`: PASS.
- `go test ./...`: FAIL in `internal/cli` only. The twelve failing test names
  exactly match the earlier baseline full-suite log; all other packages pass.
  The failures are the existing no-change validation test and eleven JEV evidence/
  legacy-bootstrap tests listed in PROJECT-STATUS. No expectations were weakened.
- Required code review found no correctness/security issue in the patch.
  Live host-assistant execution of the twelve scenarios remains unperformed.

## Evaluation limits and manual check

The adapter is natural-language instructions, not executable scheduling code.
Structural checks plus command fixtures do not prove that every host assistant
will follow them. In a disposable project, manually invoke `/sop end-end <plan>`
and record commands, user updates, SOP artifacts, and final summary. Repeat with
partial/completed state and an actual pending human gate. The assistant must use
one run, preserve state, stop at the gate, and report only evidenced results.
No live end-to-end assistant evaluation is implied by the deterministic suite.

Report fields are Plan; Current/terminal state; Tasks completed this invocation
versus earlier; Tasks remaining; Validation/tests; Review result; Routing/provider
evidence; Blockers; Human approval required; Recommended next action. Missing
metrics/evidence are UNAVAILABLE; skipped checks are NOT RUN. Prompt contents,
credentials, full internal responses, and invented Git/PR/CI operations are excluded.
