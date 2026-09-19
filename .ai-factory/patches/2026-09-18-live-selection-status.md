# Live panel selection status

## Cause
Held Gallery selection remained local until release. Go additionally rebuilt an
ID lookup across every row per selection action and invalidated status totals.

## Change
- F4 opts into 16 ms coalesced sparse selection transactions without ending the gesture.
- Original gesture baseline survives acknowledgements and reversal; unsent toggles coalesce.
- Source-index hints are checked against stable identities in Go, avoiding full-directory ID hashing.
- Warm selected counts and sizes update per changed row.
- Existing panel state and selection patches return authoritative totals without scene/catalog replacement.

## Verification
- Before fix: controller live-flush regression failed (missing API); cached-total regression failed (cache remained at revision zero).
- Go panel/nativeui/plughost suites pass; stale hint/revision, totals, sparse projection and wire tests pass.
- Gallery controller, held Insert in all five modes (live/deferred), Shift range and page navigation pass.
- 100 compact QML status updates retain content and geometry with zero sceneChanged signals: p95 448 microseconds in the test fixture.
- Warm Go transaction + status benchmark: 3.12 microseconds at 100 rows; 2.94 microseconds at 100,000 rows. These are component benchmarks, not end-to-end input latency.
- Representative one-row selection + totals payload: 519 bytes across two panel-only patches.
- 175% status leaf positions/transforms and rendered capture checked; static resource and portable Windows import audits pass.
- Full bridge suite: 55 pass, 3 fail, 1 skip. Rebuilt without this change and reproduced all three failures: panelCatalogPatchLeavesOtherSessionUntouched, inactiveGalleryDoesNotStealFocus, viewerOwnsEscapeAndZoom. Focused selection bridge tests pass.
