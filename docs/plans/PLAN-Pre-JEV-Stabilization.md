# Pre-JEV Stabilization Plan

**Type:** Implementation plan (workstream record).

**Status:** Implemented and archived by SOP (`.agent-sdlc/archive/plan-pre-jev-stabilization/`).
The path is kept because SOP's archive records this source; see [../README.md](../README.md)
for what is current.

## Project

agentic-sop

## Summary

Stabilize Agent Harness V2, harden the Ollama bootstrap, align
`sop-controller` with the native tool harness, and establish a clean
dogfooded baseline before JEV V1.

This plan is separate from `PLAN-Agent-Harness-V2.md`. Do not replace
the current Harness V2 plan.

## Architecture Boundary

``` text
agentic-sop     = workflow authority, lifecycle, validation, review/fix gates
sop-controller  = visibility and human control; delegates to SOP
Ollama harness  = controlled model/tool execution
JEV             = future decision/verification signal, not workflow authority
```

Do not duplicate SOP orchestration in the controller, harness, or JEV.

## Existing Workstreams

Do not duplicate the Safe Task Reset or SOP Performance plans.
Verify/reference those capabilities where relevant.

## PREJEV001 --- Establish Harness V2 Completion Baseline

Inspect AHV2001--AHV2013 and classify blocked tasks as failures caused
by now-fixed harness lifecycle defects or genuine task-specific defects.
Preserve LOCAL_DONE work and do not manually edit SOP state.

### Acceptance Criteria

-   current AHV task states are documented
-   blocked tasks are classified by cause
-   LOCAL_DONE tasks are preserved
-   no manual `state.db` edits occur
-   baseline build/test status is recorded

## PREJEV002 --- Prove IMPLEMENT Deterministic Completion

Verify:

``` text
DISCOVER → CHANGE → mutation-aware completion → FINALIZE
          → structured outcome → SOP validation
```

### Acceptance Criteria

-   productive IMPLEMENT returns before the hard ceiling
-   no-mutation thresholds do not prematurely disable tools
-   mutation enables deterministic finalization
-   early valid completion works
-   an end-to-end implementation produces `validation runs > 0`
-   no global iteration increase is used as the fix

## PREJEV003 --- Implement REVIEW Inspect-to-Synthesize

Target:

``` text
REVIEW → INSPECT (≤8 read-only calls)
       → SYNTHESIZE (tools disabled, ≤2 turns)
       → structured review result
```

Use a soft completion nudge around inspection call 6.

### Acceptance Criteria

-   REVIEW remains read-only
-   early completion works
-   inspection is bounded
-   inspection exhaustion enters synthesis
-   synthesis cannot execute tools
-   synthesis is bounded
-   phase-specific failure diagnostics exist
-   fake-model tests cover the lifecycle

## PREJEV004 --- Prove AHV2009 End-to-End

Retry AHV2009 after IMPLEMENT and REVIEW completion behavior is stable.

### Acceptance Criteria

-   PLAN completes
-   IMPLEMENT completes
-   deterministic validation runs
-   REVIEW completes
-   quality gate passes
-   human commit gate remains enforced
-   AHV2009 reaches LOCAL_DONE without committing

## PREJEV005 --- Reconcile AHV2001 Through AHV2007

Retry earlier blocked tasks individually and in dependency order:

``` text
AHV2001
AHV2002
AHV2003
AHV2004
AHV2006
AHV2007
```

Prefer verify-first when later work already satisfies an earlier task.

### Acceptance Criteria

-   tasks are retried individually
-   already-satisfied work is not unnecessarily rewritten
-   genuine failures remain visible
-   no manual state changes are used
-   LOCAL_DONE tasks remain intact

## PREJEV006 --- Complete Remaining Harness V2 Tasks

Complete/reconcile AHV2010--AHV2013. Preserve a generic command/external
adapter but do not require Claude.

### Acceptance Criteria

-   Harness V2 has no unexplained blocked tasks
-   Ollama tool harness is the intended native path
-   external command compatibility remains optional
-   no Claude dependency is required
-   Ollama dogfood passes
-   documentation matches the architecture

## PREJEV007 --- Split Capability Runners From Shared Harness

Refactor capability state machines out of an oversized shared harness.

Suggested shape:

``` text
internal/ollamaagent/
  harness.go
  policy.go
  plan.go
  implement.go
  review.go
  fix.go
  outcome.go
  trace.go
  tools.go
```

### Acceptance Criteria

-   shared harness owns common orchestration
-   PLAN lifecycle is isolated
-   IMPLEMENT lifecycle is isolated
-   REVIEW lifecycle is isolated
-   FIX has a clear isolated seam
-   policy remains centralized
-   behavior is preserved by tests

## PREJEV008 --- Normalize Invocation-Scoped Execution State

Keep mutation observation, phase, counters, no-progress fingerprints,
and finalization state scoped to one invocation.

### Acceptance Criteria

-   invocation A cannot contaminate invocation B
-   mutation/phase/no-progress state is invocation-scoped
-   failed/denied mutations do not count
-   reuse of one Harness across invocations is tested

## PREJEV009 --- Harden Repository-Change Reconciliation

Distinguish a pre-dirty working tree from changes caused by the current
invocation. Use invocation mutation evidence as the primary execution
signal and git/repository state as reconciliation evidence.

### Acceptance Criteria

-   pre-existing changes are not attributed to the invocation
-   successful controlled mutations are observable
-   `changes_expected` remains SOP-compatible
-   model claims cannot override observed reality
-   pre-dirty scenarios have tests

## PREJEV010 --- Replace Self-Building Bootstrap With Known-Good Binary

Remove the failure mode where every Ollama-agent invocation compiles
candidate working-tree source first.

Target:

``` text
install known-good sop-ollama-agent
  → SOP invokes installed binary
  → agent edits candidate source
  → SOP validates candidate source
```

### Acceptance Criteria

-   normal invocation does not compile current candidate source first
-   known-good agent still runs when candidate agent source does not
    compile
-   install/update workflow is deterministic
-   version/source diagnostics are available
-   no automatic Claude fallback exists

## PREJEV011 --- Normalize Capability Budgets and Policies

Review PLAN, IMPLEMENT, REVIEW, DESIGN_TESTS, FIX, and diagnosis
policies as one coherent system. Budgets are safety rails, not
substitutes for lifecycle completion.

### Acceptance Criteria

-   stale "increase budget because model explores" logic/comments are
    removed
-   PLAN/IMPLEMENT/REVIEW/FIX completion policies are explicit
-   hard ceilings and soft thresholds are distinct
-   policy is centralized and testable
-   scattered magic numbers are avoided

## PREJEV012 --- Full Agent Harness Regression Suite

Umbrella regression milestone. The full agent harness regression suite is
decomposed into focused sub-tasks, each with one responsibility and its own
validation, so any one can be understood, implemented, validated, and completed
independently. The umbrella completes only after every sub-task is validated.

``` text
PREJEV012   Full Agent Harness Regression Suite
├── PREJEV012-S6    Repository reconciliation
├── PREJEV012-S7    Pre-dirty working tree
├── PREJEV012-S8    Provider/model selection
├── PREJEV012-S9    Command adapter compatibility
├── PREJEV012-S10   Bootstrap behavior
├── PREJEV012-S11   Determinism and race hardening
└── PREJEV012-S12   Coverage closure
```

Every sub-task runs the same three-step loop: (1) verify existing coverage
first, (2) implement only the missing coverage, and (3) validate independently
with SOP's own VALIDATE stage, not the agent's self-report. A sub-task with no
real gap completes with no change (`changes_expected=false`) rather than
manufacturing work, and a requirement already proven by existing tests is
recorded, not rewritten.

A SOP plan is a flat dependency graph, so the parent/child relationship is
expressed two ways: each sub-task is named `<umbrella>-<sub>`, and this milestone
depends on every sub-task, becoming ready only once they are done. Exact commands
and the existing-coverage map live in `docs/history/PREJEV012-REGRESSION-DECOMPOSITION.md`
and `docs/tasks/prejev012/`.

### Dependencies

-   PREJEV012-S6
-   PREJEV012-S7
-   PREJEV012-S8
-   PREJEV012-S9
-   PREJEV012-S10
-   PREJEV012-S11
-   PREJEV012-S12

### Acceptance Criteria

-   every sub-task PREJEV012-S6 through PREJEV012-S12 is validated
-   standard tests require no live Ollama
-   deterministic lifecycle coverage exists
-   race tests pass
-   build passes
-   regressions produce focused failures

### Validation

``` bash
go test ./internal/ollamaagent/...
go test ./internal/e2e/...
go test ./...
go vet ./...
go build ./...
go test -race ./internal/ollamaagent/...
go test -race ./internal/e2e/...
```

### Execution

-   implement

## PREJEV012-S6 --- Repository Reconciliation

Verify repository-change detection and reconciliation: changed, unchanged, and
restored files, and that observed repository reality overrides the model's
`changes_expected` claim. Package: `internal/ollamaagent` (reconcile/outcome) and
`internal/toolharness`. Verify existing coverage first, implement only the
missing coverage, then validate independently; complete with no change when the
behavior is already covered.

### Dependencies

None

### Acceptance Criteria

-   changed, unchanged, and restored repository states are each covered
-   observed reality overrides the model's change claim in both directions
-   the emitted outcome wire shape stays SOP-compatible
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/ollamaagent/ -run 'Reconcile|ObservedRepositoryChange|ChangedSince|MutationEvidence|PreDirty|OutcomeWire'
go test ./internal/ollamaagent/... ./internal/toolharness/...
```

### Execution

-   implement

## PREJEV012-S7 --- Pre-Dirty Working Tree

Verify that pre-existing user changes are preserved and are never falsely
attributed to the agent. Package: `internal/ollamaagent`, `internal/e2e/harness`,
and `internal/e2e/lifecycle`. Verify existing coverage first, implement only the
missing coverage, then validate independently.

### Dependencies

-   PREJEV012-S6

### Acceptance Criteria

-   a pre-existing modification and an untracked file each leave the tree dirty
-   a no-change claim over a pre-dirty tree reconciles to changes_expected=false
-   a mutating invocation over a pre-dirty tree is attributed; pre-existing changes are not
-   pre-existing user changes are preserved, not reverted
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/ollamaagent/ -run 'PreDirty'
go test ./internal/e2e/... -run 'Repo|Isolation'
```

### Execution

-   implement

## PREJEV012-S8 --- Provider/Model Selection

Verify provider/model configuration, defaults, environment overrides, precedence,
and Ollama selection, plus clear failure for an invalid selection. Package:
`internal/agent` (`provider.go`) and `internal/config`. Verify existing coverage
first, implement only the missing coverage, then validate independently.

### Dependencies

None

### Acceptance Criteria

-   the effective provider resolves environment then configuration then default, reporting the source
-   an unknown configured provider fails clearly instead of falling back silently
-   the Ollama provider requires a model; the environment overrides configuration
-   an invalid selection produces a focused failure
-   standard tests require no live Ollama

### Validation

``` bash
go test ./internal/agent/ -run 'Provider|Model|FromEnv|FromConfig|Effective|Ollama|LlamaCpp'
go test ./internal/agent/... ./internal/config/...
```

### Execution

-   implement

## PREJEV012-S9 --- Command Adapter Compatibility

Verify the command-based adapter remains compatible with the stabilized Agent
Harness contract: a JSON request on stdin, raw response on stdout, structured
outcomes passed through, and diagnostics on failure. Package: `internal/agent`
(`command.go`, `harness.go`) and `internal/e2e/harness`. Verify existing coverage
first, implement only the missing coverage, then validate independently.

### Dependencies

None

### Acceptance Criteria

-   the JSON request, including its capability, round-trips through the adapter unchanged
-   every capability is accepted; an unknown capability is rejected without invoking the command
-   a structured outcome in stdout is preserved; prose yields none
-   a failing command surfaces its diagnostics and names the capability
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/agent/ -run 'Command'
go test ./internal/e2e/harness/ -run 'Adapter'
```

### Execution

-   implement

## PREJEV012-S10 --- Bootstrap Behavior

Verify the known-good `sop-ollama-agent` bootstrap/install path and that runtime
execution does not self-build candidate source. Package: `internal/agentbin`,
`scripts/install/install-sop-ollama-agent.sh`, and `scripts/agents/sop-ollama-agent.sh`. Verify
existing coverage first, implement only the missing coverage, then validate
independently.

### Dependencies

-   PREJEV012-S8
-   PREJEV012-S9

### Acceptance Criteria

-   resolution order is override, then configured home, then the default install directory
-   only a regular, executable file resolves; a missing binary is an actionable ErrNotInstalled
-   the bootstrap wrapper never compiles candidate source; the default install directory is outside the working tree
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/agentbin/...
go vet ./... && go build ./...
```

### Execution

-   implement

## PREJEV012-S11 --- Determinism and Race Hardening

Verify invocation isolation, deterministic lifecycle behavior, and race-sensitive
shared state. Package: `internal/e2e/harness`, `internal/e2e/lifecycle`, and
`internal/ollamaagent`. Verify existing coverage first, implement only the
missing coverage, then validate independently.

### Dependencies

-   PREJEV012-S6
-   PREJEV012-S7
-   PREJEV012-S8
-   PREJEV012-S9
-   PREJEV012-S10

### Acceptance Criteria

-   canned responses are returned in order; a call past the end yields a stable fallback
-   the deterministic clock and id generator advance only when asked
-   one invocation cannot observe another's recorded requests or mutation state
-   concurrency-sensitive shared state passes under the race detector
-   the focused and broader -race validation commands pass

### Validation

``` bash
go test -race ./internal/e2e/...
go test -race ./internal/ollamaagent/...
```

### Execution

-   implement

## PREJEV012-S12 --- Coverage Closure

Run the complete harness regression suite, identify remaining uncovered
acceptance criteria, and add only genuinely missing coverage. This closes the
umbrella. Package: the whole suite. Verify existing coverage first, implement
only the missing coverage, then validate independently.

### Dependencies

-   PREJEV012-S6
-   PREJEV012-S7
-   PREJEV012-S8
-   PREJEV012-S9
-   PREJEV012-S10
-   PREJEV012-S11

### Acceptance Criteria

-   the complete standard suite passes with no live Ollama
-   go vet ./... and go build ./... pass
-   the -race runs pass for the harness packages
-   every PRD coverage bullet maps to at least one passing regression test
-   each gap is closed by its named proving test or recorded as already covered
-   no unrelated feature is implemented and no correct behavior is rewritten

### Validation

``` bash
go test ./internal/ollamaagent/...
go test ./internal/e2e/...
go test ./...
go vet ./...
go build ./...
go test -race ./internal/ollamaagent/...
go test -race ./internal/e2e/...
```

### Execution

-   implement
## PREJEV013 --- Align sop-controller Configuration

Move `sop-controller` to the stabilized native Agent Harness path
instead of a project-specific coding-agent implementation.

Target:

``` yaml
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
```

Environment overrides remain supported.

### Acceptance Criteria

-   controller delegates execution to SOP
-   controller has no duplicate model tool loop
-   native Ollama harness configuration works
-   provider/model remain configurable
-   SOP remains workflow authority
-   controller UI/control behavior remains intact

## PREJEV014 --- Remove Obsolete Controller Agent Assumptions

Update controller documentation/adapters. Remove obsolete claims that
Ollama categorically cannot implement when the tool harness is
available. Do not make Claude the default implementation requirement.

### Acceptance Criteria

-   README accurately describes the native harness
-   Ollama + configurable model is documented
-   DeepSeek is described as a model, not a harness
-   Claude is not required/default
-   generic external adapter is optional if retained
-   no duplicate orchestration is added

## PREJEV015 --- Controller-to-SOP End-to-End Dogfood

Exercise:

``` text
controller
  → SOP
  → Agent Harness
  → Ollama
  → configured model
  → controlled tools
  → SOP validation/review/gate
  → controller visibility
```

### Acceptance Criteria

-   controller delegates through SOP
-   SOP remains state authority
-   native tool harness performs agent execution
-   validation/review/gates are visible through controller
-   human approval remains intact
-   no second controller state machine appears

## PREJEV016 --- Verify Recovery Paths

Umbrella recovery-regression milestone. Recovery is verified non-destructively —
resume, retry, interrupted active-task recovery, named-plan provenance, safe task
reset if it already exists, and working-tree preservation — and the milestone is
decomposed into focused sub-tasks, each with one responsibility and its own
validation, so any one can be understood, verified, and completed independently.
The umbrella completes only after every sub-task is validated.

``` text
PREJEV016   Verify Recovery Paths
├── PREJEV016-S1    Resume continues valid work
├── PREJEV016-S2    Retry requeues legal blocked work
├── PREJEV016-S3    Interrupted active-task recovery
├── PREJEV016-S4    Named-plan provenance
├── PREJEV016-S5    Safe task reset, if available
├── PREJEV016-S6    Non-destructive recovery and working-tree preservation
└── PREJEV016-S7    Recovery regression closure
```

Every sub-task is verification-first: (1) it verifies existing coverage first,
(2) it adds a test only where a real gap is demonstrated, and (3) it validates
independently with SOP's own VALIDATE stage, not the agent's self-report. A
sub-task with no real gap completes with no change (`changes_expected=false`)
rather than manufacturing work, and a requirement already proven by existing
tests is recorded, not rewritten. PREJEV016 passes when the required recovery
behavior is proven by deterministic validation, whether or not a repository change
was needed.

The areas already proven by the existing suite run in `verify-first` mode: the
deterministic validation passes, so no implementation agent is invoked and the
sub-task completes with `changes_expected=false`. The one sub-task with a genuine
coverage gap (PREJEV016-S6) and the closure sub-task run in `implement` mode, so
the agent can add the missing coverage; `verify-first` would short-circuit before
it could add anything.

A SOP plan is a flat dependency graph, so the parent/child relationship is
expressed two ways: each sub-task is named `<umbrella>-<sub>`, and this milestone
depends on every sub-task, becoming ready only once they are done. Exact commands
and the existing-coverage map live in
`docs/history/PREJEV016-RECOVERY-DECOMPOSITION.md` and `docs/tasks/prejev016/`.

### Dependencies

-   PREJEV016-S1
-   PREJEV016-S2
-   PREJEV016-S3
-   PREJEV016-S4
-   PREJEV016-S5
-   PREJEV016-S6
-   PREJEV016-S7

### Acceptance Criteria

-   every sub-task PREJEV016-S1 through PREJEV016-S7 is validated
-   resume, retry, interrupted active-task recovery, and named-plan provenance are each proven by a passing test
-   safe task reset is verified if available or recorded as not applicable
-   recovery requires no deletion of `state.db` or run history
-   recovery preserves pre-existing working-tree changes and uses no `git reset --hard` or `git clean`
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/resume/...
go test ./internal/cli/...
go test ./...
go vet ./...
go build ./...
```

### Execution

-   implement

## PREJEV016-S1 --- Resume Continues Valid Work

Verify `sop resume` continues valid interrupted work: it resolves the single
in-flight task (or the named task id), reports the next legal action through the
shared status→action decision, persists the reconciliation, and continues from the
reconciled status instead of restarting or duplicating work. Package:
`internal/resume` and `internal/cli` (`resume.go`). Verify existing coverage
first, implement only the missing coverage, then validate independently; complete
with no change when the behavior is already covered.

### Dependencies

None

### Acceptance Criteria

-   resume resolves the single in-flight task and reports the next legal action
-   a persisted recovery means a subsequent resume continues from the advanced status
-   resume with no in-flight task reports nothing to resume without mutating state
-   resume reconciles an existing branch or pull request instead of recreating it
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/resume/...
go test ./internal/cli/ -run 'Resume'
```

### Execution

-   verify-first

## PREJEV016-S2 --- Retry Requeues Legal Blocked Work

Verify retry requeues a task only when retry is legal for its blocked status,
preserves task history so the retry resumes the same task, and refuses completed,
merged, or non-blocked statuses. Package: `internal/cli` (`retry.go`) and
`internal/domain` (`Requeue`). Verify existing coverage first, implement only the
missing coverage, then validate independently; complete with no change when the
behavior is already covered.

### Dependencies

None

### Acceptance Criteria

-   retry requeues a retryable BLOCKED task to PLANNED and preserves its task history (same task id)
-   retry refuses a completed, merged, or non-blocked task
-   retry reports an exhausted retry budget instead of requeueing silently; `--force` raises the budget deliberately
-   retry never requires deleting `state.db`
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/cli/ -run 'Retry'
go test ./internal/domain/ -run 'Requeue|Transitions|Terminal'
```

### Execution

-   verify-first

## PREJEV016-S3 --- Interrupted Active-Task Recovery

Verify `sop run` recovers an interrupted active task safely: exactly one in-flight
task is resumed rather than starting another, several in-flight tasks refuse to
proceed with a clear diagnostic naming them, and a scheduled-but-missing or
remote-parked active task is refused safely. Package: `internal/cli` (`drive.go`,
`run.go`). Verify existing coverage first, implement only the missing coverage,
then validate independently; complete with no change when the behavior is already
covered.

### Dependencies

-   PREJEV016-S1

### Acceptance Criteria

-   exactly one in-flight task is resumed instead of starting another
-   multiple in-flight tasks refuse to proceed with a clear diagnostic naming the tasks
-   a scheduled-but-missing or remote-parked active task is refused precisely rather than guessed
-   recovery of interrupted active work never requires editing or deleting `state.db`
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/cli/ -run 'Active|Interrupted|Resume'
go test ./internal/cli/ -run 'Run'
```

### Execution

-   verify-first

## PREJEV016-S4 --- Named-Plan Provenance

Verify named plans cannot silently mix graphs: a run bound to a named plan cannot
silently resume against a different plan graph, and a provenance mismatch produces
a focused failure. Package: `internal/cli` (`run.go` plan resolution) and
`internal/planflow`. Verify existing coverage first, implement only the missing
coverage, then validate independently; complete with no change when the behavior is
already covered.

### Dependencies

None

### Acceptance Criteria

-   a named plan resolves to its own graph
-   a run against a different active plan stops with a focused `different plan is already active` failure
-   a changed plan source stops with a focused `plan changed` failure
-   an ambiguous or missing named-plan selection fails clearly rather than mixing graphs
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/cli/ -run 'NamedPlan|DifferentPlan|ChangedPlan'
go test ./internal/planflow/...
```

### Execution

-   verify-first

## PREJEV016-S5 --- Safe Task Reset, If Available

Determine whether a safe task-reset capability exists. If it exists, verify that it
resets only the intended task and preserves run history and `state.db`. If it does
not exist, record the requirement as not applicable and do not implement a reset
feature to satisfy this milestone. Package: `internal/cli`, `internal/domain`,
`internal/store`. Verify existing coverage first, implement only the missing
coverage, then validate independently.

### Dependencies

None

### Acceptance Criteria

-   the presence or absence of a safe task-reset capability is determined and recorded
-   if present, reset affects only the intended task and preserves run history and `state.db`
-   if absent, the requirement is recorded as not applicable and no reset feature is added
-   safe reset is never used as a substitute for non-destructive recovery
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/cli/...
go test ./internal/store/...
```

### Execution

-   verify-first

## PREJEV016-S6 --- Non-Destructive Recovery and Working-Tree Preservation

Verify that recovery preserves pre-existing user changes, does not invoke
`git reset --hard` or `git clean`, and does not delete `.agent-sdlc`, `state.db`,
or run history. Package: `internal/cli` (recovery commands) and `internal/store`.
Verify existing coverage first, implement only the missing coverage, then validate
independently. This is the one sub-task with a confirmed coverage gap.

### Dependencies

-   PREJEV016-S1
-   PREJEV016-S2
-   PREJEV016-S3

### Acceptance Criteria

-   recovery preserves a pre-existing modified tracked file and an untracked file
-   recovery does not invoke `git reset --hard` or `git clean`
-   recovery does not delete the state database or run history
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/cli/ -run 'PreservesWorkingTree'
go test ./internal/cli/...
go test ./internal/resume/...
```

### Execution

-   implement

## PREJEV016-S7 --- Recovery Regression Closure

Run the complete recovery suite plus the broader standard suite, confirm every
PREJEV016 acceptance criterion maps to at least one passing verification, add only
genuinely missing coverage, and complete with no change where behavior is already
covered. This closes the umbrella. Package: the whole suite. Verify existing
coverage first, implement only the missing coverage, then validate independently.

### Dependencies

-   PREJEV016-S1
-   PREJEV016-S2
-   PREJEV016-S3
-   PREJEV016-S4
-   PREJEV016-S5
-   PREJEV016-S6

### Acceptance Criteria

-   every PREJEV016 acceptance criterion maps to at least one passing test
-   the full recovery suite, the standard suite, `go vet ./...`, and `go build ./...` pass
-   gaps are closed by named proving tests or recorded as already covered
-   no correct recovery behavior is rewritten and no out-of-scope feature is implemented

### Validation

``` bash
go test ./internal/resume/...
go test ./internal/cli/...
go test ./...
go vet ./...
go build ./...
```

### Execution

-   implement
## PREJEV017 --- Capture Pre-JEV Performance Baseline

Umbrella measurement-and-reporting milestone. The pre-JEV performance baseline is
captured from SOP's *existing* instrumentation and already-recorded run artifacts —
no new instrumentation, no optimization, and no code change is required to complete
it. The milestone is decomposed into focused sub-tasks, each with one responsibility
and its own validation, so any one can be understood, verified, and completed
independently. The umbrella completes only after every sub-task is validated.

``` text
PREJEV017   Capture Pre-JEV Performance Baseline
├── PREJEV017-S1    Baseline evidence inventory and environment
├── PREJEV017-S2    PLAN/IMPLEMENT/REVIEW workflow measurement
├── PREJEV017-S3    Controller-driven workflow measurement
├── PREJEV017-S4    Metric availability, bottlenecks, and limitations
└── PREJEV017-S5    Baseline recording and closure
```

The baseline records, from existing artifacts only:

``` text
total time
agent time
validation time
review time
agent calls
tool calls where available
validation runs
fix cycles
```

The previously-run PREJEV017 implementation executed in `implement` mode, collected
the required evidence with no repository mutation, and because a change was expected
entered FIX; FIX then attempted unrelated test work and exhausted its iteration
budget. This milestone is therefore **verify-first**: the deterministic validation
runs before any agent, and when it passes the milestone completes with no change
(`changes_expected=false`). PREJEV017 passes when the required baseline is supported
by the existing instrumentation and the captured run artifacts, whether or not any
repository file changed. Optimization is explicitly out of scope and remains in
`docs/plans/PLAN-SOP-Performance.md`.

A SOP plan is a flat dependency graph, so the parent/child relationship is expressed
two ways: each sub-task is named `<umbrella>-<sub>`, and this milestone depends on
every sub-task, becoming ready only once they are done. Exact commands and the
metric/evidence map live in `docs/history/PREJEV017-PERFORMANCE-DECOMPOSITION.md` and
`docs/tasks/prejev017/`.

### Dependencies

-   PREJEV017-S1
-   PREJEV017-S2
-   PREJEV017-S3
-   PREJEV017-S4
-   PREJEV017-S5

### Acceptance Criteria

-   a PLAN/IMPLEMENT/REVIEW workflow is measured from existing run artifacts
-   a controller-driven workflow is measured from existing run artifacts
-   agent time and deterministic validation time are distinguishable
-   known bottlenecks are documented with the metric and evidence that support them
-   tool calls are reported only when the current instrumentation exposes them, otherwise stated as unavailable from the perf metrics
-   missing instrumentation is documented as a limitation, not implemented here
-   optimization remains deferred to the Performance plan
-   existing unrelated working-tree changes are preserved
-   successful verification may complete with `changes_expected=false`
-   a completed no-change outcome does not trigger FIX merely because no repository mutation occurred
-   every sub-task PREJEV017-S1 through PREJEV017-S5 is validated

### Validation

``` bash
go test ./internal/perf/...
go test ./internal/cli/...
go test ./...
go vet ./...
go build ./...
```

### Execution

-   verify-first

## PREJEV017-S1 --- Baseline Evidence Inventory and Environment

Confirm the repository is healthy at the captured revision and inventory the
existing measurement surface the baseline draws on: `internal/perf` (stages,
`Counts`, `Task`, `Run`, `CategoryMS`, `WriteTask`, `WriteRun`), the recorder wiring
in `internal/cli` (`run.go`, `drive.go`, `report.go`), and the already-recorded run
artifacts under `.agent-sdlc/runs/`. Package: `internal/perf`, `internal/cli`.
Verify existing coverage first, implement only the missing coverage, then validate
independently. No production code is changed.

### Dependencies

-   none

### Acceptance Criteria

-   the captured revision and working-tree state are recorded
-   the existing instrumentation surface is inventoried from the repository, and no new field or metric is claimed
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/perf/...
go test ./internal/cli/...
go test ./...
```

### Execution

-   verify-first

## PREJEV017-S2 --- PLAN/IMPLEMENT/REVIEW Workflow Measurement

Measure a PLAN/IMPLEMENT/REVIEW workflow from the existing per-task artifacts:
total time, agent time (plan + implement), validation time, review time, agent
calls, validation runs, and fix cycles, with the validation `build`/`test`/`lint`
breakdown where the runner reported it. Package: `internal/perf`, `internal/cli`.
Verify existing coverage first (`TestPerformanceBenchmarkFixture`,
`TestReportShowsPerformance`), implement only the missing coverage, then validate
independently. Tool calls are recorded only where the existing output exposes them.

### Dependencies

-   PREJEV017-S1

### Acceptance Criteria

-   a task completes the PLAN/IMPLEMENT/REVIEW lifecycle with plan, implement, validation, and review stages recorded in `stages_ms`
-   every required metric is captured from the existing artifacts, not fabricated
-   agent time and validation time are separable, with the validation breakdown present where the runner reported it
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/cli/ -run 'Performance|Report'
go test ./internal/perf/...
go test ./...
```

### Execution

-   verify-first

## PREJEV017-S3 --- Controller-Driven Workflow Measurement

Measure a controller-driven workflow from the existing run aggregate
(`internal/perf.Run`): the aggregated tasks, the run-level counts (including
`agent_calls_avoided` and validation reuse), and the `CategoryMS` agent /
validation / review split that `perf.WriteRun` and `sop report` render. Package:
`internal/perf` (`Run.CategoryMS`), `internal/cli` (`drive.go`, `report.go`).
Verify existing coverage first
(`TestValidationReuseAcrossUnchangedVerifyFirstTasks`,
`TestPerformanceBenchmarkFixture`), implement only the missing coverage, then
validate independently.

### Dependencies

-   PREJEV017-S1

### Acceptance Criteria

-   a controller-driven run writes a run-level aggregate that aggregates its tasks
-   run-level agent, validation, and review cost is distinguishable through the existing split
-   validation reuse is reported when it occurred, and a reused validation is not counted as a real run
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/cli/ -run 'Performance|Report|ValidationReuse'
go test ./internal/perf/...
go test ./...
```

### Execution

-   verify-first

## PREJEV017-S4 --- Metric Availability, Bottlenecks, and Limitations

Record which metrics the existing instrumentation actually exposes and which it
does not. Tool calls have no field in `perf.Counts`, so the baseline states them as
unavailable from the perf metrics rather than inventing a value; the missing
instrumentation is documented as a limitation, not implemented. Document each known
bottleneck with the metric and evidence that support it, and confirm the
non-blocking optimization work remains in `docs/plans/PLAN-SOP-Performance.md`. Package:
`internal/perf`, `internal/cli` (`report.go`). Verify existing coverage first,
implement only the missing coverage, then validate independently.

### Dependencies

-   PREJEV017-S2
-   PREJEV017-S3

### Acceptance Criteria

-   tool-call availability is stated from the existing instrumentation, and no new perf field is added
-   each documented bottleneck cites the captured metric and its source artifact
-   the missing instrumentation is recorded as a limitation, not implemented
-   optimization is delegated to `docs/plans/PLAN-SOP-Performance.md`, with nothing pulled forward
-   the focused and broader validation commands pass

### Validation

``` bash
go test ./internal/perf/...
go test ./internal/cli/ -run 'Report|Performance'
go test ./...
```

### Execution

-   verify-first

## PREJEV017-S5 --- Baseline Recording and Closure

Confirm `docs/history/PREJEV017-PERFORMANCE-BASELINE.md` records the captured baseline for
both measured workflows, that every PREJEV017 acceptance criterion maps to captured
evidence, and that each captured number matches an existing artifact; refresh the
document only where it is stale, and add no test or code to satisfy the milestone.
Package: the whole suite. Verify existing coverage first, implement only the missing
coverage, then validate independently. Completing with no change
(`changes_expected=false`) when the document already reflects the captured evidence
is the expected outcome.

### Dependencies

-   PREJEV017-S1
-   PREJEV017-S2
-   PREJEV017-S3
-   PREJEV017-S4

### Acceptance Criteria

-   the baseline document records the two measured workflows and the full metric set
-   every acceptance criterion maps to captured evidence, and every captured number matches an existing artifact
-   the full validation matrix passes
-   no test, code, or instrumentation is added to force a repository change
-   the milestone may complete with `changes_expected=false`

### Validation

``` bash
go test ./internal/perf/...
go test ./internal/cli/...
go test ./...
go vet ./...
go build ./...
```

### Execution

-   implement
## PREJEV018 --- Pre-JEV Readiness Gate

Required gate:

``` text
Harness V2 reconciled
  → PLAN deterministic
  → IMPLEMENT deterministic
  → REVIEW deterministic
  → validation ownership proven
  → bootstrap resilient
  → controller aligned
  → recovery proven
  → both projects dogfooded
  → performance baseline captured
  → PRE-JEV READY
```

### Acceptance Criteria

-   Harness V2 has no unexplained lifecycle blocks
-   PLAN/IMPLEMENT/REVIEW have deterministic regression coverage
-   known-good Ollama bootstrap works
-   controller delegates orchestration to SOP
-   controller documentation matches reality
-   recovery works without destructive state manipulation
-   both repositories build/test successfully
-   human approval gates remain intact
-   performance baseline exists
-   remaining issues are documented by severity
-   no critical/high issue blocks JEV

## Final Validation

For both `agentic-sop` and `sop-controller`:

``` bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Run representative SOP dogfood workflows in both repositories.

## JEV Handoff

Only after PREJEV018 passes should JEV V1 become the next focused plan.

Build JEV on the existing decision seam rather than creating another
workflow authority.

Initial architecture:

``` text
deterministic SOP decision
  ├─ baseline/default
  └─ JEV adapter
       → feature flag
       → advisory first
       → measured dogfood
       → optional required gate later
```

Use the repository's actual existing config schema. JEV begins
feature-flagged/advisory and must demonstrate value before becoming
required.

## Safety Constraints

Do not:

-   delete `.agent-sdlc` or manually edit `state.db`
-   erase run history
-   indiscriminately reset all tasks
-   use `git reset --hard` or `git clean` as recovery
-   bypass named-plan provenance or human approval
-   duplicate SOP state/orchestration in the controller
-   duplicate the Ollama harness in the controller
-   make Claude required
-   hard-code DeepSeek as the harness identity
-   remove provider/model configurability
-   make JEV workflow authority
-   make JEV mandatory before readiness passes
-   commit, push, or merge without configured human approval

## Definition of Done

The foundation for JEV is stable:

``` text
sop-controller
  → agentic-sop
  → deterministic lifecycle
  → tool-enabled Ollama agent
  → configured model
  → SOP validation/review/gates
```

The lifecycle is recoverable, dogfooded, observable, and measured. JEV
can then be introduced without simultaneously debugging foundational
execution or controller-boundary problems.
