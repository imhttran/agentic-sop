# tunnel-client patch for issue #90 (proposed, not applied)

A proposed fix for the intermittent HTTP 409 / `-32002` / `mcp_initialization_required`
in `openai/tunnel-client`, tracked in [#90](https://github.com/imhttran/agentic-sop/issues/90).
This is a patch for review only. It is not applied to any installed binary or running
tunnel, and #90 stays open until a controlled live reproduction confirms the cause.

- Baseline: `openai/tunnel-client` @ `5f99daabd4aa4a77049e6d81d54a0d8c18335397`
  (the installed revision).
- Patch: [`0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch`](0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch)
  (commit `0372c1fc`, 2 files, +193 / -8).

## What it changes
The stdio initialization guard keeps one readiness flag for the shared child, so two
paths could reject another session's calls:

1. An accepted `initialize` always cleared readiness until `notifications/initialized`
   arrived. A ready child now stays ready; an unready child still needs the full
   handshake. A server that rejects a duplicate `initialize` (go-sdk does) no longer
   leaves the child unready until restart.
2. A late `Close()` of a connection whose request already finished reset readiness and
   closed the shared pipes under a sibling request. That is now a no-op for the
   guard-enabled stdio transport. A `Close()` with the slot held, abandoned requests and
   fatal I/O still reset and close.

## Verification (run in a scratch checkout)
- Against the pinned source, four new regression tests fail; with the patch they pass.
  Three more tests pin behavior that is preserved.
- `go test -race ./...` shows no failures; the initialization tests pass with
  `-count=5 -race`.
- Not done: a live end-to-end test.

## Apply (to a local clone, not the installed binary)
```sh
git clone https://github.com/openai/tunnel-client && cd tunnel-client
git checkout 5f99daabd4aa4a77049e6d81d54a0d8c18335397
git am /path/to/0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch
go test -race ./pkg/mcpclient/...
```

## Known risks
- A re-initialize no longer revokes readiness, so a server that truly resets on one would
  answer operational calls with its own error instead of the tunnel's 409.
- The MCP specification text could not be fetched; the protocol position rests on the
  go-sdk v1.7.0 reference server.
- Neither mechanism is confirmed as the cause of the original live 409.
