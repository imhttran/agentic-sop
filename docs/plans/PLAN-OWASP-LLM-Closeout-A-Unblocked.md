# Plan A — OWASP LLM gap closeout: work that needs no policy decision

Status: READY FOR SOP RECONCILIATION; not executed.
Order: 1 of 2. Independent of [Plan B](PLAN-OWASP-LLM-Closeout-B-Decisions.md); either can run first.
Suggested branch: one branch per stage (`fix/decision-store-durability`, `security/scanner-pins-remaining`, `docs/owasp-live-evidence`).
Dependency: none.

## Project

Plan A — OWASP LLM gap closeout: work that needs no policy decision

## Summary

Close the open OWASP LLM gaps that change no approved policy: decision-store
durability (LLM04), the missing execution-time scanner pins (LLM03), and the two
live-evidence gaps (LLM01, LLM08). Source of the gap list:
[acceptance-report-2026-11-16.md](../reports/owasp-llm/acceptance-report-2026-11-16.md)
and the review of `origin/main` at `6c9500e4`. Stages are independent of each
other; the order below is cheapest first.

## OWASPA-001 — Make the decision store write atomic and its read failures visible

Priority: P2. `saveDecisionStore` (`internal/cli/memory.go`) writes
`decisions.json` with a plain `os.WriteFile`, so a crash mid-write can truncate
the store. `loadDecisionStore` returns an empty store for an unreadable or
unknown-schema file with no message, so `sop memory list` and `sop run` silently
apply no decisions. Write to a temp file in the same directory and `os.Rename` it;
on the lenient read path, print one warning line to stderr and keep returning the
empty store (the read path must not start failing runs). Reuse the existing `writeFileAtomic` helper
(`internal/cli/plan.go`). No lock: concurrent `memory`
writers are not a supported use.

### Dependencies

None

### Deliverables

- `internal/cli/memory.go` — atomic save; one-line stderr warning on an unreadable store in `memory list` and in `applicableDecisionItems` (`internal/cli/run.go`).
- `internal/cli/memory_test.go` — one test that an unreadable store warns and lists nothing, one that a save leaves no temp file behind.
- `docs/reports/owasp-llm/llm05-poisoning.md` — the two observations recorded as fixed, with test names.

### Acceptance Criteria

- A failed or interrupted save never leaves a truncated `decisions.json`.
- An unreadable store produces a visible warning on every read path and never an error exit.
- The strict write path (`loadDecisionStoreForWrite`) is unchanged.

### Acceptance Checks

- `go build ./...`
- `go vet ./internal/cli/...`
- `go test ./internal/cli/ -run Memory -count=1`
- `go test ./internal/decisionmemory/ -count=1`

## OWASPA-002 — Commit the remaining execution-time scanner pins

Priority: P2. Gitleaks and OSV-Scanner have committed
`binary_sha256_by_platform` pins; cargo-audit, the Semgrep binary, and Windows do
not. Add them with the same mechanism (no new code path expected). This extends
item 1 of [PROPOSAL-SCANNER-PIN-POLICY.md](PROPOSAL-SCANNER-PIN-POLICY.md), which
is decided for Gitleaks and OSV-Scanner; confirm with the operator that the
recorded authorization covers the additional scanners before committing a pin.
If Semgrep cannot be pinned as a single binary (it is a Python distribution),
record that as the reason and stop; do not invent a new pin type here.

### Dependencies

None

### Deliverables

- `.agent-sdlc/config.yaml` — per-platform pins for each scanner that can take one.
- `docs/reports/owasp-llm/llm03-scanner-binary-pins.md` — the pin table, how each digest was obtained, and any scanner left unpinned with the reason.

### Acceptance Criteria

- Every pinned scanner fails closed with a mismatched binary (demonstrated once per scanner, not assumed).
- A pinned scan still reaches `COMPLETE` coverage with the recorded 101/84/17/1 result, or the difference is explained.
- `security.mode`, `required_categories`, `fail_on`, and the baseline are unchanged.

### Acceptance Checks

- `go build ./...`
- `go test ./internal/security/... -count=1`

## OWASPA-003 — Run the two missing live campaigns

Priority: P3. Evidence only; no code change expected. (1) LLM01: Linux has one
model; run the existing injection campaign with a second model, as macOS already
has. (2) LLM08: the live RAG run never observed `TIMEOUT` or cancellation; run
the existing campaign with a producer delay that exceeds the deadline, and once
with cancellation. Needs a Linux host with Ollama and the disposable
PostgreSQL + pgvector setup from the first run; if either is unavailable, record
`NOT_RUN` with the reason and stop.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm01-campaign-linux-<model>.json` and an updated `llm01-injection.md`.
- An updated `docs/reports/owasp-llm/llm09-live-rag-campaign.json` (or a sibling file) and `llm09-retrieval.md`.
- Each artifact's SHA-256, model, platform, and revision recorded in the report.

### Acceptance Criteria

- Each case has one recorded verdict; a timeout is recorded as a timeout, not as a pass.
- Positive controls are included and pass.
- Synthetic canaries only; no real credential or external destination.

### Acceptance Checks

- `go test ./internal/ragclient/ ./internal/context/ ./internal/agent/ -count=1`

## OWASPA-004 — Reconcile the OWASP documents

Priority: P3. Update the coverage statement, gap register, evidence matrix, and
the plans index to the state after OWASPA-001 to OWASPA-003.

### Dependencies

- OWASPA-001
- OWASPA-002
- OWASPA-003

### Deliverables

- `docs/reports/owasp-llm/coverage-statement.md`, `gap-register-20261109.md`, `README.md`, and `docs/plans/README.md`.

### Acceptance Criteria

- Every changed claim names a test, a command, or an artifact from this plan.
- A stage that stopped early is recorded with its reason, not as done.

### Acceptance Checks

- `go build ./...`

## Risks and boundaries

No sandbox, redaction, scanner-mode, or baseline change belongs in this plan;
those are Plan B. A pin is a supply-chain control only for the bytes it names: it
does not establish publisher authenticity (pin-policy item 2, still open).

## Completion

Complete when each stage has a merged PR or a recorded stop reason, and
OWASPA-004 is merged.

## Execution contract

- Planning deliverable only. Creating this plan executed nothing.
- Refresh `origin/main` before each stage and work in a new worktree; the planning baseline is `6c9500e4`.
- Reuse existing helpers and mechanisms; fix only the gaps named here.
- Run `go build ./...`, `go vet ./...`, and `go test ./...` once before each PR. Record commands, exit status, commit, and platform.
- Never hand-edit `.agent-sdlc/state.db`. Do not change `security.mode`, `required_categories`, `fail_on`, baseline entries, or accepted residual dispositions.
- One focused PR per stage. The operator merges.
