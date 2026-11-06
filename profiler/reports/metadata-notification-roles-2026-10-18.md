# Metadata notification roles and scrolling

## Change and scope

ExternalCatalogMetadataTransaction previously announced source URL, image type,
dimensions, path, version and file-size changes for every changed metadata row,
including display-field/highlight-only changes. Masonry treats IsImageRole and
image-size roles as geometry inputs and can synchronously rewrap the layout.

Display/highlight-only changes now notify VisualSnapshotRole. Source changes,
local-path/timestamp enrichment, restored dimensions and metadata-settled changes
retain the existing full role list. Adjacent rows are coalesced only when their
notification kind matches. Source replacement therefore cannot broaden a
neighboring appearance-only notification. Identical updates remain silent.

This removes a concrete redundant path, but **does not measurably improve the
tested scrolling workload** and does not achieve constant 160 Hz.

## Regression and compatibility

The new appearanceMetadataDoesNotInvalidateImageGeometry regression applies a
mixed four-row batch: two display/highlight changes and two source-size changes.
It checks two separate contiguous notifications, exact appearance-only roles,
the retained replacement roles, updated snapshot fields and silent repetition.

The initial synthetic fixture omitted stable paths, unintentionally causing a
source-identity change. It was corrected before final verification. An artifact
control test links the original metadata translation unit from the submodule's
HEAD ahead of the current core archive; it uses the corrected fixture and fails
(one broad four-row notification instead of two distinct ranges). The normal
candidate passes. There is no duplicate-symbol override and no stale core
archive in this control test.

Selected tests at DPR 1.75 pass on Direct3D 11 and OpenGL: 13 Gallery results,
six Masonry mode results and four import/shader results per backend. These cover
revisioned metadata, deferred-version preview requeueing, cached metadata
batches, opaque source materialization, late/failed/portrait video metadata,
sparse thumbnail publication, complete-row geometry commits and cached-metadata
page navigation. No static UI layout was changed by this patch.

## Clean 160 Hz comparison

Four fresh-process A1/B1/B2/A2 captures use Downloads, 186 entries, density 152,
extent 4101.142857, active 160 Hz, DPR 1.75, Direct3D 11 and 30 ms actual wheel
events. A is the current shared-factory host before this patch; B changes the
metadata transaction only. Both retain the combined renderer and normal 5000 us
GC budget. No WPR, QML profiler, UI Automation observer, build, graphics test,
object census or model-role observer runs during these intervals. Caption
measurements are zero. Strict display intervals have both endpoints inside
their direction's scrolling window.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3403 | 12.5053 | 4 | 6.2880 | 6.3671 | 0 |
| B1 | 6.3400 | 12.4988 | 4 | 6.2873 | 6.3458 | 0 |
| B2 | 6.3489 | 12.5238 | 4 | 6.2895 | 12.4940 | 1 |
| A2 | 6.3368 | 18.7698 | 2 | 6.3116 | 12.4781 | 1 |

Controls have seven long intervals and candidates nine; neither the similar p99
nor a lower candidate maximum establishes a gain. These small counts and run
variation do not establish a regression either. The previously captured GC and
non-GC spikes remain targets.

## Live role counter and next action

A separate diagnostic capture enables F4_PROFILE_MODEL_ROLES. A native callback
counts model dataChanged notifications only during active scrolling; it performs
integer increments and serializes after capture. It observes 84 signals covering
195 row notifications, with zero snapshot-only signals/rows. Row counts can
include the same row more than once; they are not distinct file counts.

This explains why the display-only fix is not a demonstrated remedy for the
live workload. Broad notifications here may be legitimate source changes or
decoder completion; the counter does not identify their cause. Next, record
which source/geometry fields actually change and attribute the nested QML
variable-property/binding updates seen in the prior non-GC CPU trace. Do not
infer that all broad notifications are redundant, and do not treat another
factory micro-reduction as a proven fix for these bursts.

## Deployment and evidence

The normal static host builds and passes the Windows system-DLL audit. Embedded
payload generation, Go CGO_ENABLED=0 build and embedded-host extraction, reuse,
concurrency, corruption and lookup tests pass. The worktree launcher is replaced
after those gates and directly launched with --gui=qt. Its content-addressed
child hash matches the newly built normal host; stderr is empty. Only the
worktree process tree was terminated. No commit/push was requested or performed.

Host SHA256: EEDAA56C4415B1C310D3D496FC30D1C562277A1D3B4615A3D1C7897D870FE619.
Launcher SHA256: EABE714320C603B480F51040F37E416A21D6C330417C6C126765C4D6CE3DA695.

Compact traces, test results, diagnostic source and comparison scripts are under
profiler/experiments/metadata-notification-roles-2026-10-18 with SHA256 manifest.
Executables, raw PresentMon CSVs and build output are excluded from that bundle.
