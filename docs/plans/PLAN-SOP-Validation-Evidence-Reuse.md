# PLAN — SOP Validation Evidence Reuse

**Type:** Capability plan (reuse of deterministic validation evidence). **Planning only — no runtime change.**
**Status:** PROPOSED — not activated. Requires operator approval before any task runs.
**Baseline:** `agentic-sop` `main` @ `8980826`.
**Worktree:** isolated `agentic-sop-ver` on branch `plan/ver-evidence-reuse` (based on `8980826`). Primary `main`, `recovery/rag025`, and the 30 uncommitted hardening paths are **not** modified.
**Git policy:** solo-developer local mode (`workflow.mode: local`); native git, minimal overhead, no PR workflow, and no automatic commit or push.
**Policy-inputs note:** no `AGENT-POLICY.md` exists in this workspace (searched). This plan follows the operator's stated solo-developer Git intent and the repository's own `workflow.mode: local` convention instead.

## Project
agentic-sop

## Summary

SOP already ships a correctness-first reuse mechanism — the CTX-007 Verification Cache (`internal/verifcache`) — and wires it into `sop validate --cache` (`internal/cli/validate.go`). It is **not** a green field. What is missing is narrow: (a) an *effective input* content digest, (b) a validation **scope** dimension, (c) toolchain and command-policy inputs in the environment identity, (d) a portable store that permits **cross-worktree** reuse when equivalence is provable, with an append-only reuse decision, and (e) fail-closed integration plus inspectability. This plan adds exactly those by extending `internal/verifcache`, `internal/validate`, the run artifacts, and `internal/commandpolicy`; it introduces **no** new database, service, or dependency, and it does **not** duplicate the existing cache.

## Source-grounded audit (at `8980826`)

| Capability | Where | Status |
| --- | --- | --- |
| Verification cache + identity + digest | `internal/verifcache/verifcache.go:27,32-53,91-134,160-192` | EXISTS (CTX-007) |
| Identity constructors: repo / command / environment | `internal/verifcache/verifcache.go:198-202,207-220,224-236` | EXISTS |
| Reuse wired into validation | `internal/cli/validate.go:87-117` | EXISTS (`--cache`) |
| Validation checks (build/test/lint) | `internal/validate/validate.go:16-40` | EXISTS |
| Objective outcome verifier (+ per-criterion evidence) | `internal/outcome/outcome.go` (`Status`, `Criterion`, `CriterionResult`, `Verify`, `PropagateEvidence`) | EXISTS |
| Run artifacts (`outcome.json`, `state.json`, ...) | `internal/cli/outcome.go:404`; `internal/run/run.go:20,23`; `DirName=".agent-sdlc"` `internal/config/config.go:31` | EXISTS |
| Command classification (SAFE/APPROVAL/DENIED) | `internal/commandpolicy/commandpolicy.go:61-74` | EXISTS |
| Phase 8 neutrality guard (includes `verifcache`) | `internal/archtest/arch_test.go` (`phase8CorePackages`) | EXISTS |
| CI jobs (check, race, acceptance-evidence, distribution, windows) | `.github/workflows/ci.yml` | EXISTS |

### Verified gaps

1. **Content digest is unset.** `internal/cli/validate.go:98` builds `RepositoryIdentity(head, false, "")` — the content digest argument is empty, so identity pins HEAD only and cannot see uncommitted, generated, or scope-selected input differences.
2. **No scope dimension.** `verifcache.Identity` has Schema/Repository/Command/Environment/Verifier (`verifcache.go:32-44`) but no field for *which* objective/criterion/check the evidence covers; reuse is all-or-nothing over the whole command set and the whole repository.
3. **Environment is PATH-only.** `validationEnv()` returns `{PATH: ...}` (`validate.go:131-133`); toolchain version and command-policy classification are not in the identity.
4. **Dirty tree always misses.** `validate.go:92-94` runs whenever the tree is dirty, which is safe but forecloses reuse of uncommitted work whose equivalence is provable by content digest.
5. **Store is per-project and local.** The cache lives at `<dir>/.agent-sdlc/context/verify-cache.json` (`validate.go:135-137`); the identity key is worktree-independent, but there is no cross-worktree lookup.
6. **Reuse is an opaque blob.** Only the whole `testrunner.SuiteResult` JSON is stored (`validate.go:112-113`); there is no typed reuse decision, no link to the run artifact or criterion ids, and no append-only reuse audit.
7. **Failures are stored and replayed too** (`validate.go:113` stores `Passed: suite.Passed()`), which is not a correctness bug but leaves the reuse decision untyped and un-surfaced.
8. **No objective/task-level reuse.** Only `sop validate --cache` reuses; the objective verdict (`outcome.json`) is never reused.

## Smallest missing functionality

Populate and define the **effective input set** digest, add a **Scope** dimension and toolchain/command-policy inputs to the environment, make the store **portable** (cross-worktree) with an append-only decision, and wire it **fail-closed** with surfacing. Everything else already exists and must be reused, not rebuilt.

## Architecture (pinned)

- SOP owns validation orchestration and the evidence lifecycle; child projects own their native validation commands (executed by SOP, classified by `internal/commandpolicy`).
- Evidence may be reused **only** when the effective code revision, effective inputs, toolchain/environment, command, and validation scope all match.
- Reuse evidence across worktrees **only** when equivalence is verifiable (same revision + same effective-input digest + same environment/command/scope); otherwise **fail closed** (miss, re-run).
- **Never** reuse failed, incomplete, partial, hold, or untrusted evidence as `PASS`.
- Deterministic execution and the human-approval boundary are preserved; reuse changes *whether a command runs*, never *whether a human approves*.
- No new databases, services, or dependencies: extend `internal/verifcache`, the run artifacts, and existing `--cache` behavior.

## VER-001 — Evidence identity + equivalence matcher

Extend the CTX-007 identity so it keys on effective inputs, toolchain/environment, command, and scope, and expose a pure equivalence check. Package: `internal/verifcache` (not in the dirty set; safe to edit).

### Deliverables
- `internal/verifcache` — add a `Scope` component to `Identity` (objective/deliverable/check-id set, deduplicated and sorted) and a `ScopeIdentity(...)` constructor; bump `SchemaVersion` (old entries invalidate — fail-closed).
- Effective-input digest: `EffectiveInputDigest(files)` — a deterministic, sorted `path:content-hash` digest over the *relevant input set* (declared inputs, spec, acceptance checks), covering uncommitted changes; consumed via the existing `RepositoryIdentity(head, dirty, contentDigest)`.
- Environment/toolchain: extend the inputs to `EnvironmentIdentity` to include PATH, the Go toolchain version, `GOOS`/`GOARCH`, and the command-policy classification of the check argv (deterministic, model-free; classification only — no execution).
- A pure `Equivalent(prior, current Identity) bool` (no I/O, no model, no confidence).

### Acceptance Criteria
- Identity is deterministic and depends only on (effective inputs, toolchain/environment, command, scope, verifier).
- `Equivalent` is true **iff** every component is equal; a relevant-input change, toolchain change, command change, or scope change yields **false**.
- `internal/verifcache` remains provider/model neutral: the `phase8CorePackages` arch guard still passes.
- No new dependency, database, or service.

### Acceptance Checks
- `go test ./internal/verifcache/...`
- `go vet ./internal/verifcache/...`
- `go test ./internal/archtest/...`

### Dependencies
- none

## VER-002 — Portable cross-worktree evidence store + provenance/audit

Give the identity a portable home so an identical identity computed in another worktree can be found, and record every reuse decision append-only. Build on `verifcache` (extend it, or a thin sibling package that wraps it — **never** a second identity/cache implementation).

### Deliverables
- Keep the per-project `verify-cache.json`; add an **opt-in** shared location (config/env, default off) that lookup consults in addition to the local one. Cross-worktree reuse occurs only on exact identity equivalence.
- Append-only reuse decision record (hit/miss, key digest, scope, source run), following the existing run-artifact/audit conventions (`audit.jsonl` style under the run dir).
- Reuse `verifcache.Marshal`/`Unmarshal`; fail closed on schema mismatch, a corrupt entry, or an unreadable shared store.

### Acceptance Criteria
- Reuse across worktrees occurs only when revision, effective inputs, environment/toolchain, command, and scope all match; otherwise **miss**.
- Diverged revisions (e.g. `main` vs `recovery/rag025`) never share evidence by default.
- The reuse decision is recorded, inspectable, and reuses existing storage; no new service or dependency.
- Fail-closed on schema mismatch, corrupt entry, and unreadable shared store.

### Acceptance Checks
- `go test ./internal/verifcache/... -run 'Store|Reuse|CrossWorktree'`
- `go vet ./internal/verifcache/...`

### Dependencies
- VER-001

## VER-003 — Integrate reuse into orchestration + inspectability  (DEFERRED on SOP-HARD-04)

Wire the extended identity and portable store into `sop validate --cache`, and surface the reuse decision read-only. **Deferred** until the overlapping `internal/cli` hardening work (SOP-HARD-04) is reconciled and the dirty tree is quiesced.

### Deliverables
- `internal/cli/validate.go` — build the extended identity (effective-input digest incl. dirty, toolchain env, scope), consult the store, reuse on exact match, otherwise run; record the decision.
- `internal/cli` — read-only surfacing of the evidence key, reuse-vs-run, and invalidation reason via `sop report` / `sop verify`; no state transition.

### Acceptance Criteria
- Reuse occurs only on an exact match; every mismatch runs; failed/partial/hold/incomplete/untrusted evidence is never reused as `PASS`.
- Default behavior is unchanged when reuse is not configured; `sop validate` output gains reuse provenance.
- Surfacing is read-only; archives are never rewritten.

### Acceptance Checks
- `go test ./internal/cli/... -run 'Validate|Verify|Reuse'`
- `go vet ./internal/cli/...`

### Dependencies
- VER-001
- VER-002

## VER-004 — Verification: targeted tests + docs

Deterministic coverage for identity, invalidation, cross-worktree equivalence, and fail-closed behavior, plus operator documentation. All SOP-specific tests stay inside `agentic-sop`.

### Deliverables
- Unit tests (`internal/verifcache`): identity equality/invalidation matrix (code, inputs, environment/toolchain, command, scope), schema bump, cross-worktree equivalence, corrupt-store fail-closed.
- Integration tests: exact-match reuse; each invalidation; failed/partial never reused as `PASS`; reuse decision append-only.
- Docs: `docs/reference/EVIDENCE-REUSE.md` (identity model, invalidation rules, cross-worktree policy, fail-closed guarantees) and a docs-index/README touch.

### Acceptance Criteria
- Each invalidation case has a deterministic test that **fails before** the capability and **passes after**; runs in the standard suite with no external infra.
- Docs state: reuse only when code, inputs, environment/toolchain, command, and scope all match; never reuse failed/incomplete/untrusted as `PASS`; cross-worktree reuse only on verifiable equivalence.
- No child-project coupling (tests remain inside `agentic-sop`).

### Acceptance Checks
- `go test ./internal/verifcache/... ./internal/cli/...` (targeted)
- `go test -race ./internal/verifcache/...`
- `go vet ./internal/verifcache/... ./internal/cli/...`

### Dependencies
- VER-001
- VER-002
- VER-003

## Dependency graph

```mermaid
graph TD
    V1[VER-001 identity + matcher]
    V2[VER-002 cross-worktree store]
    V3[VER-003 cli integration (deferred)]
    V4[VER-004 tests + docs]
    H4[SOP-HARD-04 reconciliation]
    V1 --> V2
    V1 --> V4
    V2 --> V3
    V2 --> V4
    H4 --> V3
    V3 --> V4
```

## Concurrency and file ownership

- **`internal/verifcache/**`** — owned by Stream A (VER-001, VER-002, VER-004 unit). Not in the dirty set → safe to edit now.
- **`internal/cli/**`** — owned by SOP-HARD-04 (in flight, dirty) and then VER-003. **Serialize**: do not touch `internal/cli/{validate,outcome,run,report,status,cli}.go` while HARD-04 is uncommitted.
- **`internal/outcome/outcome.go`** — dirty (HARD-01/04). VER must **not** edit it; keep reuse in `internal/verifcache`.
- **`internal/config/config.go`** — dirty (HARD-01/02). Any VER config knob must serialize after those edits.
- **`docs/plans/**`, `docs/reference/**`** — new files; low collision risk.

## Parallel workstreams

- **Stream A (start now; isolated worktree `plan/ver-evidence-reuse`; no `internal/cli`):** VER-001 → VER-002, with VER-004 unit tests and `docs/reference/EVIDENCE-REUSE.md` written in parallel.
- **Stream B (deferred until SOP-HARD-04 is reconciled and the dirty tree is quiesced):** VER-003 (`internal/cli`) and VER-004 integration tests.

## Evidence invalidation rules

Evidence is a **miss** (re-run) when any of the following holds; a hit requires all to match:

- The cache/identity `SchemaVersion` differs.
- The repository revision changed (HEAD moved).
- The working tree is dirty in a way that changes the relevant input digest (unless the effective-input digest proves equivalence).
- Any effective-input digest changed (declared inputs, spec, acceptance checks, generated inputs).
- The command set or any argv changed.
- The toolchain/environment identity changed (PATH, Go version, `GOOS`/`GOARCH`, command-policy classification).
- The verifier version changed.
- The validation **scope** changed (different objective/deliverable/check set).
- Prior evidence is `PARTIAL`, `HOLD`, failed, incomplete, or lacks per-criterion argv + exit code — it is **never** reused as `PASS`.
- Cross-worktree reuse is requested across diverged revisions (e.g. `main` vs `recovery/rag025`) — forbidden by default.

## Testing strategy

- Prefer **targeted** package tests (`./internal/verifcache/...`, plus scoped `./internal/cli/... -run ...`) and existing trustworthy evidence (`verifcache_test.go`, the validate tests, recorded run artifacts).
- Do **not** re-run the full suite locally unless a specific coverage gap is identified; CI runs the full suite (`check`, `race`, tagged `acceptance-evidence`) as the merge gate.
- All SOP-specific tests remain inside `agentic-sop` (requirement: no child-project coupling).

## Risks

- **Too-permissive reuse** — mitigated by a fail-closed matcher, the default-preserving behavior, and never reusing non-`VERIFIED` evidence as `PASS`.
- **Concurrency collisions with the dirty SOP-HARD tree** — mitigated by serializing `internal/cli` and keeping VER-001/002 inside `internal/verifcache`.
- **Overfitting the matcher to today's `--cache`** — mitigated by explicit effective-input, environment, command, and scope components.
- **Schema bump invalidating existing cache entries** — expected and fail-closed; a miss only re-runs verification.

## Compatibility

- Existing per-project caches are invalidated by the schema bump and then repopulated; behavior never becomes less strict.
- Default behavior (reuse not configured) reproduces today's `sop validate` exactly.
- Archives under `.agent-sdlc/archive/` are read-only and never rewritten.

## Recommended execution order

1. **VER-001** (identity + matcher), with **VER-004 docs/unit** in parallel.
2. **VER-002** (portable store + audit), then its tests.
3. After **SOP-HARD-04** is reconciled: **VER-003** (cli integration + surfacing).
4. **VER-004** integration tests; then the full CI gate on the merge.

## Out of scope / non-goals

- No new database, service, daemon, or third-party dependency.
- No model- or confidence-based reuse; equivalence is deterministic only.
- No reuse of objective verdicts (`outcome.json`) as lifecycle decisions in this plan (that is SOP-HARD territory); this plan reuses *validation* evidence only.
- No modification of `recovery/rag025`, the primary dirty tree, or the 30 uncommitted hardening paths.

This plan is **PROPOSED**: do not activate, implement, commit, or push until the operator approves.

## Completion status (VER wrap-up)

Documentation only; this section changes no task.

- **VER-001** — LOCAL_DONE (SOP `PASSED`).
- **VER-002** — LOCAL_DONE (SOP `PASSED`).
- **VER-003** — code complete and targeted validation passing, but SOP state is **BLOCKED**:
  this baseline lacks the ALREADY_SATISFIED acceptance-verification hardening (SOP-HARD-03),
  so SOP rejects the agent's no-change `ALREADY_SATISFIED` claim (retry budget exhausted).
  Evidence: `.agent-sdlc/runs/VER-003/` and `docs/reports/EVIDENCE-REUSE-VER-003-recovery.md`.
- **VER-004** — regression coverage exists (`internal/verifcache/{reuse_test.go,store_test.go,ver001_test.go}`,
  `internal/cli` reuse/cache tests) and documentation is complete
  (`docs/reference/EVIDENCE-REUSE.md`); the SOP task could not run because it depends on VER-003.

Follow-up backlog (recorded, not implemented): ALREADY_SATISFIED acceptance-verification
robustness (SOP-HARD-03); approval-recovery churn (repeated NEEDS_HUMAN around no-change
completions); task-appropriate validation tiers (the LOW tier is unreachable without a
verify-first declaration, and the standard set must currently be narrowed because the full
suite hangs on a pre-existing `internal/cli` test).
