# DP-001 — Real-SOP integration smoke

> **Document class:** report · non-normative point-in-time evidence. Developer
> Preview Phase 1. SOP remains the workflow authority.

This records a reproducible smoke test of the controller against the **real** `sop`
binary, using a deterministic stub agent (SOP's supported `command` harness) so no
model or external service is invoked. Production SOP state is never touched; all
fixtures are disposable directories under `/tmp`.

## Procedure

1. Build the controller:

       go build -o /tmp/dp001/sop-controller ./cmd/sop-controller

2. Create each disposable project `P` in `approve`, `decline`:

       mkdir -p /tmp/dp001c/P/docs
       ( cd /tmp/dp001c/P && sop init )
       # patch .agent-sdlc/config.yaml: name=P, harness: command, provider: command
       cp tests/e2e/fixtures/prd.md /tmp/dp001c/P/docs/PRD.md

3. Drive the project to a genuine human gate with the stub agent (real lifecycle;
   the stub answers PLAN with a one-stage plan and IMPLEMENT with an explicit
   needs-human boundary, so SOP parks the task at a real approval gate). `sop run`
   exits non-zero at the gate — that is expected:

       ( cd /tmp/dp001c/P \
         && SOP_ACTIVITY=on \
            SOP_AGENT_COMMAND="sh <repo>/tests/e2e/fixtures/fake-agent.sh" \
            E2E_PLAN_JSON='{"project":"P","summary":"fixture","stages":[{"id":"P1","title":"Approval gate","objective":"Perform a change SOP must escalate to a human decision.","dependencies":[],"deliverables":["docs/result.md"],"acceptance_criteria":["The task reaches a human decision boundary."],"execution_mode":"implement"}]}' \
            sop run )

4. Start the controller on loopback with auth off (local default):

       SOP_CONTROLLER_PROJECTS=/tmp/dp001c/approve,/tmp/dp001c/decline \
       SOP_CONTROLLER_ALLOW_NETWORK=false SOP_CONTROLLER_TOKEN= \
       SOP_CONTROLLER_ADDR=127.0.0.1:18081 /tmp/dp001/sop-controller

5. Exercise reads and delegated commands (CSRF token read from the session cookie):

       curl -s http://127.0.0.1:18081/projects
       curl -s http://127.0.0.1:18081/approvals
       curl -s --max-time 2 http://127.0.0.1:18081/projects/approve/activity/stream
       curl -s -b <jar> --data-urlencode "csrf@<token-file>" --data-urlencode "by=operator" \
            http://127.0.0.1:18081/projects/approve/tasks/A1/commands/approve

6. Prove no direct writes: hash each `state.db` before and after the read-only calls.

## Results

| Check | Outcome | Evidence |
| --- | --- | --- |
| Discovery of a real project | PASS | controller listed `approve`/`decline`; `/discovery` 200 |
| Project / task / task-detail views | PASS | `/projects/approve`, `/projects/approve/tasks`, `/projects/approve/tasks/A1` all 200 from real `state.db` |
| Activity streaming (SSE from `activity.jsonl`) | PASS | `/projects/approve/activity/stream` emitted real `event: activity` frames |
| Approvals inbox (real `sop approvals --json`) | PASS | inbox showed both gates `A1`/`D1` with kind `NEEDS_HUMAN` |
| Approve delegation | PASS | POST `.../commands/approve` -> `sop approval A1` reports `Approval: APPROVED` |
| Decline delegation | PASS | POST `.../commands/decline` -> `sop approval D1` reports `Approval: DECLINED` |
| Retry delegation | PASS | POST `.../commands/retry-all` -> status `done` |
| Reconcile delegation | PASS | POST `.../commands/reconcile` -> status `done` (`Reconciled`) |
| Run/resume delegation | PARTIAL | POST `.../commands/start` invoked SOP (`running`); the fixture plan was already complete, so SOP returned "nothing to resume" |
| No direct controller writes | PASS | `state.db` sha256 unchanged across every read endpoint |
| Run to PASS end-to-end | BLOCKED | requires a genuine agent decision; out of scope without model approval |

## No-write guarantee

- **Code:** `internal/sopclient.OpenStore` opens SQLite read-only
  (`file:...?mode=ro`); `internal/sopclient/boundary_test.go` and
  `internal/web/boundary_test.go` assert the controller adds no mutation path.
- **Empirical:** the disposable `state.db` hashes were identical before and after
  all read endpoints; every state change observed was produced by the `sop` child
  process the controller invoked, never by the controller itself.

## Scope notes

- The stub agent is SOP's supported `command` harness; no model or external
  service is called. The `tool`/`ollama` harness is never used here.
- Paid/cloud inference, destructive operations, and production `state.db` changes
  were not performed.
