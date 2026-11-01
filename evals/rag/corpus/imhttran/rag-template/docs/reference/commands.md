# Commands

The project is driven by three commands, with convenience targets that wrap the same steps.

## Workflow

Three commands:

| Command      | What it does                                                      |
| ------------ | ----------------------------------------------------------------- |
| `cmd/ingest` | Read a document → split into chunks → embed each chunk → store it |
| `cmd/rag`    | Embed a question → retrieve the closest chunks → answer from them |
| `cmd/eval`   | Score retrieval (recall/precision) and compare a lexical reranker |

## Shortcuts

A `Makefile` wraps the same steps:

| Target             | Runs                                                  |
| ------------------ | ----------------------------------------------------- |
| `make db-up`       | start the database and apply all migrations           |
| `make db-down`     | `docker compose down`                                 |
| `make db-schema`   | apply all migrations (existing volume only)           |
| `make test`        | run the unit tests (no database needed)               |
| `make fmt-json`    | rewrite every tracked JSON file with `jq`             |
| `make integration` | run the integration tests against the database        |
| `make ingest`      | ingest `examples/loan-policy.md` (`FILE=…` to change) |
| `make ask Q="…"`   | ask a question (omit `Q` to be prompted)              |
| `make eval`        | score retrieval against `evals/retrieval.json`        |
