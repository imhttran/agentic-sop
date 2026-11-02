# Bootstrap Behavior

## ID

PREJEV012-S10

## Objective

Verify the known-good `sop-ollama-agent` bootstrap/install path and that runtime execution does not depend on self-building candidate working-tree source. Scope is limited to `internal/agentbin`, `scripts/install/install-sop-ollama-agent.sh`, `scripts/agents/sop-ollama-agent.sh`, and `internal/ollamaagent/version.go`. Do not discover unrelated subsystems.

This is a verification task: the known-good-binary bootstrap is already implemented from PREJEV010. Existing correct behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestResolvePrefersOverride`, `TestResolveUsesHomeDirectory`, `TestResolveMissingIsAnActionableError`, `TestResolveSkipsDirectories`, `TestResolveSkipsNonExecutable`, `TestResolveFallsBackPastNonExecutableOverride`, `TestCandidatesOrder`, `TestDefaultHomeIsOutsideProjectTree`, and `TestBinaryPathIsAbsolute`.
- Run the focused validation and confirm it passes: `go test ./internal/agentbin/...`.
- Run the broader validation: `go vet ./... && go build ./...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.
- Only if a real gap is demonstrated, add the single focused test and nothing more. The no-self-build case (G4) is covered by `TestBootstrapWrapperNeverSelfBuilds`; confirm it passes.

## Acceptance Criteria

- resolution order is `$SOP_OLLAMA_AGENT_BIN`, then `$SOP_OLLAMA_AGENT_HOME/sop-ollama-agent`, then the default home directory.
- only a regular, executable file is accepted; a directory or non-executable file does not resolve, and a non-executable override does not shadow a valid binary.
- a missing binary is a hard, actionable `ErrNotInstalled` error naming the install script, never an implicit build of candidate source.
- the default install directory is outside the project working tree.
- the focused and broader validation commands pass.

## Constraints

- Do not commit, push, or merge; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Any new test must be deterministic: no network, no live Ollama, and no real build of the pinned revision.

## Dependencies

- PREJEV012-S8
- PREJEV012-S9

## Execution

- implement
