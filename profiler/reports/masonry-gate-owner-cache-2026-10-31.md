# Gate ownership cache and phase attribution, 2026-10-31

A frame-scoped ancestor cache passes ownership and native-rendering regressions,
but its four live runs do not establish a consistent timing improvement. It is
not deployed. The constant-160-Hz goal remains active and unmet.

## Implemented diagnostic candidate

VectorGateOwnerScratch20261031.h memoizes the nearest registered gate root,
including absence of a gate, while the scene hierarchy and registry membership
are immutable during the constructor. It retains only node/root identities;
ready/present flags and batch membership are still checked by the original
logic. It does not retain a scene answer across constructor calls.

Storage is lazy and bounded: four leases per render thread, 8192 entries per
lease, up to 32 probes per lookup, and up to 256 walked nodes remembered per
query. Longer ancestry still follows the original walk to its answer. Saturation,
collisions or exhausted nested leases lose caching, never correctness. Epoch
wrap clears generation tags; stale pointers are never dereferenced. Maximum
entry storage is 0.75 MiB per 64-bit render thread, freed at thread exit.

The integration replaces one ancestry loop in the current owned production
batch renderer. It preserves renderer objects/libraries from the current build,
including bounded ordering scratch reuse; main.cpp adds the same frame recorder
used by the controls. Original/cached paths share one binary, selected only at
process startup. Native host SHA-256:
ebe3e759933a3a845ae86d9a329617279726ce7d644995f9d871194421d32139.
Go bytes match the current packaged app, SHA-256:
ac68c2e7c2fd259b760f91fc7c001be26d77ec2b33c86c36d1f22893a53fe613.

The constructor also has an earlier upload-membership revalidation walk. The
first candidate covers the later owner-map construction pass only. This limited
scope is explicit: it must not be described as eliminating every gate walk.
The first phase-probe generator asserted that a repeated source anchor appeared
once and stopped. Inspection found the two walks; the corrected probe measures
both. That setup failure is retained. The subsequent measurements below do not
support assuming that the unoptimized earlier walk is the dominant cost.

## Correctness verification

The new standalone C++ regression performs 1,048,576 comparisons with the
reference walk, including nested/no gates, reparenting between epochs,
deliberate table saturation, depth above 256, node-address reuse, nested lease
exhaustion, epoch wrap and another thread. A shared-ancestry workload needs 1129
gate checks versus 130000 reference checks. Four thousand warm epochs/resolutions
perform no heap allocations in the test. These are focused algorithm results,
not a claim of equivalent speedup in the application.

The same candidate renderer executable runs all 44 native renderer tests in
both modes at actual DPR 1.75. Both pass with no skips. The nine output PNGs are
byte-identical. The suite exercises batch/order changes and vector upload gate
admission/residency behavior. No static UI layout is modified, and no complete
portable rollout or full viewer acceptance claim is made.

The first renderer compile failed because Qt's cached moc file referred to a
removed Conan source directory. Copied the existing owned, corrected moc file
beside the new source and rebuilt; the installed SDK was not changed. The first
control-to-enabled launch stopped its owned process tree but observed a process
still draining during enumeration. After confirming both original PIDs were
terminal, resumed the launch; added wait_procs on that exact stopped tree for
subsequent transitions. No input capture was restarted or discarded.

## Live comparison

Order is control/enabled/enabled/control, each a fresh process, 30 seconds of
wheel input, initial contentY zero and corrected maximized startup. All input
scope guards verify Downloads, 192 entries including parent, Masonry density
152, actual DPR 1.75, 160 Hz and foreground state. No screenshots, UI Automation
or heavy analysis ran during input. External workloads/VMs/affinities and the
normal app were not changed.

| Capture | Arm | Wheel events | Render median / p99 / max, ms | Long display intervals >9.375 ms | Dropped rows |
| --- | --- | ---: | ---: | ---: | ---: |
| gate-owner-control-a1 | original | 2784 | 1.08050 / 2.89134 / 6.6829 | 3 | 1 |
| gate-owner-enabled-b1 | cached | 2781 | 1.09240 / 2.92946 / 6.6323 | 3 | 0 |
| gate-owner-enabled-b2 | cached | 2779 | 1.06025 / 2.92261 / 6.1858 | 3 | 1 |
| gate-owner-control-a2 | original | 2796 | 1.10240 / 3.00044 / 5.6917 | 3 | 1 |

All four maximum display-change intervals are approximately 18.75 ms. All long
intervals and dropped rows are preserved. Medians/p99 overlap or change direction;
this experiment does not prove a reliable tail improvement. Display-change
notifications are not optical scanout proof. The synthetic lookup improvement
does not justify a production change by itself.

## Original-constructor phase probe

A separate binary observes the original constructor, without the ownership
cache. It records registry scan, upload-membership preflight, owner-map
construction and residency validation separately, with bounded counters and
timestamps. Main writes the measurements after input. This instrumentation is
intrusive attribution evidence, not another clean performance comparison.

Native host SHA-256:
7c7610b8d6341ef4470e6c2c6fedd79b35594ac9500451d960c511ecfc200a3e.
The 30-second run contains 4799 completed constructor records, zero pending
records, no capacity exhaustion, and exact per-record phase-sum accounting.

| Phase | Sum over run, ms | Median per call, ms | p99 per call, ms | Maximum, ms | Share |
| --- | ---: | ---: | ---: | ---: | ---: |
| Registry scan | 10.9695 | 0.0021 | 0.005302 | 0.0153 | 2.35% |
| Upload-membership preflight | 35.7129 | 0.0014 | 0.030802 | 0.0748 | 7.65% |
| Owner-map construction | 398.3718 | 0.0775 | 0.173606 | 0.5684 | 85.37% |
| Residency validation | 21.6093 | 0.0049 | 0.0093 | 0.0353 | 4.63% |
| Entire constructor | 466.6635 | 0.0912 | 0.193608 | 0.6088 | 100% |

There are 46,713,580 ancestry lookups across 3,286,550 batch visits; active gates
exist in every sampled constructor, with 3–13 registered gates. Owner-map time
includes both ancestry identification and QHash/QSharedPointer container work:
it is not proof that all 398 ms is ancestor walking. The ownership-only cache
already targeted the larger phase. Do not infer from the two-loop discovery
that merely caching the earlier walk will solve the remaining spikes.

The phase-probe run still has three long display intervals and one dropped row;
render p99 is 2.8621 ms and max 6.0338 ms. The measured gate constructor remains
below 0.61 ms in this run and is not established as the cause of the much longer
display intervals.

## Decision and next evidence

Keep the candidate and its regressions as diagnostic work, with no production
rollout. Further gate work needs to distinguish lookup cost from per-frame map
construction and prove an end-to-end benefit. The more pressing investigation
is full-frame timing around GUI synchronization, input/layout bursts and late
presentation, rather than treating render-function sample counts as complete
latency attribution. Preserve onset and later display outliers explicitly.

Cold cache, the verified all-192-entry quiescent warm tour, quarter-height middle
scrolling, physical motion error, coverage holds and complete all-format/native
SVG acceptance remain unverified for the full goal. No goal completion, commit
or push is claimed. The normal Go/Qt app remains alive and unchanged; only owned
diagnostic trees are terminated. Compact evidence and hashes of external raw
data/binaries are frozen beside this report.

> AI assistance has been used to create this output.
