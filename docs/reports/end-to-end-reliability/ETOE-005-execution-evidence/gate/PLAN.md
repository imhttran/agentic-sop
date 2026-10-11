# PLAN — ETOE-005 gate fixture

## Project

ETOE-005 gate fixture

## Summary

Deterministic human-gate scenario: the controlled provider returns a destructive,
authorization-required outcome, so SOP must stop at a genuine approval gate.

## FIX-003 — Human-gated fixture task

Reach the human approval boundary. The controlled provider declares a destructive,
authorization-required operation, so SOP records a pending human approval and
stops without approving it.

### Dependencies

None

### Acceptance Criteria

- SOP records a pending human approval for the task and does not approve it.
