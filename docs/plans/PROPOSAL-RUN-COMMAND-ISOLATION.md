# Proposal: opt-in OS sandbox for `run_command`

Status: **PROPOSAL — design only, nothing implemented.** No production code, no
matrix row, and no README status changes are part of this task. Every
`IMPLEMENTED` / `VERIFIED` / `NOT_RUN` / `GAP` label in
[`../reports/owasp-llm/README.md`](../reports/owasp-llm/README.md) and
[`../reports/security-shift-left/execution-path-matrix.md`](../reports/security-shift-left/execution-path-matrix.md)
stays as it is today.

Related: `8-llm03-run-command-isolation-design.md`, the plan this proposal
answers (an agent prompt, since removed from the repository).

## Environment

Recorded at branch time for the proposal's environment section.

| Field       | Value                                                               |
| ----------- | ------------------------------------------------------------------- |
| Branch      | `security/llm03-run-command-isolation-design`                       |
| Base        | `origin/main`                                                       |
| Base commit | `f2cdffb0d013a4382b71f0c830bfe48fbd7a2210`                          |
| HEAD commit | `f2cdffb0d013a4382b71f0c830bfe48fbd7a2210` (equal to `origin/main`) |
| Host        | Darwin (macOS, arm64)                                               |

`git status --short --branch` at branch time:

```text
## security/llm03-run-command-isolation-design...origin/main
?? docs/plans/8-llm03-run-command-isolation-design.md
```

The untracked plan file `docs/plans/8-llm03-run-command-isolation-design.md` is
a pre-existing user-owned change. It was left in place: not reverted, not staged,
not committed. `main` and every other branch were not touched.

## Problem

Normal agent execution is policy-only. The in-process agent runs every
`run_command` through `internal/toolharness/harness.go` `execCaptured`, which
uses `exec.CommandContext` with no OS sandbox (matrix row 1, Plan 01 row). The OS
sandboxes exist, but only in the evaluation paths:

- `internal/evaltarget/isolation_darwin.go` — Seatbelt, deny-by-default SBPL,
  case root writable, `(deny network*)`.
- `internal/evaltarget/isolation_linux.go` — `bwrap` (not available on this host).
- `internal/dqharness/replay_exec.go` — isolated replay (Seatbelt / bubblewrap).

`evaltarget` documents that `IsolateLiveCommand` is _never used by normal agent
execution_. This proposal adds an opt-in flag that routes `run_command` through
the same sandbox family, without a second hand-written profile.

## 1. Where the profile builder lives

This section records the import-graph measurement the plan asks for. It replaces
the earlier `GAP — import graph not traced` note with observed output and one
recommendation.

### The existing profile builders (exact files and functions)

The repository already has two hand-written OS-isolation profile builders. No
third profile should be written.

| Backend             | File                                      | Function                                                         | What it builds                                                                                                                        |
| ------------------- | ----------------------------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Darwin entry point  | `internal/evaltarget/isolation.go`        | `isolationCommand(ctx, root, dir, argv, env)`                    | Calls `darwinIsolationCommand` with the fixed rule `(deny network*)`.                                                                 |
| Darwin profile      | `internal/evaltarget/isolation_darwin.go` | `darwinIsolationCommand(ctx, root, dir, argv, env, networkRule)` | Builds the deny-by-default SBPL string, resolves the executable's read paths, and returns an `*exec.Cmd` for `/usr/bin/sandbox-exec`. |
| Darwin live variant | `internal/evaltarget/isolation_darwin.go` | `liveIsolationCommand(ctx, root, dir, argv, env)`                | Same builder with one loopback network rule from `liveNetworkRule()`.                                                                 |
| Linux               | `internal/evaltarget/isolation_linux.go`  | `isolationCommand(ctx, root, dir, argv, env)`                    | Bubblewrap argument list (`--unshare-net`, read-only system binds, writable case root).                                               |
| Other platforms     | `internal/evaltarget/isolation_other.go`  | `isolationCommand` / `isolationAvailable`                        | Returns `ErrIsolationUnavailable`; no sandbox.                                                                                        |
| Replay dispatch     | `internal/dqharness/replay_exec.go`       | `isolationCommand(backend, dir, target)`                         | Picks a backend; returns `(args, env, err)`.                                                                                          |
| Replay Seatbelt     | `internal/dqharness/replay_exec.go`       | `seatbeltProfile(dir)` and `seatbeltCommand(dir, target)`        | Allow-default profile that denies network and writes outside the replay dir; `seatbeltCommand` wraps it as `-p <profile>`.            |
| Replay bubblewrap   | `internal/dqharness/replay_exec.go`       | `bubblewrapCommand(dir, target)`                                 | Canonicalizes the replay dir and binds it writable at `/replay`; returns `(bound, args, err)`.                                        |

Symbol-name check: the cited `seatbeltCommand` and `bubblewrapCommand` exist in
`internal/dqharness/replay_exec.go` with exactly those names. No discrepancy. The
`isolationCommand` name appears in both `evaltarget` and `dqharness`; they are
different symbols in different packages, and `dqharness`'s returns an argument
list while `evaltarget`'s returns an `*exec.Cmd`.

Two builders build two profiles. `evaltarget`'s is deny-by-default and
network-denied; `dqharness`'s is allow-default with a network and write deny.
They are not the same policy. Any shared package must keep both policies as
parameters, not collapse them.

### Emitted `go list -deps` output

Run from the repository root on this host. Both commands exited `0`.

`go list -deps ./internal/toolharness` — exit status `0`. The module-local
packages in the output were, in order:

```text
github.com/imhttran/agentic-sop/internal/commandpolicy
github.com/imhttran/agentic-sop/internal/git
github.com/imhttran/agentic-sop/internal/toolharness
```

The rest of the output is the Go standard library and its `internal/*` runtime
packages (`runtime`, `sync`, `os`, `os/exec`, `encoding/json`, `path/filepath`,
`crypto/*`, and so on). `internal/toolharness` does not appear in its own
dependency set as a foreign package, and it pulls no `internal/evaltarget`,
`internal/dqharness`, `internal/cli`, or `internal/state`.

`go list -deps ./internal/evaltarget` — exit status `0`. The module-local
packages in the output were, in order:

```text
github.com/imhttran/agentic-sop/internal/domain
github.com/imhttran/agentic-sop/internal/redact
github.com/imhttran/agentic-sop/internal/handoff
github.com/imhttran/agentic-sop/internal/store
github.com/imhttran/agentic-sop/internal/approval
github.com/imhttran/agentic-sop/internal/commandpolicy
github.com/imhttran/agentic-sop/internal/evaltarget
```

The rest of the output is standard library plus third-party modules
(`modernc.org/sqlite`, `modernc.org/libc`, `github.com/google/uuid`,
`github.com/mattn/go-isatty`, and others).

### Does `evaltarget` pull heavy packages? Observed: yes.

The quoted output above contains:

- `internal/store` — the SQLite-backed state layer (it is why `modernc.org/sqlite`
  and `modernc.org/libc` appear).
- `internal/approval` — the approval gate.
- `internal/handoff` — the handoff records.
- `internal/domain` — the domain model.
- `internal/commandpolicy` — the command policy the tool seam already uses.

`internal/cli` and `internal/state` do **not** appear. The heavy packages that do
appear are `internal/store` (which drags the SQLite driver and `modernc.org/libc`)
and `internal/approval`. So a `toolharness -> evaltarget` import would not form a
cycle, but it would drag the SQLite state layer and the approval gate into the
`run_command` seam.

### Is `toolharness -> evaltarget` cycle-free? Observed: the current graph is cycle-free.

The two dependency lists are the observed evidence. `internal/toolharness` depends
on `internal/commandpolicy` and `internal/git`. `internal/evaltarget` depends on
`internal/domain`, `internal/redact`, `internal/handoff`, `internal/store`,
`internal/approval`, `internal/commandpolicy`. Neither list contains the other
package, and `internal/evaltarget`'s list does not contain `internal/toolharness`.
Therefore adding `internal/toolharness -> internal/evaltarget` would not close a
cycle in the graph as observed. The observed output shows no reverse edge.

This is the observed state, not an assumption. It is also not the whole argument:
a cycle-free edge can still be a bad edge, because of the heavy packages listed
above.

### Options

| Option                                                                                                      | Cost                                                                                                                      | Risk                                                                                                                                                                                                     | Reuse                                                                |
| ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| (a) `toolharness` imports `evaltarget`                                                                      | Low: one import in `harness.go`, call `evaltarget.IsolateCommand`.                                                        | Accepted by the compiler (cycle-free per the output above), but drags `internal/store` (SQLite, `modernc.org/libc`) and `internal/approval` into the tool seam. Ties `run_command` to evaluation policy. | Reuses the builder as-is; reuses the wrong boundary.                 |
| (b) Extract one low-dependency profile package that `evaltarget`, `dqharness`, and `toolharness` all import | Medium: move the SBPL and bubblewrap builders; both existing call sites delegate; existing tests stay the regression net. | Low: one builder, two existing consumers keep their profiles as parameters, and the new package imports only stdlib.                                                                                     | Reuses one profile; moves it to where a tool seam can reach it.      |
| (c) A second profile written inside `toolharness`                                                           | Low to write.                                                                                                             | High: two profiles that drift; the `evaltarget` and `dqharness` tests no longer cover the profile the tool seam executes; the `.agent-sdlc` deny rule and the cache allow list must be maintained twice. | No reuse. Adds a third profile to a repository that already has two. |
| **Recommendation: option (b).**                                                                             |                                                                                                                           |                                                                                                                                                                                                          |                                                                      |

### Recommendation

Choose **option (b)**: extract the existing builders into one low-dependency
package, and let `evaltarget`, `dqharness`, and `toolharness` all call it.

Reasons, from the observed output:

1. `evaltarget`'s dependency set contains `internal/store` (SQLite) and
   `internal/approval`. Option (a) would put the SQLite state layer and the
   approval gate in the process path of every `run_command`, even when the flag is
   `off`. The observed output quoted above is the concrete reason to reject (a).
2. The profile text and the `.agent-sdlc` deny rule (see §3) must be proven once by
   one test set. Option (c) splits the proof: `evaltarget` and `dqharness` tests
   would no longer exercise the policy that `run_command` executes. Option (b)
   keeps one builder and one regression net.
3. The builders already differ in policy (deny-default in `evaltarget`,
   allow-default with denies in `dqharness`). The extracted package therefore takes
   the policy as a parameter and keeps both profiles byte-identical to today's.
   Step 1 of §6 proves this by string equality against the current
   `evaltarget` tests.

Neither a cycle nor a new sandbox blocks option (b), so no blocking reason is
needed.

## 2. What a real `go test ./...` / `cargo test` needs that the eval profile denies

The current profile denies by default and allows reads only under a fixed list
(`/usr`, `/System`, `/Library`, `/bin`, `/dev`, `/etc`, `/private/etc`,
`/private/tmp`, `/private/var/folders`, `/opt/homebrew`, the executable's own
ancestors) and writes only under `root`, `/private/tmp`, `/private/var/folders`.

Therefore it denies, by construction:

| Need                                                  | Today                      | Why it matters                                                               |
| ----------------------------------------------------- | -------------------------- | ---------------------------------------------------------------------------- |
| `HOME` (e.g. `~/.config/go/env`, `~/.cache/go-build`) | denied                     | Go and Cargo read config/caches under `$HOME`.                               |
| `GOCACHE` (usually `~/Library/Caches/go-build`)       | denied                     | Build cache; without it every build is cold (`-a`), and some tooling errors. |
| `GOMODCACHE` (usually `~/go/pkg/mod`)                 | denied                     | Module sources; a build fails if deps are not vendored.                      |
| Cargo `~/.cargo` (registry, git db)                   | denied                     | `cargo test` cannot resolve crates.                                          |
| `~/.rustup` toolchain                                 | denied                     | `cargo` is a rustup shim; without the toolchain it exits early.              |
| Temp dirs outside `/private/tmp`                      | denied                     | `TMPDIR` may point elsewhere; Go writes temp files.                          |
| Network for module download                           | denied (`(deny network*)`) | A first `go mod download` needs egress.                                      |

### Runtime status — measured on this host

SOP's `run_command` seam refused these probes when the agent tried them (the
refusals are recorded in [**What the eval profile denies**](#what-the-eval-profile-denies)).
The operator then ran the same probes outside that seam, on the same host, and
the measured results are recorded there. In short:

- `go env GOCACHE GOMODCACHE` exits `0` and prints
  `/Users/imhttran/Library/Caches/go-build` and `/Users/imhttran/go/pkg/mod`.
- `cargo` is present at `/Users/imhttran/.cargo/bin/cargo`, but cannot start under
  the eval-shaped profile: `execvp ... Operation not permitted`.
- `go build ./...` under the eval-shaped profile fails initializing the build
  cache under `$HOME`; with the caches redirected to a writable temp dir and a
  cold `GOMODCACHE`, it fails downloading modules (`no such host`).
- `go test ./...` under the eval-shaped profile fails: loopback listeners are
  denied, `lstat /private` is denied, SQLite-backed fixtures report `unable to
open database file (14)`, and `git init` cannot read
  `/var/select/developer_dir`.

The table above is still predicted from the profile text in
`internal/evaltarget/isolation_darwin.go`; the measured failures are quoted in
"What the eval profile denies".

## 3. Writes to `.agent-sdlc/` denied inside the repo root

The repo root is writable in the eval profile. `run_command` must not be able to
rewrite SOP state. The intended rule appends, after the `allow file-write*`, a
narrow deny:

```lisp
(deny file-write* (subpath "$AGENT_SDLC_STATE"))
```

where `$AGENT_SDLC_STATE` is the absolute `.agent-sdlc` path of the project.

### `.agent-sdlc` write denial

#### Status on this host

The SBPL override order is **verified on this host only**. SOP's command seam
refused the inline `sandbox-exec` probes, so the operator ran the experiment
outside that seam on the same host (Darwin, macOS, arm64). The commands and
output below are the ones actually observed. The result says nothing about other
macOS versions, other hosts, or Linux.

#### The two-rule profile (quoted verbatim)

The temp repo root is `/private/tmp/sbxprobe`. The profile file content was:

```lisp
(version 1)
(deny default)
(allow process*)
(allow file-read* (literal "/") (subpath "/private/tmp/sbxprobe") (subpath "/usr") (subpath "/usr/local") (subpath "/System") (subpath "/Library") (subpath "/bin") (subpath "/usr/bin") (subpath "/usr/sbin") (subpath "/sbin") (subpath "/dev") (subpath "/etc") (subpath "/private/etc") (subpath "/private/tmp") (subpath "/private/var/folders") (subpath "/private/var/select") (subpath "/opt/homebrew"))
(allow file-write* (subpath "/private/tmp/sbxprobe") (subpath "/private/tmp") (subpath "/private/var/folders"))
(allow sysctl-read)
(allow mach-lookup)
(deny network*)
(deny file-write* (subpath "/private/tmp/sbxprobe/.agent-sdlc"))
```

This is the eval-shaped profile (`darwinIsolationCommand`'s read/write/system
allows with root `/private/tmp/sbxprobe`) plus the later narrow deny. A minimal
profile without `(allow file-read* (literal "/") ...)` is not usable for this
experiment: `/bin/sh` aborts before running (`Abort trap: 6`, exit status `134`),
so it cannot demonstrate the override.

#### Probe commands (quoted verbatim)

Both probes run under the same profile instance (`-f` with one profile file):

```text
/usr/bin/sandbox-exec -f /private/tmp/sbxprobe/profile.sbpl -- /bin/sh -c 'printf ok > /private/tmp/sbxprobe/outside'
```

```text
/usr/bin/sandbox-exec -f /private/tmp/sbxprobe/profile.sbpl -- /bin/sh -c 'printf bad > /private/tmp/sbxprobe/.agent-sdlc/state.db'
```

#### Observed results (quoted verbatim)

The first probe printed:

```text
shell-init: error retrieving current directory: getcwd: cannot access parent directories: Operation not permitted
```

and exited `0`. (The `getcwd` line comes from the shell's working directory being
outside this profile's read set; it does not affect the write.)

The second probe printed:

```text
shell-init: error retrieving current directory: getcwd: cannot access parent directories: Operation not permitted
/bin/sh: /private/tmp/sbxprobe/.agent-sdlc/state.db: Operation not permitted
```

and exited `1`.

Post-run state:

```text
$ cat /private/tmp/sbxprobe/outside
ok
$ ls -l /private/tmp/sbxprobe/.agent-sdlc/state.db
ls: /private/tmp/sbxprobe/.agent-sdlc/state.db: No such file or directory
```

#### Conclusion (this host only)

On this host, the later `(deny file-write* (subpath "<root>/.agent-sdlc"))` rule
overrides the earlier, broader `(allow file-write* (subpath "<root>"))` rule. A
write to the root succeeds; a write to `.agent-sdlc/state.db` under the same
profile instance fails with `Operation not permitted` and the file is not
created. This ordering conclusion is **verified on this host only**.

#### Not-run note

No other host ran this probe. Do not infer the ordering for them.

### The protected-path check stays and remains the policy layer

The existing harness protected-path check for `.agent-sdlc/state.db`
(`namesStateDB`, `ErrProtectedPath`) is **unchanged and remains the policy
layer**. This is code inspection, not a runtime re-measurement of this task.

- `internal/toolharness/harness.go` `checkCommandIn` calls `namesStateDB(arg)`
  over `argv` before launch.
- `internal/toolharness/files.go` defines `stateDBRel =
".agent-sdlc/state.db"` and `namesStateDB`.
- A hit returns the `ErrProtectedPath` sentinel and the call is refused.

The SBPL deny rule above is a second, independent layer. It does **not** replace
`namesStateDB` / `ErrProtectedPath`. Policy still decides before launch; the
sandbox adds an OS-level backstop for paths the policy misses.

### Interaction with `security/llm07-state-write-detection`

`security/llm07-state-write-detection` is a **separate workstream**. Its
mechanism is **not asserted** here and its landing status is **UNKNOWN**. No
artifact read for this proposal describes its implementation. Both landing orders
are acceptable. The semantics of a refusal must stay distinct from a write:

- **Isolation lands first.** A sandbox denial is a refusal _before any write
  happens_. State-write detection sees no write and must **not** report a false
  state write. Re-check at that point: detection classifies the denial as a
  `sandbox refused write`, not as a `state write`.
- **Detection lands first.** It observes writes on the default unsandboxed path
  as today; detection still covers the default unsandboxed path, which remains
  the majority of runs. Re-check at merge time, when isolation lands: the SBPL
  denial (non-zero exit, no file change) is not treated as a write, and its
  evidence record names the sandbox as the actor.

These re-checks are listed in §6. No plan stage depends on that branch artifact
existing. This subsection makes no claim that either workstream has merged or
that any detection mechanism exists.

## What the eval profile denies

This section records what a real `go test ./...` and `cargo test` need that the
eval profile denies. Each row names the command, its exit status, and the exact
text observed on this host.

### Who ran the probes

SOP's `run_command` seam refused the probes when the agent tried them; those
refusals are recorded below. The operator then ran the same probes outside that
seam, on the same host, and these are the results. The refusals explain why the
agent could not produce this section itself; they do not change the measurements.

### Command seam refusals (measured during the SOP run)

The repository's `run_command` policy refused the commands the in-run probes
needed. Both refusals are quoted exactly as returned:

| Attempted command                                                                                                                                                  | Outcome                                         | Exact text                                                                                                                                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go env GOCACHE GOMODCACHE`                                                                                                                                        | refused before launch, exit status not produced | `command not allowed: "go env GOCACHE GOMODCACHE" is REQUIRES_APPROVAL`                                                                                                                                                                      |
| `/usr/bin/sandbox-exec -p '(version 1)(deny default)(allow process*)(allow file-write* (subpath "/private/tmp"))(allow file-read*)' -- /bin/sh -c 'echo probe-ok'` | refused before launch, exit status not produced | `command not allowed: shell metacharacters or globs are not allowed in "/usr/bin/sandbox-exec -p '(version 1)(deny default)(allow process*)(allow file-write* (subpath \"/private/tmp\"))(allow file-read*)' -- /bin/sh -c 'echo probe-ok'"` |

`go env` is escalated to approval by `checkCommandIn`; any inline
`sandbox-exec -p ...` carries quotes and parentheses that `SplitCommand` rejects as
shell metacharacters or globs.

### Environment inventory (measured outside the seam)

| Item              | Command                     | Exit status | Result                                                                      |
| ----------------- | --------------------------- | ----------- | --------------------------------------------------------------------------- |
| `GOCACHE` path    | `go env GOCACHE GOMODCACHE` | `0`         | `/Users/imhttran/Library/Caches/go-build`                                   |
| `GOMODCACHE` path | `go env GOCACHE GOMODCACHE` | `0`         | `/Users/imhttran/go/pkg/mod`                                                |
| Cargo presence    | `command -v cargo`          | `0`         | `/Users/imhttran/.cargo/bin/cargo`; `cargo --version` prints `cargo 1.98.0` |
| Host architecture | `uname -m`                  | `0`         | `arm64` (Darwin)                                                            |

### Profiles used for the measurements

The eval-shaped profiles were written to `/private/tmp/goprofile.sbpl`
(cold-cache probe) and `/private/tmp/goprofile2.sbpl` (warm-cache probe). Both
copy the read allow set, the write allow set (`<root>`, `/private/tmp`,
`/private/var/folders`), and `(deny network*)` from `darwinIsolationCommand`;
`goprofile2.sbpl` additionally allows read+write on the real `GOCACHE` and
`GOMODCACHE` so a cache denial and a network denial can be told apart. `go` on
this host is a trimmed Homebrew binary, so every command sets
`GOROOT=/opt/homebrew/Cellar/go/1.27.1/libexec`; without it the first failure is
`go: cannot find GOROOT directory`, which is an artifact of the Homebrew wrapper,
not of the sandbox. Neither profile is shipped; both were temporary and are not
part of this change.

### Measured needs

| Need                                           | Command (all under `sandbox-exec`)                                                                                                      | Exit | Exact observed text                                                                                                                                                                                                              |
| ---------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GOCACHE` writes                               | `-f /private/tmp/goprofile.sbpl -- /usr/bin/env GOROOT=... /opt/homebrew/bin/go build ./...`                                            | `1`  | `failed to initialize build cache at /Users/imhttran/Library/Caches/go-build: mkdir /Users: file exists`                                                                                                                         |
| `GOMODCACHE` / network                         | same profile, `GOCACHE=/private/tmp/gocache GOMODCACHE=/private/tmp/gomod GOPATH=/private/tmp/gopath` (cold)                            | `1`  | `go: downloading modernc.org/sqlite v1.59.0` then `internal/store/sqlite.go:10:2: modernc.org/sqlite@v1.59.0: Get "https://proxy.golang.org/modernc.org/sqlite/@v/v1.59.0.zip": dial tcp: lookup proxy.golang.org: no such host` |
| Cargo binary / `~/.cargo`                      | `-f /private/tmp/goprofile.sbpl -- /Users/imhttran/.cargo/bin/cargo --version`                                                          | `71` | `sandbox-exec: execvp() of '/Users/imhttran/.cargo/bin/cargo' failed: Operation not permitted`                                                                                                                                   |
| Real `go test` — loopback listener             | `-f /private/tmp/goprofile2.sbpl -- ... go test ./internal/agent -run TestSerializedRequestSeparatesAuthenticationFromContent -count=1` | `1`  | `panic: httptest: failed to listen on a port: listen tcp6 [::1]:0: bind: operation not permitted`                                                                                                                                |
| Real `go test` — SQLite state under a temp dir | `... go test ./internal/cli -run TestACPPromptBridgeReportsBusyWhenProjectLockHeld -count=1`                                            | `1`  | `acp_test.go:311: init failed: code=1 stderr=init: unable to open database file (14)`                                                                                                                                            |
| Real `go test` — `/private` metadata           | `... go test ./internal/cli -run TestACPRunACPServesInitialization -count=1`                                                            | `1`  | `invalid session/new cwd: resolve project directory "/private/var/folders/...": lstat /private: operation not permitted`                                                                                                         |
| Real `go test` — `git`/`xcode-select`          | `... go test ./internal/cli -run TestAcceptanceRecoveryAcrossApprovedAttempt -count=1`                                                  | `1`  | `xcode-select: error: unable to read data link at '/var/select/developer_dir', expected symbolic link (Operation not permitted)`                                                                                                 |

The full `go test ./...` run under `goprofile2.sbpl` produced the same classes of
failure across many packages; the rows above isolate each cause. No row was
inferred from the profile text alone.

### Reading of the measurements

- The eval profile denies the real build cache (`GOCACHE`) and the real module
  cache (`GOMODCACHE`) because both live under `$HOME`.
- With a cold module cache it cannot download: `(deny network*)` turns a module
  fetch into `no such host`.
- `cargo` cannot even start: its binary is under `$HOME/.cargo`, which the profile
  does not make readable.
- This repository's `go test ./...` needs more than caches: loopback HTTP servers,
  `/private` metadata for temp-path resolution, and the `xcode-select` path used by
  `git init`. `(deny network*)` and the profile's read set both block it.

### Linux and Windows

| Platform | Result  | Reason                                                                                                                           |
| -------- | ------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Linux    | NOT_RUN | `bwrap` is absent on this host (`seatbelt-evidence.md`: "Host: Darwin (macOS, arm64). bwrap absent."). No Linux command was run. |
| Windows  | NOT_RUN | No Windows backend exists (`internal/evaltarget/isolation_other.go`); the host is macOS. No Windows command was run or inferred. |

Neither Linux nor Windows is inferred from the macOS result. Neither was measured.

### Direction (measured basis)

The sandboxed `go`/`cargo` run needs an **explicit allow list** rather than
`$HOME` wholesale:

- read+write: `GOCACHE`, `GOMODCACHE` (read-only is not enough: Go writes to
  `GOCACHE`, and a module download writes to `GOMODCACHE`).
- read only: `$HOME/.config/go`, `~/.cargo`, `~/.rustup` (when present).
- read+write: the sandbox `TMPDIR`, created per run.
- network: **denied by default.** A download therefore fails closed with a
  clear message pointing at a pre-populated cache. An explicit opt-in
  sub-flag (`allow_network`) is discussed in §5, not assumed.

## 4. Flag, default, failure behavior, and audit

### Flag name and form

- **Environment flag (recommended): `SOP_AGENT_RUN_COMMAND_ISOLATION`.**
  Values: `off` (or unset) and `on`. It follows the existing `SOP_AGENT_*`
  family (`SOP_AGENT_COMMAND`, `SOP_AGENT_COMMAND_ISOLATION`-style names).
- **Alternative form: `Config.IsolateCommands bool`.** A caller sets it on
  `toolharness.Config` (`internal/toolharness/harness.go`, the `Config` struct).
  `New` copies it onto the `Harness` as `isolateCommands`.
- Backend selection is separate from the on/off decision: `seatbelt` on Darwin,
  `bwrap` on Linux, and unavailable where neither backend exists. No `auto` value
  is needed for the first cut; the OS decides the backend.

### Default: off, behavior unchanged

- Default is **off**. The flag is off when `SOP_AGENT_RUN_COMMAND_ISOLATION` is
  unset, empty, or `off`, and when `Config.IsolateCommands` is `false`.
- When the flag is off, `run_command` behaves exactly as it does today:
  `h.checkCommandIn` runs, `h.execCaptured` launches `argv` directly, no backend
  is looked up, no profile is built, and no audit field changes. No existing test
  needs a new fixture.

### Exact read path in `internal/toolharness/harness.go`

The decision is made in one function:

```go
func (h *Harness) runCommandWithStatus(ctx context.Context, args map[string]any) (string, bool, error)
```

(`internal/toolharness/harness.go`, currently at line 607; `runCommand` at line
602 calls it.) That is the only function that reads the flag, and it reads it at
`run_command` dispatch time, not at process start.

Order inside `runCommandWithStatus`, after the change:

1. `command, _ := args["command"].(string)` — read the request.
2. `argv, err := SplitCommand(command)` — tokenize; refuses shell metacharacters.
3. `root, err := h.selectRoot(rootSelector(args))` — pick the authorized root.
4. `h.checkCommandIn(root, argv)` — **the existing `commandpolicy`
   classification and the protected-path check. Runs before any wrap.**
5. **New:** if `h.isolateCommands` is off, jump to step 7. If it is on, call
   `sandbox.Wrap(h.root, root.path, argv, env)`. On success, use the wrapped
   `argv`; on `sandbox.ErrIsolationUnavailable`, return the sentinel error from
   step 6 with the audit denial record, and do **not** call `execCaptured`.
6. Return `fmt.Errorf("%w: %s", sandbox.ErrIsolationUnavailable, detail)`; no
   `execCaptured` call is made, so nothing runs.
7. `h.execCaptured(ctx, root.path, argv)` — unchanged, except `argv` may be the
   wrapped command.

`checkCommandIn` is unchanged. It is the policy layer; the sandbox wrap is an
additional boundary applied **after** it, never a replacement. A command that
`commandpolicy` refuses is never wrapped and never reaches `execCaptured`.

### Failure behavior when the flag is on and no backend is available

Named error sentinel:

```go
errIsolationUnavailable = errors.New("run_command isolation unavailable")
```

It is declared in `internal/toolharness/harness.go` beside the other sentinels
(`errCommandNotAllowed`, `ErrProtectedPath`, `errUnauthorizedRoot`,
`errReadOnlyRoot`) and is added to `isDenial` so the audit classifies it as a
denial.

- No backend: `sandbox.Wrap` returns `sandbox.ErrIsolationUnavailable`.
  `runCommandWithStatus` wraps it as `errIsolationUnavailable` and returns.
  `execCaptured` is **not** called, so `run_command` **executes nothing**.
  This fails closed; it does **not** fall back to the unsandboxed path.
- Backend present, profile build fails (for example the state path cannot be
  canonicalized): the same sentinel, same detail, and nothing runs. The shape
  matches `TestSeatbeltCommandCanonicalizationFailsClosed`.
- Backend present, command exits non-zero: reported exactly as today; the exit
  code and captured output are the evidence. The sandbox is not a new outcome
  class.

### Audit mapping

The existing shapes are used unchanged. **No new persisted schema is added.**

`internal/toolharness/audit.go`:

```go
type AuditRecord struct {
    Timestamp time.Time
    Tool      string
    Action    string
    Request   string
    Outcome   string
    Detail    string
}
const ( ActionAllow = "allow"; ActionDeny = "deny"; OutcomeOK = "ok"; OutcomeError = "error"; OutcomeDenied = "denied" )
```

`internal/toolharness/harness.go`:

```go
type CommandEvidence struct {
    Command string
    Cwd     string
    Exit    int
    Output  string
}
```

The `CommandEvidence` shape is used only by the existing `Config.EvidenceSink`, not
by the `AuditRecord`. Both are unchanged. The audit for `run_command` is emitted
by `Harness.audit` (called from `Run`) and by `RecordDenied`.

#### Allowed isolated run (flag on, backend present, command exits 0)

| Field                     | Value                                                                                                                                               |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AuditRecord.Tool`        | `"run_command"`                                                                                                                                     |
| `AuditRecord.Action`      | `"allow"` (`ActionAllow`)                                                                                                                           |
| `AuditRecord.Outcome`     | `"ok"` (`OutcomeOK`)                                                                                                                                |
| `AuditRecord.Request`     | `SummarizeRequest("run_command", args)`: `"cwd=<selector> <command>"`, or just `<command>` when no `cwd`                                            |
| `AuditRecord.Detail`      | `"isolate=seatbelt backend=seatbelt root=<root> profile=<sha256 dig>"` — backend name, sandbox root, and profile digest. No environment is written. |
| `CommandEvidence.Command` | `<the wrapped argv joined by a space>` (for example `"/usr/bin/sandbox-exec -p <profile> -- <program> <args>"`)                                     |
| `CommandEvidence.Cwd`     | `<the selected authorized root.path>`                                                                                                               |
| `CommandEvidence.Exit`    | the command's own exit code (`0` here); unchanged from today's path                                                                                 |
| `CommandEvidence.Output`  | the exact bounded stdout+stderr, truncation-marked; unchanged from today's path                                                                     |

#### Denied run — isolation unavailable (flag on, no backend)

`h.audit` runs with `err = errIsolationUnavailable`, which `isDenial` returns
true for:

| Field                 | Value                                                                                                        |
| --------------------- | ------------------------------------------------------------------------------------------------------------ |
| `AuditRecord.Tool`    | `"run_command"`                                                                                              |
| `AuditRecord.Action`  | `"deny"` (`ActionDeny`)                                                                                      |
| `AuditRecord.Outcome` | `"denied"` (`OutcomeDenied`)                                                                                 |
| `AuditRecord.Request` | `SummarizeRequest("run_command", args)`, as above                                                            |
| `AuditRecord.Detail`  | `err.Error()`: `"run_command isolation unavailable: missing backend <name> (looked for /usr/bin/sandbox-exec | bwrap | none)"`. The `Detail` **names the missing backend**. |

No `CommandEvidence` is produced for a denial: `execCaptured` is never called, so
there is no `Command`, `Cwd`, `Exit`, or `Output` to record. This is the existing
shape: a denial carries an `AuditRecord`, not evidence.

#### Detail suffix on the allowed run (backend name)

The `Detail` on the allowed run names the backend (`seatbelt` / `bwrap`), the
sandbox root, and a SHA-256 digest of the profile text. The digest is over the
path set, **not** the environment. No secret material is written.

#### Classification order (stated for the record)

`h.checkCommandIn` (`internal/toolharness/harness.go`) calls
`commandpolicy.Classify` / `commandpolicy.CargoClassify`, the protected-path
check `namesStateDB`, and the path-confinement check. It runs at step 4, before
step 5 wraps `argv`. Therefore:

- `commandpolicy` classification still runs first.
- The sandbox wrap is an **additional boundary**, not a replacement for
  `commandpolicy`.
- A command `commandpolicy` refuses is denied before the sandbox is consulted;
  the `Detail` names the policy class, not a backend.

#### Not stated here

- No `Outcome` value is added. The denial reuses `OutcomeDenied`; the allowed run
  reuses `OutcomeOK`. A distinct `"sandbox_unavailable"` is **not** added in this
  cut, so the persisted schema stays identical.
- No new field is added to `AuditRecord` or `CommandEvidence`. Any later field
  (for example `SandboxBackend`) must be named in a revised section of this
  proposal before use. See §5 (options) and §6 (tests).

## 5. Options and recommendation

Each option states a cost, a risk, and why it wins or loses against the
recommendation. Option C cites the recorded `go list -deps` facts from §1; those
facts are recorded output, not proof of a safe import direction.

| Option                                                                                                                     | Cost                                                                                                                                                                          | Risk                                                                                                                                                                                                                                                                                                                                      | Why it wins or loses                                                                                                                                                     |
| -------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **A. Shared low-level `internal/sandbox` package; `toolharness`, `evaltarget`, and `dqharness` all call it (recommended)** | Medium. Move the SBPL and bubblewrap builders into one new package. Two existing call sites delegate. Existing `evaltarget` and `dqharness` tests stay as the regression net. | Low. One builder serves both policies, which stay parameters. The new package imports only the standard library.                                                                                                                                                                                                                          | **Wins.** One profile text is proven once by one test set. No new heavy dependency enters the `run_command` seam. Both recorded profiles stay byte-identical to today's. |
| B. Second profile written inside `toolharness`                                                                             | Low to write.                                                                                                                                                                 | High. Two profiles drift. The `evaltarget` and `dqharness` tests no longer cover the profile that `run_command` executes. The `.agent-sdlc` deny rule and the cache allow list are maintained twice.                                                                                                                                      | **Loses.** It adds a third profile to a repository that already has two, and it splits the proof.                                                                        |
| C. `toolharness` imports `evaltarget` directly                                                                             | Lowest today. One import and one call.                                                                                                                                        | Recorded fact: `internal/evaltarget` depends on `internal/store` and `internal/approval`. An import would pull the SQLite state layer (`modernc.org/sqlite`, `modernc.org/libc`) and the approval gate into the `run_command` seam, even when the flag is off. This is the observed dependency list from §1, not a proven safe direction. | **Loses.** It reuses the right builder at the wrong boundary and ties a tool seam to evaluation policy.                                                                  |
| D. Wrap the whole agent process instead of `run_command`                                                                   | Medium. The agent process launch is outside this repository, so the change does not fit the seam at `internal/toolharness/harness.go`.                                        | High. The blast radius covers every tool and every file access, not one tool call.                                                                                                                                                                                                                                                        | **Loses.** It is far wider than the problem and it cannot be expressed at the `run_command` dispatch point.                                                              |

### Recommendation

Choose **Option A**. Reasons from the repository evidence:

1. The two builders already exist (§1). Option A reuses them. Option B writes a
   third one.
2. `internal/toolharness` depends only on `internal/commandpolicy` and
   `internal/git` (§1). Option A keeps the seam at that weight. Option C adds
   `internal/store` and `internal/approval`.
3. The `.agent-sdlc` deny rule (§3) and the network rule must be proven once.
   Option A keeps one builder and one test net. Option B splits the proof
   between packages.
4. The two existing policies differ (`evaltarget` denies by default; `dqharness`
   allows by default with denies). Option A keeps each policy as a parameter, so
   no consumer changes its profile text.

### Delivery order

Deliver in this order. Each step lands only after the prior step passes:

1. Trace the import graph again (`go list -deps`) and record the output in §1.
2. Extract the builders into `internal/sandbox`. Make `evaltarget` and
   `dqharness` delegate. Their existing tests must pass unchanged.
3. Run the §2 and §3 experiments, and quote exit status and error text.
4. Add the flag, off by default. Wire `runCommandWithStatus` in
   `internal/toolharness/harness.go` to call the builder when the flag is on,
   and to fail closed otherwise.
5. Extend the audit `Detail` as in §4.

The network sub-flag (`allow_network`) is **not recommended in the first cut**.
A module download is an explicit, separate decision with its own evidence.

This section makes no compliance claim and marks nothing verified.

## 6. Tests that would prove it

### Claims posture

This proposal is an **evidence-matrix entry candidate, not a compliance claim.**
It describes a design. It does not implement one, and it does not prove one.

- The `IMPLEMENTED` / `VERIFIED` / `NOT_RUN` / `GAP` labels in
  [`../reports/owasp-llm/README.md`](../reports/owasp-llm/README.md) and
  [`../reports/security-shift-left/execution-path-matrix.md`](../reports/security-shift-left/execution-path-matrix.md)
  stay honest and stay as they are today. Both files are unchanged by this
  proposal.
- **The design itself is not marked verified.** The runtime probes that _are_
  recorded are host-limited observations on this macOS arm64 host: the §3
  `.agent-sdlc` write-override probe and the §2 toolchain measurements. They are
  not cross-host verification and they do not generalize. No production code,
  flag, or sandbox integration exists yet.

### Reused fixtures

Each reused fixture is named with its file path as seen in the repository. The
source column states where the name and result were observed.

| Fixture                                               | File path                                       | Package      | What it proves for this design                                                                 | Source                                                                                   |
| ----------------------------------------------------- | ----------------------------------------------- | ------------ | ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `TestDarwinIsolationBlocksOrdinaryWorkspace`          | `internal/evaltarget/isolation_darwin_test.go`  | `evaltarget` | The current SBPL profile blocks a write outside the case root.                                 | `search_files`; PASS recorded in `docs/reports/security-shift-left/seatbelt-evidence.md` |
| `TestDarwinIsolationBlocksNetwork`                    | `internal/evaltarget/isolation_darwin_test.go`  | `evaltarget` | The `(deny network*)` rule blocks egress.                                                      | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestDarwinIsolationStripsSecrets`                    | `internal/evaltarget/isolation_darwin_test.go`  | `evaltarget` | The sandbox environment strips production secrets.                                             | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolateEnvRemovesProductionSecrets`              | `internal/evaltarget/runner_test.go`            | `evaltarget` | The isolated environment drops production secrets before launch.                               | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestTimeoutTerminatesChildGroup`                     | `internal/evaltarget/runner_test.go`            | `evaltarget` | A timeout kills the whole child group.                                                         | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestCancellationTerminatesChildGroup`                | `internal/evaltarget/runner_test.go`            | `evaltarget` | Cancellation kills the whole child group.                                                      | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolationBackendSelection`                       | `internal/dqharness/isolation_args_test.go`     | `dqharness`  | The backend selector picks `seatbelt` on Darwin and `bwrap` on Linux.                          | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestSeatbeltArgumentsShape`                          | `internal/dqharness/isolation_args_test.go`     | `dqharness`  | The Seatbelt command carries the expected argument shape.                                      | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestSeatbeltCommandUsesCanonicalReplayDir`           | `internal/dqharness/isolation_args_test.go`     | `dqharness`  | The Seatbelt profile binds the canonical replay directory.                                     | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestSeatbeltCommandCanonicalizationFailsClosed`      | `internal/dqharness/isolation_args_test.go`     | `dqharness`  | A profile build failure fails closed.                                                          | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayCanonicalizationFailsBeforeLaunch` | `internal/dqharness/isolation_args_test.go`     | `dqharness`  | Canonicalization failure stops the command before launch.                                      | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayReplayDirWritesSucceed`            | `internal/dqharness/isolation_args_test.go`     | `dqharness`  | A write inside the replay directory succeeds under the sandbox.                                | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayConfinementTempRoots`              | `internal/dqharness/isolation_args_test.go`     | `dqharness`  | The sandbox confines temp roots.                                                               | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayFailsClosedWithoutIsolation`       | `internal/dqharness/replay_exec_test.go`        | `dqharness`  | Without a backend the replay refuses to run.                                                   | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayRunsVerificationInFreshDir`        | `internal/dqharness/replay_exec_test.go`        | `dqharness`  | The sandboxed run executes in a fresh directory.                                               | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayScrubsCredentials`                 | `internal/dqharness/replay_exec_test.go`        | `dqharness`  | Credentials are scrubbed before the sandboxed run.                                             | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayDeniesWritesOutside`               | `internal/dqharness/replay_exec_test.go`        | `dqharness`  | A write outside the replay directory is denied.                                                | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayDeniesNetwork`                     | `internal/dqharness/replay_exec_test.go`        | `dqharness`  | A loopback listener proves the unsandboxed client connects and the sandboxed client is denied. | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayTimesOut`                          | `internal/dqharness/replay_exec_test.go`        | `dqharness`  | A sandboxed run times out cleanly.                                                             | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |
| `TestIsolatedReplayUnixSocket`                        | `internal/dqharness/replay_unix_socket_test.go` | `dqharness`  | A Unix-domain socket connection is denied on Seatbelt and bubblewrap.                          | `search_files`; PASS recorded in `seatbelt-evidence.md`                                  |

Test commands recorded in `docs/reports/security-shift-left/seatbelt-evidence.md`:

```sh
go test ./internal/dqharness/ -run 'Isolation|Isolated|Bubblewrap|Seatbelt' -v -count=1
go test ./internal/evaltarget/ -run 'Isolat' -v -count=1
go test ./internal/evaltarget/ -run 'TestTimeoutTerminatesChildGroup|TestCancellationTerminatesChildGroup' -v -count=1
```

Corrections to the earlier draft of this section: the draft named
`TestIsolatedReplayDeniesWritesOutside` and `TestIsolatedReplayDeniesNetwork`
without a path. Both exist in `internal/dqharness/replay_exec_test.go`, so they
are **corrected** to real paths. The draft also named no new test explicitly; the
new tests are listed below with the words _does not exist yet_.

### New `internal/toolharness` tests

Each row names a test that **does not exist yet**. No test file is created or
modified by this proposal. Checked against the current `internal/toolharness`
test files: no test with any of these names is present.

| New test (does not exist yet)                        | Assertion it makes                                                                                                                                                                                                                                                                                                            |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestRunCommandIsolationOffKeepsExecCapturedPath`    | With the flag off, `runCommandWithStatus` calls `execCaptured` on the plain `argv`, makes no backend lookup, and builds no profile. The wrapped-argv and backend-lookup hooks record zero calls. **Does not exist yet.**                                                                                                      |
| `TestRunCommandIsolationOnFailsClosedWithoutBackend` | With the flag on and a fake backend lookup that returns "missing", `runCommandWithStatus` returns the isolation-unavailable error, calls `execCaptured` zero times, and writes one `AuditRecord` with `Action == ActionDeny` and `Outcome == OutcomeDenied` whose `Detail` names the missing backend. **Does not exist yet.** |
| `TestRunCommandIsolationOnWrapsArgvWithSandboxExec`  | With the flag on and `sandbox-exec` present, the argv handed to `execCaptured` starts with `/usr/bin/sandbox-exec -p <profile> --` and ends with the original `argv`. The profile text comes from the shared builder. **Does not exist yet.**                                                                                 |
| `TestRunCommandIsolationDeniesStateDirWrite`         | With the flag on, a `run_command` that writes under `.agent-sdlc/` exits non-zero and the target file is unchanged or absent; a sibling write under the root succeeds. **Does not exist yet.**                                                                                                                                |
| `TestRunCommandIsolationDeniesNetwork`               | With the flag on, a loopback listener proves the unsandboxed client connects and the sandboxed `run_command` client is denied. **Does not exist yet.**                                                                                                                                                                        |

### `internal/cli` test rule

Tests in `internal/cli` resolve `/bin/true` with `exec.LookPath`, not with a
hardcoded path. The helper `trueExecutable()` in
`internal/cli/rag_true_test.go` calls `exec.LookPath("true")` first. **This
proposal adds `/bin/true` to no test.** The new `internal/toolharness` tests
above use fixture binaries in temporary directories, not `/bin/true`.

### Merge-time re-check: state-write detection

The workstream `security/llm07-state-write-detection` is separate, and its
landing status is **UNKNOWN**. No detector is assumed to exist. At merge time,
whichever lands first, re-check that a sandbox denial (non-zero exit, no file
change) is classified as a _sandbox refusal_, not as a _state write_. This
proposal makes no claim that the detector exists.

### Evidence doc to extend

`docs/reports/security-shift-left/seatbelt-evidence.md`, under a new heading that
states the evidence covers the evaluation paths **plus** the opt-in tool path,
with the exact commands run.

## 7. Unclaimed

Three residual boundaries stay outside this design. For each, this proposal
makes **no OS-isolation claim**.

### Linux runtime (bubblewrap)

- `bwrap` is **absent on this host**. `docs/reports/security-shift-left/seatbelt-evidence.md`
  records: "Host: `Darwin` (macOS, arm64). `bwrap` absent."
- Bubblewrap stays `NOT_RUN` with the existing reference to
  `docs/reports/security-shift-left/bubblewrap-verification.md`.
- **No Linux OS-isolation claim.** The macOS Seatbelt result is not inferred for
  Linux.

### Windows

- `internal/evaltarget/isolation_other.go` is the only non-Darwin, non-Linux
  isolation file. It returns `ErrIsolationUnavailable`. There is no Windows
  backend.
- When the flag is on, Windows fails closed: nothing runs. No Windows isolation is
  designed or proven.
- **No Windows OS-isolation claim.**

### `SOP_AGENT_COMMAND` command-agent path

- SOP does not launch the command agent in-process (`docs/reports/owasp-llm/README.md`
  Plan 06 row; `docs/reports/security-shift-left/execution-path-matrix.md` row 2
  classifies it command-policy only).
- The flag covers `internal/toolharness` `run_command` and nothing else. A harness
  change cannot reach the out-of-process command agent.
- **No OS-isolation claim for this path.**

### Untouched paths

Verification commands and configured agent commands (matrix rows 3 and above) are
untouched by this proposal. `docs/reports/security-shift-left/execution-path-matrix.md`
and `docs/reports/owasp-llm/README.md` are unchanged.

**The design itself remains unimplemented and unverified; only the host-limited
probes in §2 and §3 are recorded observations.**

## 8. Closeout

This section records the commands run for this design artifact, their
pass/fail result, and everything not verified on this host. It makes no
compliance claim and changes no label under `docs/reports/`.

### Commands run

| Command                                                                                                                               | Result               | Note                                                                                                                                                                           |
| ------------------------------------------------------------------------------------------------------------------------------------- | -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/usr/bin/sandbox-exec -f /private/tmp/sbxprobe/profile.sbpl -- /bin/sh -c 'printf ok > /private/tmp/sbxprobe/outside'`               | PASS                 | Exit `0`; file `outside` written with `ok`. Quoted in §3.                                                                                                                      |
| `/usr/bin/sandbox-exec -f /private/tmp/sbxprobe/profile.sbpl -- /bin/sh -c 'printf bad > /private/tmp/sbxprobe/.agent-sdlc/state.db'` | PASS (denial proven) | Exit `1`; `Operation not permitted`; `state.db` absent. Quoted in §3.                                                                                                          |
| `go list -deps ./internal/toolharness`                                                                                                | PASS                 | Exit `0`; module-local packages recorded in §1.                                                                                                                                |
| `go list -deps ./internal/evaltarget`                                                                                                 | PASS                 | Exit `0`; module-local packages recorded in §1.                                                                                                                                |
| `go env GOCACHE GOMODCACHE`                                                                                                           | PASS                 | Exit `0`; prints the two cache paths quoted in §2.                                                                                                                             |
| `command -v cargo` / `cargo --version`                                                                                                | PASS                 | Exit `0`; `/Users/imhttran/.cargo/bin/cargo`, `cargo 1.98.0`.                                                                                                                  |
| `uname -m`                                                                                                                            | PASS                 | Exit `0`; `arm64`.                                                                                                                                                             |
| `sandbox-exec ... go build ./...` (eval-shaped profile)                                                                               | FAIL (expected)      | Exit `1`; `failed to initialize build cache ... go-build`.                                                                                                                     |
| `sandbox-exec ... go build ./...` (cold caches, network denied)                                                                       | FAIL (expected)      | Exit `1`; `lookup proxy.golang.org: no such host`.                                                                                                                             |
| `sandbox-exec ... cargo --version`                                                                                                    | FAIL (expected)      | Exit `71`; `execvp ... Operation not permitted`.                                                                                                                               |
| Sandboxed targeted `go test` rows (loopback, SQLite, `/private`, xcode-select)                                                        | FAIL (expected)      | Exit `1` for each; exact text in §2.                                                                                                                                           |
| `go build ./...` / `go test ./...` / `go vet ./...` (unsandboxed, configured validation)                                              | PASS                 | Exit `0` for each; re-run after this correction.                                                                                                                               |
| `git status --short`                                                                                                                  | PASS                 | Shows `?? docs/plans/8-llm03-run-command-isolation-design.md` (pre-existing, user-owned) and `?? docs/plans/PROPOSAL-RUN-COMMAND-ISOLATION.md`. No path under `docs/reports/`. |

The two `sandbox-exec -p`/`go env` calls that SOP's command seam refused are
recorded in "Command seam refusals"; the operator ran the equivalent probes
outside the seam, and those are the rows above.

### Not verified on this host

- **Linux runtime (bubblewrap).** NOT_RUN. `bwrap` is absent on this host;
  `seatbelt-evidence.md` records "Host: Darwin (macOS, arm64). bwrap absent."
- **Windows.** NOT_RUN. No Windows backend exists
  (`internal/evaltarget/isolation_other.go`); the host is macOS.
- **`cargo test`.** NOT_RUN. There is no Cargo project in this repository. Only
  the cargo binary's behavior under the eval-shaped profile was observed.
- **The design itself.** NOT_IMPLEMENTED. No flag, no shared `internal/sandbox`
  package, and no new test exists. The sandboxed `go test` failures in §2 are the
  observed behavior of the current eval profile, not a verification of this
  proposal.

### Working-tree boundary

`git status --short` lists no production code file and no path under
`.agent-sdlc/`. `docs/reports/owasp-llm/README.md` and
`docs/reports/security-shift-left/execution-path-matrix.md` keep their original
status. The pre-existing untracked
`docs/plans/8-llm03-run-command-isolation-design.md` is unchanged.

**The design is not implemented or verified. The host-limited probes above are
the only runtime observations.**

## 9. Non-goals

- No production code in this task.
- No change to `docs/reports/security-shift-left/execution-path-matrix.md` or
  `docs/reports/owasp-llm/README.md` statuses: nothing is implemented.
- No dependency added; no new sandbox implementation.
- Publication is outside the design: the operator requested a branch and PR
  after the artifact was written, and that request changes no design claim.
