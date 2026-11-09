You are working in the agentic-sop Go repo at /Users/imhttran/agentic-workspace/agentic-sop on macOS (arm64).

Ground rules:
- Start from a fresh branch: `git fetch origin && git switch -c BRANCH origin/main`. Don't touch `main` or any other branch, and leave unrelated files alone.
- Smallest change that works. Reuse helpers that already exist, add no dependency, and build nothing that wasn't asked for.
- Tests in `internal/cli` must not hardcode `/bin/true`; resolve it with `exec.LookPath`.
- Evidence docs under `docs/reports/owasp-llm/` are an evidence matrix, not a compliance claim. Keep `IMPLEMENTED` / `VERIFIED` / `NOT_RUN` / `GAP` honest, never mark something verified that you didn't observe, and quote the exact commands and results you ran.
- Don't commit, push, or open a PR. When you finish, give me a short summary: files changed, commands run with pass/fail, and anything you couldn't verify.

Task:
BRANCH = security/llm06-scanner-process-group

Close residual limitation 5 in `docs/reports/owasp-llm/llm06-consumption.md` §6 ("Security adapter descendants are not claimed"). `internal/security/exec.go` `execRunner.run` uses a plain `exec.CommandContext`. On timeout or cancellation Go kills only the direct child, so a scanner's own children (Semgrep spawns workers) can outlive the scan.

1. Reproduce first. Add a test in `internal/security` modeled on `internal/toolharness/proc_timeout_test.go` (`TestExecTimeoutKillsProcessGroup`, `TestExecTimeoutKillsSigtermIgnoringDescendant`): a script that starts a background descendant that ignores SIGTERM, records its PID, and sleeps; run it through `execRunner{}.Run` with a short deadline; assert the descendant is gone afterward. Confirm the test FAILS on current code before changing anything. Resolve `sh`/`sleep` with `exec.LookPath`, and skip on Windows.
2. Fix it the way the repo already does it. `internal/toolharness`, `internal/agent` and `internal/evaltarget` each carry a private `proc_unix.go` / `proc_other.go` pair (`sysProcAttrKill`, `terminateGroup`). Add the same pair to `internal/security`, then in `run()` set `cmd.SysProcAttr = sysProcAttrKill()` and a `cmd.Cancel` that calls `terminateGroup(cmd.Process.Pid)` and falls back to `cmd.Process.Kill()`, exactly as `internal/toolharness/harness.go` `execCaptured` does. Keep the existing `WaitDelay`, output bounds, env allowlist and error mapping unchanged.
3. Don't consolidate the four copies into a shared package in this task. Mention it in your summary as a follow-up.
4. Run `go test ./internal/security/ -run 'ProcessGroup|Descendant' -v -count=1`, then `go test ./internal/security/... -count=1`, `go vet ./...`, `gofmt -l .`. All must pass.
5. Update `llm06-consumption.md`: rewrite residual 5 to say scanner timeout/cancellation now kills the process group on unix (name the new test), and that `proc_other.go` still kills only the direct child. Add a row to the §3a reconciliation table. Don't touch other rows.
