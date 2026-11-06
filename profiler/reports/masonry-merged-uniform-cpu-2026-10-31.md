# Remaining Masonry costs with merged-uniform sharing enabled

## Outcome

A new CPU/scheduling capture confirms that scene preparation is still primarily
scheduled work, while a separate 4.6906-ms render-thread stall crosses Qt's
render-job mutex during gallery-item destruction. The GUI thread that wakes the
renderer spends most of this interval blocked, with an NTFS/filter-manager
kernel stack on resumption. This is a useful lead for paging/I/O attribution;
the trace does not establish the specific file, fault, lock acquisition time,
or a safe implementation fix. Constant 160 Hz remains unachieved.

## Capture and validation

Case: `real-scroll-mergedcpu-b1-20261031-wheel-masonry`.
Actual Downloads, density 152, 3840×2160, 160 Hz and DPR 1.75. A fresh process
and isolated empty cache run the existing two-direction wheel scenario. This
does not substitute for separately verified-warm and middle-scroll gates.
Merged-uniform sharing is enabled; sparse sequence mode is off and frame
latency remains 2. The production default and embedded payload are unchanged.

All 1705 wheel events are active, with native foreground validation. There
are 3444 target PresentMon rows and 3195 complete measured preparation frames.
No screenshots or accessibility observations occur during the gestures.
The owned ETW session records Profile, CSwitch and ReadyThread stacks; lost
events, lost buffers and schema errors are all zero. The profile-prefix
fallback is independently checked against 433 xperf-decoded samples, all
matching. Exact linked `.text` and `.pdata` sections match the captured host.

The kernel trace adds overhead. Use it for attribution; do not compare its
frame-time totals directly with untraced A/B arms.

| Window | Display misses >9.375 ms | Maximum display interval, ms | Preparation median / p99 / max, ms | Maximum motion error, physical px |
| --- | ---: | ---: | ---: | ---: |
| First direction | 38 | 12.6553 | 0.6407 / 3.9895 / 6.1638 | 123.3701 |
| Second direction | 55 | 12.6555 | 0.6031 / 3.9079 / 5.8724 | 121.7936 |

Neither window passes the original gates. Sharing records 679128 queries,
434299 hits, 244829 uploads and 94 created buffers, with zero fallback or
allocation failures. Fewer uploads alone have not resolved the spikes.

## Scene preparation

Across 3195 frames, preparation totals 3054.531 ms: 3049.172 ms scheduled,
2.446 ms runnable and 2.913 ms blocked, with no unknown interval. Scheduled
time includes interruptions on the CPU and is not pure application instruction
time. This accounting rejects a broad explanation based on the renderer
waiting to be scheduled during preparation.

| Instrumented region | Total wall time, ms |
| --- | ---: |
| Before preparation body, including preprocessing | 685.726 |
| Render-list construction | 309.640 |
| Batching | 123.132 |
| Sorting | 25.224 |
| Gate setup | 108.049 |
| Geometry uploads | 611.090 |
| Opaque materials | 41.259 |
| Alpha materials | 1051.419 |

These are exclusive region totals, not inclusive stack-sample counts. They
describe the full run rather than one worst frame. The worst preparation
frame is fully scheduled (6.1638 ms), with 2.5736 ms before the body,
1.0878 ms in alpha materials, 0.9335 ms in uploads and 0.8801 ms in render lists.

2943 sampled instruction pointers fall inside measured preparation; five lack
a joined stack. Inclusive samples still reach merged-batch preparation (830),
preprocessing (661), material dynamic-data processing (606), geometry upload
(537) and unmap (404). Self samples include updater traversal (210), material
dynamic-data processing (129) and shared-uniform acquisition (98). Inclusive
counts overlap and must not be summed or treated as exact milliseconds.

The faster-hash microbenchmark remains a tens-of-microseconds opportunity;
this trace does not turn it into an explanation for multi-millisecond stalls.

## Render-job mutex and GUI destruction

One of the longest render frames spends 4.6906 ms blocked. Its resumption stack
contains `QBasicMutex::lockInternal` and `QQuickWindowPrivate::runAndClearJobs`
during the submission/after-swap region. The corresponding preparation is
1.0883 ms and the instrumented submission region is 5.6882 ms.

Qt's compiled source has one mutex around copying and clearing the job list:
[qquickwindow.cpp:4104](../../artifacts/qt-child-stacking/qquickwindow.cpp#L4104).
Jobs themselves run after unlocking. The same mutex is held by
[scheduleRenderJob](../../artifacts/qt-child-stacking/qquickwindow.cpp#L4076)
while dispatching or appending the job. Do not infer that the runnable's body
was executing under this lock.

The ReadyThread event comes from GUI thread 110356 in the target process.
Its exact-timestamp user stack is:

```
QQuickWindow::scheduleRenderJob
QQuickImage::releaseResources
QQuickItemPrivate::derefWindow
QQuickShaderEffectPrivate::handleItemChange
QQuickShaderEffect::itemChange
QQuickItemPrivate::derefWindow
QQuickItem::setParentItem
QQuickItem::~QQuickItem
QQmlElement<GallerySceneItem> destructor [ICF aliases=2]
QObject::event / sendPostedEvents
```

The optimized destructor address has aliases; do not claim a unique QML
delegate identity from that address alone. The image/shader release and
window-detachment chain is directly present.

During the renderer's wait, GUI accounting is 4.5780 ms blocked, 0.0282 ms
runnable and 0.0844 ms scheduled. The GUI's resumption kernel stack includes
NTFS and FLTMGR. There are no GUI CPU-profile samples within that short wait.
Thus the evidence does not show several milliseconds of GUI instructions in
`scheduleRenderJob`, nor does it support a general CPU-starvation explanation.
The exact blocking operation and file are not identified by the selected
kernel flags. Paging or filesystem activity is a hypothesis to test, not a
confirmed root cause or justification for disabling security/filesystem tools.

The longest GUI synchronization wait is 16.019 ms. Across its whole interval,
the render thread is scheduled 5.5574 ms, runnable 0.0424 ms and blocked
10.4192 ms. Preparation overlaps only 1.4679 ms. This reinforces that a
single material/preparation optimization cannot explain every long GUI wait.

## Next measurements

1. Add hard-fault and filesystem/disk attribution to a bounded capture of this
   destruction path. Determine whether GUI paging/I/O extends a mutex hold;
   retain exact stack/time correlation and inspect the file involved.
2. Identify which gallery retirement/release operations feed that path before
   changing item retention or Qt locking. Keep cleanup on the required thread
   and preserve graphics lifetime protection.
3. Continue separating CPU-heavy preprocessing/material/geometry work from
   synchronization stalls. Validate any candidate with untraced A/B runs and
   the unchanged complete cold/warm/middle acceptance gates.

The display hold has been released. The diagnostic launcher self-relaunched
once; cleanup validated the recorded host's exact executable and launcher
ancestry before stopping that tree. Normal worktree app PID 43936 remains
untouched. No production changes, commit or push were made in this capture.

Compact evidence and external SHA-256 receipts:
`profiler/experiments/masonry-merged-uniform-cpu-2026-10-31/`.

> AI assistance has been used to create this output.
