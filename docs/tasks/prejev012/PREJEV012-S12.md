# Coverage Closure

## ID

PREJEV012-S12

## Objective

Close out the Agent Harness regression suite: run the complete suite, identify any remaining uncovered acceptance criteria, and add only the genuinely missing coverage. This is the closure task for the umbrella PREJEV012 and the only task in S6 through S12 that runs in `implement` mode, because it may need to add tests. Scope is the whole suite: `internal/e2e/...`, `internal/ollamaagent/...`, `internal/agent/...`, and `internal/agentbin/...`.

Do not discover or modify unrelated subsystems, and do not rewrite correct behavior to force a change.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Run the focused validation and record the result: `go test ./internal/ollamaagent/...` and `go test ./internal/e2e/...`.
- Run the full validation matrix: `go test ./...`, `go vet ./...`, `go build ./...`.
- If any concurrency-sensitive code is touched, also run: `go test -race ./internal/ollamaagent/...` and `go test -race ./internal/e2e/...`.
- Confirm the closure tests for G1–G5 pass: `TestChangedSinceIgnoresRestoredFile`, `TestPreDirtyContentPreservedAcrossInvocation`, `TestPreDirtyRepoAcrossCapabilities`, `TestBootstrapWrapperNeverSelfBuilds`, and `TestFakeProviderConcurrentUseIsRaceFree`.
- Mark every PRD coverage bullet as covered in the final coverage map, naming the proving test for each.
- If no gap is confirmed, complete with no change (`changes_expected=false`).

## Acceptance Criteria

- the complete standard suite passes with no live Ollama instance.
- `go vet ./...` and `go build ./...` pass.
- the `-race` runs pass for the harness packages.
- every PRD coverage bullet maps to at least one passing regression test.
- each gap G1 through G5 is covered by its named proving test and passes, including under `-race`.
- no unrelated feature is implemented, and no correct behavior is rewritten.

## Constraints

- Do not commit, push, or merge; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Add only tests where a real coverage gap is demonstrated; otherwise make no change.
- Keep all tests deterministic and free of network or a live Ollama instance.

## Dependencies

- PREJEV012-S6
- PREJEV012-S7
- PREJEV012-S8
- PREJEV012-S9
- PREJEV012-S10
- PREJEV012-S11

## Execution

- implement
