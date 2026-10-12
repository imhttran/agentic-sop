# CLI and Routes

**Type:** Reference

How to run SOP Controller, the SOP commands it drives, and the HTTP routes it
serves.

## Running the Controller

```bash
go run ./cmd/sop-controller     # foreground
go build ./cmd/sop-controller && ./sop-controller   # build a binary
```

Make targets:

| Target         | Effect                                 |
| -------------- | -------------------------------------- |
| `make run`     | run in the foreground (Ctrl-C to stop) |
| `make start`   | build + run in the background          |
| `make stop`    | stop the background server             |
| `make restart` | stop, then start                       |
| `make status`  | is it running? (also pings `/healthz`) |
| `make logs`    | tail the background log                |
| `make build`   | build `./cmd/sop-controller`           |
| `make test`    | `go test ./...`                        |
| `make fmt`     | `gofmt -w .`                           |
| `make vet`     | `go vet ./...`                         |
| `make tidy`    | `go mod tidy`                          |
| `make clean`   | remove `.run` artifacts                |

## SOP Commands the Dashboard Drives

Every action delegates to the SOP CLI (`SOP_BIN`, default `sop`). The dashboard
never mutates SOP state directly. See
[../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md).

| Dashboard action    | SOP command                                          |
| ------------------- | ---------------------------------------------------- |
| Run / Start         | `sop run`                                            |
| Resume              | `sop resume`                                         |
| Validate            | `sop validate`                                       |
| Review              | `sop review`                                         |
| Report              | `sop report <task>`                                  |
| Retry               | `sop retry <task>`                                   |
| Force retry         | `sop retry <task> --force`                           |
| Retry all           | `sop retry --all`                                    |
| Reconcile           | `sop reconcile <PLAN.md>`                            |
| Reconcile listing   | `sop reconcile <PLAN.md> --list-changed --json`      |
| Accept changed task | `sop reconcile <PLAN.md> --accept-changed <TASK_ID>` |
| Approve             | `sop approve <task-id> [--by NAME] [--note TEXT]`    |
| Decline             | `sop decline <task-id> [--by NAME] [--note TEXT]`    |

SOP decides retry eligibility, reconcile outcomes, and which task runs; the
controller only reports the result. Human approval (`sop approve` / `sop
decline`) is a supported delegated operation; no `--run` is passed, and SOP
validates the gate at command time, so a stale/not-applicable decision surfaces
as a conflict rather than a fabricated success. See
[../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md).

One conceptual operation remains **unsupported** because SOP exposes no
application operation for it: `CancelRun` (SOP exposes no cancellation
operation). The controller must not advertise a control it cannot delegate; it is
not offered in the UI. Per-task `AcceptChangedTask` is now supported and
delegates to `sop reconcile <PLAN.md> --accept-changed <TASK_ID>`. The current
operation table is maintained at the retained historical path
[../history/CTRL001/BOUNDARY-CONTRACT.md](../history/CTRL001/BOUNDARY-CONTRACT.md).
Controller-boundary support is distinct from external-binary verification:
the reconciliation flags were **UNVERIFIED** in the recorded
[C2-009 sandbox run](../history/C2-009-REPORT.md), whose real-binary scenarios
were **NOT EXERCISED** (readiness **NOT READY**).

Each command is bounded by `SOP_CONTROLLER_COMMAND_TIMEOUT` (see
[CONFIGURATION.md](CONFIGURATION.md)).

## HTTP Routes

| Method | Path                                                       | Purpose                                                                                 |
| ------ | ---------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `GET`  | `/healthz`                                                 | health check                                                                            |
| `GET`  | `/`                                                        | home / landing                                                                          |
| `GET`  | `/projects`                                                | project overview (FR-1)                                                                 |
| `GET`  | `/approvals`                                               | pending-approvals inbox across projects (P0; read-only on GET)                          |
| `GET`  | `/discovery`                                               | discovery boundary view                                                                 |
| `GET`  | `/projects/{project}`                                      | workflow view (FR-2)                                                                    |
| `GET`  | `/projects/{project}/tasks`                                | task list partial                                                                       |
| `GET`  | `/projects/{project}/activity`                             | project activity (FR-4)                                                                 |
| `GET`  | `/projects/{project}/activity/stream`                      | live activity SSE stream                                                                |
| `GET`  | `/projects/{project}/activity/window`                      | bounded activity poll                                                                   |
| `GET`  | `/projects/{project}/tasks/{task}`                         | task detail (FR-3)                                                                      |
| `GET`  | `/projects/{project}/tasks/{task}/activity`                | task activity                                                                           |
| `GET`  | `/projects/{project}/tasks/{task}/recovery`                | recovery view                                                                           |
| `GET`  | `/projects/{project}/tasks/{task}/review`                  | review view (FR-5)                                                                      |
| `GET`  | `/projects/{project}/tasks/{task}/ci`                      | CI / validation view (FR-6)                                                             |
| `GET`  | `/projects/{project}/tasks/{task}/handoff`                 | handoff view (FR-7)                                                                     |
| `POST` | `/projects/{project}/commands/{verb}`                      | start a project command (FR-8)                                                          |
| `GET`  | `/projects/{project}/commands/{verb}`                      | project command status                                                                  |
| `POST` | `/projects/{project}/tasks/{task}/commands/retry`          | retry one task                                                                          |
| `POST` | `/projects/{project}/tasks/{task}/commands/retry-force`    | force retry                                                                             |
| `POST` | `/projects/{project}/tasks/{task}/commands/report`         | open report                                                                             |
| `POST` | `/projects/{project}/tasks/{task}/commands/approve`        | approve a human gate (`sop approve`)                                                    |
| `POST` | `/projects/{project}/tasks/{task}/commands/decline`        | decline a human gate (`sop decline`)                                                    |
| `POST` | `/projects/{project}/tasks/{task}/commands/accept-changed` | accept one changed executed task (`sop reconcile <PLAN.md> --accept-changed <TASK_ID>`) |

Project command verbs: `run` (also `start`), `resume`, `validate`, `review`,
`report`, `retry-all`, `reconcile`. Every `POST` requires a CSRF token; see
[../specs/SECURITY.md](../specs/SECURITY.md).

### Referenced command/route/flag accuracy

The references below describe the controller contract, not verification of
external-binary flag support:

| Referenced item                                                                                   | Evidence                                                                                                                     |
| ------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `sop run`, `sop resume`, `sop validate`, `sop review`, `sop report`, `sop retry`, `sop reconcile` | [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md) Operations table                                          |
| `sop reconcile <PLAN.md> --list-changed --json`                                                   | [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) Data Sources                                                      |
| `sop approve` / `sop decline` with `--by`, `--note` (no `--run`)                                  | [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md); [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) |
| `approve` / `decline` / `accept-changed` routes                                                   | this document's HTTP Routes table; [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md)                                  |
| `/projects/{project}/commands/{verb}` verbs                                                       | [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) (command delegates)                                               |
| `SOP_BIN`, `SOP_CONTROLLER_COMMAND_TIMEOUT`                                                       | [CONFIGURATION.md](CONFIGURATION.md)                                                                                         |

No cancel route or control exists, because SOP exposes no
cancellation operation; cancellation remains unsupported until SOP exposes it.

## Related Documentation

- [CONFIGURATION.md](CONFIGURATION.md) — environment variables.
- [STATUS-AND-RECOVERY.md](STATUS-AND-RECOVERY.md) — statuses, stages, recovery.
- [../guides/GETTING-STARTED.md](../guides/GETTING-STARTED.md) — first run.
