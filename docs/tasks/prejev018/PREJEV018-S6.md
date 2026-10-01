# PREJEV018-S6 — Bootstrap Resilient

Verify that the known-good Ollama bootstrap works and is resilient.

## Objective

Verify the known-good `sop-ollama-agent` bootstrap path works end to end, can be
repeated without destructive state manipulation, and surfaces failures clearly
rather than passing silently.

## Dependencies

- PREJEV018-S1

## Requirements

- Inspect `internal/agentbin`, `scripts/install/install-sop-ollama-agent.sh`,
  `scripts/agents/sop-ollama-agent.sh`, and `docs/history/OLLAMA-DOGFOOD.md`.
- Verify existing coverage first; add a test only where a real gap is demonstrated.
- Confirm the runtime never self-builds candidate source and the default install
  directory is outside the working tree.
- Document the known-good procedure and the recorded bootstrap run.

## Acceptance Criteria

- the known-good Ollama bootstrap works end to end
- bootstrap can be repeated without destructive state manipulation
- bootstrap failures are surfaced clearly rather than silently passed
- the focused and broader validation commands pass

## Validation

``` bash
go test ./internal/agentbin/...
go vet ./... && go build ./...
```

## Execution

- verify-first
