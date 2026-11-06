# Upload-stage attribution and queued geometry experiment

2026-10-29, `zoin_branch/cached-pixel-grid`. Constant 160 Hz without spikes
remains incomplete. This turn adds a bounded native probe, lossless scheduler
attribution and a tested performance experiment. The experiment is not promoted;
the working portable app is unchanged. No commit or push was made.

## What the upload stall actually contains

The prior turn established that stalls persist with detailed RHI recording and
per-ancestor counter writes disabled. `UnmapStages20261029.h` now separates
buffer-pool selection, create/recreate and data-update spans inside the real
renderer `unmap`. Only scopes at least 100 microseconds are published into a
16,384-record pre-touched array. The renderer transformation reverses exactly.
The original Qt node updater and all existing rendering/admission policies stay
in place. Timings include observer overhead; they are wall spans, not GPU time.

Six probe tests pass, including setup/cleanup: inclusive threshold, incomplete
record protection, overflow protection, and ordered live scope metadata.
Four import-isolation checks and the Windows system-DLL-only audit pass.

`real-scroll-unmap-stages-scheduler-20261029-wheel-masonry` uses the actual
Downloads folder (191 entries), Masonry density 152, 3840×2160, 160 Hz, DPR 1.75,
a fresh cache (zd), and two ten-second rapid wheel blocks. Standard content,
source acquisition and hardware media decoding remain enabled. No compiler,
tests, offline analyzer or UI Automation observer runs during input. The owned
kernel trace has zero lost events/buffers or schema/ProcessTrace errors and
covers both complete windows.

The most useful observations are individual **native vertex/index unmap**
scopes, not summed nested upload timings:

| Material / bytes | Whole scope ms | Data update ms | Scheduled / blocked ms | Selection / creation |
| --- | ---: | ---: | --- | --- |
| Smooth color / 2,880 | 4.7635 | 4.7632 | .0439 / 4.7114 | Neither executed; same existing buffer |
| SVG curve stroke / 2,054,448 | 2.5563 | 2.5335 | 2.5563 / 0 | .0004 / .0218 ms |
| SVG curve fill / 619,212 | 1.3519 | 1.3463 | 1.3504 / 0 | .0001 / .0051 ms |
| SVG curve stroke / 1,344,420 | .5551 | .5364 | .5551 / 0 | No pool selection; .0184 ms creation |

The tiny existing-buffer update reproduces a wait without selecting or creating
a resource. The large SVG update tails in this capture are almost entirely
scheduled. These are distinct costs. The trace does not expose the driver lock
owner or establish which D3D map type caused the wait; no such claim is made.

This capture still fails the goal: 6 and 10 missed refreshes, preparation maxima
6.5411 and 7.7684 ms, display maxima 12.6323 and 13.3116 ms. Maximum motion
error is 101.4067 and 116.6061 physical px, with zero coverage holds. A miss is
still a display interval over 9.375 ms; motion still requires maximum 2 px.

## Candidate: enqueue geometry through Qt's resource-update batch

The diagnostic candidate changes only the D3D11 Dynamic vertex/index write
route in `Renderer::unmap`. Instead of the direct full-update shortcut, it uses
the existing `updateDynamicBuffer` resource-update batch. Other backends retain
the original route. It preserves requested bytes, buffer ownership, painter
order, geometry, clipping, source queues and admission. A runtime flag in the
same executable selects candidate/control; no production setting is added.

This is an experiment in submission placement, not an assumption that enqueueing
eliminates the driver's work. Queued geometry bypasses the direct geometry-map
helpers, leaving synchronization to the ordinary QRhi update path.

A native feature-usage test renders 24 consecutive size/color changes at
actual DPR 1.75, checks rendered foreground/background pixels each time and
checks the leaf's scene-space physical origin (14,14) and unit transforms.
The control passes pixel checks but fails the assertion that queued geometry
was exercised. The candidate passes all three results including setup/cleanup.
The final screenshot was inspected. The expanded native suite passes **49
results, zero failures/skips**, including painter order, clipping, allocation,
vector admission and retained-batch cases. Four imports and the DLL audit pass.

The first expanded-suite invocation omitted the native-test opt-in and uniform
capacity flag: it records 6 passes, 1 failure and 42 skips. That incomplete
invocation is retained and is not evidence of passing coverage. The corrected
environment is recorded in `full-native-corrected-tests.txt`.

## Capture qualification and one rejected attempt

The first queued capture (`b1`, cache ze) fails a diagnostic capacity guard.
Its direct first-map proof table fills with entries that the queued path never
consumes (94 overflows). It is excluded from the performance comparison.

The subsequent candidate explicitly disables that now-unused direct first-map
tracker. It asserts zero tracker devices/maps, zero geometry-reuse direct maps,
and more than 1,000 actual queued updates. The control retains its original
direct first-map policy and asserts its use. This is a documented diagnostic
configuration difference; it does not remove a correctness guard from an API
that still uses it. Both paths retain all other feature/error/capacity guards.

Both candidate and control use the same executable, original node updater,
disabled detailed RHI recorder, bounded unmap observer, and normal scrollbar
viewport/persistence path. Fresh application caches are used; OS caches are
not flushed. No ETW scheduler recording runs during these four comparisons.
Only the separate attribution capture above has a scheduler trace.

## Four valid live comparisons: no reliable display improvement

Order is candidate / control / control / candidate. The repeated candidate is
`b1-r2`; subsequent arms are `a1`, `a2`, `b2`. Full capture stems are
`real-scroll-deferred-geometry-<arm>-20261029-wheel-masonry`; caches are zf, zg,
zh and zi. All pass activity, target PID, actual display, real input, capacity,
source/feature-use and error guards. Each has two ten-second input blocks.

| Arm | Preparation p99, blocks 0 / 1 ms | Preparation maximum, blocks 0 / 1 ms | Misses, blocks 0 / 1 | Display maximum, blocks 0 / 1 ms |
| --- | --- | --- | --- | --- |
| Candidate b1-r2 | 3.5455 / 3.5223 | 7.6944 / 6.7301 | 13 / 4 | 18.7632 / 12.6777 |
| Control a1 | 3.4403 / 3.7673 | 5.9943 / 11.5789 | 9 / 3 | 12.5571 / 12.5698 |
| Control a2 | 3.2146 / 4.1087 | 6.3973 / 12.6146 | 7 / 11 | 12.5953 / 18.7384 |
| Candidate b2 | 3.0250 / 3.2433 | 6.1702 / 7.7678 | 4 / 7 | 12.5270 / 12.5226 |

Candidate totals are 17 and 11 misses versus control totals 12 and 18. Some
preparation percentiles improve, but display tails are mixed; these runs do
not establish a repeatable display benefit. All eight blocks fail maximum
motion error (64.17–110.30 px); all have zero coverage holds. The strict gate
is unchanged. No warm-only subset is substituted for these cold/loading results.

The first candidate's largest preparation span has only .6528 ms in uploads
but 2.5412 ms alpha-material preparation and 1.7583 ms render-list construction.
The second candidate's largest span has .7693 ms uploads, 2.5958 ms render lists
and 2.0852 ms before the renderer body. One control frame instead contains
9.7344 ms alpha-material preparation. Those comparison frames lack scheduler
traces, so their running/wait split is unknown. Moving geometry writes does
not remove the other preparation and submission costs already observed.

## Outcome and next constraints

The queued-geometry experiment remains unpromoted. The next optimization should
address measured repeated scene preparation and material/resource setup, while
keeping occasional update/submission waits separately attributable. Another
blind cache or pool enlargement is not supported by this experiment. A shared
ancestor-block cache requires explicit invalidation tests for dirty blocking
state, reparenting and mutations during preprocess before it can be trusted.

All source prototypes stay under `artifacts`; production sources and the
portable payload are unchanged in this continuation. The normal worktree app
is preserved; diagnostic processes are terminated only by exact launcher path.
No commit/push is performed.

Primary scripts: `build-unmap-stages-20261029`,
`analyze-unmap-stages-20261029.py`, `build-deferred-geometry-20261029`,
`capture-deferred-geometry-20261029.py` and
`analyze-deferred-geometry-20261029.py`. Compact output is in the matching
`unmap-stages-20261029` and `deferred-geometry-20261029` directories.
`live-attribution.json` holds exact per-scope scheduler joins;
`live-comparison.json` holds all valid comparison blocks.

Twelve older JSON traces were NTFS-compressed with before/after SHA256 checks;
paths and data are preserved. A disk-space guard was added before the final
capture. Raw traces and compiled outputs remain local and are hash-referenced
from the experiment snapshot, not intended for commits.

AI assistance has been used to create this output.
