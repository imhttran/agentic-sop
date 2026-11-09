# OWASP LLM — remaining gap register (2026-11-09)

This register records the remaining Plan 01, 03, 04, 05, and 10 gaps from the
current `main`, so each one has a disposition and an owner before any code
changes. It is a point-in-time report, not a compliance claim.

Baseline: `main` at `93c72857` (2026-11-09 UTC). No open pull request claimed any
of these items when this register was written. Merged evidence that is **not**
reopened: PR #297 (provider truncation rejection), PR #298 (retrieval
deadline/cancellation), PR #300 (bounded Plan 03 live disclosure), PR #303 (Linux
child/grandchild cancellation).

Dispositions:

- **Defect** — the current code contradicts an existing, enforced contract, and a
  focused regression can demonstrate it.
- **Missing evidence** — the behavior exists but is unverified on the stated
  platform or path.
- **Decision** — closing the item requires a policy, trust, schema, or acceptance
  decision that this work cannot make on the operator's behalf.
- **Limitation** — the boundary is documented as out of scope and stays explicit.

| # | Item | Contract | Exact path | Existing evidence | Disposition |
| - | ---- | -------- | ---------- | ----------------- | ----------- |
| 1 | Plan 05 GAP-4 — per-document index integrity | `Identity.Digest` is a digest over `(relative path, file content SHA-256)` for every indexed file; `Document` rows carry `Class`, `Authority`, `Lifecycle`, `AuthoritySource`; `Identity` claims to capture "the repository state the index corresponds to" (`internal/repoindex/index.go:160-168`) | `internal/repoindex/index.go` (`Build`, `Identity`), `internal/repoindex/helpers.go` (`classifyAuthority`, `readSOPMetadata`), `internal/cli/index.go` | `internal/repoindex/index_test.go` (determinism/authority); `docs/reports/owasp-llm/llm05-poisoning.md:361-364`; no test covers metadata-driven identity | **Defect** (aggregate identity omits SOP-metadata-derived document rows) plus a **decision** for per-row digests/schema |
| 2 | Plan 05 GAP-5 — RAG `document_hash` | The contract maps `documents.content_hash` to the ingestion fingerprint of the full source document; `text` is the returned chunk (`docs/specs/RAG-RETRIEVAL-CONTRACT.md:100-121`). Algorithm and canonicalization are not defined | `internal/ragclient/ragclient.go:85-96` (`Evidence.DocumentHash`), `internal/ragclient/ragclient.go:273-291` (decode) | `internal/ragclient/ragclient_test.go:41,89` round-trip the literal `sha256:abc`; no comparison with returned text; `llm05-poisoning.md:364` | **Limitation** (SOP never receives the full document) + **decision** for an additive chunk-hash contract |
| 3 | Plan 04 — local scanner integrity | Acquisition pins archives/binaries; dependency adapters MUST run `--version` and match the pin; Semgrep/Gitleaks pins are descriptive and scanning is version-checked only (`docs/specs/SECURITY-SCANNER-ADDON.md:439-451`; `llm04-supply-chain.md:219`) | `scripts/security/*`, `scripts/checks/security-integration.sh`, `.github/workflows/ci.yml:71-85`, `internal/security/{semgrep,gitleaks,osv,cargoaudit}.go` | `scripts/checks/test-fetch-pinned.sh` (cache hits re-verified); manifest pins; CI download cache is re-verified; execution verifies version only | **Decision** (same-version binary substitution in a writable local tools directory is outside the current contract); no deviation from an enforced MUST was found |
| 4 | Plan 01 — macOS isolation evidence | OS isolation is demonstrated only on the isolated-replay and evaltarget paths; normal agent execution is policy-only (`docs/reports/security-shift-left/seatbelt-evidence.md`, `execution-path-matrix.md`) | `internal/dqharness` (Seatbelt profile `replay_exec.go:132-140`), `internal/evaltarget` | `seatbelt-evidence.md`: 18 PASS/3 SKIP (`dqharness`), 9 PASS (`evaltarget`) on Darwin arm64; `llm03-agency.md` still records macOS Seatbelt `UNVERIFIED` | **Missing evidence** plus a reproduced **boundary limitation**: Seatbelt `(deny network*)` does not mediate AF_UNIX `connect`, so a host Unix socket outside the replay directory stays reachable (Linux bubblewrap's restricted view is the only path with a test) |
| 5 | Plan 10 — Slack delivery | The webhook client accepts a 2xx as confirmed delivery; Slack mrkdwn is escaped; no channel is enabled by default (`docs/guides/NOTIFICATIONS.md`) | `internal/notify` (`slack.go`, `webhook.go`, `dispatch.go`), `internal/cli/notify.go` | `internal/notify/*_test.go`; `internal/cli/notify_acceptance_test.go:158` delivers to a loopback `httptest` receiver; live delivery is `NOT_RUN` (`llm10-output-handling.md:180`) | **Missing evidence** (one live message) gated on explicit operator authorization; HTTP acceptance is not rendering evidence |
| 6 | Plan 03 — live-evidence acceptance | The bounded campaign is evidence, not approval; manual operator acceptance is required (`llm02-disclosure.md:247-259`) | `docs/reports/owasp-llm/llm02-campaign.json`, `llm02-disclosure.md` | Campaign artifact sha256 `0c8cc0ff…`, revision `9c32e3d2…`, merged in PR #300 (`6de95c33`); `DISCLOSE-003` is `PLANNED`, the recorded outcomes are `PARTIAL`, and no acceptance record exists | **Decision** (designated approver must accept the exact artifact); model-visible content disclosure and the absence of general DLP stay explicit |

## Cross-cutting boundaries

- No new service, evaluator, gate, or permission framework is required by any
  item above.
- Provider model provenance, publisher authentication, and general DLP remain
  outside SOP ownership; none of the items above changes that.
- The repository's report-only security mode and retrieval defaults are unchanged.
