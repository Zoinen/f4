# Coverage: `internal/ttyx`

## Status

The first coverage slice adds deterministic lifecycle and no-display checks for
`Overlay`. These checks cover the public guard paths that do not require an X11
server; the existing X11-backed tests remain responsible for server behavior.

The second slice adds deterministic checks for source labels, cached session
queries, and process-id matching. It also does not require an X11 server.

The package's broader coverage work remains outside this slice.
