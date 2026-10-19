# tunnel-client patch for issue #90 (proposed, not applied)

A proposed fix for the intermittent HTTP 409 / `-32002` / `mcp_initialization_required`
in `openai/tunnel-client`, tracked in [#90](https://github.com/imhttran/agentic-sop/issues/90).
This is a patch for review only. It is not applied to any installed binary or running
tunnel, and #90 stays open until a controlled live reproduction confirms the cause.

- Baseline: `openai/tunnel-client` @ `5f99daabd4aa4a77049e6d81d54a0d8c18335397`
  (the installed revision).
- Patch: [`0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch`](0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch)
  (commit `c9349dc0`, 2 files, +309 / -10). Revised after review; earlier versions were
  `0372c1fc` and `560e70c3`. The last revision only adds a comment documenting the
  `childEpoch` safety invariant; no runtime behavior changed.

## What it changes
The stdio initialization guard keeps one readiness flag for the shared child, so three
paths could reject another session's calls:

1. An accepted `initialize` always cleared readiness until `notifications/initialized`
   arrived. A ready child now stays ready; an unready child still needs the full
   handshake. A server that rejects a duplicate `initialize` (go-sdk does) no longer
   leaves the child unready until restart.
2. A late `Close()` of a connection whose request already finished reset readiness and
   closed the shared pipes under a sibling request. That is now a no-op for the
   guard-enabled stdio transport.
3. (Added in review.) A late `Close()` of an abandoned connection (read or write failure)
   could reset readiness that by then described a replacement child a sibling had
   re-handshaken. The reset now applies only if no reset happened since that connection
   started (a per-transport child epoch). The destructive close of the pipes is kept, so
   possible late responses are still purged.

A `Close()` with the slot held, in-flight requests and fatal I/O still reset readiness and
close.

## Review findings (second pass)
- **Protocol / server lifecycle (re-initialize keeps readiness).** Checked in source, not in
  the specification text (it could not be fetched):
  - go-sdk v1.7.0 `ServerSession.initialize`: a duplicate `initialize` returns an error and
    the session stays initialized.
  - TypeScript SDK `Server._oninitialize` (9bc40e9): overwrites client capabilities; no
    gate on `initialized`, so requests are accepted throughout.
  - Python SDK (2af9e30) `runner.py` / `connection.initialize_accepted`: the request gate
    opens when `initialize` is recorded, and a repeat does not close it.
  - SOP's `sop mcp` server is stateless.
  None of these reverts to a state where operational requests are invalid after a
  duplicate `initialize`, so keeping readiness matches them. Older SDK versions and other
  servers were not checked.
- **Blocking gap found and fixed:** the first patch left a case where an abandoned
  connection's late `Close()` revoked a sibling's fresh readiness (new test
  `TestStdioInitializationLateCloseOfAbandonedConnectionKeepsReplacementReady` failed on the
  first patch). Item 3 above fixes it.
- **Checked, no change needed:** read errors, write errors, cancellation and concurrent
  ownership keep their prior behavior (abandoned and in-flight requests still reset and
  close; response-deadline retirement still preserves readiness); `lifecycleDone` is cleared
  on every new acquire; `go test -race` found no data race, including a 50-connection
  concurrent late-Close test.

## Verification (run in a scratch checkout)
- Against the pinned source six new tests fail; against the first patch one fails; with
  this patch all pass. Other new tests pin preserved behavior.
- `go test -race ./...` shows no failures; the initialization and serialized tests pass
  with `-count=10 -race`.
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
  answer operational calls with its own error instead of the tunnel's 409. None of the three
  reference SDKs or SOP does.
- A `Close()` of a connection that holds the slot but never wrote still resets readiness
  and closes the shared pipes (upstream pins this in `TestStdioInitializationReadyClearsOnCloseOrFatalIO`).
  Left unchanged; a cancellation between connect and write can still force a child respawn.
- The destructive close of an abandoned connection still closes the shared pipes, which can
  disturb a sibling in the same child; kept to purge late responses.
- Neither mechanism is confirmed as the cause of the original live 409.
