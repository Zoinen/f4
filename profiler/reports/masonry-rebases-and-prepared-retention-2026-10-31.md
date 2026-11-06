# Masonry rebases and bounded prepared-root retention, 2026-10-31

## Outcome

Reject bounded prepared-root retention as a scrolling optimization. The complete
same-host control increases preparation tails and display misses. It is left
as an isolated diagnostic prototype; production source, the installed Qt SDK,
the embedded portable payload and the normal worktree app are unchanged.
The constant-160-Hz goal remains active and unachieved.

## Rebase hypothesis checked against preserved captures

`artifacts/analyze-loading-rebases-20261031.py` verifies the original raw SHA-256
receipts and analyzes all six preceding pipeline-key captures. Cold first
windows have 25–30 rebase notifications each, almost all with zero anchor delta.
Cold reverse windows and all warm windows have no rebases. Across the four cold
first windows, only two renderer-body durations above 3 ms occur within 20 ms
after a rebase; 162 occur outside that neighborhood. Cold reverse windows still
have 10, 38, 39 and 62 renderer-body durations above 3 ms despite zero rebases.
Thus rebases do not explain most spikes. These body durations exclude the
before-body updater work and are not the complete native preparation phase.

Subtree changes correlate much more strongly with cost. For cold A2, renderer
body p99 with subtree addition/removal is 3.668/3.949 ms versus 1.216/1.251 ms
without it. Upload p99 is 1.547/1.477 versus 0.089/0.078 ms. Warm A1 shows the
same association at lower cost: body p99 1.481/1.588 versus 0.497/0.535 ms.
These are associations, not proof that each counter event causes a spike.
Addition/removal counters count Qt shadow-subtree visits, not allocations.

## Controlled candidate

During native Masonry motion only, already-prepared delegate roots intersecting
the viewport plus one viewport above/below receive the existing prepared-scene
retention binding. The normal viewport clip, pending-publication guard, source
identity, geometry, target-leading materialization, media requests and SVG
rendering remain intact. Clearing the owning camera restores ordinary culling.
The experiment does not enlarge the materialized-file or decode working set.
The observed peak is 118 retained roots; the counter is zero with the flag off.

This tests whether avoiding exact screen-edge subtree removal is useful. It
does not claim that all descendants stop changing or that materialized roots
remain allocated indefinitely. No extra rounding or per-leaf transform is added.

Correctness checks pass in both modes:

- 31 helper cases, including bounded-region edges and fractional geometry.
- 46 native renderer cases at actual DPR 1.75.
- Seven native motion cases, including offscreen batch residency with no pixels
  escaping the viewport, owner clearing, pending publication and leaf transforms.
- Thirty paired rendered PNGs are byte-identical; the small thin-stroke caption
  capture was inspected.
- Three installed-tree-independent QML import checks and system-DLL audit.

## Initial incomplete capture and corrected probe

The first candidate capture exceeded the upload-route probe capacity: 870,244
reservations versus 524,288 rows. It remains marked capture-invalid. Frame,
native-phase and motion streams were complete, and an ordinary PID-filtered
PresentMon CSV was reconstructed for limited diagnostic analysis. The original
capture return code remains 1. Its apparent reverse-direction reduction from
113 to three misses is not acceptance evidence: recording behavior changed
after overflow. Raw data, the failed guard, analysis failure logs and limitations
are preserved.

A fresh executable changes only the upload probe capacity to 2,097,152 in both
the renderer and host snapshot reader. Both control modes are repeated with
this identical executable, original acceptance gates and fresh isolated caches.
All correctness/import/dependency checks pass again; all thirty image pairs
remain identical. Both captures pass capture-validity checks and have complete
upload streams: 401,897 events in A versus 868,338 in B.

## Complete live results

Actual Downloads folder, 192 entries including parent, density 152, normal
initial viewport, desktop 3840×2160 at 160 Hz and DPR 1.75. The ordinary other
panel remains present. Two 30-second wheel windows per capture. Native
foreground is stable, with zero inactive wheel events. No screenshot, UIA,
build or analysis runs during the measured gestures. Pipeline-key mode remains
original in both arms; typed registry/word hash remain on, uniform snapshot
observer remains off. No external workload, priority or affinity change.

| Arm / pass | Preparation median ms | Native preparation p99 ms | Display misses >9.375 ms | Maximum motion error px |
| --- | --- | --- | --- | --- |
| A / 1 | 0.5465 | 3.1967 | 84 | 204.338 |
| A / 2 | 0.5340 | 3.4293 | 111 | 122.230 |
| B / 1 | 0.7745 | 5.2668 | 119 | 121.473 |
| B / 2 | 0.7660 | 5.1698 | 139 | 234.376 |

All four windows have zero coverage holds and fail the original strict display
and maximum-2-px motion gates. No failed pair/outlier is removed. Native phase
and renderer-body tools use slightly different percentile indexing; the table
uses the native phase summary consistently.

Keeping more roots active reduces blocked notifications but increases overall
renderer work. A's two windows have 510,320/489,721 addition visits versus
B's 698,106/702,483; removals rise from 447,075/429,363 to 610,984/615,293.
Whole-capture updater visits rise from 4,871,967 to 8,730,624, while shared
uniform queries rise from 1,926,902 to 2,896,492. These counters explain why a
lower blocked-notification count alone cannot justify the change. The small
A/B sample establishes no universal performance guarantee; it is sufficient
to reject promotion of this candidate under the measured workload.

## Build isolation recovery

The first helper compile inherited a forward-slash `/Fo` path and accidentally
overwrote an older diagnostic helper object. Captured executables, raw traces,
source and the normal app were unaffected. The older object was rebuilt from
its unchanged source/command. Restoring only its COFF build timestamp reproduced
the complete original SHA-256 from the pre-existing frozen receipt. The
incorrect object and recovery receipts are retained. New compile commands
validate their resolved `/Fo` destination before execution. The import-only
link graph has different controller membership; its renderer membership was
checked separately when resuming the expanded-probe build.

## Evidence and next scope

`artifacts/loading-rebases-20261031/analysis.json` contains the rebase/culling
analysis; `retention-summary.json` includes both the incomplete preliminary run
and complete controls, with validity flags. Build receipts, exact sources,
helper/native/motion tests and images reside under `prepared-root-retention-20261031`
and `prepared-root-full-probe-20261031`. Raw packet locations/hashes are in the
per-case `postsync-probe-20261031` receipts. Only verified duplicate packets are
retired. Old build relocation and transparent raw compression preserve file
contents and original usable paths, with SHA-256 receipts.

Final verification confirms that all three diagnostic build receipts still
match, including the restored original helper object. Neither diagnostic host
is running; both normal app processes remain alive at their original executable
hashes. Transparent NTFS compression of the completed older `native-cpu.etl`
recovered about 1.12 GB on D without changing its logical bytes or path (SHA-256
checked before and after). Final-state and compression receipts are included.

The next investigation should identify which changes invalidate batch geometry
and expand updater traversal, distinguishing delegate retirement, screen-edge
culling and intrinsic preview gates. A replacement must reduce that work
without increasing the active scene as this experiment does. Do not promote
the candidate, broaden retention, or infer a fix merely from the correlation.
The verified-warm and middle-motion acceptance scopes remain required for any
future successful candidate. No commit or push was performed.
