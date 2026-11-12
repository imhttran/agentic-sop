# LLM03 scanner integrity: execution-time evidence (OWASP gap 9)

**As of:** `main` after PR #410. **Scope:** the hash-then-exec race for optional
scanner binary pins (`internal/security/pinned.go`), and the platforms where the
race fix was run. The policy items (committed per-platform pins, publisher
signatures, required categories) are not decided here; they are in
[PROPOSAL-SCANNER-PIN-POLICY.md](../../plans/PROPOSAL-SCANNER-PIN-POLICY.md).

## The race and its closure

Before the fix, the pin was hashed and the original path was then executed, so a
writer could replace the file between the hash and the exec. The fix in
`pinnedExecutor.check` copies the resolved binary into a private directory while
hashing the same bytes, and executes that copy. A mismatch fails before any process
starts. The copy is removed after the run.

## Named test

`TestPinnedExecutor` (`internal/security/pinned_test.go`) covers the swap. A
recording executor rewrites the original file after the pin check. The test asserts
that the executed path is a private copy, that the copy's digest is the pinned
digest, and that the copy is removed after the run. A later one-byte replacement
fails with `binary pin mismatch` and runs nothing.

## Platform record

| Platform | Test run | Result | Notes |
| --- | --- | --- | --- |
| macOS arm64 (host) | `go test ./internal/security -run 'TestPinnedExecutor\|TestRulesPin' -count=1` | PASS | Host run for this evidence. |
| Linux arm64 (`golang:1.27` container) | same command, containerized | PASS | arm64, not amd64. |
| Linux amd64 | not run | NOT_RUN | No amd64 runner was used for this evidence. |
| Windows | not run | NOT_RUN | The test writes POSIX-mode executables and uses `/bin/sh` scripts. Windows execution of the copy path is not verified. |
| noexec temp mount | not measured | NOT_RUN | Documented shortcut: a binary on a noexec temp mount fails to start and reports `ErrUnavailable`. That fails closed; it is not a bypass. Not reproduced here. |

## Provider model provenance is outside SOP

Scanner-binary pins do not establish the provenance of a model. The model SOP sends
requests to is named by the operator and is not pinned by digest; SOP does not load
weights or receive an immutable model digest. Provider model provenance, publisher
authentication of scanners, and general DLP remain outside SOP. See
[llm04-supply-chain.md](llm04-supply-chain.md), "Model provenance".
