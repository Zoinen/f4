# FISH+ shared-session binary cancellation

## Report

Opening a remote video locally, downloading and switching tabs produced
`fishplus: session is out of sync`.

## Root cause

The session correctly serialized requests but did not consistently preserve
response boundaries on cancellation. `readFullCtx` closed the transport on
any cancellation, including a cancelled preview using the download's session.
When cancellation occurred during the header read, `readResponse` discarded
the already-consumed frame header and scanned raw file bytes as protocol lines.
A short caller deadline (Quick View uses 500 ms) also immediately poisoned
an otherwise responsive session.

## Regression-first evidence

`TestCancelledBinaryResponsePreservesSharedSession` uses an in-memory peer and
`testing/synctest` to cancel a preview at the header and mid-payload while a
download waits on the same session. The binary payload contains fake framing
markers; the queued download must receive exactly its own `movie` bytes.
Before implementation the header case timed out and the payload case reported
a closed pipe. The two deadline variants also failed before the deadline fix.

## Changes

- Finish the in-flight binary frame within the existing drain grace period,
  retaining exclusive reader ownership, then drain the response terminator.
- If cancellation arrives with a consumed frame header, discard exactly the
  announced number of bytes before scanning for the terminator.
- During normal responses, translate caller deadline expiry into cancellation
  of the reader while retaining the original deadline error for the caller.
  Handshake deadlines and transport backstops remain unchanged.
- Add optional standard-library debug logging for binary drain/timeout.
- Verify an unfinished frame still closes on grace expiry or explicit Close;
  all synthetic-test goroutines must terminate before the test returns.

## Verification

- Binary cancellation/deadline/timeout/Close regressions: 50 repeated passes.
- Entire FISH+ and NetFox test suites passed.
- Related remote execution, local-open, progress and document-loading tests passed.
- Architecture and command-palette coverage checks passed.
- Rebuilt the embedded-Qt static Go launcher; Linux static-launcher audit passed.

The user's live Windows connection was not automated, so the exact UI sequence
still needs a retry in the rebuilt application. No remote writes or automatic
reconnect/replay were introduced.

## Prevention

Cancellation coverage must interrupt actual partial frames, not only cancel
between complete reads. Always prove a subsequent request's byte integrity,
including marker-like bytes inside the discarded payload. Further coverage
could exercise the tab-switch sequence against a Windows SSH fixture.

Tags: fishplus, cancellation, deadline, shared-session, binary-framing
