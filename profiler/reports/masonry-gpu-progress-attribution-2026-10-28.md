# Some map stalls occur after earlier render-context work has completed

A nonblocking event-query probe confirms earlier-frame completion before 16
of 19 render-thread map/upload calls exceeding one millisecond in the measured
scrolling windows. Three of those calls also have no later tracked commands on
that context. This narrows the explanation beyond waiting for an earlier draw.
It does not prove the entire GPU was idle, exclude another device or identify
a lock owner. No production optimization is made; constant 160 Hz remains open.

## Probe meaning and validation

The diagnostic inserts a bounded D3D event marker after the render context's
frame commands/resolve and before Present. Before map/upload calls, it polls
the latest marker with `D3D11_ASYNC_GETDATA_DONOTFLUSH`. It never waits or flushes
for the live probe. Eight query slots per device and 32 device slots are
bounded; exhausted slots produce an unknown classification rather than a wait.
Queries are released before device destruction.

Tracked draws, dispatches, transfers and clears since the marker are counted.
A completed marker with later commands is not treated as proof that current
frame work has completed. A pending marker is also not proof of GPU saturation:
submission/driver behavior can delay its signal. The live output distinguishes
complete, pending, query error, no marker and unmarked-latest-frame cases.

API wall brackets start after the polling operation. Original RHI call brackets
contain the new polling overhead, and are not substituted for the inner API
times. Resource identity is joined by pointer plus owning device and the most
recent preceding creation record. Exact containing RHI brackets supply row
pitch/map type. Source transformations reverse exactly to the prior RHI probe;
all original COM arguments/results and resource policy remain unchanged.

The lifecycle fixture initially failed twice when testing a second marker
after only synthetic CPU activity. Those failures and the initial source are
retained. Adding real buffer uploads between markers makes the intended
GPU-work test pass in 199 ms, without extending the timeout. This validates
the actual-work case; it does not establish a universal empty-submission
completion guarantee. The enabled and disabled fixture invocations each
report three passes including setup/cleanup. They check completed/unknown
classification, tracked-activity reset, value/reference forwarding, thresholded
recording and query release. The fixture uses Flush only to submit its test
work; live probe code does not.

The instrumented native suite passes 44 checks with no skips/failures at
expected actual DPR 1.75, requested D3D11. Import/compiled-host checks pass
four cases, and the system-DLL audit passes. No static UI layout changes.
These are diagnostic gates, not a completed portable rollout.

## Actual capture

Downloads, 190 entries, Masonry density 152, actual 160 Hz and DPR 1.75.
Qt PID 123436, render thread 26576. Two ten-second rapid-wheel blocks use a
fresh isolated application cache. Focus, input count, recorder capacity and
target-PID presentation checks pass. OS disk cache and Go metadata are not
cleared. No builds/tests, offline analysis or UI Automation polling run during
input. Default four workers, hardware decoding, all media types and production
atlas policy remain; the preceding experimental optimizations are absent.

| API and resource | Actual API wall time | Earlier-frame marker | Later tracked commands | Query poll time |
| --- | ---: | --- | ---: | ---: |
| Atlas upload, 4096x4096 BGRA, row pitch 560 | 12.8550 ms | Complete | 4 | 0.9 us |
| Same atlas, row pitch 996 | 11.3218 ms | Complete | 6 | 0.5 us |
| Map WRITE_DISCARD, 1,536-byte dynamic constant buffer | 6.5401 ms | Complete | 0 | 1.5 us |
| Same atlas, row pitch 580 | 4.7150 ms | Complete | 10 | 0.2 us |
| Same atlas, row pitch 1872 | 4.0946 ms | Pending | 4 | 0.5 us |

The constant-buffer case has a completed matching latest marker and no later
tracked commands. It exceeds the 6.25 ms frame budget despite completed earlier
context work. This is consistent with the earlier scheduler allocation-lock
stacks, but a scheduler trace was not collected here; this capture cannot
separate execution, descheduling and blocked time or establish the same lock
path for this individual call.

The upload cases all refer to native atlas resource `75209fa0`, created earlier
on the same render device. Later commands exist, so the completed marker cannot
rule out dependencies on current-frame transfers. The row pitch is source
stride, not total transfer bytes or proof that the whole atlas was uploaded.
These are the original ordinary texture-upload path, not readback or video
conversion uploads.

## Overhead and control

The render device records 3,238 frame markers, zero skipped markers and zero
query-creation errors. Worker devices have no window-frame markers and their
unclassified records are not used to claim completion. Across device lifetime,
941,077 polls consume 227.4505 ms of measured aggregate polling wall time,
about 0.2417 us each. This excludes some surrounding lookup/counter/recording
cost and marker command cost; it is not a complete observer-overhead estimate.

A fresh same-executable control disables GPU-progress queries/markers while
retaining the original RHI recorder. It confirms zero GPU-progress records and
device slots. Both captures retain valid presentation data:

| Arm | Cold / next display gaps >9.375 ms | Cold / next max gap | Coverage holds |
| --- | --- | --- | --- |
| Probe enabled / PID 123436 | 10 / 3 | about 18.77 / 12.64 ms | 0 / 0 |
| Probe disabled / PID 35888 | 4 / 2 | about 12.54 / 12.51 ms | 0 / 0 |

This single ordered pair does not isolate query overhead from loading and
run-to-run variance. The enabled capture is for attribution, not a clean
performance baseline, and does not establish an improvement. Both still fail
the requested no-spike/motion end state.

## Next intervention

Do not assume that waiting for a previous frame or reducing small explicit
allocations alone will fix the stalls. The next focused texture experiment
should compare the current UpdateSubresource path with a bounded staging-copy
path for ordinary atlas subregions. Reuse only completed staging slots, poll
without waiting, preserve source pitch/region/pixels, and retain the original
path for unsupported or exhausted cases. Measure complete thumbnail throughput,
memory and actual display/motion. Previous smaller-atlas and single-padded-upload
experiments already lacked a repeatable cadence benefit and should not be
repeated as if they were new fixes.

Dynamic constant-buffer write batching is another target, but requires correct
binding ranges and lifetime management; blindly using NO_OVERWRITE could corrupt
data still used by another draw. Any candidate needs rendered/native regressions
and live validation before rollout.

Sources, tests (including failures), receipts, enabled/disabled packets and
analysis are frozen in the corresponding experiment directory. Large binaries
have external hash references. Diagnostic processes were stopped; the normal
portable app is unchanged and remains alive. No commit or push.

AI assistance has been used to create this output.
