# Masonry scrolling during cold gallery preparation

## Finding

Cold gallery preparation exposes a reproducible render-thread stall that the
warm-cache captures did not cover. One cold run has 13 missed displayed
intervals in its first ten-second block, reaching 18.9581 ms. A scheduler repeat
has 19. Its largest preparation span takes 16.4278 ms, including 14.3443 ms
blocked and only 2.0788 ms scheduled CPU. This is a different mechanism from
the warm prepared-row-window issue.

Disabling hardware video decoding in a diagnostic control does not remove the
failure: 26 misses, maximum 31.1284 ms. This control is not promoted. The
application clock, graphics backend, production media settings and portable
binary remain unchanged. Constant 160 Hz is still unachieved.

The previous turn was progress: longer warm tests exposed a remaining tail and
produced a passing lossless scheduler repeat. This turn closes the missing cold
preparation scenario and identifies blocked render-thread time during it.

## Scenario validity and isolation

The new diagnostic host uses the current production archives and reversibly
instrumented native controller. Its only additional behavior is test harness
setup and observation:

- Before constructing models, configure the existing `f4-qt-host` storage
  namespace and an absolute, empty, newly created diagnostic cache root.
  Freeze it through `StorageLocations::cacheRoot()` and record the actual root
  in the packet. Later saved cache-location settings cannot redirect it.
- Leave the user's cache, settings and source files intact. The three roots
  are `artifacts/cold-gallery-cache-20261027-a`, `-b` and `-c`. They are not
  deleted or reused between cold passes.
- Start PresentMon before the app and queue the gesture immediately after
  launching. Remove the normal two-second settling delay. Input begins once
  the panel is available; no readiness question or manual input is required.
- Observe thumbnail-cache pending/store/miss/frame counters every 100 ms with
  a bounded 1024-row array. Cache API calls are through the runtime, without
  UI Automation or per-frame object-tree traversal. Record observer wall time.
- Use the ordinary production renderer, static Qt libraries, default cache
  capacities and worker counts. No experimental renderer/RHI object is linked.

The first default run proves overlap rather than merely assuming an empty
cache is sufficient. In block 0, stores rise from 0 to 152 while input is
active, with up to 55 requests pending. Block 1 stores another 37 frames, with
up to 29 requests pending. There are 768 model-change signals covering 1138
rows. Thus the test exercises asynchronous completion during scrolling.

This is a cold **gallery** cache, not a cold operating-system file cache or a
cold Go directory/metadata cache. The source remains the actual Downloads
folder, with 190 entries, density 152, D3D11, 160 Hz and DPR 1.75. No builds,
tests, offline analysis or UIA polling run during input. Every wheel event must
be active, and each run meets the existing 1500-event requirement. Recorder
capacities are checked. The observer's maximum wall cost is 43.8 us, 40.9 us
and 42.6 us across the three passes; it remains an instrumentation caveat.

## Captures

Each case contains two ten-second rapid-wheel blocks, with endpoint reversals.
All use distinct processes and fresh cache roots because storage-root changes
are forbidden once in use. These are intentional scenario resets, not restarts
after an observation timeout.

| Case/block | Display gap p99 | Display gap maximum | Misses over 9.375 ms | Coverage holds |
| --- | ---: | ---: | ---: | ---: |
| Default A / 0 | 7.2938 ms | 18.9581 ms | 13 | 0 |
| Default A / 1 | 6.3794 ms | 18.7538 ms | 6 | 0 |
| Default B + scheduler / 0 | 12.3558 ms | 18.7548 ms | 19 | 4 |
| Default B + scheduler / 1 | 6.3022 ms | 6.3950 ms | 0 | 0 |
| Software video C / 0 | 12.4621 ms | 31.1284 ms | 26 | 1 |
| Software video C / 1 | 6.3090 ms | 6.4017 ms | 0 | 0 |

Default A fails the unchanged displayed-gap and motion gates. Its first-block
maximum motion error is 203.42 physical pixels when a displayed interval spans
nearly three nominal refresh periods. Do not hide this with a good median.
Coverage holds are independently reported: cold B and C still have some while
preparation is delayed. The warm directional fix does not guarantee preparation
can keep up through these longer blocks.

Prefixes:

- `real-scroll-cold-gallery-cache-20261027-wheel-masonry`, Qt PID 81124.
- `real-scroll-cold-gallery-scheduler-20261027-wheel-masonry`, Qt PID 7536.
- `real-scroll-cold-software-video-20261027-wheel-masonry`, Qt PID 40272.

## Scheduling attribution

The default B scheduler trace has zero lost events, buffers lost, schema
errors and ProcessTrace errors. Its verified raw size is 163,315,712 bytes;
the compressed copy is 44,822,977 bytes. Only the owned matching trace session
was started/stopped. Exported scheduling records include the Qt GUI/render
threads and enumerated DWM threads.

The first-block largest spans are:

| Span | Wall | Scheduled CPU | Runnable | Blocked |
| --- | ---: | ---: | ---: | ---: |
| Preparation | 16.4278 ms | 2.0788 ms | 0.0047 ms | 14.3443 ms |
| Recording | 5.1305 ms | 0.9220 ms | 0.0388 ms | 4.1697 ms |
| Submission | 13.8648 ms | 1.4626 ms | 0.0464 ms | 12.3558 ms |
| Render total | 19.8833 ms | 6.5828 ms | 0.0811 ms | 13.2194 ms |

Unknown attribution time is zero. These maxima are separate spans, not one
frame whose component times can be summed. The long blocked intervals carry
raw scheduler wait-reason value 28. That alone does not identify a lock owner,
call stack or responsible graphics API. A blocked span is not automatically
GPU execution time.

After preparation settles, block 1 has a 2.2822 ms maximum preparation span
entirely scheduled, a 4.2061 ms maximum render total, no coverage holds and no
missed refreshes. This narrows the next probe toward cold resource creation,
upload and submission calls rather than adding more layout rounding or changing
the animation clock.

## Hardware-video falsification

Downloads contains five MP4 files among its mixed image, document, archive and
other entries. Hardware video decode was one plausible source of concurrent
graphics work. The diagnostic C child inherited
`QT_FFMPEG_DECODING_HW_DEVICE_TYPES=,`, the empty-device-list control documented
by [Qt Multimedia 6.11](https://doc.qt.io/qtforpython-6.11/overviews/qtmultimedia-advanced-ffmpeg-configuration.html).
This is a private diagnostic backend control. The normal already-running
app's environment and saved media settings were not modified.

This intervention did not eliminate cold stalls and is not evidence for
disabling hardware decoding in production. The passes are not a randomized
identical-timing benchmark, so do not claim that software decoding causes the
larger maximum merely from this comparison. The control does not prove the
backend selection of each individual codec; targeted resource-call timing is
still required.

## Build and runtime verification

The diagnostic host passes four import-isolation/compiled-host cases and the
system-DLL audit. Build receipts pin sources, compiled inputs and the parent
production baseline. No application source, dependency archive or embedded
payload was changed in this step. The normal portable worktree app remains
launcher 43936 / Qt 119460 with its previously verified embedded host hash.
Only the exact owned cold diagnostic launcher tree was closed after analysis.
No commit or push was made.

## Next discriminating probe

Time actual D3D11 resource calls in the blocked preparation/recording/submission
intervals: buffer Map/Unmap, texture creation, UpdateSubresource/copies and
Present. Preserve default behavior and use bounded records; measure the added
observer cost. Do not infer a device wait solely from wall time or wait reason.

The cached Qt 6.11.1 `qrhid3d11.cpp` source is available at
`artifacts/qt-6.11.1-source/qrhid3d11.cpp`, SHA256
`010bb583db827ae20d00a1f186b4df5b574a90c4c3d85c9ecbbb60cbd75814c7`.
The original Conan source path in the old compile command no longer exists.
Any new RHI probe must establish compatibility with the current SDK/archive,
retain exact source restoration, and pass native rendering/import checks;
availability of that cached source alone is not proof of an identical baseline.
Existing begin-pass probes identify reusable compile/test mechanics but must
not silently pull earlier renderer or RHI experiments into the host.

Evidence snapshot:
`profiler/experiments/masonry-cold-preparation-stalls-2026-10-27/`, containing
diagnostic source, capture packets, presentation/scheduler CSVs, analyses,
build and cache-isolation receipts, logs and a SHA256 manifest. Large binary
traces, executables, objects and generated cache data stay in artifacts and
are represented by external hashes rather than copied into the snapshot.

> AI assistance has been used to create this output.
