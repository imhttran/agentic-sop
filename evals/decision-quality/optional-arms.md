# RDM-004 — Optional arms C and P

These arms are recorded explicitly. Neither is a mandatory deliverable.

## Arm C — local hybrid keyword plus semantic retrieval

**Status: not evaluated.**

Reason: RDM-004 does not implement semantic retrieval. The repository has no
local embedding provider or semantic index in the inspected surface. Semantic
retrieval is the scope of the conditional follow-up `RDM-004B`, which runs only
when failure analysis identifies relevant evidence missed because of wording
differences.

No semantic extension is implemented under RDM-004.

## Arm P — existing PostgreSQL backend

**Status: not evaluated.**

Reason: no PostgreSQL backend was confirmed available in discovery. Discovery
found no PostgreSQL retrieval adapter in the inspected surface. Arm P is not
run. If a later stage confirms the backend is available, P is recorded with its
retrieval mode explicitly stated.

## Effect on the gate

The pilot gate only requires an evaluated retrieval arm (B) to pass against A.
C and P do not block the decision. Because C is not evaluated, the hybrid-pilot
branch of the decision is unreachable.
