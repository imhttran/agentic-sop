# PLAN — SOP Execution Reliability

**Status:** ACTIVE. Converted from the operator's draft into SOP's canonical stage shape. The Rust
workflow demonstration (draft SOP-006) is deferred to a separate run; see "Deferred work".

## Project

SOP Execution Reliability

## Summary

Fix the SOP execution, verification, and recovery gaps exposed by the Rust migration: command
authorization, prerequisite checks, criterion coverage, verdict gating, no-progress recovery, and
execution evidence. A later run demonstrates the fixes on a small disposable Rust project with a
candidate binary.

Scope excludes RAG application changes, database provisioning, UI work, security-scanner
additions, and migration execution.

Constraints for every stage:

- Inspect and reuse existing code before adding new code. No Cargo support exists on main today
  (no branch or worktree contains it), so SOP-001 implements it rather than consolidating it. The
  verdict gate already exists (internal/outcome, commit 78abfd7) and is reused, not duplicated.
- Preserve historical outcomes, approvals, and continuation counters. Never edit state.db
  directly.
- Keep agent-tool Cargo authorization (internal/toolharness) separate from acceptance-check
  Cargo authorization (internal/cli/outcome.go authorizeAcceptanceCommand). Both default off.
- Existing defaults stay unchanged: every new option is opt-in.
- Do not commit, push, merge, or replace the installed binary.

## SOP-001 — Add opt-in Cargo authorization and enforce the verdict gate on dependents

Add two independent, default-off permissions: one that lets the agent tool harness run supported
Cargo commands, and one that lets plan acceptance checks run them. Classify supported Cargo
commands (build, test, check, clippy, fmt --check) through internal/commandpolicy only when the
relevant permission is enabled; reject unsupported subcommands and arguments, shell syntax, and a
--manifest-path that resolves outside the repository before execution. Confirm that a failed
required acceptance check prevents a dependent task from executing, reusing the existing verdict
gate. Document the configuration keys, defaults, and the authorization boundary in
docs/reference/configuration.md. Existing Go and read-only Git classification stays unchanged.

### Dependencies

None

### Deliverables

- internal/commandpolicy/cargo.go
- internal/commandpolicy/cargo_test.go
- internal/toolharness/cargo_test.go
- internal/cli/acceptance_cargo_test.go
- internal/config/cargo_permissions_test.go

### Acceptance Criteria

- Agent-tool and acceptance-check Cargo permissions operate independently and default off.
- Supported Cargo commands execute only when their relevant permission is enabled.
- Unsupported arguments, shell syntax, and manifest-path escapes are rejected before execution.
- Failed required acceptance prevents dependent-task execution.
- Existing Go and read-only Git behavior remains compatible.

### Acceptance Checks

- `go test ./internal/config -run TestCargoPermissionsDefaultOff`
- `go test ./internal/config -run TestCargoPermissionsIndependent`
- `go test ./internal/commandpolicy -run TestCargoRequiresPermission`
- `go test ./internal/commandpolicy -run TestCargoRejectsUnsupportedArguments`
- `go test ./internal/commandpolicy -run TestCargoRejectsManifestPathEscape`
- `go test ./internal/toolharness -run TestAgentToolCargoExecutesWhenEnabled`
- `go test ./internal/cli -run TestAcceptanceCargoIndependentOfAgentTool`
- `go test ./internal/cli -run TestFailedRequiredAcceptanceBlocksDependentTask`
- `go test ./internal/commandpolicy ./internal/toolharness`
- `go vet ./...`

## SOP-002 — Detect execution prerequisites before implementation

Before invoking the implementation model, check each declared validation and acceptance command
against the authorization path that will actually run it, and check declared tools (on PATH),
manifests (file exists inside the repository), and required environment variables (presence
only). Report a specific setup blocker instead of calling the model. Preflight never executes a
task command to decide readiness and never prints a secret value. A setup failure consumes no
implementation continuation budget.

### Dependencies

SOP-001

### Deliverables

- internal/cli/preflight.go
- internal/cli/preflight_test.go

### Acceptance Criteria

- A command-policy mismatch is detected before a model call.
- Missing tools, manifests, or declared environment variables produce a specific setup blocker.
- Secret values are never printed.
- Setup failures do not consume implementation continuation budgets.
- Preflight does not execute arbitrary task commands to determine readiness.

### Acceptance Checks

- `go test ./internal/cli -run TestPreflightPolicyMismatchBeforeModelCall`
- `go test ./internal/cli -run TestPreflightMissingTool`
- `go test ./internal/cli -run TestPreflightMissingManifest`
- `go test ./internal/cli -run TestPreflightMissingEnvVarNeverPrintsValue`
- `go test ./internal/cli -run TestPreflightConflictingPermission`
- `go test ./internal/cli -run TestPreflightFailureKeepsContinuationBudget`
- `go test ./internal/cli -run TestPreflightExecutesNoTaskCommand`
- `go vet ./...`

## SOP-003 — Require evidence for every required criterion

Inspect the plan compiler (internal/planner) and the outcome resolver (internal/outcome,
internal/cli/outcome.go). Add an explicit mapping from each required acceptance criterion to an
executable acceptance check or to supported manual evidence. Record validation results separately
from acceptance results. An unmapped required criterion, a failed check, or missing manual
evidence prevents VERIFIED and downstream advancement. Manual evidence requires an explicit,
traceable acceptance decision. Existing persisted outcomes are never rewritten; a legacy plan
without mappings receives an actionable compatibility result instead of silent verification.

### Dependencies

SOP-002

### Deliverables

- internal/outcome/criteria.go
- internal/outcome/criteria_test.go

### Acceptance Criteria

- An unmapped required criterion cannot produce VERIFIED.
- Passing validation does not satisfy acceptance automatically.
- Failed or missing required evidence prevents verification and downstream advancement.
- Manual evidence requires an explicit, traceable acceptance decision.
- Historical outcomes remain unchanged.
- Legacy plans receive an actionable compatibility result.

### Acceptance Checks

- `go test ./internal/outcome -run TestUnmappedRequiredCriterionNotVerified`
- `go test ./internal/outcome -run TestPartialCriterionMappingNotVerified`
- `go test ./internal/outcome -run TestFailedCriterionCheckBlocksAdvancement`
- `go test ./internal/outcome -run TestValidationPassAcceptanceFailNotVerified`
- `go test ./internal/outcome -run TestManualEvidenceRequiresDecision`
- `go test ./internal/outcome -run TestHistoricalOutcomesUnchanged`
- `go test ./internal/outcome -run TestLegacyPlanCompatibilityResult`
- `go test ./internal/outcome ./internal/planner`
- `go vet ./...`

## SOP-004 — Improve bounded no-progress recovery

Classify a failed attempt as an authorization failure, a missing prerequisite, or implementation
no-progress, and detect repeated discovery without repository changes. Reuse the existing routing
(internal/router, internal/adaptiveroute) and recovery budgets (internal/recovery,
internal/continuation). An unchanged failure is never retried blindly; each recovery records its
reason and the material change in approach; model fallback respects configured routing and
permissions; continuation limits survive a process restart; exhaustion produces one actionable
blocker report.

### Dependencies

SOP-003

### Deliverables

- internal/recovery/noprogress.go
- internal/recovery/noprogress_test.go

### Acceptance Criteria

- Unchanged failures do not trigger blind automatic retries.
- Each recovery records its reason and material change in approach.
- Model fallback respects configured routing and permissions.
- Continuation limits survive process restarts.
- Exhaustion produces one actionable blocker report.

### Acceptance Checks

- `go test ./internal/recovery -run TestDiscoveryOnlyAttemptIsNoProgress`
- `go test ./internal/recovery -run TestUnchangedFailureNotRetried`
- `go test ./internal/recovery -run TestPermissionDenialIsAuthorizationFailure`
- `go test ./internal/recovery -run TestRecoveryRecordsReasonAndChange`
- `go test ./internal/recovery -run TestProductiveContinuationAllowed`
- `go test ./internal/recovery -run TestFallbackRespectsRouting`
- `go test ./internal/continuation -run TestContinuationLimitSurvivesRestart`
- `go test ./internal/recovery -run TestExhaustionSingleBlockerReport`
- `go test ./internal/recovery ./internal/continuation`
- `go vet ./...`

## SOP-005 — Preserve complete evidence and reliable counts

Save complete command output as run artifacts with bounded report summaries: each failed check
records argv, working directory, exit status, and a full-log artifact, subject to the existing
secret redaction and retention rules. Count invocations, attempts, continuations, model calls,
approvals, and direct interventions separately (internal/runmetrics); counts survive a restart
without double-counting a repeated report, and missing historical data is reported as UNKNOWN.
Record a candidate binary's source identity and checksum.

### Dependencies

SOP-004

### Deliverables

- internal/runmetrics/counts.go
- internal/runmetrics/counts_test.go
- internal/cli/check_evidence_test.go

### Acceptance Criteria

- Each failed check records argv, working directory, exit status, and a full-log artifact.
- Evidence follows existing secret-redaction and retention rules.
- Invocations, attempts, continuations, model calls, approvals, and direct interventions are counted separately.
- Available counts survive restart without double-counting repeated reports.
- Missing historical data is reported as UNKNOWN.
- Candidate binaries have recorded source identity and checksum.

### Acceptance Checks

- `go test ./internal/cli -run TestFailedCheckRecordsFullLogArtifact`
- `go test ./internal/cli -run TestCheckEvidenceRedactsSecrets`
- `go test ./internal/runmetrics -run TestCountsSeparatedByKind`
- `go test ./internal/runmetrics -run TestCountsSurviveRestart`
- `go test ./internal/runmetrics -run TestDuplicateReportNotDoubleCounted`
- `go test ./internal/runmetrics -run TestMissingHistoryIsUnknown`
- `go test ./internal/runmetrics -run TestCandidateBinaryProvenance`
- `go test ./internal/runmetrics`
- `go vet ./...`

## Deferred work

The draft's SOP-006 (demonstrate a complete bounded Rust workflow) is not a stage of this plan.
The installed runner cannot execute it: its acceptance checks allow only `go` and read-only `git`,
and its agent tools exclude Cargo, which SOP-001 adds. After SOP-001..SOP-005 pass, a separate
run builds a candidate binary from this branch into a scratch path (the installed binary stays
unchanged) and drives a disposable Rust project through it, covering implementation, a controlled
acceptance failure that blocks a dependent task, supported recovery, and an accurate final report.
The RAG migration stays paused.
