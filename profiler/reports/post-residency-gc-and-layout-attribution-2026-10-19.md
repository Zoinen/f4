# Post-residency GC and layout attribution

Constant 160 Hz remains unachieved. This investigation changes diagnostics and
records new evidence; it does not change the running application's GC policy.
The previously deployed bounded thumbnail texture cache remains enabled.

## Correcting the runtime configuration assumption

The recent controlled captures explicitly set QV4_GC_TIMELIMIT=2. The normal
host does not set that environment variable or call setGCTimeLimit. Qt 6.11.1's
GCStateMachine constructor assumes 60 fps and chooses `(1000 / 60) / 3`, or 5 ms,
when the variable is absent. A fresh diagnostic launch without the override
confirms gcBudgetUs=5000 and an empty gcTimeLimitEnv. Thus those earlier 2 ms
captures are controlled experimental configurations, not exact reproductions of
the default running application's GC setting. The cache comparisons retain their
relative meaning, but must not be described as production-default absolute timing.

## Acquisition

Current production Gallery core, original delegate roots and nearby scheduling,
bounded texture residency, original Qt GC algorithm, D3D11, DPR 1.75, 160 Hz,
2194 × 1186, Downloads 186 entries, density 152, 30 ms boundary-bounce wheel input.
Each stage contains two ten-second measured windows. Cold and warm share the
same process/engine. Builds, analysis and UI Automation do not run during capture.

The diagnostic inserts native scopes into the current delegate TU and the exact
Qt GC/incubation TUs. The Qt qv4mm diff contains observers only; no smaller mark
batch, incremental sweep, ownership change or spare-delegate prototype is used.
GUI preparation buffers remain below 262,144 records; GC/incubation below 32,768.
The minimal recorder has no per-frame scene observer. Layout snapshots serialize
before/after input; bounded height-change events record only layout changes.

| Configuration | PID / GUI / render | Matched cold/warm switches | QPC offset spread |
| --- | --- | --- | --- |
| Explicit 2 ms | 101520 / 78352 / 118928 | 75,558 / 65,646 | 900 ns each |
| Default 5 ms | 118720 / 85828 / 76588 | 80,549 / 99,313 | 900 ns each |

Raw QPC switch identity/order matching validates alignment with xperf relative
time. All four trace metadata records report zero lost events/buffers. Sampled
stacks preserve host ICF aliases; driver/kernel offsets remain unresolved.
Scope durations are wall time and overlap; they are not independent CPU totals.

## Actual default-setting misses

The 2 ms diagnostic has three cold displayed intervals above 9.375 ms and zero
warm. Its cold miss producer intervals are 11.6062, 15.3 and 9.2597 ms. GC-state
union within them is 2.0813, 3.2644 and 6.4162 ms. The first two also intersect
about 2.1/2.4 ms of incubation and 2.8/2.4 ms of native preparation. These scopes
overlap and must not be added as disjoint costs.

The production-default 5 ms diagnostic has five cold and five warm displayed
misses. Cold maximum displayed interval is 18.7128 ms; its 17.272 ms producer
interval includes 9.1115 ms of GC. Warm displayed intervals range 12.4968–12.5265
ms. Three warm corresponding producer intervals contain 6.4822, 6.2264 and
7.9481 ms GC; GUI scheduled running is 7.3398, 8.5287 and 9.7335 ms respectively.
The render thread mainly waits in those intervals. Samples identify GC transition,
mark drain and the atomic sweep. This corroborates the earlier atomic-sweep
finding; it does not justify reintroducing rejected incremental-sweep changes.

Two default warm misses and four default cold misses have no GC in their nearby
producer context. Warm non-GC producer intervals are 9.6177 and 9.1555 ms; their
render scheduled-running time is 4.6467 and 3.6265 ms. Nearby samples include SG
node/geometry updates, D3D11 shader-resource bindings and driver code, buffer-data
assignment and F4VectorGateFrame construction. Sparse inclusive samples do not
establish exclusive cost for any one function. The vector gate and resource
binding paths are candidates for bounded native timing, not proven bottlenecks.

The normal 5 ms nearby callback maximum reaches 7.0847 ms warm, including 6.2264
ms GC. Incubation maximum is 1.5543 ms warm with all requests specifying 2 ms.
GC setting, incubation budget and preparation budget are distinct controls.

## Explaining the extent variation

The old scrollExtent field is a start-of-measurement snapshot. New snapshots show
the explicit-2-ms cold content height changes 5172 → 5174 → 5189. Rows receive
real dimensions, including rows 113/114 changing from 16×9 metadata placeholders
to 1280×720 and 1172×720. Warm height stays 5189, revision 14 throughout input;
its texture pool holds 42,591,568 nominal bytes / 153 entries. No warm height
events occur.

The default cold pass changes 5172 → 4885 → 4888 → 4684 → 4720. The warm pass is
stable at 4720. Comparing final size inputs, exactly two rows differ from the
5189-height process: index 138 reports 1×1 instead of 1024×1024, and index 139
reports 1×1 instead of 812×9. Their neighboring Masonry rows reflow. Thus the
469-pixel difference is real differing fallback-size input, not merely the old
field being stale. File identities/reasons for these differing results remain
unproven. The attempted name role is absent/empty in this facade; do not invent
filenames from filesystem enumeration order.

## Settled same-process clean GC comparison — rejected policy change

A separate host uses normal Qt libraries, no native GC/preparation probe and no
WPR. After two settling captures, all four measured passes in each sequence use
the same PID/engine. Full row-size/geometry arrays match before and after every
pass and between budgets. No height-change event occurs during measurement.
Each measured pass contains two ten-second windows. All measured snapshots have
content height 4720, extent 3649.142857, the intended folder/window/DPR/density,
and active-window wheel events. GC budget is explicitly recorded as 5000/2000 us.

| Sequence / pass | Budget | Long displayed intervals | Maximum |
| --- | ---: | ---: | ---: |
| First A1 | 5 ms | 18 | 156.2507 ms |
| First B1 | 2 ms | 0 | 6.4524 ms |
| First B2 | 2 ms | 1 | 12.5002 ms |
| First A2 | 5 ms | 3 | 12.5229 ms |
| Reverse B3 | 2 ms | 2 | 12.5037 ms |
| Reverse A3 | 5 ms | 1 | 12.4976 ms |
| Reverse A4 | 5 ms | 3 | 12.5810 ms |
| Reverse B4 | 2 ms | 19 | 37.5037 ms |

The first sequence favors 2 ms (21 versus one miss); the reverse favors 5 ms
(four versus 21). Combined: 25 at 5 ms and 22 at 2 ms, 80 measured seconds each.
The large bursts have no native attribution in these clean captures. Their cause
is unknown; do not label the 156 ms or 37 ms outliers as GC based on a setting.
Heap history and other changing work are not reset by changing a budget. This
does not establish a repeatable reduction in missed refreshes, so no refresh-based
GC-budget production change is retained. It agrees with earlier caution against
blindly lowering constants, despite the configuration mismatch now being explicit.

## State and next measurement

The normal worktree launcher PID 123096 / cached host PID 34000 remains alive and
responding with the previously verified bounded-residency build. Diagnostic
launcher trees were stopped only by exact worktree-specific executable path.
No commit or push was performed. Large older derived CSV exports were losslessly
gzip-compressed and SHA-256 verified before removing their plain copies; original
ETL recordings remain local.

Next, attribute the intermittent bursts under unchanged production-default GC,
including forced versus incremental collections and native heap/cycle state.
Time vector-gate bookkeeping and D3D11 resource-binding preparation before
optimizing either. Verify source/fallback inputs when comparing different
processes. This turn establishes a reliable matched-layout harness and rules out
a budget-only fix; it does not establish constant 160 Hz.

AI assistance has been used to create this output.
