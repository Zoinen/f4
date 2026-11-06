# Completed-frame geometry reuse reduces map tails, but does not finish the goal

A guarded D3D11 experiment replaces geometry-buffer WRITE_DISCARD only when
earlier context work is proven complete and no later CPU-recorded work may
read it. It passes safety/rendering tests and exercises hundreds of thousands
of maps in Downloads. Admitted map maxima remain below 0.3 ms in both enabled
runs. Whole-scene presentation still spikes, so this is not promoted and the
160 Hz goal remains incomplete.

## Safety rule and scope

The path is restricted to dynamic buffers used exclusively as vertex or index
buffers in beginFullDynamicBufferUpdateForCurrentFrame. Uniform buffers and
mixed usage retain their existing path. A completed latest event marker is
required; unknown/pending completion keeps DISCARD. A bounded marker pool
polls without waiting or flushing.

CPU-recorded work matters even before D3D executes it. The experiment marks
the context unsafe on queued draws, indexed draws, resource-update batches,
dispatches and external-rendering sections. The marker after frame execution
resets that state. This deliberately rejects reuse after any such work rather
than trying to guess resource overlap. The current single-immediate-context
QRhi ownership/threading contract is assumed. This is not a general guarantee
for arbitrary native clients using exported buffers through other contexts.

Only a proven-safe enabled case uses NO_OVERWRITE. Failed reuse maps fall back
to DISCARD; neither live enabled run records a failed reuse map. The candidate
does not add a buffer pool, change contents/binding ranges, turn off media or
reduce decode concurrency. Source replacements reverse exactly to the prior
RHI probe, and it contains no staging-upload or uniform-capacity candidate.

The lifetime restriction follows the API's
[NO_OVERWRITE requirements](https://learn.microsoft.com/en-us/windows/win32/api/d3d11/ne-d3d11-d3d11_map):
the application must avoid overwriting data still used by the input assembler.
It is not a blanket substitution of map flags. Both comparison arms retain the
same query/eligibility observer; only the selected mapping policy differs.

## Tests and retained failure

Enabled and disabled safety-fixture runs each report **five passes**, zero
failures/skips, including setup/cleanup. Real vertex/index buffers test:

- reuse after a completed event following an actual GPU copy;
- forcing DISCARD after a queued read, even if the previous marker completed;
- preserving the bytes read by that queued copy while subsequent data changes;
- unknown/latest-marker mismatch falling back safely;
- constant buffers retaining DISCARD; and releasing queries on device teardown.

The fixture initially failed both geometry rows at a second empty-submission
marker: readback had already drained its copy. This repeated a known test
limitation and was a fixture mistake. Its source and failing result are kept.
A fresh GPU copy immediately before the marker makes the intended actual-work
test pass without extending timeout. No valid performance conclusion is drawn
from the empty-submission failure.

All **44 native rendering checks pass**, zero failures/skips, at expected actual
DPR 1.75/requested D3D11. Their cleanup verifies that reuse actually occurred:
1,902 NO_OVERWRITE maps, 109 unproven DISCARD maps, zero failed reuse maps.
The suite includes rendered geometry/order/opacity and scene-origin/unit-vector
checks; it does not prove every native integration or arbitrary external context.
Four import/compiled-host checks and the system-DLL audit also pass. No static
UI layout or portable embedded payload changes occur.

## Live on/off/off/on sequence

Actual Downloads, 190 entries, Masonry density 152, actual 160 Hz/DPR 1.75.
Each fresh process has an empty separate application cache and two ten-second
rapid-wheel blocks. All captures pass unchanged focus, input-count, capacity
and target-PID presentation checks. Analysis also verifies bounded device
metadata and zero query-creation errors. No builds/tests, offline analysis or
UI Automation observer run during input. OS disk cache and Go metadata are not
cleared. Default workers, hardware decoding and all media remain supported.

| Arm / Qt PID | NO_OVERWRITE / DISCARD calls | Admitted NO_OVERWRITE max | DISCARD max | Cold / next missed refreshes | Last stored entries |
| --- | --- | ---: | ---: | --- | ---: |
| On / 62116 | 278,043 / 919 | 0.1273 ms | 2.1359 ms | 7 / 1 | 187 |
| Off / 52516 | 0 / 251,231 | n/a | 2.4179 ms | 7 / 2 | 204 |
| Off repeat / 79952 | 0 / 257,865 | n/a | 1.8875 ms | 7 / 1 | 205 |
| On repeat / 86448 | 258,762 / 2,082 | 0.2885 ms | 2.3071 ms | 5 / 4 | 206 |

Map counts cover recorded process activity; maxima use calls wholly within
the measurement windows. Map records are thresholded at 50 us, so absent slow
records do not mean zero cost. The enabled slow-map counts are nine/twelve
admitted records, plus eleven/ten DISCARD records; controls have 83/59 slow
DISCARD records. Actual QRhi map API brackets exclude the completion test;
outer legacy RHI wrappers include its overhead and are not substituted for
inner map times.

Stored entries are cache snapshots, not unique files or per-item latency.
Different work completion and two ordered comparisons are not statistical
proof of a presentation improvement. All blocks have zero coverage holds but
still fail the no-spike/motion end state. Max display intervals remain roughly
12.5–18.8 ms. The previous 13.83 ms geometry-map outlier is not reproduced in
these controls, so its elimination cannot be claimed from this comparison.

Completion was proven for about 99% of these direct geometry-map calls. This
supports reuse eligibility for this workload rather than an experiment that
never changes the relevant path. The live queued-work rejection count is zero
because these maps occur before command batches are queued; the queued-work
fallback is exercised by the native safety fixture. Failure/pending cases
remain on the original path.

## Remaining stalls and decision

The first enabled capture still has a **7.6152 ms CreateBuffer** for an actual
159,324-byte dynamic vertex buffer. Exact creation/resource matching confirms
the descriptor, rather than inferring it from a label. The repeat has a
**6.8236 ms CreateTexture2D** for a 550x308 BGRA shader-resource texture.
Texture uploads still reach 6.8311 ms. Controls have texture creations near
10 ms and uploads near 8–10 ms. These API wall times do not identify CPU,
GPU execution, descheduling or a lock owner without a same-capture scheduler
trace.

The local map-latency reduction repeats; consistent whole-scene improvement
does not. Keep the guard experimental. Next work must address the remaining
creation/upload paths and how they interact with this validated map path,
without converting a local success into an unsupported rollout. Classify
actual texture-factory/atlas behavior before changing it; do not assume every
slow texture creation is a large atlas. Measured examples include ordinary
443x248 and 550x308 textures.

Sources, tests (including the failed fixture), receipts, native rendered
captures, all four live packets and analysis are frozen in the matching
experiment directory. Large binaries have external hashes. Only diagnostic
process trees were stopped; the normal portable app remains unchanged/alive.
No commit or push.

AI assistance has been used to create this output.
