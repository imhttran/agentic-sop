# Implementation Plan

## Project

Provider Integration

## Summary

Add a decision provider behind an existing provider-neutral contract without
granting it any governance authority.

## Planning and Discovery Constraint

Repository discovery performed inside a task is work performed by that task, not a
prerequisite capability. Only externally supplied runtimes, permissions, tools,
artifacts, or already completed dependencies belong in Requires.

## Capabilities

### Go build/test toolchain — EXISTS

Evidence: a working Go toolchain is present in the checkout.

### Provider-neutral decision seam — EXISTS

Evidence: the contract package is present in the repository.

### MLX runtime — UNKNOWN

Evidence: not inspected yet.
Owner: environment.

## PROV-001 — Capture repository truth

Establish the exact current state of the repository.

Inspect and record each of the following from the repository itself:
- the current provider interface;
- existing provider implementations, including Nimble and Julia where present;
- the repository's current provider registration/factory and CLI selection
  mechanism;
- configuration and provider enablement behavior;
- the existing test structure and package layout.

Every item above is a discovery target that this task inspects and records; none is
an externally supplied prerequisite.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/provider/truth.md`

### Acceptance Criteria

- Repository state is recorded.
- The provider interface and request/result types are identified with file/symbol evidence.
- No production code is changed.

## PROV-002 — Verify capability and transport

Determine with evidence which capabilities are available on each candidate runtime.

### Dependencies

- PROV-001

### Requires

- Go build/test toolchain
- Provider-neutral decision seam

### Deliverables

- `docs/reports/provider/transport.md`

### Acceptance Criteria

- Each capability is classified.
- No production provider implementation is created.
