# MIG-001 — Reconciled Local Source and Native Baselines

Status: **RECONCILED — all three local checkouts observed.**
Date: 2026-10-13.
Method: direct operator inspection plus SOP `validate` with the project `.env`
loaded. No service was started or restarted; no live model or browser probe was
run.

Access note: sibling checkouts were made readable to the SOP tool harness as
**read-only authorized roots** through the supported `SOP_WORKSPACE_ROOTS`
configuration (a JSON array of `{"path","mode"}`; mode `read` refuses every write
tool). The primary root (this project) remains read-write. Confinement was not
disabled or widened beyond the three repositories named in the plan.

Secret values are never printed. Configuration is recorded as **key names only**.
No token, session value, weight, or `.env` value appears below. Source
repositories were treated **read-only**: only `go build/test/vet` were run in
them (which write only to the Go build cache, never to the repository).

---

## 0. Objective-level result

| Check      | Command          | Label | Evidence                                                                  |
| ---------- | ---------------- | ----- | ------------------------------------------------------------------------- |
| Core build | `go build ./...` | PASS  | exit 0                                                                    |
| Core test  | `go test ./...`  | PASS  | exit 0 (see §1.4 — the earlier environment-induced failures are resolved) |
| Core vet   | `go vet ./...`   | PASS  | exit 0                                                                    |

Verified both directly and **through SOP with the project `.env` loaded**
(`sop validate` → `PASS BUILD`, `PASS UNIT_TEST`, `PASS LINT`).

---

## 1. agentic-sop (core)

### 1.1 Source ref

| Field            | Value                                                                        |
| ---------------- | ---------------------------------------------------------------------------- |
| Branch           | `main` (tracks `origin/main`)                                                |
| HEAD             | `54aa6a1a8f73a3b6debef0c54472351c2a817080`                                   |
| Staged changes   | none                                                                         |
| Unstaged changes | 5 test files (see §1.2)                                                      |
| Untracked        | `docs/plans/PLAN-Repository-Consolidation.md`, `docs/reports/consolidation/` |
| Module           | `github.com/imhttran/agentic-sop` (`go.mod` declares `go 1.27.1`)            |

### 1.2 Work ownership

- `internal/cli/decision_evidence_{crossmodule,scheduled,}_test.go`,
  `internal/config/config_test.go`, `internal/config/decision_provider_test.go`
  — **this task's change**: the decision tests now explicitly neutralize the
  decision environment variables they depend on (see §1.4). +24 lines, no
  production code touched.
- `docs/plans/PLAN-Repository-Consolidation.md` — **user-owned** running plan.
- `docs/reports/consolidation/` — the MIG-001 deliverable (this report).

### 1.3 Go toolchain

- `go version`: `go1.27.1 darwin/arm64`.
- All three repositories were built/tested with this toolchain.

### 1.4 Baseline commands (core) and the environment-induced failure

| Command          | Label | Notes                                                     |
| ---------------- | ----- | --------------------------------------------------------- |
| `go build ./...` | PASS  | clean compile across module                               |
| `go test ./...`  | PASS  | green under SOP with `.env` loaded and from a clean shell |
| `go vet ./...`   | PASS  | no vet diagnostics                                        |

Root cause of the previously observed `go test ./...` failure (now resolved):
SOP applies the project `.env` to the validation subprocess environment, so
`SOP_DECISION_ENABLED` / `SOP_DECISION_PROVIDER=adapter` /
`SOP_DECISION_ADAPTER_COMMAND` leaked into the decision tests. Tests that assert
the _default_ decision provider (`deterministic`) or the _absence_ of a provider
therefore saw the operator's adapter configuration and failed:

- `internal/config` — `TestParseDefaults`, `TestDecisionProviderAdapterRequiresCommand`,
  `TestDecisionProviderRejectsUnsupported`.
- `internal/cli` — `TestCrossModuleLivePathEndToEnd` (`provider off is unchanged`),
  `TestScheduledTaskDecisionEvidenceEscalates` (`no provider continues`),
  `TestDecisionEvidenceLivePathEscalates` (`disabled capability is unchanged`).

Classification: **environment-induced**, not a code defect in the packages under
test. The fix makes the tests control their own environment (clearing the
decision variables via the existing `clearDecisionEnv` convention) without
stripping runtime configuration and without modifying the user-owned `.env` or
the production configuration precedence. The five tests now pass both from a
clean shell and under SOP.

### 1.5 Installed sop binary

| Field   | Value                                                              |
| ------- | ------------------------------------------------------------------ |
| Path    | `/Users/imhttran/go/bin/sop`                                       |
| Version | `sop dev`                                                          |
| SHA-256 | `c55514149d5100894afc1d47152bf5900142bb7b84755f8963d3b2c35c2a0c68` |

### 1.6 Configuration keys (names only, secret-free)

Model routing: `SOP_MODEL_DEFAULT_CLASS`, `SOP_MODEL_FALLBACK_CLASS`,
`SOP_MODEL_ALLOW_CLOUD_FALLBACK_FOR_LOCAL`, and per-class
`SOP_MODEL_{SMALL,MEDIUM,LARGE}_{PROVIDER,NAME,LOCALITY}` /
`SOP_MODEL_SMALL_FALLBACK_{PROVIDER,NAME,LOCALITY}`.

Decision adapter: `SOP_DECISION_ENABLED`, `SOP_DECISION_PROVIDER`,
`SOP_DECISION_ADAPTER_PROVIDER`, `SOP_DECISION_ADAPTER_COMMAND`,
`SOP_DECISION_ADAPTER_MODEL` (plus `SOP_DECISION_FALLBACK`, present in `.env` but
not read by the Go loader).

Harness/roots: `SOP_WORKSPACE_ROOTS` (read-only sibling access for this run),
`SOP_TOOL_AUDIT_LOG`, `SOP_OLLAMA_TRACE_LOG`, `SOP_COMMAND_EVIDENCE_LOG`.

Actual `.env` / `.env.bak` values are **not** reproduced.

### 1.7 Active SOP plan state (read-only) and conflicts

- Active plan: `plan-repository-consolidation` from
  `docs/plans/PLAN-Repository-Consolidation.md`; 12 tasks MIG-001..MIG-012.
- Human-facing copy (`docs/plans/PLAN-Repository-Consolidation.md`) and machine
  copy (`.agent-sdlc/plan.json`) must not diverge silently; divergence is
  **reported, not resolved**. `.agent-sdlc` was not modified by this task.

### 1.8 Running service contract (observed read-only)

Not exercised. The locally running harness/controller/Caddy were **not** stopped,
restarted, reconfigured, or probed. Recorded as **UNKNOWN**. No service
manipulation occurred.

---

## 2. sop-controller

Local checkout: `/Users/imhttran/agentic-workspace/projects/sop-controller`.

| Item                      | Value                                                                                |
| ------------------------- | ------------------------------------------------------------------------------------ |
| Branch                    | `main`                                                                               |
| HEAD                      | `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` (matches the plan snapshot)               |
| Staged/unstaged/untracked | none (clean working tree)                                                            |
| Module                    | `sop-controller` (`go.mod` declares `go 1.27.0`)                                     |
| `go build ./...`          | **PASS**                                                                             |
| `go test ./...`           | **PASS** (`sop-controller`, `internal/config`, `internal/sopclient`, `internal/web`) |
| `go vet ./...`            | **PASS**                                                                             |

### 2.1 Optional controller browser (Playwright) suite

Status: **NOT RUN.** Development-only Playwright/WebKit suite
(`playwright.config.ts`, `tests/e2e/`). Node `v26.8.2` and `node_modules` are
present, but the suite builds and starts the real controller against disposable
SOP projects and drives the installed `sop` CLI (i.e. real governed execution and
live model calls). That is the MIG-011 dogfood, not a read-only baseline check,
so it was not executed here. It is **not** labeled passing, and Go build/test/vet
do not require Node.

---

## 3. sop-decision-adapters

Local checkout: `/Users/imhttran/agentic-workspace/projects/sop-decision-adapters`.

| Item                      | Value                                                                                                                                              |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Branch                    | `main`                                                                                                                                             |
| HEAD                      | `f319f60df470705db220d67875fdefbe4666e5da`                                                                                                         |
| Staged/unstaged/untracked | **local delta present** — modified `decision/result.go`, `internal/providers/fallback/fallback.go`, `internal/providers/fallback/fallback_test.go` |
| Module                    | `github.com/imhttran/sop-decision-adapters` (`go.mod` declares `go 1.22`)                                                                          |
| `go build ./...`          | **PASS**                                                                                                                                           |
| `go test ./...`           | **PASS** (`cmd/sop-decision-adapter`, `decision`, `internal/providers/{clef,fallback,julia,nimble}`, `internal/shadow`, `tests`)                   |
| `go vet ./...`            | **PASS**                                                                                                                                           |

**Local delta / divergence (must be preserved, not overwritten):** the local
checkout is **ahead of the plan's inspected snapshot** `78bba4ed37e6a6cd40e737140c3e7cb9ca10697c`
— the local HEAD is `f319f60d…`, and the fallback provider plus `decision/result.go`
carry uncommitted edits. These are operator-owned changes and are recorded as the
authoritative local source, not inferred from the committed snapshot. The
uncommitted fallback edits are covered by the passing `internal/providers/fallback`
tests.

### 3.1 Julia helper offline tests

`python3 tools/julia/test_infer.py` — **PASS** (26 tests, `OK`). The helper is
present and runs offline; model weights remain an external, optional capability
and were not exercised.

### 3.2 Local oMLX and Bespoke integration source / configuration

Source and configuration **not established** in this baseline: the oMLX
chat-completions and Bespoke `/score` paths are operator-local live services and
were not probed. Recorded **UNKNOWN / unavailable**, not PASS.

### 3.3 Model contracts

| Contract                               | Status                                       |
| -------------------------------------- | -------------------------------------------- |
| oMLX chat-completions request/response | **UNKNOWN** — live path not established here |
| Bespoke `/score` request/response      | **UNKNOWN** — live path not established here |
| CLEF→Nimble automatic failover         | **UNKNOWN / not demonstrated live**          |

The committed `/v1/systemone` transport is **not** substituted for working local
behavior. Provider migration (MIG-006) remains **blocked** by these unknown live
contracts until an operator-local, bounded, non-destructive reconciliation is
recorded.

---

## 4. Summary matrix

| Repo                  | HEAD      | build | test | vet  | Julia helper | Playwright |
| --------------------- | --------- | ----- | ---- | ---- | ------------ | ---------- |
| agentic-sop (core)    | `54aa6a1` | PASS  | PASS | PASS | n/a          | n/a        |
| sop-controller        | `f8502a5` | PASS  | PASS | PASS | n/a          | NOT RUN    |
| sop-decision-adapters | `f319f60` | PASS  | PASS | PASS | PASS         | n/a        |

Labels: PASS / FAIL / SKIPPED / NOT RUN / UNKNOWN. No PASS is claimed for a
skipped, not-run, or unknown check, and no result is recorded for anything not
actually observed.

---

## 5. Gating conclusions for downstream stages

- The core `go test ./...` failure was **environment-induced** (operator `.env`
  leaking into decision tests) and is **resolved** by test isolation, not by
  weakening validation or editing `.env`. Core validation is green.
- **sop-decision-adapters has diverged locally** (HEAD past the plan snapshot plus
  uncommitted fallback edits). MIG-002 history/provenance mapping must treat this
  local delta as authoritative; do not reset it to the snapshot.
- **MIG-006 (provider migration) is BLOCKED** by the unknown live oMLX/Bespoke
  contracts; do not guess them from the committed `/v1/systemone` transport.
- Sibling checkouts are now readable by the SOP tool harness as **read-only**
  roots via `SOP_WORKSPACE_ROOTS`, so MIG-002..MIG-005 can inspect real local
  source instead of assuming the committed snapshots.

## 6. Non-manipulation attestation

- No service was stopped, restarted, reconfigured, or manipulated; the disposable
  controller/Playwright stack was not started.
- `.env` / `.env.bak` were not modified.
- No commits, pushes, resets, or clean operations were performed.
- Sibling repositories were read-only: only `go build/test/vet` were run.
- User-owned changes in the working tree were preserved.
- No secrets were exposed; configuration is recorded as key names only.
