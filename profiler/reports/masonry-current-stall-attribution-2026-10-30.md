# Current Masonry renderer: CPU and synchronization stalls

## Outcome

A fresh stack/scheduler capture of the current diagnostic renderer establishes
that its scene preparation is overwhelmingly scheduled work, rather than time
waiting to run. It also exposes a distinct QObject mutex wait during image-node
synchronization. Reducing only the longest preparation outliers will not remove
all display misses. Keep the frame queue default and the original acceptance
gates. No production source, Qt SDK, or embedded payload was changed.

## Capture and decoder validation

Case: `real-scroll-stallcpu-a1-20261030-wheel-masonry`. Same pinned
`f4-qt-host-overlay-empty-fastpath-20261029.exe`, preprocessing dependency cache,
overlay fast path, default frame-latency limit 2, real Downloads folder,
192 catalog entries, density 152, actual DPR 1.75 and 160 Hz at 3840×2160.
Fresh isolated cache, two ten-second wheel windows, 1679 wheel events and zero
inactive events. Native foreground ownership remains stable in both windows.
Original capture-validity checks pass. The second direction is not verified-warm.

An owned kernel ETW session records Profile, CSwitch and ReadyThread stacks.
The session was stopped after the workload, with zero event or buffer loss and
zero decoder errors. Its 837,550,080-byte raw trace remains on C:, SHA-256
`7dc5c914b3a1c6eb9bec2d1b8cee0eb44b6d7d95b3e94fcbee4896cc97cc95e3`.
Self-tests cover stack/sample prefixes; all 222 selected CPU samples independently
match xperf's decoder by thread, instruction pointer and timestamp. This matters
because the installed TDH schema needs the validated SampledProfile fallback.

An independently relinked map has exactly matching `.text` and `.pdata` sections
and image base compared with the captured executable. Symbolization uses this
map and unwind ranges, preserving identical-code-folding aliases. It does not
reuse the previous renderer's addresses. Preparation has 2651 sampled IPs,
one unmatched stack; GUI analysis has 6286 samples, five unmatched stacks.

This is intrusive attribution, not a claim of uninstrumented throughput.
No UI Automation or screenshots ran during gestures. The trace still fails
the original display/motion gates: blocks have 58/1 intervals above 9.375 ms,
maxima 18.7449/12.5076 ms, maximum motion errors 114.70/7.35 physical pixels,
and zero coverage holds. Full cold, verified-warm and middle coverage remains due.

## Preparation costs

Across 3195 measured frames, preparation totals 2770.2314 ms: 2765.8216 ms
scheduled, 2.5893 ms runnable, 1.8205 ms blocked, zero unknown. Scheduled time
is 99.84%; it includes possible interrupt/DPC interruption and must not be
called pure executed CPU instructions.

The worst preparation is 5.9664 ms, with 4.7732 ms scheduled, 1.0531 ms blocked
and 0.1401 ms runnable. Its before-renderer-body work is 5.2988 ms. Another
5.8944-ms preparation has 5.8920 ms scheduled and no blocked time, spread across
uploads 1.7572, before-body 1.2480, alpha materials 1.1209 and render lists
1.0160 ms. A third 5.6833-ms pass spends 2.9858 ms in uploads. There is no
single driver wait that explains all these costs.

Inclusive stack samples identify these concrete paths:

- Before-body: batch updater traversal (302 samples), preprocessing eligibility
  (146), dependency path registration (100), and associated allocations.
- Uploads: `uploadBatch` (504), `unmap` (359), dynamic-buffer update (240),
  D3D11 map (193), including driver work. These counts overlap along call stacks.
- Render lists: tagged-root rebuild (259), unmerged sequence construction (64),
  hash insertion and batch sorting. The sequence snapshot builds full element
  position maps before and after list updates; ordering safeguards remain required.
- Alpha materials: merged-batch preparation (688), dynamic material data (520),
  `QRhiBufferData::assign` (109), pipeline-key construction and resource bindings.

Node addition/removal counters must not be interpreted as allocations without
checking culling: `DirtySubtreeBlocked` can remove/re-add existing shadow subtrees.
Do not replace vector graphics with textures or lower quality to avoid this work.

## GUI synchronization and image connection wait

The GUI spends 3762.66/2698.89 ms scheduled, 40.57/25.19 ms runnable and
6196.77/7275.93 ms blocked in the two windows. Long waits are rooted in
`QSGThreadedRenderLoop::polishAndSync`, with the render thread doing both work
and waiting. These totals include ordinary pacing, not solely pathological waits.

The largest GUI wait is 16.3553 ms. During it, the render thread is scheduled
6.9495 ms, runnable 0.0398 ms and blocked 9.3660 ms. Render resumption stacks
identify a 3.001-ms begin-frame wait, event-dispatch waits, and a distinct
2.7345-ms `QBasicMutex::lockInternal` wait reached through
`QMetaObjectPrivate::disconnect` / `QObject::disconnectImpl` inside
`QQuickImage::updatePaintNode` during scene synchronization.

Qt 6.11.1 source has a texture-provider connection replacement in
`QQuickImageTextureProvider::updateTexture`: it disconnects the old texture's
destroyed notification when the texture changes. The optimized stack places
that operation in `updatePaintNode`. This identifies a connection-lock path;
it does not prove which texture, connection mutex, or other thread owns the lock.
Removing notification lifetime protection would be an unsafe speculative fix.

The next GUI waits, 15.2144 and 13.0289 ms, overlap two begin-frame waits each,
totalling 9.23 and 9.05 ms blocked. Moving synchronization outside an active
graphics frame would break the already-tested resource-callback contract.

## Cross-check against the four untraced pacing captures

The new correlation uses the original display-window predicate: both endpoints
of each displayed interval must lie inside the measurement window. A first draft
included the idle gap before the first displayed frame; it was corrected before
these counts were used. Missing GUI synchronization phases are omitted, not
represented as zero-duration synchronization.

Of the default-limit arms' 177 display misses, only 29 have preparation above
3 ms in the current or immediately preceding frame. Missed-frame preparation
medians are roughly 1.07–1.53 ms versus 0.48–0.57 ms for normal intervals.
This is temporal association, not proof that a particular preparation frame
caused the display miss. Queueing, recording and synchronization also matter.

## Next implementation targets

1. Remove repeated reflective ancestry work in pixel-grid registration without
   changing alignment. Current GUI samples repeatedly reach
   [PixelGridContext::add](../../third_party/ZoinGallery/src/pixelgrid/PixelGrid.cpp#L154)
   and `isGridBoundary` through `QObject::property` / `indexOfProperty` during
   scope re-registration. Reuse shared watched-node boundary information, with
   explicit invalidation for scope installation and reparenting. Add 175% leaf
   geometry/render regressions and measure registration before promoting it.
2. Avoid redundant sequence/hash construction in tagged-root render-list updates,
   while retaining outsider-interleaving and stale-pointer safety tests.
3. Identify the image texture-provider connection and readier responsible for
   the 2.7345-ms synchronization wait before considering a lifetime-safe fix.
4. Repeat complete original cold/warm/middle gates after any candidate; isolated
   CPU wins or the favorable second direction are insufficient for completion.

The small `rowLoaded` lookup prototype remains separate; 101 GUI self samples
confirm that hotspot but do not make it the explanation for a 16-ms wait.
No change was committed or pushed. Compact scripts, exact-map verification,
decoded accounting, observer receipts and external trace hashes are frozen in
`profiler/experiments/masonry-current-stall-attribution-2026-10-30/`.
