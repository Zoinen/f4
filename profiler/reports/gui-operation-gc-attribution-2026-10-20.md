# GUI operation and GC attribution, 2026-10-20

The QML-engine pause is now identified as collector work, not merely inferred
from its receiver class. A 6.5351 ms atomic sweep overlaps a delayed GUI update.
Separately, cold catalog-row delivery consumes 2.9324 ms without GC. Warm windows
contain no scene-application operations but still contain GC pauses.
No production code or GC setting changed. Constant 160 Hz remains incomplete.

## Current-state measurement

Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz display, actual
wheel input every 30 ms. Every run contains two 10-second cold and two 10-second
warm measurement windows. The compiled geometry pool is 16 MiB per category;
there is no pool override. The original collector retains its 5000 us budget.
The diagnostic pixmap cache is 2 MiB. No build, test, offline analysis, UIA
observer, or scheduler trace runs during input windows.

The existing original-collector GC-phase library is reused. It substitutes the
memory-manager translation unit with timing scopes, not incremental-sweep or
GC-budget changes. Actual-link receipts hash that library, its source and probe
header, plus the current Quick library and explicit renderer/controller objects.
The scene-apply translation unit is replaced explicitly in the link response;
there is no duplicate-symbol override.

New fixed-buffer scopes cover applyFrameDecoded, dispatch, finalization, stream
commit, and synchronous catalog row/metadata signal delivery. Labels contain
only protocol message types. Nested spans must not be summed. No per-message
logging or JSON construction runs inside these scopes. Dispatch, phase, operation,
and GC records all remain below their caps. Durations are wall time and include
instrumentation and possible preemption.

## Exact application and GC paths

In the GUI control's cold window:

- panel_catalog_rows apply: 2.9398 ms. Finalization: 2.9337 ms. Synchronous
  panelCatalogRowsReceived delivery: 2.9324 ms. Dispatch and stream-commit work
  are tiny by comparison. The row delivery overlaps a 2.8470 ms callback-to-GUI
  update delay. This does not prove the whole time is one model function.
- panel_catalog_metadata delivery peaks at 1.7317 ms.
- A GC state-15 DoSweep event takes 6.5351 ms, entered from event-loop
  continuation (trigger 3), and overlaps a delayed update with 5.2707 ms
  callback-to-delivery latency. A separate DoSweep takes 5.4811 ms.

Warm windows contain zero scene-application operations. DoSweep reaches
5.5401 ms; MarkDrain reaches 4.7505 ms. Another delayed update overlaps an
allocation-triggered series of MarkWeakValues steps followed by a 2.5038 ms
MarkDrain, entered from allocString (trigger 1). Thus queued scene application
does not explain the warm pauses, and collector work is not solely asynchronous.

These results agree with the earlier atomic-sweep investigation. The rejected
1 ms budget trial and incremental-sweep lifetime experiments are documented in
the prior reports; this run does not repeat or promote those prototypes.

## Native motion comparison

Long intervals mean greater than 9.375 ms, not the stricter 6.25 ms nominal
period. Display intervals require both display endpoints and the current present
inside input windows. Producer intervals require both producer endpoints inside.

| Run | Input seconds | Long producer intervals | Long displayed intervals | Max display gap |
| --- | ---: | ---: | ---: | ---: |
| GUI control, all probes | 40 | 9 | 5 | 12.5053 ms |
| Native, vsync observer absent | 40 | 5 | 1 | 12.4916 ms |
| Native repeat, matching probes | 40 | 9 | 0 | 6.4108 ms |

The first native run omitted the optional vsync observer. It is retained and
labelled; only the repeat matches the GUI control's probe selection. Fresh
processes and evolving cold metadata are further limits. These few runs do not
prove a universal smoothness benefit or justify enabling native motion by default.

Each native run contains 6400 within-window adjacent render-thread motion pairs
from the same animation job. Neither has a coverage stall where requested motion
advances at least one physical pixel but constrained visual motion advances less
than one physical pixel. This checks the numeric camera, not visual completeness
of every newly revealed thumbnail or panel-chrome behavior.

The first native capture contains a 6.3979 ms sweep during which a camera commit
is recorded. Native motion is therefore not wholly tied to GUI progress. In the
matched repeat, a cold panel_catalog_rows delivery reaches 10.1099 ms and atomic
sweep reaches 6.5298 ms, while the displayed intervals stay below 9.375 ms.
These are overlapping work, not additive exclusive CPU costs.

## Start-of-motion distinction

The first native run's displayed gap occurs 8.8867 ms after the second warm
window begins. Its previous displayed present was issued before the input
window, although both display-change endpoints lie inside the window. Its
producer interval is therefore excluded from steady input statistics while the
displayed gap is correctly retained in the table. The GUI control's displayed
misses occur 120 ms to 5.7 seconds into scrolling, with previous presents inside
the input windows. Do not hide the native startup gap or mislabel it as a
steady-scroll GC failure. Start-of-motion latency remains part of the goal.

## Consequences and next work

The evidence supports two distinct work streams:

1. Trace cold catalog-row delivery into bridge normalization/model application
   and synchronous delegate rebinding. Verify redundant work before changing
   transaction boundaries, request sizes, or notifications.
2. Complete native-motion lifecycle and panel-chrome validation, including
   startup after idle, cancellation, source/layout changes, and rendered content
   coverage. The current syncNativePose still publishes logical contentY on GUI
   callbacks; native rollout is intentionally not enabled yet. Confirm delivery
   and motion through busy GUI periods with longer clean, alternating captures.

The atomic collector cannot be bounded by just reducing a nominal time limit.
Any collector scheduling/granularity change remains a separate correctness task
with the cached-shape lifetime regression and original collector as controls.
The current data does not establish that every native rendering spike is solved.

The normal embedded worktree app remains responsive with host SHA256
`cc59ce7f9aa7bf6d2e40154bee453666b75424b6df079b725fd127697851756d`.
Only exact diagnostic launcher trees are stopped. No commit or push was made.
Sources, receipts, native captures, analysis, and a verified manifest accompany
this report; executables and raw platform traces are not added to the archive.

AI assistance has been used to create this output.
