# Masonry: persistent preprocessing entries

The incremental prototype removes most original ancestor queries and improves
median CPU preparation in one valid control/candidate pair. **Constant 160 Hz
without spikes remains unachieved.** Preparation tails and missed displayed
intervals remain. Later trials fail workload/focus guards, so they do not
establish a repeatable improvement. This remains a diagnostic host; no production
Qt library, embedded payload, normal application, commit or push is changed.

## Implementation

The prior prototype discarded its whole dependency table whenever a known
preprocessing entry changed. This version keeps records by node identity, with
stable reusable record IDs and a fast previous-order lookup. QSet order or
membership changes no longer discard unrelated records. Reverse ancestor sets
identify which records to retire on removal/reparenting; local block/opacity
changes invalidate only affected values. Retiring a record unlinks its stored
ancestor edges without dereferencing removed nodes. Reused addresses start with
fresh records. The original Qt snapshot and deleted-node skip remain intact.

Forced updates, unknown notification provenance, root/updater changes, callback
mutations and unsupported updater types retain conservative original-query or
full-reset behavior. Overflow entries use original queries; a capacity failure
does not repeatedly rebuild a full table on the next unchanged pass. Capacity
rollback does not advertise freed capacity when the graph returns to the same
occupancy. Callback-time cache storage remains alive until the active scope
exits, including renderer/updater replacement.

Logical limits remain 16,384 records, 32,768 ancestor keys and 131,072 edges per
renderer, eight renderers per thread. The live runs have no capacity fallback.
These limits are not an exact allocator-byte budget. Retained hash/set bucket
capacities after pathological history still need an allocated-memory audit and
hardening before promotion.

## Regression and native verification

Before implementation, `knownReplacementPreservesUnrelatedCachedAnswer` fails
against the prior dependency prototype, with its feature enabled: replacing one
known entry unnecessarily repeats an unrelated leaf's original query. The
failure is preserved. An initial test-build error caused by not passing the
baseline preprocessor define to moc is retained separately and is not counted
as that regression.

The final prototype passes:

- 12 helper results covering persistent replacement, unrelated changes,
  targeted block/opacity invalidation, reparenting, equal-size membership
  replacement, ancestor-address reuse, callback mutation, root exclusion,
  forced fallback, updater-type guards and link-capacity fallback.
- Nine existing eligibility results, recompiled against the expanded state.
  The ordering test now expects correctly reused answers when the feature is
  on, while retaining original answer checks and the old expectation when off.
  The capacity fixture allows unaffected admitted records to remain cached;
  overflowing entries still execute original queries, within the edge bound,
  without repeated full-table rebuilds.
- Three actual native mutation results (including init/cleanup), with block,
  opacity, reparenting, same-address replacement and preprocessing-flag changes.
  Actual DPR 1.75, native D3D11, scene/unit-transform and rendered-color checks.
  The rendered visible fixture was inspected.
- 48 native scene tests, 27 shader tests, three embedded-QML import-isolation
  results and the Windows system-DLL audit. Existing baseline graphics flags
  are retained. No static UI layout is changed.

## Observer failure and isolation

The first cold control completed its gestures but PresentMon produced no CSV.
It is rejected. PresentMon subsequently exited zero without a CSV during an
independent native fixture calibration as well. A short owned raw ETW capture
contains DXGI Present events and 618 VSync records, with zero lost events,
buffers or property errors. The input desktop is Default and WTS session flags
indicate unlocked; a locked desktop is not assumed to explain the failure.

Disabling **GPU-duration tracking only** restores PresentMon output while
keeping display tracking enabled. The native calibration records 93 displayed
rows, positive display-completion times and no dropped rows. Offline API-only
replay also produces output, but API-only data is not used to pass display gates.
The underlying reason that GPU-duration tracking prevents final output is not
resolved.

All subsequent comparison runs use the same `--no_track_gpu` observer mode,
with display tracking and the original display/motion/coverage gates unchanged.
No GPU-duration claim is made. Comparisons below are within this observer mode,
not pooled with older captures using GPU-duration collection.

## Valid pair

Real Downloads, 191 entries, Masonry density 152, 3840×2160 / 160 Hz / actual
DPR 1.75. Fresh isolated application cache per process; OS caches not flushed.
Two ten-second rapid-wheel blocks. Same executable, with only the dependency
feature flag varied. Existing CPU, AV and buffer options remain enabled; normal
thumbnail workers, no conversion lane or context prewarm. No tests/builds/UI
Automation/offline analysis run during gestures.

The only fully valid pair is `display-a1` control (1,718 active wheel events)
and `display-b1` candidate (1,673). Both pass all capture-validity checks.

| Case | Block | Before-body median, ms | Preparation median/max, ms | Display max, ms | Gaps >9.375 ms | Motion max, physical px | Holds |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 control | 0 | 0.2635 | 0.6538 / 6.9295 | 12.4220 | 1 | 59.650 | 0 |
| A1 control | 1 | 0.2443 | 0.6132 / 7.2023 | 18.8013 | 10 | 114.036 | 0 |
| B1 candidate | 0 | 0.1139 | 0.5184 / 9.1026 | 12.5204 | 4 | 73.998 | 0 |
| B1 candidate | 1 | 0.1390 | 0.5535 / 8.4420 | 12.4394 | 3 | 62.873 | 0 |

Process-total original ancestor queries fall from **723,269 to 29,651**;
cached answers rise from 373,852 to 1,111,183. Candidate full-graph resets are
**12**, individual path builds 22,447, explicit entry retirements 18,566 and
order lookups 481,080. These totals include startup/loading; retirements do not
include records discarded by full resets. Peak graph: 2,703 ancestors,
10,763 links, zero overflows or capacity bypasses.

This supports the mechanism and a median CPU saving, but not elimination of
spikes: candidate preparation maxima are worse, every block fails the original
maximum-motion gate, and the pair still has seven candidate missed intervals
versus eleven in control. One ordered pair is not a population-level result.

## Retained rejected trials

The intended repeated comparison is incomplete. Rejected data is retained and
not silently rerun until a favorable result appears:

| Trial | Wheel events | Inactive events | Validity |
| --- | ---: | ---: | --- |
| Original A1, default GPU observer | retained | retained | no PresentMon CSV |
| Display B2 | 1,349 | 0 | below 1,500-event requirement |
| Display B3 | 1,485 | 1,081 | input-count and focus failure |
| Display A2 | 1,527 | 845 | focus failure |

All display trials report 160 Hz and DPR 1.75. B2's descriptive timing still
contains severe stalls: 43.7573 ms maximum display interval, 14/87 missed
intervals and one coverage hold in its second block. This is not ignored as
irrelevant merely because the workload is invalid. B3/A2 fail the original
motion analyzer's active-window assertion; their motion results remain
unavailable. Offline target-row extraction is labeled in the analysis source
and does not change the rejected workload status.

The cause of later focus/input-rate instability is unresolved. It also affects
the later control, so it is not attributed to the candidate alone. Native
foreground and scheduling correlation is needed before collecting a complete
repeat matrix. The user has authorized captures without readiness questions.

## Next work and evidence

Keep the CPU candidate diagnostic, audit retained allocation capacities, and
resolve capture focus/input-rate instability. Repeat cold and verified-warm
wheel plus quarter-height middle with the original zero-miss, maximum-motion
and zero-hold gates. Then use a current-code scheduler/graphics trace to target
the remaining preparation tails and buffer/driver waits; reduced medians do
not establish the goal.

Frozen evidence:
`profiler/experiments/masonry-preprocess-incremental-2026-10-29`.
Source, failed/passing tests, observer calibrations, captures and analyses are
retained. Large executables/objects and raw observer ETW/exports have external
hashes. Inactive binaries and completed packets were NTFS-compressed with
before/after SHA256 equality. Only the exact diagnostic launcher tree was
stopped; normal worktree PID 43936 was verified alive.

AI assistance has been used to create this output.
