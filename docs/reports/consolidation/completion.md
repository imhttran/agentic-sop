# Repository consolidation — completion report

Plan: `docs/plans/PLAN-Repository-Consolidation.md` (plan ID `plan-repository-consolidation`).
State at this initial report: **11 of 16 tasks `LOCAL_DONE`**, `ACTIVE`; no pending approvals;
no commits, pushes, service restarts or repository retirement. (Superseded by _Final status_
and _Reconciliation_ below; retained as history.)

The closure validation for this migration is recorded in
[closure-evidence.md](closure-evidence.md): the operator-authorized relative link check
(`broken links: 0`) and `git diff --check`, the `go build/test/vet` validation gates, evidence
provenance, and the external-completion reasons for C5/C7.

## Tasks and execution method (recorded honestly)

| Task                                                    | Status     | Method                                                                                                                                                                                                                                                       |
| ------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| MIG-001 Reconcile local source / baselines              | LOCAL_DONE | SOP run (`ALREADY_SATISFIED` after the baseline was corrected)                                                                                                                                                                                               |
| MIG-002 Preserve histories / package mapping            | LOCAL_DONE | SOP run (gate PASS)                                                                                                                                                                                                                                          |
| MIG-003 adapter `decision/` + `tools/julia/`            | LOCAL_DONE | **deterministic staging**, accepted by SOP via `task complete --external`                                                                                                                                                                                    |
| MIG-003A adapter providers + shadow                     | LOCAL_DONE | deterministic staging → `--external`                                                                                                                                                                                                                         |
| MIG-003B controller config + sopclient                  | LOCAL_DONE | deterministic staging (byte-exact + rewrite) → **`ALREADY_SATISFIED`**                                                                                                                                                                                       |
| MIG-003C controller web + embedded assets               | LOCAL_DONE | deterministic staging → `--external`                                                                                                                                                                                                                         |
| MIG-003D module deps + verify all imported              | LOCAL_DONE | SOP run (`VERIFIED`)                                                                                                                                                                                                                                         |
| MIG-004 unified CLI + wrappers                          | LOCAL_DONE | **implemented directly in-session**, recorded `--external` (validation PASS)                                                                                                                                                                                 |
| MIG-005 integrate controller without changing authority | LOCAL_DONE | SOP run (gate PASS). Recorded outcome is `PARTIAL` (stale): it named `scripts/controller/` and `integrations/controller/` as "not present" at 11:44, but both support assets now exist (added 13:29 under MIG-008); SOP classifies the plan `PLAN_COMPLETE`. |
| MIG-006 advisory integration + fallback                 | LOCAL_DONE | **implemented directly** (SOP `IMPLEMENT_NO_PROGRESS`) → `--external`                                                                                                                                                                                        |
| MIG-007 configuration precedence                        | LOCAL_DONE | **implemented directly** → `--external`                                                                                                                                                                                                                      |
| MIG-008 installers + support assets                     | PLANNED    | —                                                                                                                                                                                                                                                            |
| MIG-009 CI + regression suites                          | PLANNED    | —                                                                                                                                                                                                                                                            |
| MIG-010 README + references                             | PLANNED    | —                                                                                                                                                                                                                                                            |
| MIG-011 end-to-end dogfood regression                   | PLANNED    | —                                                                                                                                                                                                                                                            |
| MIG-012 readiness + retirement report                   | PLANNED    | —                                                                                                                                                                                                                                                            |

SOP's tool-driving model performs no writes on fresh multi-file engineering tasks
(recurring `IMPLEMENT_NO_PROGRESS`); deterministic or direct implementation recorded
through SOP's supported completion mechanisms is used where that occurs — never a
fabricated mutation and never a manual `state.db` edit.

## Harness fixes (with regressions)

- Environment isolation for decision, model-routing and adapter-runtime env:
  `internal/config`, `internal/cli` (decision + `TestRunPlanUsesConfiguredProvider`),
  `internal/decisionadapter/providers/clef/config_test.go`, and
  `internal/decisionadapter/cli/main_test.go`.
- Acceptance-check reconciliation hydration (`internal/planflow`) — unchanged checked
  tasks are not spuriously "changed" after reload.
- Confined deterministic primitives `copy_file` and `rewrite_file` (idempotent), and
  the FINALIZE/PLAN/REVIEW synthesis-correction steering.
- Regressions: `TestReconcileUnchangedCheckedPlanAfterReloadHasNoChanges`,
  `TestNoDirectoryToolFileCollisionIsActionable`, `TestWriteFileCreatesParentDirectories`,
  `TestCrossRootReadToPrimaryWrite`, `TestCopyFileIsDeterministicCrossRootImport`,
  `TestRewriteFileIsExactAndIdempotent`, `TestControllerImportSequenceIsIdempotent`,
  `TestFinalizeCorrectionDemandsTheOutcomeJSON`, `TestUnifiedControllerAndDecisionCommands`,
  `fallback_bounds_test.go`.

## Architecture and command compatibility

One Go module `github.com/imhttran/agentic-sop`. Unified commands:
`sop run`, **`sop controller serve`**, **`sop decision decide|shadow`**, with thin
legacy wrappers `cmd/sop-controller` and `cmd/sop-decision-adapter` over the same
shared implementations (`internal/controller.Serve`, `internal/decisionadapter/cli.Run`).
Core package locations, the canonical decision seam and profile bridge, controller
security (read-only stores, auth/CSRF/session), Julia support, skills and CI are
preserved. The controller delegates to the configured `sop` executable (`SOP_BIN`
overrides). Advisory decisions cannot lower risk, approve work or bypass governance.

## Validation

- `go build ./...`, `go vet ./...` **PASS**; `gofmt` clean.
- Full `sop validate` gate **PASS** (BUILD/UNIT_TEST/LINT), both under the project
  `.env` and under the execution routing env.
- Import equivalence verified deterministically (source vs destination after the
  documented rewrites).
- Live checks: oMLX / Bespoke local contracts **BLOCKED** (not probed; no service
  manipulation); CLEF→Nimble failover **PASS** in fixtures, **live BLOCKED**.

## Source / history preservation

Sibling repositories unchanged (HEADs + uncommitted deltas intact; read-only) and remain
intact as the rollback path; no retirement or deletion has occurred. Bundles/rollback from
MIG-002 remain valid. At the time this section was first written **no history-integration
commit had been created**; the ancestry-preserving merge is now **committed** as
`a8082b6154dd88fd8ac9fd18af5df5e682a751a6` (parents core/controller/adapter — see
_Consolidation merge commit — COMMITTED_ below).

## Files changed (uncommitted, awaiting approval)

Harness: `internal/toolharness/{files,harness,mutation,harness_test,write_scenarios_test,import_sequence_test}.go`,
`internal/ollamaagent/{plan,review,prompt,implement,activity,evidence,deliverable,finalize_guidance_test}.go`,
`internal/planflow/planflow.go`, `internal/cli/{cli,cli_test,controller,controller_decision_test,decision,decision_evidence_crossmodule_test,decision_evidence_test,jev_evidence_test,jev_lifecycle_test,modelroute_test,reconcile_checks_reload_test}.go`,
`internal/controller/sopclient/{start,start_test}.go`, `internal/controller/web/dogfood_test.go`,
`internal/config/config_test.go`, `internal/config/decision_provider_test.go`.
Migration: `decision/`, `tools/julia/`, `internal/decisionadapter/` (+`cli/`, fixtures),
`internal/controller/` (+`service.go`, `web/`, `assets/`), `cmd/sop-controller/`,
`cmd/sop-decision-adapter/`, `.env.example`, `docs/reports/consolidation/*`,
`docs/plans/PLAN-Repository-Consolidation.md`. Performance regressions: none observed
(unified `sop` is +26.9 % in binary size vs the pre-consolidation baseline; startup <10 ms both).

## Remaining (MIG-008 … MIG-012)

Installers/support assets (008), CI/regression suites (009), README/references (010),
end-to-end dogfood + performance (011), and the readiness/retirement report (012).
Their acceptance requires installer/CI edits and live dogfood against a disposable
project, and must not be done shallowly; they are `PLANNED` behind their dependencies.

## Startup / configuration (verified)

```
sop run                      # governed lifecycle
sop controller serve         # read-only dashboard (SOP_BIN overrides the delegate)
sop decision decide ...      # advisory decision, e.g. -provider nimble -file req.json -json
```

## Ready for commit approval

All work above is uncommitted. No commit, push, deploy or retirement is performed;
this report is the reviewable artifact requested before commit approval.

---

## Final status: ALL 16 TASKS LOCAL_DONE

MIG-011 and MIG-012 are now recorded (external, validation PASS). MIG-011's controller
run path was fixed (`internal/controller/sopclient/start.go`): the Run action executes a
started-but-incomplete plan via `sop run`, never the read-only `sop resume`, with a
regression (`start_test.go`) proving the delegated argv. An authenticated
controller-triggered run executed T001 to `LOCAL_DONE` with report/validation evidence.
Live Nimble/Bespoke probe PASS; live CLEF/oMLX consultation PASS (`provider=clef
model=clef-4bit`, ~1.7 s; the earlier 5 s deadline failure and the ambiguous-id malformed
response are preserved separately in the final Reconciliation);
failover and installation checks PASS. Candidate rebuilt from the current tree:
`sop` `5ea4151ac8835a36f977491b21751469ad4d9ac695c499afb0141bdcafa7c0a8` (byte-reproducible).

No commits, pushes, personal installation, service restarts, user-owned `.env` edits or
repository retirement occurred. All work is uncommitted and ready for review.

---

## Reconciliation with acceptance evidence (final)

All 16 tasks are `LOCAL_DONE`, each with SOP-verified criteria and a passing validation
gate. The earlier "objective is PARTIAL" note is resolved:

- **VERIFIED — live CLEF/oMLX consultation.** The earlier `UNRESOLVED` record combined two
  distinct observations, both preserved in `dogfood.md`: (a) a real **5 s-bounded deadline
  exceeded** (the model's first-token latency exceeds 5 s), and (b) a **malformed payload for an
  ambiguous question id** (the default probe `-decision-id example -state probe` repeats one
  string as both context and question id, and the 4-bit model then mis-keys its answer). Under
  the production 30 s bound a descriptive question id is served by the clef primary itself
  (`provider=clef model=clef-4bit`, ~1.7 s, no fallback). This is **not** a claim that no timeout
  occurred. `internal/decisionadapter/providers/clef/chat.go` is a byte-identical import of the
  sibling repo, so its prompt is intentionally **not** modified (MIG-003B equivalence).
- **VERIFIED — injected-failure failover** to Nimble (0.42–0.53 s, `served_provider:"nimble"`),
  provider attribution, both-unavailable (adapter `unavailable`, exit 3) and deterministic
  fallback (fixtures), governance preservation, and per-path latencies.
- **Harness corrections this pass** (recorded via `sop memory`, each with a regression):
  - `internal/cli` `TestMain` now neutralizes the whole model-routing environment namespace,
    so a per-invocation `SOP_MODEL_*` override no longer fails lifecycle tests before they
    reach the behavior under test.
  - `sop controller serve --help`/`-h` now prints usage and exits 0 **before** `Serve` — no
    runtime-configuration load, project discovery or listener; regression
    `internal/cli TestControllerServeHelpIsSideEffectFree`.
- **MIG-005 recorded `PARTIAL` (stale, reconciles clean).** Its outcome named two missing
  deliverables — `scripts/controller/` (`sop-agent.sh`, `https-proxy.sh`, `keychain-token.sh`)
  and `integrations/controller/` (`Caddyfile`) — that did not exist when MIG-005 ran (11:44)
  but do now (13:29, delivered under MIG-008's support-assets scope). All four of MIG-005's
  deliverables are present, its acceptance checks pass, and `sop continue --check` reports
  `PLAN_COMPLETE` (16/16 satisfied). The original `PARTIAL` outcome is preserved as history;
  no waiver or manual state edit was used.
- **COMMITTED — history integration (merge `a8082b6154dd88fd8ac9fd18af5df5e682a751a6`).**
  Full-history bundles were produced outside tracked source and verified (`git bundle verify`
  = complete history; restore `fsck --full` clean) and now live in the durable backup (below):
  `core.bundle` `38089deb…4219e`, `controller.bundle` `de33fdbc…7f3b35`,
  `adapter.bundle` `536715c3…a9bbee9`, plus `decision-adapters-delta.patch` `6ea5cc9c…38c99b`.
  The ancestry-preserving integration is now **committed** as merge
  `a8082b6154dd88fd8ac9fd18af5df5e682a751a6`, whose parents are the three source tips:
  core `54aa6a1a8f73a3b6debef0c54472351c2a817080`,
  controller `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` and
  adapter `f319f60df470705db220d67875fdefbe4666e5da`; ancestry was verified with
  `git merge-base --is-ancestor`. The commit did not rewrite ancestors.
  _(History: the integration was first demonstrated non-committingly via
  `git merge --no-commit -s ours --allow-unrelated-histories <controller-tip> <adapter-tip>`,
  which recorded **both** tips in `.git/MERGE_HEAD` and left the tree identical to core
  (`54aa6a1`); that earlier prepared-and-uncommitted state is retained here as history.)_

The `--allow-unrelated-histories` flag was required: without it git refused the merge
(`rc=128`), which the originally documented command omitted.

Nothing above erases history: the `LOCAL_DONE` records, prior run evidence, the earlier
`UNRESOLVED` CLEF record and the failed-run evidence are all preserved.

---

## Objective acceptance and remaining limitations

- **Acceptance: PASS (local only).** All 16 tasks `LOCAL_DONE`; `sop validate`
  BUILD/UNIT_TEST/LINT PASS under the project `.env` and under the execution routing env;
  `sop continue --check` → `PLAN_COMPLETE` (16/16). No commit, push, CI, deploy, merge or
  repository retirement has occurred.
- **Remaining limitations (honest):**
  1. **The consolidation merge commit was prepared and verified before commit** — a single
     ancestry-preserving merge (parents core/controller/adapter) carrying the 259-path content.
     It is now **committed** as merge `a8082b6154dd88fd8ac9fd18af5df5e682a751a6`; the earlier
     prepared-but-uncommitted state is retained as history.
  2. **Controller licensing resolved** — `sop-controller` ships no `LICENSE`, but the operator
     confirmed ownership and authorized Apache-2.0 for the imported controller source
     (`controller-provenance.md`); no applicable notice existed to preserve.
  3. **CLEF/oMLX is id-reliability-limited** — a descriptive question id is served; an ambiguous
     id is mis-keyed; the imported prompt is intentionally unchanged.
  4. **Live both-unavailable with both real backends is not exercised** (Nimble is running); the
     full path is proven with disposable fixtures/testservers.
  5. **Performance:** unified `sop` binary is +26.9 % size vs the pre-consolidation baseline;
     startup <10 ms both. CLEF consultation ≈1.7 s; injected failover ≈0.5 s.

## New verification evidence (this pass — earlier outcomes preserved)

- **MIG-005 re-verified through SOP** — `sop verify MIG-005` reprints the historical `PARTIAL`
  (preserved, not rewritten); the two missing deliverables now exist and all three MIG-005
  acceptance checks re-run PASS; evidence appended via `sop memory` (append-only).
- **Full both-unavailable path** — `internal/cli TestDecisionEvidenceAdapterBothUnavailable`
  drives `SOP lifecycle → adapter connector → real unified sop decision → clef primary
(unreachable) → nimble backup → deterministic fallback`, using a closed loopback port and a
  disposable `/score` testserver (no running service touched). It asserts `decision-evidence.json`
  (`outcome_class`, `failure_label`, empty choice/confidence, `served_provider`,
  `fallback_reason`, `baseline_action == final_action`) and governance preservation.
- **Import equivalence re-check** — deterministic: 181 files, 177 byte-equal, 4 documented
  deviations (`import-rewrites.md`).
- **Harness fixes this pass** — `internal/cli` `TestMain` model-env neutralization; and
  `sop controller serve --help`/`-h` side-effect-free (`internal/cli
TestControllerServeHelpIsSideEffectFree`).
- Earlier outcomes (the CLEF 5 s deadline failure, the ambiguous-id malformed response, the
  MIG-005 `PARTIAL`, the failed-run evidence) are retained, not erased.

## Durable backup and restore

Backups were **moved out of `/tmp`** into a durable directory outside all three repositories
(and outside `/tmp`):

`/Users/imhttran/agentic-workspace/backups/sop-consolidation-20261014` (dir mode `700`;
`state/` files mode `600`). Canonical manifest: its `MANIFEST.md` (mirrored at
`docs/reports/consolidation/backup-manifest.md`). **Refreshed after the final edits** (v2
tracked/untracked snapshots; the v1 snapshots are preserved):

| Artifact                                           | sha256             |
| -------------------------------------------------- | ------------------ |
| `bundles/core.bundle`                              | `38089deb…4219e`   |
| `bundles/controller.bundle`                        | `de33fdbc…7f3b35`  |
| `bundles/adapter.bundle`                           | `536715c3…a9bbee9` |
| `patches/decision-adapters-delta.patch`            | `6ea5cc9c…38c99b`  |
| `patches/agentic-sop-uncommitted-tracked.v2.patch` | `c1aeaf28…978a1b`  |
| `patches/agentic-sop-untracked.v2.tar.gz`          | `7d6ba1df…cdd9db`  |
| `commit-file-list.txt` (259 paths)                 | `6ff7e722…7e18c`   |
| `state/state.v4.db`                                | `52390c72…2ef0d`   |

Copy hashes verified equal to the `/tmp` originals; bundles re-verify and restore clean from
the new location (refs 110/5/4); `PRAGMA integrity_check` = `ok`, `user_version` = 4. The
tracked patch is byte-stable (no tracked file changed after the first snapshot); the untracked
archive grew because the finalised docs are untracked.

```sh
B=/Users/imhttran/agentic-workspace/backups/sop-consolidation-20261014
# committed history:
git init /tmp/restore-core && git -C /tmp/restore-core fetch "$B/bundles/core.bundle" '*:refs/remotes/bundle/*'
git -C /tmp/restore-core fsck --full
# UNCOMMITTED work (not in any bundle) — v2 is current:
git -C <checkout> apply "$B/patches/agentic-sop-uncommitted-tracked.v2.patch"
tar -xzf "$B/patches/agentic-sop-untracked.v2.tar.gz" -C <checkout>
# completeness check: the restored untracked set equals the 259-path list
git -C <checkout> ls-files --others --exclude-standard | sort -u | diff - "$B/commit-file-list.txt"
```

Git bundles contain **only committed history**; uncommitted work is captured separately by
design. No secrets are included (`.env` is git-ignored and excluded from the untracked tarball).

## Controller provenance and licensing (resolved)

The controller import is recorded in `docs/reports/consolidation/controller-provenance.md`:
source `imhttran/sop-controller` @ `f8502a500e52420ff0b4935b8abc0ebb5b218f2b`, clean tree, the
imported paths and the documented modifications (only `internal/controller/config/config.go`,
`internal/controller/sopclient/start.go`, `internal/controller/web/dogfood_test.go` and the
`cmd/sop-controller` wrapper deviate beyond the import rewrites). **No source `LICENSE` file
exists**, but the operator has **confirmed ownership and authorized licensing the imported
controller source under Apache-2.0** (2026-10-14), so it is covered by this repository's
existing Apache-2.0 `LICENSE`. **No applicable notice existed to preserve** (no `NOTICE`/
`COPYING` file and no per-file header in either source). Adapter Apache-2.0 attribution is
preserved.

## Exact proposed commit scope

**One ancestry-preserving consolidation merge commit** on `agentic-sop` `main`, whose tree is
the consolidated content and whose parents are the three tips:

| Parent     | Tip                                        |
| ---------- | ------------------------------------------ |
| core       | `54aa6a1a8f73a3b6debef0c54472351c2a817080` |
| controller | `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` |
| adapter    | `f319f60df470705db220d67875fdefbe4666e5da` |

Content = the explicit **259-path** migration list (`commit-file-list.txt`): **23 modified
tracked** + **236 added**, **0 deleted**. Top-level: `internal/` 197, `tests/` 18, `docs/` 15,
`tools/` 12, `decision/` 7, `scripts/` 4, `cmd/` 2, `integrations/` 1, plus `.env.example`,
`Makefile`, `README.md`.

- **Verified in a disposable fixture**: performing the non-committing octopus `ours` merge and
  staging exactly this 259-path list produced a tree identical to the real working tree
  (**0/259 mismatches**) with `.git/MERGE_HEAD` = both sibling tips; **no commit was created**
  and the fixture was discarded.
- **Adapter delta included**: the proposed tree's
  `internal/decisionadapter/providers/fallback/fallback.go` equals the adapter's _uncommitted_
  working-tree file (modulo the import rewrite), so the local delta is carried by the commit's
  content, not only by the merge parent.
- **Excluded (unrelated / out of scope):** `.zed/settings.json` (a pre-existing editor
  sandbox-permission change), `.agent-sdlc/` (runtime state, git-ignored), `.env`, binaries and
  temporary fixtures. **No `git add -A`** — stage the explicit list (see procedure below).
- No new license/attribution file is added (the unified repo keeps its existing Apache-2.0
  `LICENSE`, which now explicitly covers the operator-confirmed controller source).

## History-integration parents (review only)

| Parent                                   | Tip                                        | Local-only commits                      |
| ---------------------------------------- | ------------------------------------------ | --------------------------------------- |
| core (`agentic-sop` `main`)              | `54aa6a1a8f73a3b6debef0c54472351c2a817080` | none (== origin/main)                   |
| controller (`sop-controller` `main`)     | `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` | none                                    |
| adapter (`sop-decision-adapters` `main`) | `f319f60df470705db220d67875fdefbe4666e5da` | **3** (`057604e`, `e3e4787`, `f319f60`) |

The adapter's 3 local-only commits are included because the bundles are `--all`. The adapter's
**uncommitted** delta (3 files) is in no sibling commit, so the merge _parent_ does not carry
it — but it **is** present in the union commit's content (the imported
`internal/decisionadapter/providers/fallback/fallback.go` matches the adapter's working-tree
file). `patches/decision-adapters-delta.patch` is the rollback copy for the sibling checkout,
not something that must be applied after the merge.

## Honest dogfood result

Autonomous SOP implementation **stalled** on fresh multi-file engineering tasks (recurring
`IMPLEMENT_NO_PROGRESS`: the tool-driving model made no write attempts). The consolidation was
delivered by **external implementation plus SOP verification**: SOP runs for
MIG-001/002/003D/004/005; deterministic operator staging recorded through SOP's supported
completion mechanisms for MIG-003/003A/003B/003C; direct in-session implementation recorded
`--external` for the remainder. Every completion was SOP-verified (validation gate PASS); no
mutation was fabricated and `state.db` was never hand-edited.

---

## Remaining boundaries

Each boundary is recorded exactly once with an unambiguous status. None of these
statements infers current local SOP state from historical documents, and none claims
a fresh CI pass.

1. **Archived state — not rechecked.** The operator-local SOP archive, installed binaries,
   live services and backup media were **not rechecked** by this closure. The current SOP state
   was verified directly (`sop status` / `sop approvals`); it is **not inferred** from the
   historical repository documents above, and a repository merge **does not verify machine
   deployment** — the committed merge only guarantees repository ancestry and content, not the
   running local deployment.
2. **Migration CI — not independently reverified.** Migration CI for the consolidation
   merge `a8082b6154dd88fd8ac9fd18af5df5e682a751a6` is **not independently reverified**
   (workflow lookup returned no runs); **no fresh CI pass is claimed**. Only the completion
   report's recorded local BUILD/UNIT_TEST/LINT evidence exists, and it is read as
   historical, not current.
3. **Legacy retirement — no retirement.** Legacy notices, issue/release redirection
   and archival remain **separate decisions**; **no legacy repository was retired or
   deleted**, and the sibling histories remain the rollback path.
4. **Provider reliability and performance limitations — neither erased nor waived.**
   The recorded provider limitations in this completion report remain in force (see
   _Objective acceptance and remaining limitations_ above: CLEF/oMLX id-reliability,
   live both-unavailable not exercised, performance/size figures). This closure references
   them without erasure, softening or waiver.
5. **Go runtime validation — ran and passed.** `go build ./...`, `go test ./...` and
   `go vet ./...` ran and passed through SOP's validation gates; no additional runtime suites
   were run. Documentation validation also passed: the relative documentation link check
   (0 broken links) and `git diff --check` (PASS).

## Consolidation merge commit — COMMITTED (`a8082b6154dd88fd8ac9fd18af5df5e682a751a6`)

This is the single ancestry-preserving merge commit. Its **parents** preserve the sibling
histories; its **content** is the staged 259-path migration list — the `ours` strategy only
guarantees the merge never conflicts on content, it does **not** carry the imported content
(the content comes from staging). The merge is **committed** on `agentic-sop` `main` as
`a8082b6154dd88fd8ac9fd18af5df5e682a751a6`, with parents core
`54aa6a1a8f73a3b6debef0c54472351c2a817080`, controller
`f8502a500e52420ff0b4935b8abc0ebb5b218f2b` and adapter
`f319f60df470705db220d67875fdefbe4666e5da`.

The procedure below is retained as the historical record of how the merge was prepared and
performed (review only — the commit already exists; do NOT re-run).

```
# 0. Prereqs: index/working tree preserved; core main at 54aa6a1 (verify with git rev-parse).
B=/Users/imhttran/agentic-workspace/backups/sop-consolidation-20261014

# 1. Load the sibling tips as parents (no content change):
git fetch "$B/bundles/controller.bundle" 'refs/heads/main:refs/remotes/controller/main'
git fetch "$B/bundles/adapter.bundle"    'refs/heads/main:refs/remotes/adapter/main'

# 2. Set up the 3-parent merge without committing (tree still == core main):
git merge --no-commit -s ours --allow-unrelated-histories \
    refs/remotes/controller/main refs/remotes/adapter/main

# 3. Stage ONLY the explicit migration list (no blanket `git add -A`):
git add --pathspec-from-file="$B/commit-file-list.txt"

# 4. Create the single merge commit (3 parents + consolidated tree):
git commit -m "feat(consolidate): unify sop-controller and sop-decision-adapters into agentic-sop\n\nAncestry-preserving; parents include the sop-controller and sop-decision-adapters tips."

# 5. Verify ancestry for every imported tip:
git merge-base --is-ancestor refs/remotes/controller/main HEAD && echo controller-ancestor-ok
git merge-base --is-ancestor refs/remotes/adapter/main    HEAD && echo adapter-ancestor-ok

# Rollback (before pushing): git reset --hard <pre-merge-sha>
```

Notes: a dirty index is a stop-and-report boundary, never auto-stashed or reset. The adapter's
_uncommitted_ delta is already inside the staged content; the separate patch
(`patches/decision-adapters-delta.patch`) is the rollback copy for the sibling checkout.
