# Security Scanner Final Findings Triage

**Type:** Historical evidence (non-normative)
**Date:** 2026-11-09
**Baseline:** `main` @ `ac15f84b` (PR #264 merged)
**Policy:** report-only; no change to `security.mode` or `required_categories`

## Reproduction

A scan of the latest-main worktree reproduced the repository-source state:

| Scanner     | Status    | Coverage | Languages       |
| ----------- | --------- | -------- | --------------- |
| Gitleaks    | COMPLETED | COMPLETE | repository-wide |
| Semgrep     | COMPLETED | COMPLETE | Go              |
| OSV-Scanner | COMPLETED | COMPLETE | Go              |

The latest-main scan was `PASS` with 67 findings: 53 baseline-suppressed and
14 unsuppressed `MEDIUM` advisories. There was no unsuppressed `HIGH` and no
Semgrep coverage gap.

The primary checkout reproduced the historical `HIGH` and the 12 oversized /
39 `.semgrepignore` skips because it contains machine-local artifacts that are
absent from a clean checkout. Those artifacts are not repository source. After
excluding `.sop-evidence` from repository-source scope, the primary scan has no
blocking findings; the remaining Semgrep skips are 3 local generated files and
the same 39 ignored local files.

## HIGH finding classification

| Field | Value                                                                    |
| ----- | ------------------------------------------------------------------------ |
| Path  | `.sop-evidence/pr-114-closeout/local-work/projectdetect-exec_test.go:38` |
| Rule  | `dangerous-exec-command`                                                 |
| Code  | `exec.Command("sh", "-c", command)`                                      |

Classification: **historical evidence**, not active repository code.

Evidence:

- `.sop-evidence/` is untracked and ignored via `.git/info/exclude`; the
  repository documents it as a local run-evidence directory, not source.
- The file is a copied historical test fixture listed in
  `.sop-evidence/pr-114-closeout/MANIFEST.sha256` with digest
  `71c720c25f890d0e97a0c009ab6ef7aa4660648f69d38d92ad1f05a35fc748c2`.
- Direct reproduction with the pinned Semgrep rules returns the same
  `dangerous-exec-command` result at line 38.
- The file is not part of the module build and is not imported by repository
  code.

Action: preserve the evidence artifact unchanged and exclude `.sop-evidence`
from repository-source scanning as machine-local evidence. No baseline
suppression and no artifact modification.

## MEDIUM finding triage

All 14 are in `_test.go` files. Each was inspected at the call site; none has a
production call path or untrusted input.

| Finding                                                                                              | Evidence                                                                                                   | Classification / action                                                 |
| ---------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `internal/agent/openaicompatible_execution_test.go:44` — `no-direct-write-to-responsewriter`         | `httptest` handler writes a fixed JSON response.                                                           | Test-server false positive; no change.                                  |
| `internal/agent/provider_test.go:199` — `no-direct-write-to-responsewriter`                          | `httptest` handler writes a test-case body.                                                                | Test-server false positive; no change.                                  |
| `internal/agent/provider_test.go:291` — `no-direct-write-to-responsewriter`                          | Same test handler for error cases.                                                                         | Test-server false positive; no change.                                  |
| `internal/cli/ollama_e2e_test.go:41` — `no-direct-write-to-responsewriter`                           | `httptest` handler writes a marshaled fixed response.                                                      | Test-server false positive; no change.                                  |
| `internal/cli/provider_validation_task_test.go:39` — `no-direct-write-to-responsewriter`             | `httptest` handler writes fixed probe JSON.                                                                | Test-server false positive; no change.                                  |
| `internal/cli/zz_operator_acceptance_acc008_test.go:158` — `unsafe-reflect-by-name`                  | Test helper uses `FieldByName` with constant field names asserted by the acceptance test.                  | No untrusted reflection; no change.                                     |
| `internal/controller/web/recovery_test.go:474` — `cookie-missing-httponly`                           | Test client constructs a CSRF cookie for an HTTP test request; this is not a server-issued session cookie. | Test-only false positive; no change.                                    |
| `internal/controller/web/recovery_test.go:474` — `cookie-missing-secure`                             | Same client-side CSRF cookie in an HTTP test.                                                              | Test-only false positive; no change.                                    |
| `internal/decisionadapter/providers/clef/chat_test.go:39` — `no-fprintf-to-responsewriter`           | `httptest` handler uses `%q` to JSON-escape fixed content.                                                 | Test-server false positive; no change.                                  |
| `internal/decisionadapter/providers/nimble/score_test.go:77` — `no-io-writestring-to-responsewriter` | `httptest` handler writes a constant JSON body.                                                            | Test-server false positive; no change.                                  |
| `internal/ollamaagent/ollama_retry_test.go:70` — `no-io-writestring-to-responsewriter`               | `httptest` handler writes a scripted test body.                                                            | Test-server false positive; no change.                                  |
| `internal/ollamaagent/ollama_retry_test.go:73` — `no-io-writestring-to-responsewriter`               | Same scripted test body.                                                                                   | Test-server false positive; no change.                                  |
| `internal/provider/openai/provider_locality_test.go:24` — `no-io-writestring-to-responsewriter`      | `httptest` handler writes a fixed `/v1/models` payload.                                                    | Test-server false positive; no change.                                  |
| `internal/store/notifications_test.go:40` — `string-formatted-query`                                 | `countRows` interpolates a table name supplied only by test call sites.                                    | No user input reaches the query; test-helper false positive; no change. |

The findings remain visible as advisories. No baseline entries were added and
no test code was changed, because the changes would add test-only churn without
a material security benefit.

## Semgrep coverage

Latest-main Semgrep coverage is `COMPLETE` for Go. The historical `PARTIAL`
result came from primary-checkout artifacts:

- **Oversized files:** the historical primary scan reported 12. After excluding
  `.sop-evidence`, 3 remain: the `sop` binary and `.run` logs/spike binaries.
  They are generated local files, not tracked source, and are absent from a clean
  checkout.
- **39 `.semgrepignore` matches:** ignored local files such as `.env`,
  `.playwright-mcp` snapshots, build output, and local SOP state. They are not
  tracked source.
- No tracked source file was skipped in the latest-main worktree.

No `.semgrepignore` or rule-set weakening was introduced. The pinned rules still
cover Go and Rust; the Rust rules remain provisioned and the existing
`TestPinnedProductionRulesCoverRust` integration check passes.

## Verification

| Check                                                      | Result                                                 |
| ---------------------------------------------------------- | ------------------------------------------------------ |
| `go test ./... -count=1`                                   | PASS                                                   |
| `go vet ./...`                                             | PASS                                                   |
| `go test ./internal/security/ ./internal/config/ -count=1` | PASS                                                   |
| `go test ./internal/cli/ -run 'Security\|Scope' -count=1`  | PASS                                                   |
| Rust real-Semgrep integration tests                        | PASS                                                   |
| Latest-main `sop security scan --report-only`              | PASS / COMPLETE, no blocking findings                  |
| Gitleaks                                                   | COMPLETED / COMPLETE, no ignored-local-secret findings |
| OSV-Scanner                                                | COMPLETED / COMPLETE, Go covered                       |
