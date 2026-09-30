# PLAN --- JEV Implementation

## Project

agentic-sop

## Summary

Add JEV as an optional quality and review capability inside
`agentic-sop` without replacing SOP's existing orchestration,
validation, review, state machine, or human approval boundaries.

JEV should act as an additional engineering-quality signal that SOP can
invoke at well-defined lifecycle boundaries. The initial implementation
must sit behind configuration and a feature flag so existing SOP
workflows behave exactly as they do today unless JEV is explicitly
enabled.

JEV provides analysis. SOP retains execution authority.

## Objective

Allow SOP to optionally run JEV during the engineering lifecycle to
identify implementation-quality problems before work is considered
complete.

JEV must not directly change SOP task state, approve or complete tasks,
bypass validation/review, commit, open or merge PRs, retry indefinitely,
or modify the working tree.

## Architecture

``` text
SOP
 |
 +-- IMPLEMENT / FIX
 +-- VALIDATION
 +-- REVIEW
 +-- optional JEV
       |
       +-- structured findings
       +-- severity
       +-- evidence
       |
       +-- SOP quality policy
              |
              +-- PASS -> LOCAL_DONE
              +-- FAIL -> existing FIX / STOP
```

JEV must be optional, read-only in V1, structured, evidence-oriented,
and bounded.

# JEV001 --- Define JEV Capability Boundary

Establish the domain boundary between SOP and JEV.

### Acceptance Criteria

-   JEV has a clearly defined interface.
-   SOP remains the lifecycle owner.
-   JEV cannot directly modify persisted task state.
-   Existing execution providers remain usable without JEV.
-   Architecture is documented.

# JEV002 --- Add JEV Configuration and Feature Flag

Add explicit configuration. JEV must be disabled by default.

Conceptually:

``` yaml
quality:
  jev:
    enabled: false
```

### Acceptance Criteria

-   Disabled by default.
-   Existing configs remain compatible.
-   Enabling requires explicit configuration.
-   Invalid configuration produces a focused error.
-   No state deletion/recreation is required.

# JEV003 --- Define Structured JEV Result Model

Represent status, findings, severity, category, location/evidence, and
summary.

Suggested statuses: `PASS`, `FINDINGS`, `INCOMPLETE`, `ERROR`.

Suggested severities: `INFO`, `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`.

### Acceptance Criteria

-   Results are machine-readable.
-   Multiple findings are supported.
-   Evidence is preserved.
-   Malformed output fails closed.
-   Parsing has deterministic tests.

# JEV004 --- Add JEV Provider Boundary

Create a small replaceable analyzer interface, conceptually:

``` go
type Analyzer interface {
    Analyze(ctx context.Context, req Request) (Result, error)
}
```

Provide only needed task, acceptance, diff/repository, validation, and
review context. Do not expose task-state mutation authority.

### Acceptance Criteria

-   Replaceable analyzer interface.
-   Deterministic fake analyzer for tests.
-   No real model required by lifecycle tests.
-   No persistence mutation authority.

# JEV005 --- Implement Ollama JEV Adapter

Reuse the existing Ollama agent/provider infrastructure. Do not create a
second provider stack. Model selection remains configuration-driven.

### Acceptance Criteria

-   Existing provider infrastructure reused.
-   Model configurable.
-   No required hardcoded model.
-   Provider failures are focused errors.
-   Execution bounded.
-   Tests require no live Ollama.

# JEV006 --- Add Read-Only JEV Tool Policy

Initially permit only inspection operations such as
reading/searching/listing files and inspecting diff/context. Deny
repository mutation, destructive Git, commit, push, and merge
operations.

Reuse existing capability/tool policy.

### Acceptance Criteria

-   JEV cannot mutate repository files.
-   Destructive Git operations denied.
-   Denied tool requests remain bounded.
-   Deterministic policy tests.
-   IMPLEMENT/FIX policies unchanged.

# JEV007 --- Integrate JEV Into the Quality Lifecycle

Preferred initial flow:

``` text
IMPLEMENT
 -> VALIDATE
 -> REVIEW
 -> optional JEV
 -> QUALITY DECISION
```

Use the repository's existing quality seam if discovery identifies a
better integration point.

### Acceptance Criteria

-   Runs only when enabled.
-   Does not replace validation or review.
-   Receives task-specific context.
-   Existing lifecycle transitions remain authoritative.
-   Disabled behavior remains compatible.

# JEV008 --- Add JEV Quality Policy

Initial policy:

``` text
INFO     -> report
LOW      -> report
MEDIUM   -> report
HIGH     -> fail quality gate
CRITICAL -> fail quality gate
```

Quality failures must enter the existing FIX lifecycle rather than a
JEV-specific retry machine.

### Acceptance Criteria

-   HIGH/CRITICAL can fail quality.
-   Lower severities report without blocking by default.
-   Existing FIX budget remains authoritative.
-   No independent JEV retry counter.
-   No infinite loops.

# JEV009 --- Feed JEV Findings Into FIX Context

When JEV fails quality, provide actionable
severity/file/line/finding/evidence to the existing FIX capability.

### Acceptance Criteria

-   FIX receives relevant findings.
-   Findings remain associated with task/run.
-   FIX remains responsible for mutation.
-   JEV remains read-only.
-   Existing fix-cycle limits enforced.

# JEV010 --- Persist JEV Results as Run Artifacts

Persist analysis alongside run diagnostics,
e.g. `.agent-sdlc/runs/<TASK>/jev.json`, following repository
conventions.

JEV artifacts are diagnostic evidence, not workflow state.

### Acceptance Criteria

-   Results available after run.
-   Historical results preserved.
-   Reports distinguish JEV from validation/review.
-   Existing persistence remains authoritative.
-   No state.db deletion/migration workaround.

# JEV011 --- Add CLI Reporting

Show concise results such as:

``` text
JEV: PASS
0 blocking findings
```

or actionable findings with severity and source location.

### Acceptance Criteria

-   User can tell whether JEV executed.
-   Blocking findings visible.
-   Non-blocking findings accessible.
-   Report path shown when useful.
-   Disabled JEV adds no unnecessary noise.

# JEV012 --- Add Deterministic Unit and Integration Tests

Required cases:

1.  Disabled JEV preserves behavior.
2.  Enabled JEV runs after required gates.
3.  PASS allows completion.
4.  INFO/LOW/MEDIUM do not block by default.
5.  HIGH/CRITICAL fail quality.
6.  Malformed output fails closed.
7.  Provider error never becomes PASS.
8.  JEV cannot write files or perform destructive Git.
9.  Findings reach FIX.
10. Successful FIX can be revalidated/re-analyzed.
11. Repeated failure respects fix-cycle limits.
12. Results persist diagnostically.
13. Human approval remains intact.
14. Automatic blocked recovery remains compatible.
15. Completed-plan handoff/reconciliation remain compatible.

### Acceptance Criteria

-   Deterministic.
-   No network.
-   No live Ollama.
-   No dependency on real model output.
-   Existing suites continue passing.

### Coverage

All fifteen cases above are satisfied by deterministic tests in the existing
suites; no new tests were required (`changes_expected=false`). Each case is
mapped to the test(s) that prove it. Layers: `internal/jev` (analyzer, policy,
payload), `internal/quality` (severity policy and the gate), and `internal/cli`
(lifecycle integration through the real quality seam: validate -> review -> JEV
-> `quality.Evaluate` -> FIX). No test uses the network, a live Ollama server,
or real model output; the doubles are `sequencedJEV`, `jevTestAgent`, and
`fakeChat`.

1.  Disabled JEV preserves behavior.
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVDisabledNeverInvokesAnalyzer`
    -   `internal/cli/jev_payload_test.go` `TestJEVPayloadsAbsentWhenDisabled`
    -   `internal/config/jev_test.go` `TestJEVMissingFlagYieldsDisabled`, `TestJEVExplicitFalseYieldsDisabled`
    -   `internal/cli/jev_report_test.go` `TestRenderJEVReportDisabledAddsNoNoise`
2.  Enabled JEV runs after required gates.
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVReceivesTaskValidationAndReviewContext` (the JEV request carries the validation and review outcomes, so both gates produced their evidence first)
    -   `internal/cli/jev012_test.go` `TestJEVPASSAllowsCompletion` (the analyzer is invoked exactly once)
3.  PASS allows completion.
    -   `internal/cli/jev012_test.go` `TestJEVPASSAllowsCompletion`
4.  INFO/LOW/MEDIUM do not block by default.
    -   `internal/quality/jev_policy_test.go` `TestJEVSeverityPriority`, `TestEvaluateWithJEV`
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVNonBlockingFindingDoesNotBlockOrReachFix`
5.  HIGH/CRITICAL fail quality.
    -   `internal/quality/jev_policy_test.go` `TestEvaluateWithJEV`
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVBlockingFindingFeedsFixAndResolves`, `TestJEVBlockingFindingExhaustsFixBudget`
6.  Malformed output fails closed.
    -   `internal/cli/jev012_test.go` `TestJEVMalformedOrIncompleteResultFailsClosed`
    -   `internal/quality/jev_policy_test.go` `TestNewJEVEvidenceValidation`
    -   `internal/cli/jev_payload_test.go` `TestJEVPayloadsFailClosedOnMalformedAndIncomplete`
    -   `internal/jev/ollama_test.go` `TestMalformedOutputIsFocusedError`
    -   `internal/jev/jev_test.go` `TestResultValidate`
7.  Provider error never becomes PASS.
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVAnalyzerErrorFailsClosed`
    -   `internal/jev/ollama_test.go` `TestProviderFailureIsFocusedError`, `TestProviderTimeoutIsFocusedError`
8.  JEV cannot write files or perform destructive Git.
    -   `internal/jev/policy_test.go` `TestPolicyDeniesMutatingOperations`, `TestPolicyClassifyCommand`, `TestPolicyAllowsReadOnlyOperations`, `TestPolicyDeniesUnknownOperation`, `TestPolicyDenialsAreDeterministic`, `TestPolicyDenialNamesOperation`, `TestImplementFixPolicyUnchanged`
    -   The Ollama analyzer exposes no tool surface, so the read-only boundary is the policy contract plus the analyzer's lack of mutation capability.
9.  Findings reach FIX.
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVBlockingFindingFeedsFixAndResolves`, `TestJEVActionableOnlyFindingsReachFix`
    -   `internal/cli/jev_payload_test.go` `TestJEVFixContextRendersCategoryAndActionableMessageDistinctly`
    -   `internal/jev/payload_test.go` `TestPayloadsForTaskProjectsActionableFields`, `TestPayloadsForTaskKeepsCategoryAndMessageDistinct`
10. Successful FIX can be revalidated/re-analyzed.
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVBlockingFindingFeedsFixAndResolves` (JEV runs before and after the fix; the post-fix result is PASS)
    -   `internal/cli/jev012_test.go` `TestJEVPersistsDiagnosticArtifact` (history records FINDINGS then PASS)
11. Repeated failure respects fix-cycle limits.
    -   `internal/cli/jev_lifecycle_test.go` `TestJEVBlockingFindingExhaustsFixBudget` (3/3 cycles, then `NEEDS_HUMAN`)
12. Results persist diagnostically.
    -   `internal/cli/jev012_test.go` `TestJEVPersistsDiagnosticArtifact` (`jev.json` plus the append-only `jev-history.jsonl`)
    -   `internal/cli/jev_report_test.go` `TestRenderJEVReportPass`, `TestRenderJEVReportFindingsShowsBlockingWithLocation`, `TestRenderJEVReportNonBlockingAccessible`, `TestRenderJEVReportShowsReportPath`, `TestRenderJEVReportFailClosedNeverPass`
13. Human approval remains intact.
    -   `internal/cli/jev012_test.go` `TestJEVPASSAllowsCompletion` (asserts the human approval gate)
14. Automatic blocked recovery remains compatible.
    -   `internal/cli/jev012_test.go` `TestJEVEnabledRecoveryRemainsCompatible`
15. Completed-plan handoff/reconciliation remain compatible.
    -   `internal/cli/jev012_test.go` `TestJEVEnabledCompletedPlanHandoffRemainsCompatible`, `TestJEVEnabledReconcileRemainsCompatible`

### Validation

``` text
go test ./internal/jev/... ./internal/quality/... ./internal/config/...
go test -run JEV ./internal/cli/...
go test ./...
go vet ./...
go build ./...
```

# JEV013 --- Add End-to-End Dogfood Fixture

Prove:

``` text
implementation -> validation PASS -> review PASS -> JEV PASS -> LOCAL_DONE
```

and:

``` text
implementation -> validation PASS -> review PASS -> JEV HIGH
 -> quality FAIL -> FIX -> validation -> review -> JEV PASS -> LOCAL_DONE
```

and fail-closed behavior for unavailable/malformed JEV.

### Acceptance Criteria

-   All flows demonstrated deterministically.
-   No external model required by automated tests.
-   No manufactured success.
-   Lifecycle state remains truthful.

# JEV014 --- Performance and Invocation Metrics

Capture JEV invocation count, duration, provider/model, tool calls,
finding count, and blocking finding count.

### Acceptance Criteria

-   JEV cost/time distinguishable from IMPLEMENT/FIX.
-   Metrics remain diagnostic.
-   Missing metrics never imply PASS/FAIL.
-   Existing performance reports compatible.

# JEV015 --- Documentation and Configuration Examples

Document enabling/disabling, architecture, provider behavior, severity
policy, lifecycle placement, troubleshooting, and safety boundaries.

Explicitly document:

``` text
JEV analyzes.
SOP decides.
IMPLEMENT/FIX changes code.
```

### Acceptance Criteria

-   New user can enable JEV without reading implementation.
-   Default disabled behavior clear.
-   Failure behavior documented.
-   Provider configuration documented.
-   Safety boundaries documented.

# JEV016 --- Final Regression and Compatibility Gate

Verify compatibility with normal and explicit `sop run`, PLAN
reconciliation and `--accept-changed`, completed-plan handoff, automatic
blocked recovery, explicit retry, IMPLEMENT/FIX, verify-first, human
approval, and dirty working trees.

## Validation

``` bash
go test ./internal/cli/...
go test ./internal/scheduler/...
go test ./internal/planflow/...
go test ./internal/resume/...
go test ./internal/quality/...
go test ./...
go vet ./...
go test -race ./...
go build ./...
```

Use actual package ownership if JEV introduces a dedicated package.

### Acceptance Criteria

-   Full tests pass.
-   Race detector passes.
-   Vet passes.
-   Build passes.
-   Disabled behavior remains backward compatible.
-   No destructive recovery introduced.
-   No safety gate bypassed.

# Safety Invariants

Never require:

``` bash
rm .agent-sdlc/state.db
git reset --hard
git clean
```

Preserve working-tree changes, task/run history, PLAN provenance,
validation/review evidence, and human approval.

JEV must not manufacture validation PASS, review PASS, CI PASS, PR
creation, merge completion, or task completion.

# Provider Strategy

Reuse the existing provider architecture:

``` text
JEV capability
      |
existing agent/provider abstraction
      |
Ollama local/cloud
      |
replaceable model
```

JEV must not be synonymous with one model.

# Out of Scope for V1

-   autonomous JEV code modification
-   JEV-owned scheduling/retry loops
-   automatic commits/PRs/merges
-   remote CI emulation
-   multi-agent debate
-   distributed JEV workers
-   community sharing
-   Slack/SMS notifications
-   MCP exposure
-   dashboard-specific JEV UI
-   cloud JEV service architecture

# Definition of Done

JEV V1 is complete when it is optional and disabled by default; SOP
remains orchestration authority; JEV is read-only and returns structured
evidence-backed findings; blocking findings feed the existing FIX
lifecycle; failures fail closed; execution is bounded and measurable;
diagnostics persist; deterministic tests require no model/network;
recovery/reconciliation/human approval remain compatible.

Preserve the core rule:

``` text
Provider ≠ Agent Harness ≠ Model

SOP = orchestration and execution authority
JEV = engineering analysis and quality signal
Model = replaceable reasoning engine
```
