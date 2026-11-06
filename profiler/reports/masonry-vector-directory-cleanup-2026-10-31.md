# SVG temporary-file cleanup off the GUI thread

## Result

The normal portable worktree app now removes generated SVG files on a dedicated
single-worker executor after their last file pin is released. Source acquisition,
QML compilation pins, revision reuse and vector rendering retain their existing
behavior. A failing-before regression passes, related SVG tests pass, and live
file-I/O tracing verifies cleanup moved off the GUI thread.

This is a specific removal of GUI-thread filesystem work, not a solution to all
scrolling spikes. The new captures still fail the original 160 Hz display and
motion gates. In particular, the earlier 4.6906-ms render-job mutex wait has
not been attributed conclusively to this cleanup.

## Investigation

The previous CPU trace found a render thread waiting in
`QQuickWindowPrivate::runAndClearJobs`, readied by the GUI thread from
`QQuickWindow::scheduleRenderJob` during image/shader detachment. The GUI thread
was blocked for most of that interval and resumed through NTFS/FLTMGR kernel
code. This motivated a new capture with hard-fault, disk and file-I/O events.

Case `real-scroll-faultcpu-b1-20261031-wheel-masonry` retains actual
Downloads, density 152, 3840×2160, 160 Hz/DPR 1.75, fresh process/isolated empty
cache, default frame latency 2, sparse mode off and the same merged-uniform
prototype enabled. All 1711 wheel events are active; native foreground checks
pass. ETW records no lost events/buffers or schema errors. The profile decoder's
fallback is independently verified against 472 xperf samples, all matching.

The Microsoft xperf decoder reports 49 hard faults on the target GUI/render
threads across the capture, but **none overlaps either measured scrolling
window**. This rejects paging as an explanation for this run's measured spikes;
it does not explain the previous trace's unreproduced mutex wait or justify
preloading the whole executable. Event interpretation follows Microsoft's
[hard-fault schema](https://learn.microsoft.com/en-us/windows/win32/etw/pagefault-hardfault).

File-I/O records identify generated `Prepared.qml` files and their containing
temporary directories being enumerated/deleted by the GUI thread. The source
ownership chain confirms the cause: `VectorSource::preparedFiles` used the
default shared-pointer deleter for `QTemporaryDir`. Its last release can happen
on the GUI thread when a surface/component/revision retires, making recursive
filesystem removal synchronous there. The new trace's six measured Delete
operations themselves complete in 3–5 microseconds each. Those operation times
do not account for the whole directory destructor or prove a multi-millisecond
stall; the implementation change enforces the intended thread boundary.

Preparation remains a separate CPU-heavy path: across 3194 complete measured
frames, 3116.220 ms wall time contains 3111.582 ms scheduled, 2.354 ms runnable
and 2.284 ms blocked. Scheduled time includes CPU interruptions. The worst
preparation frame is 9.8952 ms, almost entirely scheduled, spread across uploads
(3.0425 ms), preprocessing/before-body work (2.3287 ms), alpha materials
(1.9914 ms) and render lists (1.0108 ms). A faster temporary-directory deleter
does not eliminate those renderer costs.

## Implementation and regressions

[VectorSource.cpp](../../third_party/ZoinGallery/VectorSource.cpp) now creates
prepared directories with a custom shared-pointer deleter. Last release queues
the directory's destruction onto one dedicated `QThreadPool` worker. The pool
is initialized during worker-side preparation, drains at shutdown, and has a
direct-delete fallback only after late static teardown has destroyed it. It
does not use or occupy viewer/source-acquisition workers. No source files are
removed while any directory pin remains.

[VectorSourceStorage_p.h](../../third_party/ZoinGallery/VectorSourceStorage_p.h)
provides the internal factory with an explicit executor for deterministic tests;
that executor must outlive its handles and queued work. Tests in
[GallerySessionTest.cpp](../../third_party/ZoinGallery/tests/GallerySessionTest.cpp)
cover paused-worker retirement, executor shutdown draining and independent
prepared-file pins after the source handle expires. Existing cancellation/pin
tests now wait for eventual file removal, while continuing to check the original
strong/weak source lifetimes.

The paused-worker regression first ran with the original synchronous behavior:
4 results passed and 1 failed because the file had already been removed on the
caller. With deferred cleanup all 5 results pass. Related native SVG checks
pass (13 results, zero skips), covering local/virtual SVG/SVGZ acquisition,
source pins, cancellation, readiness and resize geometry. After a coherent
normal CMake rebuild, the focused session checks plus metadata classification
pass against the rebuilt core archive (19 results, zero skips), with no
`VectorSource` implementation-object override.

No static UI layout, scene transforms, caption extents or raster/vector quality
settings changed in this fix. The existing geometry/rendering path is retained.

## Live thread verification and performance limits

Post-fix case `real-scroll-cleanuplive-b1-20261031-wheel-masonry` uses the same
scenario and diagnostic flags, with the new source implementation linked into
the host. All 1669 wheel inputs are active and native foreground checks pass.
ETW loss/schema errors are zero; 295 independent xperf profile-prefix samples
match. There are 3446 target PresentMon rows. No screenshot or accessibility
query runs during either capture's gestures.

The same file-I/O check is run against both traces, restricted to the measured
windows and target process. It requires actual vector file deletions, so a
capture with no retirement work cannot pass by omission.

| Capture | Measured Delete events | GUI thread | Render thread | Other cleanup thread |
| --- | ---: | ---: | ---: | ---: |
| Before | 6 | 6 | 0 | 0 |
| After | 6 | 0 | 0 | 6, one worker |

The post-fix operation-completion times are 3–6 microseconds. This proves the
thread change and preserved cleanup, not a repeatable frame-time improvement.

| Capture/window | Misses >9.375 ms | Display max, ms | Preparation p99 / max, ms | Motion error max, physical px |
| --- | ---: | ---: | ---: | ---: |
| Before/first | 37 | 13.0282 | 4.1204 / 9.8952 | 112.1765 |
| Before/second | 53 | 13.0381 | 3.8350 / 5.7375 | 127.0864 |
| After/first | 35 | 13.0767 | 3.8842 / 7.6374 | 123.3064 |
| After/second | 51 | 13.1539 | 4.2285 / 8.5113 | 122.2298 |

These are two intrusive attribution captures, not an A/B/B/A performance study.
The second direction is not verified-warm. Neither capture establishes the
full cold/warm/middle objective, and the motion-error limit remains 2 px.

## Portable rebuild and running identity

The normal CMake host rebuild compiles the current core, including the cleanup
factory. The normal host links the existing, previously pinned production Qt
Quick/QML archives; their hashes match the production checkpoint. No diagnostic
renderer/main object is present in its link inputs, and no SDK archive changes.
Both diagnostic and normal Windows dependency audits pass with system DLLs only.

Static import isolation passes for the diagnostic (3 results), then the normal
host's import-isolation/compiled-host checks pass (4 results). Packaging verifies
the gzip payload decompresses to the newly built host. The Go launcher uses
`CGO_ENABLED=0`, the `f4_embedded_qt_host` tag and the system Go cache. Payload,
extraction/reuse, concurrent first launch, corruption rejection and embedded-host
lookup tests pass in `internal/plughost`.

The existing Windows long-response-file problem stopped the first normal test
build before it compiled the session test. A first relink consequently picked
up its stale object (9 passed, 5 failed, including missing new methods). That
result was rejected and retained. The corrected verifier explicitly selects
the newly compiled current session-test object, checks object/source timestamps,
links it against the coherent core archive without a source override, and uses
one response argument per line. The final 19/4 results above are from that
corrected verification. The old response-file and stale-object failure logs
remain in the evidence.

The worktree's portable app was replaced only after staging and verification;
the exact previous launcher remains available for rollback. It launched directly
with `--gui=qt`, all profiling/development-host overrides removed. The freshly
extracted child hash matches the new host, startup stderr is empty, and visual
inspection confirms the settled window and worktree header render normally.

- Launcher: `D:\Code\f4-zoin\f4-zoin.exe`, parent PID 23488.
- Qt child PID: 84780.
- Launcher SHA-256: `77168fcfb1b912c0c604ab95f8404c903f0d0570db6efef83f2407511503f80e`.
- Qt host SHA-256: `67e1609cdda71525e5e1de5738ff22de827c551319431ffcacae023fb8c41d7c`.
- Fresh cache generation: `62f6794198c6d68991bc5d26077ca9a645556d290b4278d644a45a71881108d8`.

The canonical `D:\Code\f4\f4.exe` and other checkouts are untouched. Owned
diagnostic processes/ETW sessions/display holds have ended. Inactive diagnostic
intermediates were archived with full hashes and original paths preserved by
junctions to recover build space. No commit or push was made.

## Remaining work

Continue targeting measured preprocessing/dependency-table, render-list,
material-preparation and geometry-transfer costs. Keep synchronization stalls
separate. The 4.6906-ms job-lock wait remains unexplained rather than marked
fixed. Every candidate still needs the unchanged complete cold, verified-warm
and quarter-height middle-scroll gates before declaring constant 160 Hz.

Evidence: `profiler/experiments/masonry-vector-directory-cleanup-2026-10-31/`.

> AI assistance has been used to create this output.
