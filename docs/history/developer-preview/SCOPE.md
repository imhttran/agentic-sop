# Preview Scope and Freeze

> Revision note (operator-verified, 2026-10-09): this document corrects the
> PREVIEW-001 deliverable's Git statements. The original model-generated version
> is preserved unmodified in the PREVIEW-001 run artifacts
> (`.agent-sdlc/runs/PREVIEW-001/`). SOP history and the task's `LOCAL_DONE`
> status are **not** rewritten; this is a deliverable correction only.

## Preview checkout revision (the revision this preview runs on)

- Commit SHA: `d9f77223d4a996c4aca190b40d39a4bbf3f0deb2`
- Branch: `main` (local tip)
- **Relationship to remote:** this checkout is **ahead of `origin/main` by 3
  commits** and is **NOT** a pushed `main` revision. It is reachable on the remote
  only via `origin/harden-001` (which was branched from it).

## Pushed baseline (`origin/main`) — verified independently

- Commit SHA: `d181bbcb0a84fe342436c34b0a5f991054b011c6`
- Branch: `origin/main`
- Origin: `https://github.com/imhttran/agentic-sop.git`
- Verification: `git rev-parse origin/main` ⇒ `d181bbcb0a84fe342436c34b0a5f991054b011c6`.

## Deferred hardening checkpoint (verified independently)

- Branch: `harden-001` — **unmerged**; acceptance enforcement stays **default OFF**.
- Checkpoint SHA: `1ed7189a48dabfcc14735e9fb660b00c7ad4b855`
- Remote: `git rev-parse origin/harden-001` ⇒ `1ed7189a48dabfcc14735e9fb660b00c7ad4b855`.
- This checkpoint is **not** part of the preview baseline. The earlier statement
  that "no hardening branch or checkpoint exists" was incorrect and is retracted.

## Preview boundary

- The preview is bounded to the checkout at `d9f7722`. No component is claimed
  preview-ready without evidence.

## Acceptance notes (operator-verified)

- **Criterion 1** (baseline recorded exactly **and matches a pushed `main`
  commit**): **UNMET** — the checkout revision `d9f7722` is not pushed to
  `origin/main` (local `main` is ahead by 3). The pushed `origin/main` baseline is
  `d181bbc`. Recorded as a conflict, not weakened.
- **Criterion 2** (declare `harden-001` deferred with its checkpoint SHA): **MET**
  (above).
- **Criterion 3** (no component claimed preview-ready without evidence): **MET**.
