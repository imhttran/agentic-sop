# Validation Evidence Reuse

**Status:** Implemented in the isolated worktree `agentic-sop-ver` (VER-001 identity,
VER-002 portable cross-worktree store, VER-003 `sop validate --cache` integration and
surfacing). Not yet committed. VER-004 provides the regression coverage cited below.
**Package:** `internal/verifcache` (CTX-007 Verification Cache), extended; CLI wiring in
`internal/cli/validate.go` + `internal/cli/validate_reuse.go`.

## Purpose

SOP reruns a project's native validation commands (build/test/lint) through
`sop validate`. Re-running an identical validation is pure waste. Validation Evidence Reuse
lets SOP **reuse a prior PASS result** instead of re-running it — but only when the result
provably still applies. Reuse is a *narrowing* of what runs, never a weakening of what is
verified.

## The identity

Evidence is keyed by a deterministic identity (`verifcache.Identity`). Every component
participates in the key digest; two identities are interchangeable only when equal
(`verifcache.Equivalent`).

| Component | Meaning | Constructor |
| --- | --- | --- |
| `Schema` | identity/storage schema version | `verifcache.SchemaVersion` (`verifcache/2`) |
| `Repository` | revision state: HEAD + dirty flag | `RepositoryIdentity(head, dirty, contentDigest)` |
| `Inputs` | effective inputs: relevant input files (path + content hash) | `EffectiveInputIdentity` / `EffectiveInputDigest` |
| `Command` | the exact verification command/suite | `CommandIdentity` |
| `Environment` | relevant environment and toolchain | `EnvironmentIdentity` (+ `ToolchainIdentity`, `EnvironmentInputs`/`EnvironmentDigest`) |
| `CommandPolicy` | command-policy classification of the commands | `CommandPolicyIdentity` |
| `Scope` | the objective/criterion/check set the evidence covers | `ScopeIdentity` |
| `Verifier` | verifier identity and version | caller-supplied |

Constructors are deterministic, pure, and set-based (trim, dedupe, sort), so enumeration
order and duplicates never perturb a key. `internal/verifcache` performs no I/O and consults
no model, network, or confidence score; classification strings come from
`internal/commandpolicy` but the package does not depend on it.

### Effective inputs

`identityInputs` (CLI) enumerates the project config plus any check command that names a
single repository-relative file, and hashes their current contents (including uncommitted
changes) into `Inputs`. A changed relevant input changes the digest. Two worktrees with
identical effective inputs produce the same identity.

### Toolchain and command policy

`ToolchainIdentity(goVersion, goos, goarch)` and `EnvironmentInputs`/`EnvironmentDigest`
fold PATH, the Go toolchain version, GOOS/GOARCH, and the command-policy classification of
the check argv into `Environment`/`CommandPolicy`. A changed toolchain or reclassification
invalidates the entry.

### Scope

`ScopeIdentity` keys the evidence to the objective/criterion/check identifiers it covers.
Evidence produced for one scope is never reused for another.

## Cross-worktree reuse

An identity records no worktree path, so an identity computed in one worktree equals one
computed in another worktree **iff** revision, effective inputs, command,
environment/toolchain, command policy, scope, and verifier match. The portable store
(`verifcache.SharedStore`) is **opt-in** via `SOP_VERIFY_CACHE_SHARED` and disabled by
default; lookup consults the local cache first, then the shared store, and returns the first
exact identity match. Diverged revisions do not match and cannot share evidence.

## Fail-closed reuse

`verifcache.Decide(prior, current)` and `Cache.Reusable(id)` grant reuse only when the
identity matches **and** the stored verdict is `VerdictVerified`. Anything else fails
closed: no/missing/unreadable evidence, an identity mismatch, or a verdict of
`FAILED`/`INCOMPLETE`/`PARTIAL`/`HOLD`/`UNTRUSTED`/unrecorded is never reusable as PASS. A
miss means "run the verification"; it is never a negative result. `sop validate --cache` is
always a miss on a dirty working tree.

## Invalidation rules

Reuse is a **miss** when any of the following changes: the schema version; the repository
revision (HEAD moves); the effective-input digest (a relevant input changes, including an
uncommitted change in the relevant set); the command set or any argv; the environment or
toolchain identity; the command-policy classification; the validation scope; or the
verifier version.

## Provenance and audit

Reuse decisions are written append-only as JSONL (`verify-cache-decisions.jsonl`) beside the
run artifacts, recording the hit/miss, key digest, scope, consulted store, source run, and
reason. `sop validate` prints a read-only `reuse:` provenance line (reuse vs run, key, store,
invalidation reason, and any decision-record warning); the decision is read back read-only by
`sop report`/`sop verify`. Archives are never rewritten and no state transition occurs.

## CLI integration

`sop validate --cache` builds the extended identity, consults the (local, optionally shared)
store, reuses on an exact identity match with a VERIFIED verdict, otherwise runs and stores
the result with its verdict. The default path is local-only and behavior is unchanged apart
from the added provenance line. A decision-record write that fails sets `validateReuse.RecordErr`,
which is surfaced as a warning so a lost audit record is never silent.

## Known limitations

- The full `go test ./...` suite cannot run in this environment: a pre-existing `internal/cli`
  test hangs on a SQLite commit (10-minute timeout). The historical failure is preserved at
  `.agent-sdlc/runs/VER-001/validation.full-suite.FAILED.json` and is never reclassified as PASS.
- This baseline lacks the SOP-HARD-03 acceptance-verification hardening, so an
  already-implemented task can be classified `ALREADY_SATISFIED` and rejected (see backlog).
- The gate's validation scope for these tasks is `internal/verifcache` + `internal/archtest`
  (the affected packages); CLI integration is evidenced by build/vet plus the targeted reuse
  tests, not by the full CLI suite.

## Non-goals

No new database, service, daemon, or third-party dependency; no model-, network-, or
confidence-based reuse; equivalence is deterministic only.
