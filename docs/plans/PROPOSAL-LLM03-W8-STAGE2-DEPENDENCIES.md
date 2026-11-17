# Decision packet — W8 Stage 2 (required dependency scanning)

**Status:** proposal, read-only. **No configuration, baseline, or residual change is
made by this document.** Stage 1 (PR #503) is merged; `dependencies` is still
**not** required.
**Base:** `origin/main` @ `b3722cb1` (2026-11-16), darwin arm64, Go 1.27.1.
**Predecessors:**
[PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md](PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md) §8–§11,
[llm03-scanner-integrity-evidence.md](../reports/owasp-llm/llm03-scanner-integrity-evidence.md).

## 1. Current verified state (Stage 1, merged)

Clean tree, pinned adapters:

```
verdict PASS, coverage COMPLETE, execution failures 0
findings 98, suppressed 82, unsuppressed 16, blocking 0
mode required, required_categories [sast, secrets], required_mode ready=true
baseline 82 entries: 0 stale, 0 suppressions without an approved entry
```

One merged-then-fixed defect is relevant here: `sop security inspect` used to
project readiness from **every** selected adapter, so the optional `cargo-audit`
adapter (added by #502, no Rust content) made the projection report
`required-mode readiness: false` while the scan reported `PASS`. PR #507 scopes the
projection to `required_categories` and adds the regression table. This packet
assumes that fix.

## 2. What Stage 2 changes

Exactly one line:

```yaml
security:
  required_categories: [sast, secrets, dependencies]   # was: [sast, secrets]
```

No baseline entry is added or removed. No `fail_on`, `unknown_severity`,
`stale_advisory_database`, `scope`, or adapter change.

**Verified projection** (alternate config, tracked config untouched):

| Configuration | Verdict | Detail |
| --- | --- | --- |
| `[sast, secrets, dependencies]`, valid fresh DB | **`PASS`** | coverage `COMPLETE`, `ready=true`, 98 / 82 / 16 / 0 |
| same, OSV database missing | **`INDETERMINATE`** | `required category dependencies has no COMPLETE go coverage` |

## 3. The finding that decides this stage

**Stage 1 already fails closed on OSV database state, even though `dependencies`
is not required.** `incompleteReasons` in `internal/security/policy.go` is built
from every *enabled* adapter, not only required ones, so the required gate is
already coupled to the dependency data. Measured on the Stage 1 config:

| OSV database | osv-scanner status | scan verdict | reason |
| --- | --- | --- | --- |
| valid, 8 days old | COMPLETED / COMPLETE | `PASS` | no blocking findings |
| **missing** (no layout) | FAILED / UNKNOWN | `INDETERMINATE` | `adapter osv-scanner is FAILED` |
| **stale** (169 days, policy deny) | COMPLETED / COMPLETE | `INDETERMINATE` | `advisory database is stale (policy: deny)` |
| **corrupt** (digest mismatch) | FAILED / UNKNOWN | `INDETERMINATE` | `OSV database digest … does not match its acquisition metadata` |

Two consequences:

1. **Stage 1's green state is not free.** Every environment that runs `sop run`
   must hold a present, intact, ≤90-day-old OSV database, or the gate is
   `INDETERMINATE`. A fresh clone fails closed until provisioning runs. This was
   not documented before; it is now.
2. **Stage 2 does not add a new failure mode — it makes the existing one
   explicit.** With `dependencies` required, the same four states fail with a
   clearer reason (`required category dependencies has no COMPLETE go coverage`).

### Operator decision this surfaces

The scan's global fail-closed coupling to a *non-required* adapter is either
intended strictness or an over-reach. This packet does **not** propose changing
it: scoping `incompleteReasons` to required categories would weaken fail-closed
behaviour and needs its own authorization. Recommendation: keep it and document
the provisioning prerequisite.

## 4. Offline OSV provisioning and verification

Provisioning is explicit and separate from scanning
(`scripts/security/fetch-osv-databases.sh`); a scan never downloads and never
falls back to the live API. Each archive carries
`<eco>/all.zip.metadata.json` with `downloaded_at`, `ecosystem`, `sha256`, `size`
and `source`.

New verification tooling (this PR):

- `scripts/checks/osv-db-verify.sh` and `make osv-db-verify` check the layout,
  recompute each archive digest against its metadata, and enforce the 90-day
  freshness boundary (`osvScannerStaleMaxAge`), with no network access.
- `make osv-db-fetch` provisions the default ecosystems (`Go`, `crates.io`).

Measured behaviour of the checker:

| Database state | Checker | Scan |
| --- | --- | --- |
| valid, 8 days old | exit `0` — `OK (2 ecosystem(s), max age 90 days)` | `PASS` |
| missing / empty directory | exit `1` — `missing Go: …/all.zip` | `INDETERMINATE` |
| absent directory | exit `1` — `missing database directory` | `INDETERMINATE` |
| stale (169 days) | exit `1` — `stale Go: acquired … (169 days > 90)` | `INDETERMINATE` |
| corrupt (truncated archive) | exit `1` — `corrupt …: digest … != metadata …` | `INDETERMINATE` |
| invalid metadata | exit `1` — `invalid …: acquisition metadata absent` | `INDETERMINATE` |
| boundary override `--max-age-days 1` on an 8-day DB | exit `1` | — |

The checker and the scanner agree on every case, so a green `make osv-db-verify`
is a necessary precondition for a green gate.

### Refresh cadence

90 days is the hard boundary. Recommended operational cadence: refresh monthly
(≈30 days), so a single missed refresh does not fail the gate. There is no
automatic refresh; nothing downloads during a scan or `sop run`.

## 5. Scanner integrity and version-pinning requirements

| Requirement | Gitleaks | Semgrep | OSV-Scanner | cargo-audit |
| --- | --- | --- | --- | --- |
| Version pinned in config | 8.30.1 | 1.183.0 | 2.7.0 | 0.22.2 |
| Execution-time binary pin | **linux/darwin × amd64/arm64** | **none** (pip `--require-hashes`) | **linux/darwin × amd64/arm64** | **linux/amd64 + darwin/arm64 only** |
| Rules pin | default rules | `rules_sha256 d90ff1b1…` | n/a | n/a |
| Publisher signature | none | none | none | none |

For a required `dependencies` category the two gaps that matter:

1. **cargo-audit is unpinned on `linux/arm64`, `darwin/amd64` and Windows.** The
   manifest only carries `x86_64-unknown-linux-gnu` and `aarch64-apple-darwin`.
   On those platforms the dependency gate would run an unpinned binary — the
   exact supply-chain weakness the pins exist to close. Options: add manifest
   entries and pins for the missing targets, or exclude Rust dependency scanning
   from the support claim on those platforms.
2. **No publisher signatures anywhere.** A digest proves the bytes match a prior
   review, not who produced them. This is a pre-existing accepted limit; changing
   it needs its own decision.

`dependencies` also requires Rust content to carry a `Cargo.lock`. With Rust
detected and a lockfile missing, readiness is false and the scan fails closed
(`TestSecurityInspectReadinessRejectsMissingObligation`). That is intended, but it
means a Rust repository with an uncommitted lockfile is blocked.

## 6. CI and local compatibility

| Environment | Impact |
| --- | --- |
| **CI (this repository)** | No workflow runs `sop run`, so no CI verdict changes. `security-integration` already provisions the pinned scanners and uses a controlled offline OSV fixture. A job that ran the gate would need `make osv-db-fetch` plus a freshness check before the run. |
| **Local development** | `sop run` fails closed unless (a) Gitleaks 8.30.1, Semgrep 1.183.0 and OSV-Scanner 2.7.0 are on `PATH`, and (b) `make osv-db-verify` passes. This is already true for Stage 1 because of §3; Stage 2 only changes the error message. Document the setup in the scanner guide before activation. |
| **Fresh clone / air-gapped** | No database is committed. Provision with `make osv-db-fetch`, copy it in for air-gapped hosts, and refresh within 90 days. |
| **Windows** | No committed Windows pins for Gitleaks, OSV-Scanner or cargo-audit, and `run_command` is unavailable under B1. Recommend keeping Windows outside the required-gate support claim until those pins exist. |
| **Rust repositories** | Require a committed `Cargo.lock`; otherwise the gate fails closed by design. |

## 7. Acceptance criteria

Positive:

| Criterion | Evidence |
| --- | --- |
| Clean tree, valid ≤90-day DB, `required_categories [sast, secrets, dependencies]` | `PASS`, coverage `COMPLETE`, `required_mode ready=true`, exit `0` |
| `make osv-db-verify` on the same database | exit `0` |
| `sop security inspect` | `required-mode readiness: true` |
| Existing suite | `go test ./...` green; `TestSecurityInspectReadinessRejectsMissingObligation` still passes |

Negative (each must fail closed, never `PASS`):

| Control | Expected |
| --- | --- |
| OSV database missing | `INDETERMINATE`, `required category dependencies has no COMPLETE go coverage` |
| OSV database stale (>90 days, policy deny) | `INDETERMINATE`, stale reason |
| OSV database corrupt (digest mismatch) | `INDETERMINATE`, adapter `FAILED` |
| `make osv-db-verify` on each of the above | exit `1` with a specific reason |
| Rust content without `Cargo.lock`, `dependencies` required | readiness false, scan `INDETERMINATE` |
| Unpinned scanner binary | adapter `FAILED` ⇒ `INDETERMINATE` (measured in Stage 1) |
| Changed-files scope with required mode | `INDETERMINATE` (required gate cannot pass on changed-files evidence) |
| New unsuppressed HIGH finding | `FAIL` (measured in Stage 1) |

## 8. Rollback

1. Revert §2 in one commit: `required_categories: [sast, secrets]`. No baseline,
   schema, state, or artifact change is involved.
2. The OSV database and its tooling can stay; with `dependencies` not required the
   gate still fails closed on a degraded database (§3), which is current Stage 1
   behaviour.
3. Do **not** roll back by setting `security.enabled: false`: that is explicitly
   not a required-gate exemption.
4. Verify with `sop security scan --json`: `mode: required`,
   `required_categories: [sast, secrets]`, verdict `PASS`.

## 9. Remaining risks and open decisions

| # | Risk / decision | State |
| --- | --- | --- |
| R1 | Stage 1 gate depends on OSV freshness even though `dependencies` is optional (§3) | Documented; keeping as-is is recommended, changing it needs authorization |
| R2 | `cargo-audit` unpinned on `linux/arm64`, `darwin/amd64`, Windows | Open; needs pins or a scope exclusion |
| R3 | No publisher signature verification | Open, pre-existing |
| R4 | Windows outside the support claim (pins + sandbox) | Open; document |
| R5 | Rust repositories require a committed `Cargo.lock` | By design; document |
| R6 | OSV data is a point-in-time snapshot; advisories published after acquisition are not seen | Inherent; mitigated by the 90-day boundary and a monthly cadence |
| R7 | Readiness (required-category projection) is less strict than the scan verdict | Intentional after #507; documented |

## 10. Exact operator authorization wording

> I authorize W8 Stage 2 as specified in
> `docs/plans/PROPOSAL-LLM03-W8-STAGE2-DEPENDENCIES.md`: add `dependencies` to
> `security.required_categories` so it reads `[sast, secrets, dependencies]`, and
> require `make osv-db-verify` to pass in every environment that runs the gate.
> This authorizes that configuration change and the accompanying documentation
> only. It does not add publisher signature verification, does not add binary pins
> for cargo-audit on `linux/arm64`, `darwin/amd64` or Windows, does not change the
> OSV freshness boundary, does not add or remove any baseline entry, and does not
> accept any residual risk.

If R2 must be closed first, the alternative wording replaces the configuration
change with the pin work and defers Stage 2 until the pins are merged.
