# Connector submission — PB-1 / q-1

The decision was submitted by the external planner through the OpenAI Secure MCP Tunnel
surface (`SOP Status Spike Fresh`). The operator did not submit it, and did not resubmit it.

## Reported by the connector

| Field | Value |
| --- | --- |
| task | `PB-1` |
| question | `q-1`, revision `1`, attempt `0` |
| answer | `a-2` |
| state | `ANSWERED` |
| action | `continue` |
| body | `accepted` |
| idempotency key | `pe4-q1-1` |
| plan revision | `plan@a09bfa55b609c235c9a7ce28f8c7557ce73fb65fb726bf90539230594a4e53ba` |
| `question_run_epoch` | `2026-10-26T17:24:12.633178Z` |

## Persisted record (verified against the store)

`questions.json` in this archive is the authoritative persisted state:

- question `q-1` → `ANSWERED` at submission, later `APPLIED` after consumption;
- answer `a-2` → body `accepted`, `answered_by: chatgpt`, idempotency key `pe4-q1-1`,
  `expected_revision: 1`, `chosen_action: continue`;
- outcome `o-3` → `ACCEPTED`, revision 1, same idempotency key;
- outcome `o-4` → `APPLIED` after `sop planner continue --apply`;
- continuation `cont:q-1:2026-10-26T17:24:12.633178Z` → `COMPLETED`.

The persisted answer and binding match the connector's report exactly.

`answered_by: "chatgpt"` is caller-supplied metadata (SPIKE-006), not cryptographic
provenance. The operator reports that the submission came from the external client through
the connector.
