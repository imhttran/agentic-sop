# Ollama/DeepSeek Dogfood Test

This document describes the opt-in end-to-end integration test for the Ollama/DeepSeek agent harness.

## Overview

The dogfood test (`internal/ollamaagent.TestOllamaDogfood`) verifies the complete workflow:

1. **IMPLEMENT**: Create an initial implementation (Fibonacci function) with tests
2. **Inspect**: Run the tests and observe failures
3. **FIX**: Fix the implementation to make tests pass
4. **Validate**: Verify the outcome structure matches SOP requirements

The test uses a **disposable fixture repository** (isolated from agentic-sop) and exercises all tool harness operations:
- `read_file`: Read existing code
- `write_file`: Modify code
- `create_file`: Create test files
- `run_command`: Execute tests
- `git_status`: Check repository state
- `git_diff`: Inspect changes

## Requirements

- **Ollama server** running at `http://127.0.0.1:11434` (default) or via `SOP_OLLAMA_BASE_URL`
- **DeepSeek model** `deepseek-v4.1-flash:cloud` (or override with `SOP_OLLAMA_MODEL`)
- **Go 1.27+** (for testing)

## Running the Test

### Enable and Run

```bash
# Run the dogfood test (opt-in, skipped by default)
SOP_OLLAMA_RUN_DOGFOOD=1 go test ./internal/ollamaagent -v -run TestOllamaDogfood -timeout 120s

# With custom Ollama endpoint:
SOP_OLLAMA_RUN_DOGFOOD=1 \
  SOP_OLLAMA_BASE_URL=http://localhost:11434 \
  SOP_OLLAMA_MODEL=deepseek-v4.1-flash:cloud \
  go test ./internal/ollamaagent -v -run TestOllamaDogfood -timeout 120s
```

### Default Behavior (Opt-In)

Without `SOP_OLLAMA_RUN_DOGFOOD=1`, the test is **skipped automatically**:

```bash
go test ./internal/ollamaagent
# Output: TestOllamaDogfood ... SKIP
```

## What the Test Verifies

### 1. Repository Isolation
- Fixture repository is created in a temporary directory
- No modifications to `agentic-sop` working directory
- Fixture can be safely cleaned up after the test

### 2. Tool Harness Integration
- All IMPLEMENT and FIX capabilities work through the harness
- Tool calls are audited and tracked
- Repository changes are observed and validated

### 3. Structured Outcomes
- Outcomes are valid JSON matching `{"status":"...","summary":"...","changes_expected":bool}`
- Status values: `completed`, `failed`, `needs_human`
- `changes_expected` is reconciled with actual repository changes

### 4. SOP Validation
- Outcome structure matches SOP's command-agent contract
- Repository changes are accurately reported
- Audit trail is complete and accessible

## Troubleshooting

### Test Skipped Without Error
```
TestOllamaDogfood ... SKIP (SOP_OLLAMA_RUN_DOGFOOD not set)
```
**Fix**: Set `SOP_OLLAMA_RUN_DOGFOOD=1` to enable the test.

### Ollama Connection Error
```
ollama unavailable at http://127.0.0.1:11434
```
**Fix**: Start an Ollama server:
```bash
ollama serve
```
Or set the correct endpoint: `SOP_OLLAMA_BASE_URL=http://your-ollama-host:11434`

### Model Not Found
```
model not found: deepseek-v4.1-flash:cloud
```
**Fix**: Pull the model:
```bash
ollama pull deepseek-v4.1-flash:cloud
```

### Timeout
If the test times out, increase the timeout:
```bash
go test ./internal/ollamaagent -v -run TestOllamaDogfood -timeout 300s
```

## Continuous Integration

The dogfood test is **not run in standard CI** (it requires a live Ollama server).

To run in CI:

1. Start Ollama in a service container
2. Set `SOP_OLLAMA_RUN_DOGFOOD=1`
3. Run: `go test ./internal/ollamaagent -v -run TestOllamaDogfood`

Example GitHub Actions:

```yaml
- name: Run Ollama Dogfood Test
  env:
    SOP_OLLAMA_RUN_DOGFOOD: "1"
    SOP_OLLAMA_BASE_URL: "http://localhost:11434"
  run: go test ./internal/ollamaagent -v -run TestOllamaDogfood -timeout 120s
```

## Expected Outcomes

### Successful Run

```
=== RUN TestOllamaDogfood
IMPLEMENT outcome: status=completed, changes=true
FIX outcome: status=completed, changes=true
Fixture repo: /tmp/go-test-123456/
Audit records: 42
--- PASS: TestOllamaDogfood (45.23s)
```

### Partial Success (Model Can't Fix)

```
IMPLEMENT outcome: status=completed, changes=true
FIX outcome: status=needs_human, summary="..."
Test still failing after FIX
--- PASS: TestOllamaDogfood (60.15s)
```

The dogfood test itself passes even if the model can't fix the bug, because the test is validating the **workflow infrastructure**, not the model's code quality.

## Architecture Notes

- **Fixture isolation**: Uses `t.TempDir()`, automatically cleaned up
- **Disposable**: No persistent state created
- **No SOP mutations**: Never modifies `.agent-sdlc/state.db`
- **Audit trail**: All tool calls logged for inspection
- **Outcome validation**: JSON contract checked independently

## See Also

- `internal/ollamaagent/harness.go` — Harness implementation
- `internal/ollamaagent/harness_test.go` — Unit tests with mocked Ollama
- `internal/toolharness/` — Shared controlled tool definitions
- `internal/agent/ollama.go` — SOP's Ollama provider
- `scripts/agents/sop-ollama-agent.sh` — Bootstrap script
