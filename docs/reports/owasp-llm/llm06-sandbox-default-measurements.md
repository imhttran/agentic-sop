# LLM06 — run_command sandbox: default-on measurements (OWASP gap 13)

**Status:** partial. Measured on macOS only. The Linux socket gap is **not** verified:
this host has no bubblewrap, so the Linux socket fix cannot be reproduced or tested here.

## Measured on macOS arm64 (Darwin 25.6, Go 1.27.1)

| Measurement | Result |
| --- | --- |
| `go test -count=1 ./internal/run/`, outside the sandbox, fresh build cache, warm module cache | 4.4 s per run (2 runs) |
| The same command inside the sandbox profile, with the build cache under `/private/tmp` | Fails before compiling: `go: could not create module cache: stat /Users/…/go/pkg/mod: operation not permitted` |
| Earlier attempt with the build cache under the test temp directory | Fails: `mkdir /var: file exists` (the `/var` symlink path is not usable by the cache setup) |

What this means for default-on:

- A sandboxed Go command cannot use the module cache under `$HOME` on this machine, so a
  default-on sandbox would break Go-based checks until the module cache is made available
  inside the profile. The documented rule "run `go mod download` first" does not help here:
  the cache is stat-denied, not merely missing a module.
- The build-cache overhead measured outside the sandbox is about 4.4 s for one package.
  The sandboxed run could not be timed, so the sandbox-specific overhead is unmeasured.

## Not measured

- Which existing tests fail inside the sandbox, across the full suite. Only one package was tried.
- Behavior on a platform with no sandbox available.
- The Linux bubblewrap socket gap (under `$HOME`) and the cold-cache overhead on Linux.

## Decision for the operator

Default-on needs the module-cache question answered first. Options to evaluate: expose a
read-only module cache inside the profile, or keep the sandbox opt-in with a startup
notice. Do not change the default until a full-suite run inside the sandbox is measured.
