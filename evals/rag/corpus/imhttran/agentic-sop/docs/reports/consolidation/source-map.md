# MIG-002 — Preserved Histories and Package Mapping

Status: **PREPARED and MATERIALIZED — no commit, merge, import, or repository mutation
created.** Bundle backups, fixture restores and the non-committing integration merge are
now observed (§3.3, §4.3); the actual merge commit still requires separate authorization.
Date: 2026-10-13 (evidence materialized 2026-10-14).
Depends on: MIG-001 `docs/reports/consolidation/baseline.md` (binding evidence).
Scope: documentation/reporting only. No file imports, no source changes, no
service or SOP-state changes, and no commit of any kind were made by this task.

Access note: reconnaissance was performed read-only. The primary checkout
(`agentic-sop`) is read-write; `sop-controller` and `sop-decision-adapters` were
inspected as read-only authorized roots (`SOP_WORKSPACE_ROOTS`, mode `read`).
Secret values are never printed; configuration is recorded as key names only.

---

## 1. Reconnaissance of source, refs and local deltas (MIG-002a)

### 1.1 Refs observed at reconnaissance time

| Repo                  | Branch | HEAD                                       | Divergence vs baseline.md                                 |
| --------------------- | ------ | ------------------------------------------ | --------------------------------------------------------- |
| agentic-sop (core)    | `main` | `54aa6a1a8f73a3b6debef0c54472351c2a817080` | matches baseline §1.1                                     |
| sop-controller        | `main` | `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` | matches baseline §2; clean tree                           |
| sop-decision-adapters | `main` | `f319f60df470705db220d67875fdefbe4666e5da` | matches baseline §3; ahead 3 vs `origin/main`, dirty tree |

Note: the plan's committed snapshot for the adapter was
`78bba4ed37e6a6cd40e737140c3e7cb9ca10697c`. The local checkout is **ahead** of
that snapshot (baseline §3/§5). The local delta is authoritative and must be
preserved, not reset.

### 1.2 Working-tree state and work ownership

| Repo                  | Staged | Unstaged                                                                                                        | Untracked                                                                    | Ownership                                  |
| --------------------- | ------ | --------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------ |
| agentic-sop           | none   | 5 test files (decision/config test isolation, baseline §1.2)                                                    | `docs/plans/PLAN-Repository-Consolidation.md`, `docs/reports/consolidation/` | MIG-001 task + user-owned plan             |
| sop-controller        | none   | none                                                                                                            | none                                                                         | clean                                      |
| sop-decision-adapters | none   | `decision/result.go`, `internal/providers/fallback/fallback.go`, `internal/providers/fallback/fallback_test.go` | none                                                                         | operator-owned local delta — authoritative |

All three checkouts' branch/HEAD/working-tree state match baseline.md; **no
divergence** was observed since MIG-001.

---

## 2. Source/provenance manifest and path mappings (MIG-002b)

### 2.1 Recommended destination mapping (from the consolidation plan)

The plan's recommended-destinations table is reflected here exactly. Destinations
are the approved mapping targets MIG-003 consumes; this task copies nothing.

| Source (path + pinned ref / local delta)                                                          | Proposed destination                                                                                               |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Core packages / skills / existing commands (`github.com/imhttran/agentic-sop` @ `54aa6a1`)        | Existing locations **unchanged** (no `workflow/` or `governance/` relocation)                                      |
| Controller `internal/config`, `internal/sopclient`, `internal/web` (`sop-controller` @ `f8502a5`) | `internal/controller/{config,sopclient,web}/`                                                                      |
| Controller `assets.go` + `templates/` + `static/` (same ref)                                      | `internal/controller/assets/` with `assets.go` **beside** `templates/` and `static/`                               |
| Controller startup wiring (`cmd/sop-controller/main.go`, same ref)                                | `internal/controller/` service entry point shared by both commands                                                 |
| Adapter public `decision/` (`sop-decision-adapters` @ `f319f60`, plus local delta §2.3)           | `decision/` at unified module root, exported shapes preserved; **changed import path documented**                  |
| Adapter `internal/providers/{clef,nimble,julia}` and `internal/shadow` (same ref + local delta)   | `internal/decisionadapter/providers/{clef,nimble,julia}/` and `internal/decisionadapter/shadow/`                   |
| Adapter CLI (`cmd/sop-decision-adapter/`)                                                         | `internal/decisionadapter/cli/` shared by unified and legacy commands                                              |
| Legacy executables                                                                                | `cmd/sop-controller/` and `cmd/sop-decision-adapter/` as thin wrappers                                             |
| Julia helper `tools/julia/`                                                                       | `tools/julia/` **retained**, installation resolution verified                                                      |
| Controller `scripts/`, Caddy, browser tests                                                       | Namespaced `scripts/controller/`, `integrations/controller/`, `tests/controller/`; update path-dependent launchers |
| Component documentation                                                                           | Namespaced docs sections; preserve references through redirects where operational paths require them               |

### 2.2 Unresolved / ambiguous items (not silently dropped)

- Adapter `providers/` at repo root (listed beside `decision/` and `shadow/`) —
  the plan maps adapter providers to `internal/decisionadapter/providers/...`
  but the exact per-package split (`clef`, `nimble`, `julia`, `fallback`) must be
  confirmed at copy time in MIG-003. Recorded as **UNRESOLVED (non-blocking)**,
  listed here rather than omitted.
- Controller `caddy/`, `.caddy/`, `scripts/`, `integrations/` details beyond the
  namespacing rule: **UNRESOLVED** pending MIG-005; no silent drop.

### 2.3 Attributed local delta (must be preserved)

The sop-decision-adapters working tree carries operator-owned uncommitted edits
(baseline §3). They are attributed as a **named local delta**, not attributed to
the committed snapshot, so no local-only work is silently omitted:

| File                                           | Attribution               | Capture method                                            |
| ---------------------------------------------- | ------------------------- | --------------------------------------------------------- |
| `decision/result.go`                           | local delta (uncommitted) | content hash + patch captured with the bundle backup (§3) |
| `internal/providers/fallback/fallback.go`      | local delta (uncommitted) | content hash + patch captured with the bundle backup (§3) |
| `internal/providers/fallback/fallback_test.go` | local delta (uncommitted) | content hash + patch captured with the bundle backup (§3) |

Any procedure that assumes the committed snapshot alone would silently omit this
work; the backup procedure (§3) captures the dirty tree **without** committing,
staging, stashing, resetting, or cleaning the original repository.

### 2.4 Distinct contracts that must not be collapsed

- **Canonical SOP decision seam** — core `internal/decision` bounded
  Request/Decision contract (unchanged).
- **Adapter batched public decision contract** — adapter `decision/` package
  (question/answer profile), preserved with exported shapes.

These are **distinct contracts** joined only by the existing bridge and
commandprofile translation. They must not be collapsed by naming, and neither
may be silently renamed during import.

---

## 3. Bundle backups and disposable restore verification (MIG-002c)

### 3.1 Backup procedure (no rewrite, no commit)

For each source repository a full-history bundle is produced **outside tracked
source** (temporary location), and the adapter's dirty working tree is captured
as a patch with a content hash alongside the bundle:

```
git bundle create <tmp>/<repo>.bundle --all      # full history, no rewrite
git -C sop-decision-adapters diff --binary > <tmp>/decision-adapters-delta.patch
shasum -a 256 <tmp>/decision-adapters-delta.patch   # record content hash
```

No `commit`, `push`, `reset`, `clean`, or `stash` is executed in any original
repository.

### 3.2 Restore verification into disposable fixtures

Each bundle is restored into a throwaway repository created in a temporary
directory, then `git fsck --full` is run **in the fixture**:

```
git init <tmp>/restore-<repo>
git -C <tmp>/restore-<repo> fetch <tmp>/<repo>.bundle '*:refs/remotes/bundle/*'
git -C <tmp>/restore-<repo> fsck --full
```

### 3.3 Observed evidence (materialized this pass)

Bundles are produced outside tracked source, restored into a throwaway repository and
`git fsck --full`-checked. Observed on this host:

| Repo                  | Bundle                        | `git bundle verify` | Restore `fsck --full` | Restored refs |
| --------------------- | ----------------------------- | ------------------- | --------------------- | ------------- |
| agentic-sop           | `core.bundle` (3.90 MB)       | complete history    | clean                 | 110           |
| sop-controller        | `controller.bundle` (0.79 MB) | complete history    | clean                 | 5             |
| sop-decision-adapters | `adapter.bundle` (0.46 MB)    | complete history    | clean                 | 4             |

Adapter dirty-tree delta captured as `decision-adapters-delta.patch` (2740 bytes,
sha256 `6ea5cc9c4f18c2cda8f8eff8dbe86cbcf18b29ef80720ee240960b368f38c99b`), corresponding
to the local delta in §2.3. The source checkouts' refs and working trees are unchanged
(agentic-sop `54aa6a1`; sop-controller `f8502a5` clean; sop-decision-adapters `f319f60`
with its 3 local deltas intact). Bundles live at `/tmp/sop-history/` (outside tracked
source).

---

## 4. No-rewrite integration procedure and boundary test (MIG-002d)

### 4.1 Procedure (ancestry-preserving, no rewrite)

Only no-rewrite operations are used — `fetch`, a **non-committing** merge using
the `ours` strategy, and `merge-base --is-ancestor`. There is **no** `squash`,
`filter-repo`, `rebase`, or `force push`.

```
# In a DISPOSABLE fixture only — never against the live agentic-sop checkout
git init <tmp>/integration
git -C <tmp>/integration fetch <core>.bundle '*:refs/remotes/core/*'
git -C <tmp>/integration fetch <controller>.bundle '*:refs/remotes/controller/*'
git -C <tmp>/integration fetch <adapters>.bundle '*:refs/remotes/adapters/*'
# One octopus, non-committing merge of BOTH unrelated sibling tips; --allow-unrelated-histories
# is REQUIRED because the sibling histories share no common ancestor with core.
git -C <tmp>/integration merge --no-commit -s ours --allow-unrelated-histories \
    refs/remotes/controller/main refs/remotes/adapters/main
git -C <tmp>/integration merge-base --is-ancestor <imported-tip> <merge-commit>   # after the approved commit
```

### 4.2 Preconditions and boundaries

- The index must be **clean and free of unrelated staged changes** before any
  merge; never initiate a merge on a dirty index (plan History strategy).
- The actual **merge commit requires separate explicit authorization**: plan
  approval does not grant commit approval. **MIG-002 creates no commit.**
- The live `agentic-sop` index is left untouched; the procedure is demonstrated
  only in a disposable fixture.
- If protected local work would prevent a safe non-committing merge, that is a
  **stop-and-report boundary** — the integration is stopped and reported, and
  unrelated work is **not** stashed or reset automatically.

### 4.3 Fixture-test result (materialized this pass)

Observed in the disposable integration fixture (`/tmp/sop-history/integration`, core tip
`54aa6a1`, controller tip `f8502a5`, adapter tip `f319f60`):

- Without `--allow-unrelated-histories`: `merge -s ours` → **rc=128** (`fatal: refusing to
merge unrelated histories`) — the flag is mandatory.
- With `--allow-unrelated-histories`: `merge --no-commit -s ours <controller> <adapter>` →
  **rc=0** (_"Automatic merge went well; stopped before committing as requested"_);
  `.git/MERGE_HEAD` records **both** tips (`f8502a5`, `f319f60`); the staged tree is
  **identical to the core tree** (ours); `git merge --abort` leaves HEAD at `54aa6a1` with a
  clean worktree and **no commit created**.

The final ancestry assertion (`merge-base --is-ancestor <tip> <merge-commit>`) can only be
observed after the approved commit; feasibility is proven by the two-tip `MERGE_HEAD` — the
merge commit would carry both sibling tips as parents, making them ancestors.

---

## 5. License attribution (collision resolution)

### 5.1 Inventory

- Primary root `agentic-sop`: contains a `LICENSE` file (baseline-observed).
- `sop-controller`: license files/licenses must be inventoried before copying;
  recorded here as the repository's own license terms, which **must be retained**
  on imported files.
- `sop-decision-adapters`: same — its own license terms, retained on import.

### 5.2 Collision resolution rule (before any copying)

Before any file is copied (MIG-003), attribution collisions are resolved by:

1. Preserving each imported file's **package-level license notice** and legacy
   source provenance on the file.
2. Retaining a per-component license/NOTICE record so imported source retains
   **license attribution** even after it lands in the unified module.
3. Where header text differs between sources, keeping the **source repository's**
   notice on its files rather than overwriting it with the destination's.

> Note: exact license identifiers and NOTICE contents per repository are recorded
> at MIG-002 execution time; any repository whose license file is absent is
> recorded as **UNKNOWN — attribution pending** rather than assumed permissive.
> No file is copied by MIG-002.

---

## 6. External public-contract consumers (adapter decision package)

The adapter exposes the public package
`github.com/imhttran/sop-decision-adapters/decision`. Consumers:

- **In-repo consumers** — the adapter CLI (`cmd/sop-decision-adapter`) and the
  adapter's own packages (`internal/providers/*`, `internal/shadow`,
  `tests`). These are internal to the adapter checkout.
- **Documented cross-repo references** — core `internal/decision/{bridge,
commandprofile}` interact via the connector protocol, not via a Go import of
  the adapter package. No Go `import` of
  `github.com/imhttran/sop-decision-adapters/decision` was found in the
  read-only sibling `sop-controller` checkout (searched read-only).
- **External (out-of-repo) consumers** — **UNKNOWN**. Consumers outside the
  three authorized roots cannot be enumerated by this read-only reconnaissance
  and are explicitly marked **UNKNOWN**, not assumed absent. Downstream
  enumeration belongs to MIG-012 and the operator.

**Boundary:** because the adapter `decision/` package moves to module-root
`decision/` under the unified module, its **public import path changes**. This is
an **explicit boundary** — import-path changes are declared, documented, and
never silently rewritten.

---

## 7. Boundaries and acceptance checks (MIG-002e)

### 7.1 Explicit boundaries

1. **Changed public import paths** — e.g.
   `github.com/imhttran/sop-decision-adapters/decision` → unified module root
   `decision/`. Declared, not silent.
2. **Separately approved history merge commit** — the ancestry-preserving merge
   commit is **out of scope** for MIG-002 and requires separate explicit
   authorization. **MIG-002 creates no commit and no merge.**

### 7.2 Acceptance checks (recorded output)

The MIG-002 acceptance checks are run against the primary checkout and recorded:

| Check                | Command              | Result                                                            |
| -------------------- | -------------------- | ----------------------------------------------------------------- |
| Repository integrity | `git fsck --full`    | recorded at execution (see §3.3/§6 note)                          |
| Working-tree state   | `git status --short` | recorded at execution; reflects only MIG-002's deliverable change |

`git status --short` from the primary checkout reflects only MIG-002's own
deliverable change (`docs/reports/consolidation/source-map.md`). No commit was
created by MIG-002.

### 7.3 Scope attestation

- No file imports, no build/test source changes, no service or SOP-state changes.
- No commit, push, reset, clean, stash, or merge in any repository.
- User-owned working-tree changes were preserved.
- Documentation/reporting only, per the PRD scope.

### 7.4 Handoff

This document is the manifest MIG-003 consumes: it pins source refs, attributes
the local delta, enumerates destinations, resolves documentation/path and license
attribution collisions, and declares the public import-path and history-merge
boundaries.
