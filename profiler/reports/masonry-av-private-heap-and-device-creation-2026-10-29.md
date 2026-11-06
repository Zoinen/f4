# Masonry: FFmpeg heap isolation and video-device creation

## Outcome

The 160 Hz goal is **not achieved**. A diagnostic FFmpeg private heap reduced
the total missed display intervals in this four-capture comparison from 31 to
19, and shortened the sampled geometry-update tails. It did not eliminate
spikes: one candidate interval was 25.0209 ms, versus 19.3546 ms for the worst
control interval. Every block failed the existing motion-error limit.

The candidate is **not promoted**. The portable application and its embedded
payload are unchanged. No commit or push was made.

The earlier trace also now has a more specific explanation for its separate
Direct3D allocation contention: nearby user stacks on the waking thread show
**FFmpeg creating a D3D11 video device inside VideoThumbnailRunner**. This
supports investigating reuse of compatible hardware-device contexts, rather
than treating this as an unidentified background GPU allocation.

## Experiment and fidelity

Diagnostic host: `artifacts/f4-qt-host-av-private-heap-20261029.exe`.
Sources, build identities, tests and analyses:
`artifacts/av-private-heap-20261029/`.
Frozen text/image evidence and external artifact hashes:
`profiler/experiments/masonry-av-private-heap-and-device-creation-2026-10-29/`.

The experiment starts from the unmap-stage diagnostic host, preserving its
original geometry-shadow behavior, original node updater, first-use geometry
maps, original 100-entry asynchronous buffer pool, bounded uploads, uniform
reuse/capacity, and precompiled atlas shader. Neither the rejected lazy-shadow
candidate nor the ancestor cache is enabled.

Both arms use the same executable. `F4_PROFILE_AV_PRIVATE_HEAP=0` routes the
three FFmpeg allocation primitives to their original CRT implementations;
`=1` routes them to a private Windows heap. All other experiment flags and
capture guards are identical.

The exact local libavutil 59.39.100 `mem.c` and its configuration are hashed in
`source-contract.json`. The configuration enables 64-byte SIMD alignment and
`HAVE_ALIGNED_MALLOC`, and disables POSIX memalign and memory poisoning.
Only `_aligned_malloc`, `_aligned_realloc`, and `_aligned_free` within that
translation unit are redirected. FFmpeg's original allocation limits, checked
array multiplication, zero-size handling and higher-level helpers remain in
the compiled source. The host link map resolves `av_malloc`, `av_realloc` and
`av_free` to the replacement `mem.obj`; the archive's original memory object
is not linked alongside it.

The private heap is growable, serialized, and process-lived. Each aligned
allocation stores its original heap pointer and requested length immediately
before the aligned address. Reallocation allocates/copies/frees and leaves the
old allocation intact on failure. There is no fallback that mixes private-heap
and CRT pointers. This retains hardware video decoding and all thumbnail work.
Windows documents private growable heaps and serialized cross-thread use in
[HeapCreate](https://learn.microsoft.com/en-us/windows/win32/api/heapapi/nf-heapapi-heapcreate);
its default allocation alignment is insufficient for this build's 64-byte
requirement, so explicit alignment is necessary.
[HeapAlloc](https://learn.microsoft.com/en-us/windows/win32/api/heapapi/nf-heapapi-heapalloc)

All four captures used the real Downloads folder, all 191 entries, Masonry
density 152, native text, the actual 3840×2160 / 160 Hz / DPR 1.75 display,
fresh isolated gallery-cache roots, and two ten-second rapid-wheel blocks.
The cold-start normal-scrollbar method was retained; no verified warm tour
preceded measurement. The operating-system file cache was not flushed.
PresentMon sessions were case-owned. No ETW stacks, UI Automation observer,
tests, builds or offline analyses ran during the gestures.

## Correctness checks

- Original-source allocation helpers: 11 control groups and 12 candidate
  groups pass, including zero-size allocation/reallocation, 64-byte alignment,
  growth/shrink content preservation, calloc, overflow/maximum-size rejection,
  forced allocation failure, realloc ownership, and cross-thread realloc/free.
  The candidate allocates and frees all 8,223 tracked blocks.
- Actual CPU decoding: all six Downloads videos, five frames per video,
  three independent decoder lifecycles, four decoder threads. Packed visible
  pixel hashes match exactly between arms in every comparison, for H.264 and
  HEVC. The candidate frees all 27,735 tracked allocations with no failures or
  live blocks after teardown. This checks decoder data/ownership; it is not a
  claim that hardware decoding was exercised by this particular test.
- Native renderer: 48 passed, zero failed/skipped at actual DPR 1.75.
- Shader rendering: 27 passed, zero failed/skipped, including rendered-pixel
  comparisons and scene-origin/unit-transform checks.
- Installed-Qt-independent QML imports: 3 passed, zero failed/skipped.
- The diagnostic host passes the Windows portable-dependency import audit.
  This is not a portable rollout: no embedded payload was regenerated.

The initial shader verification used the build directory as its working
directory, so relative shader paths were missing and the run timed out after
180 seconds. `shader-initial-wrong-cwd-results.txt` preserves the failures.
The harness now executes rendering tests from the repository root; the corrected
run completes in approximately 5.25 seconds. No shader or assertion was changed.

No static UI layout changed in this experiment. Heap-creation-failure injection,
long-running playback, device-removal behavior and broader allocation workloads
remain additional requirements before considering a production allocator patch.

## Live comparison

Cases use prefix `real-scroll-av-private-`, suffix
`-20261029-wheel-masonry`, and arms `a1`, `b1`, `b2`, `a2` in that order.
Cache suffixes are `zw`, `zx`, `zy`, `zz`, respectively. A is CRT/control;
B is the private heap. Preparation is CPU-wall scope duration in milliseconds,
not a pure scheduled-CPU measurement.

| Arm/block | Preparation median | Preparation p99 | Preparation max | Display max | Intervals >9.375 ms | Max motion error, physical px |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1/0 | 0.7374 | 2.9802 | 6.4563 | 19.3546 | 9 | 238.469 |
| A1/1 | 0.7311 | 4.0978 | 8.4743 | 12.6533 | 6 | 89.815 |
| B1/0 | 0.7631 | 3.1729 | 6.0503 | 12.5140 | 2 | 74.317 |
| B1/1 | 0.6808 | 3.7056 | 8.0823 | 12.5667 | 6 | 101.771 |
| B2/0 | 0.82065 | 3.6622 | 6.4882 | 25.0209 | 5 | 299.314 |
| B2/1 | 0.6688 | 4.0563 | 8.6656 | 12.4922 | 6 | 96.129 |
| A2/0 | 0.7437 | 3.8479 | 9.8929 | 12.5731 | 10 | 99.996 |
| A2/1 | 0.7401 | 3.7945 | 6.4043 | 12.5181 | 6 | 104.761 |

Every block has zero coverage holds. Median display intervals are approximately
6.25 ms, but neither that median nor the reduced total miss count proves
constant 160 Hz. Every arm fails the original strict gates.

B1 and B2 report 559,119 and 446,493 private allocations, respectively, with
matching frees, zero failures and zero live blocks at snapshot time. These
counts are cumulative across the process, including initialization and work
outside the input windows; they are not per-frame counts or matched workloads.

The fine unmap observer records only operations whose total duration reaches
100 µs. Within the input windows on the main render thread:

| Arm | Recorded operations, blocks 0/1 | Worst total, blocks 0/1 ms | Worst create/recreate in block 0 ms |
| --- | ---: | ---: | ---: |
| A1 | 25 / 6 | 1.5040 / 0.1374 | 0.0314 |
| B1 | 13 / 4 | 0.6417 / 0.1602 | 0.0328 |
| B2 | 21 / 0 | 0.3784 / no recorded operation | 0.1585 |
| A2 | 22 / 1 | 3.3172 / 0.1291 | 2.9051 |

The candidate has shorter recorded geometry tails in this comparison. These
samples are threshold-selected, not the complete distribution. A missing sample
does not mean zero cost. No scheduler trace was taken during these clean
comparisons, so they do not establish the cause of the remaining 25 ms gap or
prove that every previous heap wait disappeared.

## Newly attributed Direct3D readier

This is additional offline analysis of the **earlier** trace
`real-scroll-unmap-wait-stacks-20261029-wheel-masonry`, not a stack trace of
the new private-heap captures. Trace SHA256:
`84306f4dd69bbcf97020115a1c28dbaac1f6b192c9825811197cb74cdbe94ad8`.

The previously identified render wait was a 3.1446 ms text-mask VBO unmap,
including 2.8979 ms blocked. Its principal 2.5535 ms wait is associated with a
ReadyThread event at QPC 9898232898780, emitted while thread 60796 in the same
Qt process released a graphics allocation push lock. The ReadyThread stack
itself contains kernel frames only.

Searching nearby switch-in stacks for **that same waking thread** now finds
user fragments at **−0.4757 ms and +0.0821 ms** around the wake event. Both
contain:

```
D3D11CreateDevice
  d3d11va_device_create
    av_hwdevice_ctx_create
      QFFmpeg::loadHWContext
        QFFmpeg::HWAccel::findDecoderWithHwAccel
          QFFmpeg::CodecContext::create
            QFFmpeg::PlaybackEngine::createStreamAndRenderer
              QFFmpegMediaPlayer::setMediaAsync
                VideoThumbnailRunner::run
```

Windows frames include layered-device/driver-instance construction; the earlier
stack also includes `NDXGI::CDevice::AllocateCB`. Application symbols use the
previously byte-verified map. Seven Windows module images are verified against
the trace's image sizes/timestamps before resolving their symbols.

This is strong temporal/call-stack evidence for thumbnail video-device creation
around the lock release. It is not a captured allocation-object address or a
user stack on the exact ReadyThread event. Template/ICF wrapper names are not
evidence that the corresponding overlay-query API was called.

The official
[Qt 6.11.1 implementation](https://github.com/qt/qtmultimedia/blob/v6.11.1/src/plugins/multimedia/ffmpeg/qffmpeghwaccel.cpp)
calls `av_hwdevice_ctx_create` directly in `loadHWContext`, without retaining a
reusable device there. Its source URL and SHA256 are recorded locally. This
source inspection supports the next experiment; it is not proof that all other
Qt codec state may safely be shared.

## Next work

Investigate a bounded hardware-device-context reuse experiment restricted to
thumbnail requests with identical default D3D11 device parameters. Preserve
hardware decoding, independent codec/frame contexts, reference-counted device
lifetime, FFmpeg's device synchronization and failure fallback. Test multiple
simultaneous decoders, device loss/replacement and pixel equality before a live
comparison. Do not infer that sharing codec or frame contexts is safe merely
because device contexts can be referenced.

The private heap remains a separate diagnostic variable. Re-test the actual
remaining render stalls after addressing device creation; retain all cold,
verified-warm-wheel and quarter-height-middle gates. No source-acquisition,
geometry, caption, coverage, alignment or motion requirement is relaxed.

AI assistance has been used to create this output.
