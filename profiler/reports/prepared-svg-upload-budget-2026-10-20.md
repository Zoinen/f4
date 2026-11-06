# Budget reconstructed SVG batches without flashing cached surfaces

2026-10-20. Goal: constant 160 Hz without spikes. **Not achieved.**
This turn fixes a verified source-readiness/GPU-residency shortcut, adds regressions,
measures two corrected candidate runs, and replaces only this worktree's Qt app.
No commit or push was made. Native scrolling remains disabled by default.

## Implemented behavior

The renderer no longer treats previouslyPrepared as proof that its current batches
are resident. A subtree restored after culling enters the same bounded upload queue
as fresh geometry (existing 1 MiB / 32-batch / 2 ms soft limits; one oversize batch
may progress alone). Stable resident animation behavior remains unchanged.

GalleryVectorUploadGate retains its render-thread progress observer after source
readiness. Pending uploads request repaint directly, without per-frame GUI progress
notifications. A completion requests the presentation frame it needs, then stops.
Source replacement reconnects GUI readiness publication as necessary.

When the final missing upload is admitted, the renderer can prepare the complete
surface in that frame. Candidate draw lists are filtered only after all preparation
results are known. Incomplete/failed surfaces do not leak partial drawing, and the
original opaque/alpha painter ordering is preserved. This also allows a cached small
SVG to return immediately without an extra background frame.

## Regressions and verification

Before the renderer change, prepared re-entry uploaded all24 fixture batches in one
frame on both D3D11 and OpenGL (byte budget permits at most5). Before the host progress
change, the integration fixture stalled at5/24 resident batches after re-entry with
no further GUI input. Both regressions pass after the changes.

The first candidate introduced an empty frame on cached return. The existing
vectorPanelSceneCacheReusesAndBoundsSurfaces test caught it on both backends. That
candidate was rolled back from the running app. The general complete-surface handoff
fixed the regression without weakening the cache or all-or-nothing rendering tests.

Final corrected version:

- Renderer suite:33 checks per D3D11/OpenGL backend,0 failures or skips, including
  batch ordering, overlap, buffer reuse, partial-admission invisibility, priority,
  oversize/empty progress, cancellation and re-entry.
- Gallery integration:12 checks per backend,0 failures or skips, including autonomous
  re-entry, cache identity/render reuse, viewer swaps, gradients/embedded images,
  and Details/Columns/Grid/Icons/Masonry at175%.
- Native camera suite:9 checks per backend,0 failures or skips, including retarget,
  geometry rebases, endpoint clamping, pixel-grid leaves/transforms and rendered
  native/GUI equivalence. Software fallback passed too.
- CI source-patch tests:31 passed. The serialized strict CI gate policy was compared
  directly with the tested renderer source.
- Resource-only import isolation and Windows system-DLL audit passed. Payload extraction
  tests and CGO_ENABLED=0 launcher build passed using the system Go cache.

One early test runner accidentally produced DPR3.0625 by multiplying system scale;
the corrected runner disables automatic high-DPI scaling and asserts actual1.75.
Both the failing-before re-entry case and passing renderer suite were rerun at1.75.
Disk exhaustion also interrupted CMake generation; verified lossless map compression
recovered space and regeneration/build subsequently completed.

## Measurements

Real production GalleryScrollAnimation controller, Downloads186 entries, Masonry
152 density, DPR1.75, screen160Hz,30ms wheel interval, default GC5000us, diagnostic
pixmap cap2MiB. Cold/warm each contain two10-second native input windows. General
frame recorder disabled; bounded native probes enabled. No build/test/analysis or
UI Automation ran during input windows. These timings describe the experimental
native controller, not a claim about the normal default GUI scrolling path.

The final versions are real-scroll-prepared-v2 and its repeat. The three earlier
prepared-budget runs belong to the rejected first handoff and are retained as
historical evidence, including the severe warm outlier. Its cause is not established.

| Capture (40 seconds each) | Producer gaps >9.375 ms | Preparation max (ms) | Display gaps >9.375 ms | Display max (ms) |
|---|---:|---:|---:|---:|
| real-scroll-upload-identities | 65 | 7.0475 | 0 | 6.4346 |
| real-scroll-prepared-budget | 23 | 5.1349 | 0 | 6.4905 |
| real-scroll-prepared-budget-repeat | 111 | 13.6588 | 48 | 24.9839 |
| real-scroll-prepared-budget-confirm | 62 | 5.3342 | 0 | 6.4396 |
| real-scroll-prepared-v2 | 22 | 4.3073 | 0 | 6.4956 |
| real-scroll-prepared-v2-repeat | 14 | 6.0934 | 0 | 6.3674 |

Producer intervals are counted only when both endpoints lie inside the input window.
The original phase-attribution list included3 boundary-crossing idle intervals in
each40-second capture. Reusing the existing within_producer_window predicate corrected
those counts; displayed-frame analysis already had the correct endpoint checks.
Earlier chat counts68/25/17 therefore become65/22/14. Preparation and display maxima
are unaffected. Immutable earlier reports are not silently rewritten.

Final v2 samples show0 ready-and-present SVG slow uploads in both cold/warm windows,
where the baseline had221 cold and239 warm bypass uploads. Similar geometry bytes
still transfer (about166–169MB per20-second stage); this is scheduling,
not a claim to remove source reads or geometry traffic.

Both final captures have0 displayed intervals above9.375ms. Nevertheless they retain
producer gaps, and one final run records4.4242ms command recording /9.2436ms total
render-to-frame-end. The repeat reaches6.0934ms preparation /9.1198ms render-to-frame-end.
The original first candidate's unexplained broad stall reached13.6588ms preparation
and24.9839ms displayed intervals. Do not discard that outlier or claim constant160Hz.
Next attribution should distinguish CPU work/preemption from RHI recording/submission
waits, then address remaining ordinary uploads and verify the normal rollout path.

## Deployed Windows worktree build

Only D:/Code/f4-zoin/f4-zoin.exe was replaced and launched directly with --gui=qt.
No F4_EXT_UI_PATH override is used. The previous working launcher remains backed up.

Launcher SHA256: `dd796d6bbe276143b7f738ef53ddd2d61586be9cbf091344adf75b1ccdc952a9`.

Host SHA256: `20398cb670603337c2fe9bba2f837711135380c3bc050f6fbd99e11dc5d67ec8`.

Live launcher PID80040, Qt PID120840; responsive window verified.
