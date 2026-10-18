# PLAN — SOP-006 Disposable Rust Workflow

## Project

SOP-006 Rust Demo

## Summary

A disposable Rust project used to demonstrate the candidate SOP binary through
implementation, a controlled acceptance failure, dependency blocking, and a
supported rerun recovery.

## S001 — Implement greet

Implement a Rust library whose `greet` function returns `hello` and whose unit
test executes and passes.

### Dependencies

None

### Deliverables

- `Cargo.toml`
- `src/lib.rs`

### Acceptance Criteria

- greet returns hello
- the Rust test suite executes and passes

### Acceptance Checks

- `cargo test --quiet`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1

## S002 — Implement farewell

Add a `farewell` function returning `goodbye` and extend the Rust test suite to
cover both functions. This stage depends on S001.

### Dependencies

S001

### Deliverables

- `Cargo.toml`
- `src/lib.rs`

### Acceptance Criteria

- farewell returns goodbye
- the full Rust test suite executes and passes

### Acceptance Checks

- `cargo test --quiet`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
