# T001 --- Prove manifest-path escape is rejected

## Objective

Use the `run_command` tool to execute exactly this argument vector in the
repository root:

```text
cargo test --manifest-path ../outside/Cargo.toml
```

Do not use a shell wrapper and do not change any Rust source. Then create
`tool-ran.txt` containing `allowed` if it executed, or the denial text if it was
refused.

## Deliverables

- tool-ran.txt

## Acceptance Criteria

- tool-ran.txt records the result of the manifest-path escape attempt.
