# Preserve running thumbnails across relayout, 2026-10-31

## Result and remaining goal

A local thumbnail relayout was cancelling running read/decode work and its
shared-cache owner admission. Large PNGs repeatedly restarted without ever
publishing. The corrected path cancels only queued panel-thumbnail jobs,
releases those exact admissions, and lets the configured bounded workers
finish their existing immutable tiers. New viewport/viewer priorities remain
under the existing demand scheduler. Source invalidation, disabling previews
and shutdown retain their existing cancellation paths. Expensive-source
preservation remains unchanged.

The first corrected cold capture eliminates recurring reverse-pass thumbnail
work and markedly improves its observed timing. It does **not** achieve the
full goal: both candidate windows fail unchanged motion acceptance; the first
also has two missed display intervals. This is one baseline/candidate sequence,
not replicated ABBA evidence of a guaranteed frame-time gain. Verified warm
and quarter-height middle-motion checks and a portable application rollout
remain outstanding.

## Caller attribution

Two bounded observational builds extend the preceding per-source observer.
The first records the return address inside the two-argument cache cancellation
method. It correctly identifies the one-argument `cancelRequests(owner)`
wrapper, rather than the policy caller. The second propagates the wrapper's
caller through a nested, thread-local RAII scope. It preserves the direct caller
for calls which bypass the wrapper. Addresses are image-relative RVAs, resolved
against each exact linked executable's map and independently checked against
its PE runtime-function boundaries. No map from another build is reused.

In `real-scroll-postsync-thumbnailcallerscopecold-b1-20261031-wheel-masonry`,
every measured cancellation resolves to
`ExternalCatalogModel::cancelAllDecodeRunners()`: 462 events in the first
window and 371 in the reverse window. All reverse cancellations concern seven
repeated sources. The preceding direct-wrapper capture independently observes
437/280 cancellations through the panel-wide wrapper. This rules out the
targeted source-invalidation overload as the measured caller.

Caller observation adds no product cancellation-policy change. Both builds
pass the helper/cache/renderer/motion/import/dependency checks. The final scope
helper has 45 test cases, including nested scope restoration and independence
between threads. Thirty rendered images match the prior baseline.

## Reproduce before fixing

The existing network single-flight test in
`third_party/ZoinGallery/tests/GallerySessionTest.cpp` now also has a direct-local
data row. The identical extracted test body was compiled against the untouched
cancellation baseline. The network row passes; the local row fails because
`provider->materializations` is 2 rather than 1 after a single relayout.
The original failing executable, output, extracted-body hash and link response
are retained in `thumbnail-relayout-regression-20261031`.

Only after observing that failure was the policy changed. The same test then
passes for both local and network sources. An additional public-API regression
blocks three actual running source requests, queues an obsolete same-panel job,
another panel's job and a viewer job, then replaces the panel's queue. It
verifies the exact removed source/target/transform, empty-namespace no-op,
idempotence, retention of all three running requests, and eventual completion
of the viewer and other panel. Five QtTest cases pass with no skips.

## Implementation

- `DecodeManager::cancelQueuedPanelThumbnailRequests()` inspects only its
  queue, removes matching panel-thumbnail read/decode/cache/video runners and
  returns their complete request descriptors. It neither cancels workers nor
  starts the old queue while the viewport lease is being replaced.
- The local branch of `ExternalCatalogModel::cancelAllDecodeRunners()` releases
  exactly those returned cache admissions, preserving active ownership. Stale
  model waiters are cleared as before; a fresh request coalesces with running
  work and immutable completion can populate a covering tier.
- No new rendering/layout arithmetic, per-frame rounding, queue thread or
  persistent setting is introduced. No new QML, Go protocol or Qt SDK patch
  belongs to this fix.

The corrected diagnostic host also passes 46 native renderer cases, seven
motion cases, three isolated-resource import checks and the system-DLL audit.
All 30 native/motion PNGs match the preceding scope baseline byte-for-byte.
The portable Go payload has not yet been rebuilt; the normal app remains the
last working build. Diagnostic hosts were launched directly through their
explicit worktree-owned host override, then stopped by exact path/ancestry.

## Live measurements

Real `C:\Users\xs\Downloads`, 192 entries including parent, Masonry density
152, actual DPR 1.75, 160 Hz, isolated cold cache, normal initial viewport,
frame latency 2. Native foreground remains stable for all measurement windows.
Prepared-root retention is off, original pipeline-key mode is retained,
typed registry and uniform word hash are on. No source/event capacity overflow
or scene-registry mismatch was accepted.

Baseline: `real-scroll-postsync-thumbnailcallerscopecold-b1-20261031-wheel-masonry`.
Candidate: `real-scroll-postsync-thumbnailrelayoutcold-b1-20261031-wheel-masonry`.

| Case/window | New requests | Panel-wide cancels | Exact queued releases | New publications | Preparation p99 | Missed display intervals | Max motion error |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Baseline first | 515 | 462 | 0 | 57 | 3.3565 ms | 94 | 122.1275 px |
| Baseline reverse | 378 | 371 | 0 | 0 | 3.2893 ms | 110 | 215.4399 px |
| Candidate first | 146 | 0 | 87 | 61 | 2.0674 ms | 2 | 98.7242 px |
| Candidate reverse | 0 | 0 | 0 | 0 | 1.6548 ms | 0 | 3.4156 px |

Counts are timestamped within each 30-second measurement window; requests can
cross boundaries. The candidate first window also has 12 pending coalescences
and five reused publications. Candidate reverse has no timestamped non-hit
cache event of any kind. Hits are aggregated over the full capture, so the
absence of event rows does not mean the cache was not consulted.

Coverage holds are zero in every window. Acceptance remains maximum display
interval 9.375 ms, maximum physical motion error 2 px, coverage holds zero.
Candidate display maxima are 12.5091 ms / 6.4739 ms. The 98.7242 px cold error
is around 1.056 seconds into the first window; it and the following compensating
step remain included. No missing-frame pair or outlier was dropped to claim a
pass. The independent native frame-duration maxima are 12.2732 / 9.6384 ms;
these are a different clock/metric from display intervals.

The next useful target is the residual cold delivery jump and remaining
warm-motion error, followed by replicated cold/warm/middle acceptance. Retaining
the running-job fix is supported by the deterministic regression and source
trace; its exact throughput benefit still needs repetition.

## Evidence handling

Large packets and binaries remain outside the compact report evidence. Owned
completed maps, raw packets and diagnostic executables were NTFS-compressed
to preserve limited disk space, with full SHA-256 equality checked before and
after. No evidence was deleted and no system storage/memory setting changed.
The normal Go/Qt process paths remain unchanged. No commit or push was made.

Compact evidence is frozen in
`profiler/experiments/masonry-running-thumbnail-preservation-2026-10-31/`:
452 source/script/result/image files and 48 verified external receipts.
Whitespace verification passes for all five changed cancellation/test files.
