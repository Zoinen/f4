# Middle-button autoscroll on first traversal, 2026-10-17

The backward-reset bug is fixed. Cold rendering is still not consistently within
the 6.25 ms budget for 160 Hz.

## Reproduction and cause

The previous fast captures already contained position reversals that the
stationary-frame check missed. `middle-final-fast-prime-masonry.json` contains
227.79 -> 0 and 1270.38 -> 0 transitions while the downward gesture remained
active; `middle-final-fast-warm-masonry.json` also resets from 2126.79 to about 0.

`GalleryViewportController.scheduleViewportUpdate()` protected the wheel's
NumberAnimation but not middle-button scrolling. A late content-height/layout
notification scheduled viewport reconciliation, which restored the pre-gesture
saved offset. The cursor did not move, so this was an obsolete restore.

The same deferral now covers the middle-button gesture. Starting it cancels
queued initial restores. Ending it first publishes the actual viewport offset
and then processes deferred reconciliation. The timer also checks ownership
before applying a restore queued earlier in the event loop.

## Verification

The regression uses real QTest middle-click and pointer motion of one quarter
of the screen height. It triggers the actual content-height reconciliation path
while moving, asserts monotonic scrolling, and checks the stopped/persisted
offset. Its original Masonry failure was 572.533 -> 0. The pre-fix run reported
eight failures; the Columns fixture did not change its horizontal extent with
the vertical padding update. Both queued and late-update variants pass now.

Final DPR-1.75 targeted runs: native modes 25 passed, software modes 24 passed,
renderer/gesture lifecycle 6 passed, isolated imports/production-host startup
4 passed. Native motion tests include actual text/image scene origins, unit
transforms, and rendered output at rest. Portable Windows import auditing and
embedded payload/extraction tests passed. These targeted results do not claim
that the previously documented unrelated full-suite failures were resolved.

## Live results and remaining limitation

The actual folder was `C:\Users\xs\Downloads`, 183 entries, density 152, DPR 1.75,
on the 160 Hz display. A 300-logical-pixel pointer displacement is approximately
one quarter of the 1186-logical-pixel maximized window height. Each run uses the
real autoscroll controller and FrameAnimation, four seconds down and four up.
Fresh-process captures create a fresh presentation but retain the normal disk
thumbnail cache. Builds/tests did not run during the captures.

`middle-quarter-first-masonry` reaches the full folder extent with zero reverse
transitions. It displayed 620/636 presents, none discarded. Display-interval p99
was 12.5005/6.3030 ms, with 14/1 intervals above 9.375 ms. Native frame gaps
reached 27.5089 ms; render CPU reached 14.5997 ms.

`middle-quarter-warm-masonry` also has no reversals: 637/637 displayed presents,
none discarded, median 6.25 ms, p99 6.2901/6.3000 ms, and no displayed interval
over 9.375 ms. Caption measurement count is zero in both runs.

Native dirty-node tracing attributes the cold complete/full rebuilds to the SVG
branch. ETW CPU bursts include `QSGBatchRenderer::Renderer::uploadBatch`,
`Renderer::unmap`, `QD3D11Buffer::create`, and buffer copies. This is evidence
of geometry upload/allocation work; it does not establish that one allocation
policy alone is the root cause. The CPU trace includes recorder overhead and
the diagnostic host's PNG captures; it is used for attribution, not FPS claims.

Experiments with early SVG branch/clip placement, a 16 MiB buffer reuse pool,
geometric buffer capacity, and initial dynamic buffers did not materially
improve the cold display cadence. They are excluded from the product change.
Only the verified restoration fix is added in this follow-up. Further work
must reduce/reuse cold scene geometry uploads without delaying requested
previews or changing vector rendering into raster rendering.

## Rollout

The normal static host and embedded Go executable were rebuilt, and the
worktree app was restarted directly with `--gui=qt`. Diagnostic renderer
overrides and the experimental buffer-pool environment setting were cleared.
The running host must match SHA-256
`879adeba505c73b039c89f1bdb18208c5b9e2e3747307615d30caab51e547259`;
the gzip payload/cache key is
`14e8d5faae10865600aed03c6a3248007198ee146d5438c34f5bfe37e2537aa2`.
Identity and capture evidence use the `artifacts/middle-quarter-*` prefix.

AI assistance has been used to create this output.
