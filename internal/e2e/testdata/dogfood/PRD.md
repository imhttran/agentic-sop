# Dogfood PRD --- URL Shortener

Build a small URL shortener service.

## Requirements

- Accept a long URL and return a short code.
- Resolve a short code to its original URL.
- Persist mappings so they survive a restart.
- Expose an HTTP API for creating and resolving links.
- Include unit tests for the core logic.
- Include integration tests that exercise the HTTP API end to end.
- Package the service as a Docker image.
- Run the test suite in GitHub Actions on every pull request.

## Constraints

- Standard library only; no external runtime dependencies.
- The service must start with a single command.
