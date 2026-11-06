# Masonry: isolating an animated desktop observer

## Outcome

The Computer Use helper was adding substantial desktop composition work while
the benchmark ran. A supported reset of this task's Node REPL client caused its
helper process to exit normally. No user application, VM, driver setting, system
display setting, or production F4 code was changed.

After that reset, the clean 30-second quarter-height middle and rapid-wheel
captures both recorded zero missed display intervals, zero dropped presents,
and zero coverage holds. Middle camera/display timing error was at most 0.733
physical pixels. This is meaningful warm-scrolling progress, not completion of
the constant-160-Hz objective. Cold loading, wheel curve/physical-motion coverage,
rendered-pixel verification, and broader sustained acceptance remain outstanding.

The preceding goal turn narrowed a visible hitch to a DWM display slot. This
turn obtains direct scheduling and composition-origin evidence and changes the
measurement setup accordingly. Older results are retained; this does not show
that every previous application spike was caused by the observer.

## Trace scope and validation

All valid runs use actual Downloads, 192 entries, Masonry density 152, actual
DPR 1.75 and 160 Hz, normal maximized geometry, and foreground focus. Each timed
gesture follows a verified tour of all entries over 17 quiescent viewports.
Timed cache stores, misses and frame-count deltas are zero. Quarter-height
pointer displacement is 296.5 logical pixels, with the existing 64-logical-pixel
fixture reversal margin.

The existing host `fce1f63a6d5cec4714375073e7a69de2ca8f078ed0f7a96dd307aedf6f8904d1`
has bounded read-only camera instrumentation; there is no new production
renderer or animation change. E2/E3/E4 use PresentMon GPU tracking, a DWM/DXGI/
DxgKrnl trace, and a kernel scheduling trace. Treat them as attribution controls.
No UIA, screenshots, build, or offline analysis runs during timed input.

Provider keywords come from the installed Windows manifests. User tracing uses
DWM Composition/DetailedFrameInformation/DwmFrameRate/ProcessAttribution/
Scheduling, and DxgKrnl Base/GPUScheduler/Present. Kernel tracing includes
thread lifecycle, context switches, ready-thread, DPC and interrupt events.
Each owned session has a distinct identity, bounded files, and ownership checks
before stopping. Other trace sessions are left untouched.

Both ETLs for each of E2/E3/E4 have zero lost events, zero lost buffers, and
successful readers, with first/last timestamps covering the full input window.
The targeted TDH decoder records unsupported structures explicitly; it does not
silently interpret them. The full-run DWM summaries used below need none of
those structures and have zero metadata/property errors or unsupported fields.
Thirteen selected DWM present-start events independently match xperf's task,
thread, and scalar payload decoding. QPC/xperf offset variation is under 1 us.

E2 recorded no miss and is retained as a control. E3 recorded one 12.4802-ms
display gap and one dropped application present. DWM's previous and current
display timestamps uniquely match the application's. Its render phase around
the missed slot lasted 3.3092 ms, of which 3.302 ms was scheduled on CPU and only
0.0072 ms was off CPU. Thus a long ready-thread wait does not explain that scope.
This trace has no sampled caller stacks or retired-instruction/frequency data;
it does not distinguish more instructions from slower execution of similar work.

## Composition origin and client reset

E3's DWM `CHANNELANIMATIONS_ACTIVE` events identify process 21496,
`codex-computer-use.exe`, with eight active WinRT animations on all 4,800 measured
frames. Composition-origin events also include that process. This remains
present without any per-frame Computer Use calls; omitting UIA calls alone is
not enough to remove this observer.

Resetting only this task's Node REPL kernel through the supported tool released
its client. The helper then exited; no process-termination command was used on
it. The same F4 diagnostic process stayed active. Another complete warm tour
preceded E4, and the same workload and trace configuration were retained.

| DWM metric | E3: helper present | E4: client released |
| --- | ---: | ---: |
| Active helper WinRT animations | 8 | 0 |
| Render phase p50 / p99 (ms) | 1.2149 / 1.810456 | 0.3025 / 0.612210 |
| Render phase max (ms) | 3.3092 | 1.3268 |
| Draw calls p50 / p99 | 29 / 31 | 1 / 1 |
| Draw entries p50 / p99 | 141 / 141 | 1 / 1 |
| GPU busy p50 / p99 (ms) | 1.3535 / 1.556312 | 0.6292 / 0.825309 |
| Application missed intervals / drops | 1 / 1 | 0 / 0 |

These work counters establish that the observer was contributing composition
work. A single timing pair does not prove the reset explains all variation or
guarantees future no-miss delivery. E2 also had no misses with the helper present.
E4's maximum display interval was 7.8178 ms; do not equate its no-miss count with
proof of the original 2-pixel motion bound.

## Clean captures after reset

E5/E6 retain PresentMon 1.x delivery metrics with GPU tracking disabled. No
additional user/kernel trace is active, and Computer Use remains disconnected.
The same native motion probe records bounded samples. These are lighter
delivery checks, distinct from E2/E3/E4.

| Metric | Middle E5 | Rapid wheel E6 |
| --- | ---: | ---: |
| Input observations | 2991 | 2815 |
| Native motion commits | 4801 | 4773 |
| Render p99 / max (ms) | 2.389178 / 4.5172 | 2.541565 / 4.7521 |
| Maximum display interval (ms) | 6.5061 | 6.4605 |
| Missed intervals / dropped presents | 0 / 0 | 0 / 0 |
| Coverage holds | 0 | 0 |
| Max native constant-velocity error (physical px) | 3.61e-13 | not a constant-velocity fixture |
| Max camera/display error (physical px) | 0.732809 | curve bound not established |

All 4,783 comparable constant-velocity middle pairs are retained. Thirteen
reversal pairs, one first-frame present boundary, and one final partial commit
remain explicit; no dropped-present gap is hidden. Wheel retains its 25 frames
without a new motion commit, one first-frame boundary, and every input revision.
These results are native-camera and PresentMon evidence, not optical scanout or
rendered-leaf image evidence. No full performance acceptance is asserted.

## Regression guard and storage failures

`profiler/capture_environment.py` is a read-only Windows preflight. It detects
the measured helper, rejects an unlabelled clean capture while it is active,
and allows explicitly labelled attribution controls without marking them clean.
It never kills a shared helper or another client's process. A failed query or
unsupported platform cannot pass. Nine regression tests pass, including exact
process-name matching, multiple clients, malformed PIDs, and query failure.
The helper-absence receipt after E5/E6 is a point-in-time observation, not a
continuous process monitor.

The first D1 attempt filled D: and left an empty input JSON. Its ETLs and
PresentMon output remain preserved but are invalid for performance acceptance.
Its kernel logger eventually reached the configured file cap; authoritative
logger inventory confirmed both owned sessions had stopped. The normal app
remained alive. Capture data for subsequent runs was redirected to C:, while
the executable remained inside this worktree. Closed probe binaries were copied
and hash-verified before source paths became symlinks. Valid E2/E3/E4 closed
traces were NTFS compressed and checked against their original logical hashes.
The invalid D1 traces were also preserved and compressed, but have no
pre-compression hash receipt. No raw trace or user file was deleted.

## Reproduction and remaining work

Use `profiler/README.md` for the supported client-release and pre/post guard
sequence. Preserve application focus checks and complete cache admission.
Do not reconnect Computer Use during a clean capture. Use its supported reset
again if it was needed to prepare a later window; never terminate another
client's helper to obtain a clean result.

Next, measure cold first-entry loading and validate the actual rendered wheel
and middle motion without reintroducing the animated helper. Keep all original
display, physical-motion, coverage, SVG/image-quality, and 175%-alignment gates.
The production portable build and running normal app remain unchanged. The
diagnostic instance is left alive for the next checks; all owned trace sessions
are stopped. No commit or push was made.

The [evidence manifest](../experiments/masonry-dwm-observer-isolation-2026-10-31/manifest.json)
indexes six hash-verified valid ETLs, ten verified archived executables, curated
analysis results and reader sources. It records the unchanged normal build and
the still-live diagnostic instance separately. Raw traces and frame packets
remain at the recorded external locations; executables and raw traces are not
copied into the experiment snapshot.
