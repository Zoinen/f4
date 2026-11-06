# Avoiding rebuilds for opacity outside the renderer tree

## Verified problem and fix

Qt 6.11.1's batch renderer handles `DirtySubtreeBlocked` before its ordinary
node lookup. When that notification also contains `DirtyOpacity`, it requests
a full rebuild unconditionally, including when the node has no shadow-tree
entry because its ancestor is already blocked. There are no batches for that
subtree to invalidate in this case.

The dependency source patch now requires an existing shadow node (`sn`) for
that particular opacity-triggered full rebuild. Existing subtrees retain their
original behavior. Geometry uploads, merging, clipping and buffer ownership
remain Qt's normal implementation; none of the rejected buffer-transfer
experiments are included.

The regression in `qt/host/tests/F4QtBatchRootOrdersTests.cpp` fails against the
previous library. Its latched counter catches any full rebuild during an opacity
transition inside an already blocked drawing subtree, even if another frame
follows. It checks unchanged rendered pixels, then reveals the subtree and
verifies visible opacity transitions through zero, half opacity and full opacity.
The earlier ancestor-reservation/overlapping-alpha regression also passes.

`ci/patch-qt-dependencies.py` applies the change with a strict source anchor;
tests verify composition, idempotence, valid recipe Python and rejection of an
unknown recipe. The local clean library's source was compared byte-for-byte
(after newline normalization) with the CI transformation of the previously
verified drawing-order patch. No Qt version or ABI change is involved.

## Attribution captures

The diagnostic library records exact rebuild sites during synchronization and
rendering. It uses bounded thread-local arrays, with owner attribution added
after rendering. It does not provide uninstrumented frame-time measurements.
Some very busy frames overflow their individual-event array; aggregate counters
still update. Do not interpret truncated event lists as complete causal counts.

All named captures explicitly identify Downloads, 184 entries, density 152,
actual DPR 1.75 and a 160 Hz screen. Initial extent is 4161.142857 logical pixels.
The workload is wheel delta 120 every 30 ms, four seconds in each direction.

- `rebuild-causes160-wheel-masonry`: 23 complete render-list rebuild frames.
  Observed triggers include clip insertion/removal, opacity notifications and
  exhausted drawing-order reservations. These are exact source-site observations,
  rather than deductions from a rebuild flag sampled before Qt's node updater.
- `vector-publication160-wheel-masonry`: an extra GallerySceneItem gate around
  pending vector surfaces still leaves 23 complete rebuilds. Its 27 focused
  tests pass, but the experiment does not independently reduce rebuilds and was
  reverted, including its temporary product regression.
- `blocked-opacity160-wheel-masonry`: the Qt opacity guard eliminates the
  recorded opacity-triggered rebuilds. There are 15 complete rebuild frames;
  clip transitions and reservation exhaustion remain. This pair includes the
  same temporary vector gate on both sides; the final product does not need it.
- `rebuild-padding2048-wheel-masonry` and
  `blocked-opacity-padding2048-wheel-masonry`: increasing Qt's existing reservation
  parameter removes reservation exhaustion as the initiating cause in these
  captures. The combined experiment leaves eight complete rebuild frames,
  all initiated by clip insertion. Its unmap-upload total is 119,020,944 bytes.
  The larger reservation is **not enabled in the product**; precision/range and
  cross-scene bounds need verification before choosing a reservation policy.

The checkout acquired unrelated native-icon/fallback changes between the first
attribution runs and later captures. They are preserved and included in the
rebuilt host. Do not treat all end-to-end timings in this sequence as a controlled
performance comparison.

## Final clean-library measurements

The final diagnostic host links the production library with the opacity guard
and the original SVG presentation, with the normal four-order reservation.
Detailed source hooks and the scene-cause observer are disabled. Basic native
frame observers and PresentMon remain enabled. No UI Automation observer or CPU
sampler runs during measurement.

`opacity-release160-cold-wheel-masonry` is the first sweep in a fresh process;
`opacity-release160-warm-wheel-masonry` follows it. Both are active, actual DPR
1.75, screen 160 Hz and 2194 by 1186 logical pixels. The warm extent is
4179.142857 after asynchronous folder content settles, so the two runs do not
have identical initial geometry.

| Capture/direction | Display median | Display p95 | Display p99 | Maximum gap |
|---|---:|---:|---:|---:|
| Cold down | 6.2501 ms | 6.2753 ms | 18.7343 ms | 31.2603 ms |
| Cold up | 6.2498 ms | 6.2704 ms | 12.4921 ms | 31.2498 ms |
| Warm down | 6.2499 ms | 6.2616 ms | 6.2915 ms | 12.4939 ms |
| Warm up | 6.2499 ms | 6.2684 ms | 6.3128 ms | 12.4516 ms |

Each warm direction has one display interval above 9.375 ms. Zero recorded
presents are discarded; that does not establish a new frame at every refresh.
Native production p99 is 16.0679 ms cold and 9.4232 ms warm. Cold loading still
has real spikes, including render CPU intervals above 10 ms.

One cold return outlier includes a harness boundary artifact: the harness sets
contentY to its original extent before the first return wheel event. The prior
frame is 26.996 ms before that event, and the next is 4.1244 ms after it, producing
a 31.1204 ms native interval that spans idle time. Other return spikes occur
well inside scrolling and remain genuine. The table preserves raw measurements;
future captures should separate startup input latency and continuous movement,
and remove that artificial repositioning rather than silently trim outliers.

The warmed middle-button run (`opacity-release160-warm-middle-masonry`) uses a
296.5 logical-pixel pointer offset, one quarter of the 1186-pixel window height.
It also remains below the uninterrupted 160 Hz requirement; its native p95 is
7.9921 ms, p99 9.7121 ms and maximum 13.0658 ms. It is not a first-entry test.

## Verification and rollout

- Native renderer suite at 175%: four fixture/test results passed, zero failed.
  Both the original and latched-counter regressions failed before the fix.
- Qt recipe patch tests: 15 passed.
- Focused vector suite on the final library: 26 passed, covering SVG/SVGZ,
  virtual files, failure/skip behavior, cancellation, leases, all panel modes,
  viewer transitions, animation and scene-space alignment at actual DPR 1.75.
  The small-caption/thin-stroke rendered capture was inspected.
- Resource-only QML imports and compiled-host startup: four results passed.
- Windows system-DLL audit and tagged embedded-host extraction/reuse tests pass.
  The embedded payload and CGO-disabled Go launcher are rebuilt using the system
  Go cache. Final host SHA256:
  `b2cf3b47cd76e22a0c5953448ede7ddd50343a21c13328e21525a1b69334748b`.
  The restarted `D:/Code/f4-zoin/f4-zoin.exe --gui=qt` runs the matching extracted
  host in cache generation `fca87a3d8266c888e4a2cc7fec2f150673df0cfa99fa57fac80600c3a48f238d`.
  Its runtime title identifies `zoin_branch/cached-pixel-grid`, startup stderr is
  empty, and no diagnostic host or padding override is supplied.

Constant 160 Hz is **not achieved**. Next targets are clip/batch-root lifecycle
changes and bounded reservation policy, followed by remaining scheduling gaps.
The corruption reproducers from the geometry-cache experiments remain excluded
from the product. No new per-frame pixel-alignment traversal is introduced.

AI assistance has been used to create this output.
