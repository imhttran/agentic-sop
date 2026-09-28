# Command Adapter Compatibility

## ID

PREJEV012-S9

## Objective

Verify that the command-based agent adapter remains compatible with the stabilized Agent Harness contract: a JSON request on stdin, raw response on stdout, a structured outcome passed through, and failures carrying diagnostics. Scope is limited to `internal/agent` (`command.go`, `harness.go`) and `internal/e2e/harness` (the `Adapter` fake). Do not discover unrelated subsystems.

This is a verification task: the behavior is already implemented from AHV2002 and AHV2010. Existing correct behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestCommandAgentReturnsStdout`, `TestCommandAgentSerializesCapability`, `TestCommandAgentAcceptsAllCapabilities`, `TestCommandAgentRejectsUnknownCapabilityWithoutInvokingHarness`, `TestCommandAgentFailureIncludesDiagnostics`, `TestCommandAgentReturnsOutcome`, `TestCommandAgentProseHasNoOutcome`, `TestCommandHarnessSatisfiesHarnessInterface`, `TestCommandHarnessExecuteDelegates`, `TestCommandHarnessExecutePreservesOutcome`, `TestHarnessFromConfigLegacyCommand`, and `TestAdapterContract`.
- Run the focused validation and confirm it passes: `go test ./internal/agent/ -run 'Command'`.
- Run the broader validation: `go test ./internal/e2e/harness/ -run 'Adapter'`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.
- Only if a real gap is demonstrated, add the single focused test and nothing more; otherwise record the gap for PREJEV012-S12.

## Acceptance Criteria

- the JSON request, including its capability, round-trips through the command adapter unchanged.
- every capability (PLAN, DESIGN_TESTS, IMPLEMENT, REVIEW, FIX, DIAGNOSE_FAILURE) is accepted, and an unknown capability is rejected without invoking the command.
- a structured outcome in stdout is parsed and preserved, while prose yields no outcome.
- a failing command surfaces its stderr diagnostics and names the capability.
- the command harness satisfies the `Harness` interface and preserves outcomes.
- the focused and broader validation commands pass.

## Constraints

- Do not commit, push, or merge; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Do not require Claude or any specific external agent.

## Dependencies

- none

## Execution

- implement
