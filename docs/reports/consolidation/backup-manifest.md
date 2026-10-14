# SOP repository-consolidation — durable backup manifest

Location: `/Users/imhttran/agentic-workspace/backups/sop-consolidation-20261014` (outside all
three repositories and outside `/tmp`; directory mode `700`, `state/` files mode `600`).
Refreshed 2026-10-14 **after the final edits** (v2 snapshots; the earlier v1 snapshot is
preserved). Contains **no secrets** (no `.env`, tokens, keys or credentials).

Source tips pinned (verified unchanged at refresh time):

| Repo       | Path                             | Tip                                        |
| ---------- | -------------------------------- | ------------------------------------------ |
| core       | `agentic-sop`                    | `54aa6a1a8f73a3b6debef0c54472351c2a817080` |
| controller | `projects/sop-controller`        | `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` |
| adapter    | `projects/sop-decision-adapters` | `f319f60df470705db220d67875fdefbe4666e5da` |

Local-only (unpushed) commits: **adapter** has 3 (`057604e`, `e3e4787`, `f319f60`); core and
controller have none. `--all` bundles include every local ref.

## Contents and hashes (sha256)

### `bundles/` — full history, one per repo (`git bundle create … --all`)

| File                | sha256                                                             |
| ------------------- | ------------------------------------------------------------------ |
| `core.bundle`       | `38089debc6de6152cb3634bc3ead5b61d692886d172d3d14787865dfe0b4219e` |
| `controller.bundle` | `de33fdbcb2e7149f5147c4c24464dbb8321405a1d1f3001330fe08d5397f3b35` |
| `adapter.bundle`    | `536715c342fcdf420c24993c18f9d9dab0e8403bf4d59a61240df38c2a9bbee9` |

### `patches/` — uncommitted work (NOT in any bundle)

| File                                                   | sha256                                                             | Notes                                                                        |
| ------------------------------------------------------ | ------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| `decision-adapters-delta.patch`                        | `6ea5cc9c4f18c2cda8f8eff8dbe86cbcf18b29ef80720ee240960b368f38c99b` | adapter working-tree delta (3 files); rollback copy for the sibling checkout |
| `agentic-sop-uncommitted-tracked.v2.patch`             | `c1aeaf28641b54449cd2958f238d43e73a7d5882fc730f6b690823b1f1978a1b` | `git diff HEAD --binary` (current)                                           |
| `agentic-sop-untracked.v2.tar.gz`                      | `198a25100b0cafba71e0759c06ab73b7de4b54a23c56802dc0c667c87de70d99` | 235 untracked source/doc files                                               |
| _(superseded)_ `agentic-sop-uncommitted-tracked.patch` | `c1aeaf28…` (identical)                                            | v1 earlier snapshot                                                          |
| _(superseded)_ `agentic-sop-untracked.tar.gz`          | `fc140e93de63760183dc4c2bfcc93ee8551f3d090d9bb0698171c06876632504` | v1 earlier snapshot                                                          |

### `commit-file-list.txt` — the explicit 259-path migration scope

| File                   | sha256                                                                                                       |
| ---------------------- | ------------------------------------------------------------------------------------------------------------ |
| `commit-file-list.txt` | `6ff7e72286c044061ffe11e22718308daa947bc10b0503373f1fa3a36df7e18c` (259 paths = 23 modified + 236 untracked) |

### `state/` — SQLite state backup (schema v4) + recorded plan/config

| File                | sha256                                                             |
| ------------------- | ------------------------------------------------------------------ |
| `state.v4.db`       | `52390c72e0e5ab2a4fb9cf519d20cb2e6da434a3579dcf9ac0ff02aa9bf2ef0d` |
| `plan.v4.json`      | `5e0dad1f033418c0b16546940b2f39153e5c835227fed1e460ea51302d19b5d7` |
| `plan.meta.v4.json` | `985e046f50f9a220f0ea33b48e34e1d3e34deddcd57e7d45b05172d621bae4a9` |
| `config.v4.yaml`    | `9e77f4b2ba94239e6d902aa6c6b8d388617a83db57f8f443b566f49cd3f5a7b0` |

## Verification performed

- Copy hash-equality: every `bundles/`, `patches/`, `state/` file matches its `/tmp` origin.
- Bundle integrity: `git bundle verify` = "complete history" for all three; a fresh
  `git init` + `fetch` passes `git fsck --full` (restored refs: core 110, controller 5,
  adapter 4).
- SQLite: `PRAGMA integrity_check` = `ok`, `PRAGMA user_version` = `4`.
- Archive completeness: the 235 archived paths **plus** this manifest (excluded from the
  archive, see below) equal the 236-path untracked set exactly.

## Restore procedure

```sh
B=/Users/imhttran/agentic-workspace/backups/sop-consolidation-20261014

# Full history of any repo into a throwaway clone:
git init /tmp/restore-core
git -C /tmp/restore-core fetch "$B/bundles/core.bundle" '*:refs/remotes/bundle/*'
git -C /tmp/restore-core fsck --full

# Uncommitted consolidation work (v2 is current), on top of core tip 54aa6a1:
git -C <agentic-sop-checkout> apply "$B/patches/agentic-sop-uncommitted-tracked.v2.patch"
tar -xzf "$B/patches/agentic-sop-untracked.v2.tar.gz" -C <agentic-sop-checkout>
# restore this manifest too (it is excluded from the archive):
cp "$B/MANIFEST.md" <agentic-sop-checkout>/docs/reports/consolidation/backup-manifest.md

# Adapter working-tree delta (on top of adapter tip f319f60):
git -C <sop-decision-adapters-checkout> apply "$B/patches/decision-adapters-delta.patch"

# SQLite state (only if the live state must be rolled back — operator action):
cp "$B/state/state.v4.db" <agentic-sop-checkout>/.agent-sdlc/state.db
```

## Important limitations

- **Git bundles contain only committed history.** Uncommitted work (the 23 core tracked
  changes and the 3 adapter deltas) is captured separately under `patches/`; a bundle restore
  alone does **not** recreate the working tree.
- This manifest (`docs/reports/consolidation/backup-manifest.md`) is **excluded from the
  untracked archive** to keep the recorded archive hash stable and self-consistent; its canonical
  copy is this `MANIFEST.md`. Restore it as shown above.
- No binaries, runtime state, or service data are included. No secrets. Backups are outside the
  repositories and `/tmp`.
- Read-only recovery snapshot: restoring it does not itself commit, push, merge, retire a
  repository or restart a service.
