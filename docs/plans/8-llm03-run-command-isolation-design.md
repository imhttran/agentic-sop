You are working in the agentic-sop Go repo at /Users/imhttran/agentic-workspace/agentic-sop on macOS (arm64).

Ground rules:
- Start from a fresh branch: `git fetch origin && git switch -c BRANCH origin/main`. Don't touch `main` or any other branch, and leave unrelated files alone.
- Smallest change that works. Reuse helpers that already exist, add no dependency, and build nothing that wasn't asked for.
- Tests in `internal/cli` must not hardcode `/bin/true`; resolve it with `exec.LookPath`.
- Evidence docs under `docs/reports/owasp-llm/` are an evidence matrix, not a compliance claim. Keep `IMPLEMENTED` / `VERIFIED` / `NOT_RUN` / `GAP` honest, never mark something verified that you didn't observe, and quote the exact commands and results you ran.
- Don't commit, push, or open a PR. When you finish, give me a short summary: files changed, commands run with pass/fail, and anything you couldn't verify.

Task:
BRANCH = security/llm03-run-command-isolation-design

Design only. No production code in this task.

Normal agent execution is policy-only and not OS-sandboxed (row 1 of `docs/reports/security-shift-left/execution-path-matrix.md`, Plan 01 row of `docs/reports/owasp-llm/README.md`). The OS sandboxes already exist: `internal/evaltarget/isolation_darwin.go` (Seatbelt, deny-by-default profile, root writable, network denied) and `isolation_linux.go` (bwrap), plus `internal/dqharness/replay_exec.go`. The in-process agent runs every `run_command` through `internal/toolharness/harness.go` `execCaptured`. Write a proposal for an opt-in flag that runs those commands inside the existing sandbox.

Write `docs/plans/PROPOSAL-RUN-COMMAND-ISOLATION.md` covering:
1. Where the shared profile builder should live. `evaltarget` documents that `IsolateLiveCommand` "is never used by normal agent execution", so check layering and import cycles before proposing that `toolharness` import it. Prefer moving or reusing over a second profile.
2. What a real `go test ./...` / `cargo test` needs that the eval profile denies: Go build and module caches (`go env GOCACHE GOMODCACHE`), `HOME` reads, Cargo's `~/.cargo`, temp dirs, and network for a module download. Verify each on this Mac by running a command under a draft profile with `sandbox-exec -p`, and record what failed and why. Don't guess.
3. Writes to `.agent-sdlc/` denied inside the repo root (in SBPL a later `deny file-write*` subpath rule overrides the earlier allow; verify that on this Mac), and how that interacts with the state-write detection task (`security/llm07-state-write-detection`) if it lands first.
4. The flag name, the default (off), failure behavior when no backend is available (fail closed when the flag is on), and what the audit trail records.
5. Options with a recommendation, the tests that would prove it (reuse the fixtures in `internal/evaltarget` and `seatbelt-evidence.md`), and what stays unclaimed: Linux runtime (no bwrap here), Windows, and the command-agent path (`SOP_AGENT_COMMAND`) that SOP doesn't launch in-process.

Don't change the matrix or README statuses: nothing is implemented. Include the exact `sandbox-exec` experiments you ran and their results.
