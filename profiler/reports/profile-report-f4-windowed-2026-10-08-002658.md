# F4: small-window video playback profile

## Finding

Small-window playback is GPU-bound on this machine. The decoder supplies
29.97 frames/s, but the video filter processes only about 22–23 distinct frames/s.
The render thread spends its long stalls in frame completion/presentation, not
scene synchronization or CPU-side scene rendering.

The relevant distinction is **viewport size, not the maximized window flag**.
A large, explicitly unmaximized window is smooth. Small viewports select a
half-size video pyramid whose first pass does not use the existing linear-color
conversion cache. Larger viewports use that cache. This is the strongest
source-level explanation for the measured GPU pressure; individual shader-pass
GPU execution times were not captured.

## Environment and method

- Date: 2026-10-08, Europe/Moscow.
- Platform: Fedora Linux Asahi Remix 44, ARM64, KWin/Wayland, Apple M2.
- Existing Qt frontend: Qt 6.11.1, hardware OpenGL 4.6, Mesa 26.1.8,
  renderer `Apple M2 (G14G B0)`, FFmpeg 7.1.5.
- Display scale in this session: 150% / DPR 1.5. No UI layouts were changed.
- Media: `/home/zoin/Downloads/IMG_20260527_193242+.mp4`,
  H.264, 3840×2160, approximately 29.97 frames/s, duration approximately 86 s.
- Profile mode: native Qt scene-graph phases, video-frame revisions, and DRM
  GPU scheduler events, attached to the already running release build.
- Go process: 849783; Qt GUI process/thread: 849831;
  Qt scene-graph render thread: 849918; KWin thread: 1295.
- Checkout HEAD: `634b264285757d85a86fbb755985a8067e03d7c3`;
  ZoinGallery HEAD: `5a17c34dc11a340c0f65e6744063e14564a2f02b`, with existing
  uncommitted changes. These commits alone do not describe the running build.
- Go executable SHA-256:
  `9f1bee85bd9eed21b52644901f342ce0c2da214de026a1b7bf51787fc5d9d3a9`.
- Running Qt host SHA-256:
  `408362218dfbfda682bc5a0eba93f76ea056a0b92a74c7c0bc4894eb2a56ef6d`.
  The cached executable and build-tree executable have the same hash.

Each controlled capture closed/reopened the same clip, allowed approximately
2 s for startup/flight to settle, and recorded 12 s of playback. The first and
last second were excluded, leaving approximately 10 s per condition. Window
geometry and maximize flags were verified through KWin. Its scripting interface
is documented in the [KWin API](https://develop.kde.org/docs/plasma/kwin/api/).
Screenshots were taken outside measurement intervals.

All reported event traces use `perf record -a -c 1`: every selected event is
recorded, with no frequency sampling or call-stack collection. Probe addresses
and native material-field offsets were checked against this executable's
symbols/disassembly. No lost-event records were found in these four captures.

An initial maximized capture found the clip stopped at its end and was excluded.
Two initial detailed captures accidentally used global frequency sampling
(`-F 99`), which also samples uprobes; their event counts and paired phase
durations are invalid and excluded. The exact-event captures below replaced
them. Separate render-thread `perf stat` checks do not use those sampled traces.

The release build has no QML debugging support. There is no `.qtd` trace and no
measurement of individual QML bindings, JavaScript handlers, GC allocations, or
pixmap-cache events. No debug rebuild, application restart, Qt/FFmpeg rebuild,
software-rendering switch, or production-source edit was performed.

## Frame delivery and rendering

Frame-end time is the wall-clock gap between successive
`QQuickWindow::afterFrameEnd` events, not a display-presentation timestamp.
Lower and more consistent gaps are smoother. p95 means 95% of measured gaps
are at or below that value. At a 60 Hz display, one refresh is approximately
16.67 ms; this 29.97 fps clip naturally needs approximately 33.37 ms between
new video frames, so a 33 ms video-frame gap alone is not a stall.

| Condition | Logical window size | Video filter output, physical px | Decoded frame arrival, frames/s | Fresh video-filter submissions, frames/s | Frame-end p95 / p99 / max, ms | Frame-end gaps >50 ms |
| --- | --- | --- | ---: | ---: | --- | --- |
| Original small window | 1026×653.33 | 1538×866 | 29.97 | 22.47 | 67.94 / 70.00 / 72.18 | 71/302 |
| Small window, Codex minimized | 1026×653.33 | 1538×866 | 29.97 | 23.14 | 66.39 / 66.81 / 67.07 | 71/318 |
| Large, **not maximized** | 1640×980 | 2460×1384 | 29.97 | 29.95 | 18.36 / 19.68 / 22.23 | 0/599 |
| Maximized, not fullscreen | 1707.33×1021.33 | 2560×1440 | 29.98 | 30.00 | 18.07 / 19.44 / 21.58 | 0/599 |

In the small-window trace, decoded-frame arrival p95 was 34.85 ms and maximum
37.36 ms: no arrival gaps exceeded 50 ms. However, the first video reduction
submitted only 223 distinct revisions across a consecutive source-revision
span of 297. Seventy-four revisions inside that span were skipped/coalesced
before video filtering, approximately 25%. The minimized-background control
processed 231 revisions across a span of 299, still skipping approximately 23%.

Large-window and maximized video submissions had consecutive revisions:
300/300 and 299/299 respectively. The difference between 299 submissions and
300 accept events is a trace-boundary effect, not a gap within that revision
sequence. Both smooth conditions had one fresh-video-submission gap slightly
over 50 ms, so they were not absolutely jitter-free.

The scene graph completes approximately 60 frames/s in the smooth conditions,
but the video remains 29.97 fps. Frame-end count is not fresh-video-frame count:
some frames only composite the retained presentation texture. These are not
claims of 60 fps video or optical/compositor presentation rates.

## Where the stalls occur

These are inclusive wall-clock durations, including waits; they are not
exclusive CPU costs and must not be added together because phases nest.

| Native phase | Small-window p50 / p95 / max, ms | Maximized p50 / p95 / max, ms |
| --- | --- | --- |
| Scene synchronization | 0.041 / 0.101 / 3.098 | 0.047 / 0.100 / 1.170 |
| Resample-node preprocessing, per invocation | 0.070 / 0.113 / 1.139 | 0.053 / 0.155 / 1.036 |
| CPU-side scene rendering/command submission | 0.199 / 0.287 / 1.274 | 0.223 / 1.129 / 2.256 |
| `QRhiGles2::endFrame` | 30.852 / 67.415 / 71.858 | 16.087 / 17.353 / 19.550 |

The small-window CPU-side scene work is short while frame completion repeatedly
takes several display refresh periods. The long tail is not an expensive
scene-sync or QML layout loop in the measured steady-state render phases.

| Condition | F4 GPU queue→run p95, ms | F4 GPU run→fence completion p95, ms | KWin GPU completion p95, ms |
| --- | ---: | ---: | ---: |
| Small window | 0.787 | 75.971 | 62.720 |
| Small window, Codex minimized | 0.624 | 74.933 | 22.211 |
| Large unmaximized window | 0.778 | 15.123 | 2.980 |
| Maximized | 0.648 | 12.257 | 2.517 |

GPU jobs are attributed using the submitting thread at queue time and matched
to run/done events by fence identity. Completion latency includes dependencies,
overlap, and GPU contention; it is **not isolated shader execution time** and
the durations of overlapping jobs must not be summed into a frame budget.

Minimizing Codex reduced its captured GPU submissions from 2572 to 4 in the
trimmed interval, yet F4's long completion tail and dropped video revisions
remained. Background work aggravates compositor contention, but it is not the
primary cause of the small-window playback problem.

Separate 10 s checks measured 474.80 ms of render-thread CPU time in windowed
playback and 836.95 ms maximized: approximately 4.75% and 8.37% of one core.
Both checks recorded zero page faults on that thread. These values exclude
decoder/GUI threads, and do not prove that the entire system is free of memory
pressure; substantial system swap usage was present during the session.

## Why a smaller viewport selects expensive work

### [ViewerResample.qml:95](/home/zoin/Documents/Codex/2026-08-31/i-have-f4-project-fork-on/f4/third_party/ZoinGallery/qml/ViewerResample.qml:95)

`levelForSize` selects one half-size pyramid level for the 3840×2160 clip when
the output is 1538×866. The first output is 1920×1080, followed by a final
1538×866 pass. At 2460×1384 or 2560×1440, even the first half-size level would
be smaller than the requested output, so zero pyramid levels are selected.
This decision depends on dimensions, not window-manager maximize state.

### [ViewerResample.qml:155](/home/zoin/Documents/Codex/2026-08-31/i-have-f4-project-fork-on/f4/third_party/ZoinGallery/qml/ViewerResample.qml:155)

The full-precision source-conversion cache is deliberately excluded for pyramids:

```qml
&& requiredLevels === 0 && !nearestNeighbor
```

Exact native draw probes confirm the branch: the small window's 1920×1080
video reduction has `intermediate=1`, `admitted=0`, and no selected linear
texture. Both larger conditions have `admitted=1` and a selected linear texture
on their direct video filter. Hardware-fast sampling is off in all measured
steady-state quality passes.

### [ViewerResample.qml:224](/home/zoin/Documents/Codex/2026-08-31/i-have-f4-project-fork-on/f4/third_party/ZoinGallery/qml/ViewerResample.qml:224)

The first pyramid reduction reads `videoFrameSource` directly. It has no
source-conversion-cache request/provider wired into it. Its result is retained
in a 1920×1080 RGBA8 `ShaderEffectSource`; the final reduction reads that image.
This is not just a different display-window size: it introduces a separate
full-quality filtering pass over the original video planes.

### [viewer_resample.frag:290](/home/zoin/Documents/Codex/2026-08-31/i-have-f4-project-fork-on/f4/third_party/ZoinGallery/resources/viewer_resample.frag:290)

With a ready conversion cache, `linearSample` fetches an RGBA32F linear texel:

```glsl
if (ubuf.videoEnabled == 2)
    return texelFetch(linearVideoSource, pixel, 0);
```

Without it, each selected filter tap reads video planes, converts color, and
decodes the transfer function into linear light. The
[reduction loop:417](/home/zoin/Documents/Codex/2026-08-31/i-have-f4-project-fork-on/f4/third_party/ZoinGallery/resources/viewer_resample.frag:417)
has up to 8×8 taps per output pixel. Repeating that conversion inside the
1920×1080 first-pass convolution is the principal optimization target indicated
by these traces. A precise breakdown among plane reads, color arithmetic,
transfer functions, and driver scheduling still requires per-pass GPU timing.

### [ViewerResample.qml:60](/home/zoin/Documents/Codex/2026-08-31/i-have-f4-project-fork-on/f4/third_party/ZoinGallery/qml/ViewerResample.qml:60)

The presentation cache is active in the small window: there are 223 expensive
video-reduction submissions, 223 final-filter submissions, and 303 cheap
nearest-texel presentation-composite submissions. Therefore this particular
stall is not evidence that the finished high-quality filter is being rerun on
every otherwise redundant scene-graph frame. The expensive missing optimization
is before that cache, in source-video conversion for the pyramid.

## Recommended next step — not implemented

Allow the first video-pyramid reduction to reuse the existing bounded
full-precision source conversion, with one conversion per consumed video
revision. Preserve the existing linear-light filter, source sampling grid,
RGBA8 pyramid output, dithering, resting pixel alignment, and fallback behavior.
Do not substitute lower-quality bilinear playback filtering.

The existing
[ViewerLinearVideoTexture.cpp:195](/home/zoin/Documents/Codex/2026-08-31/i-have-f4-project-fork-on/f4/third_party/ZoinGallery/ViewerLinearVideoTexture.cpp:195)
already tracks source identity/revision and updates its float texture only when
needed. Sharing/admission must remain bounded: a 3840×2160 RGBA32F texture alone
is approximately 126.6 MiB, so creating another conversion resource per level
would be inappropriate, especially with current memory pressure.

Before accepting an implementation, verify output equivalence against the
current reference path, resource-budget/failure fallbacks, one conversion per
revision, pause/no-redraw behavior, and a fresh small-window hardware profile.
Review the affected QML cache/pyramid wiring structurally as well as testing its
rendered output. No performance gain or image-equivalence result for that
unimplemented change is claimed here.

## Artifacts and restored state

Native captures and parsed summaries remain in
`/tmp/f4-windowed-profile.KjrniY/` (temporary; not preserved across reboot):

- `windowed-exact.data`, `.txt`, `.json`: original small-window playback.
- `quiet-window-exact.data`, `.txt`, `.json`: small window with Codex minimized.
- `large-window-exact.data`, `.txt`, `.json`: large unmaximized window.
- `maximized-exact.data`, `.txt`, `.json`: maximized, playing clip.
- `analyze.py`: event parser, trimmed cadence, revision, phase, and fence analysis.
- `windowed-playing-cpu-stat.txt`, `maximized-cpu-stat.txt`: render-thread counters.
- `restored-paused-verified.png`: final window state, video paused at approximately 00:25.

The application remains alive with the same executable hashes. Original
windowed geometry was restored (x=441.804714, y=50.760202, width=1026,
height=653.333333), Codex was unminimized, and the video was paused. All temporary
`f4play` probes, owned KWin scripts, the diagnostic DBus receiver, and the
diagnostic input daemon were removed/stopped. Existing source changes were
left untouched; only this report was added to the checkout.

> AI assistance has been used to create this output.
