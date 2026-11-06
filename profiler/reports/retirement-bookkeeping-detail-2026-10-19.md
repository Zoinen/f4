# Retirement bookkeeping detail — 2026-10-19

The 160 Hz goal remains incomplete. This diagnostic measures the work that
the earlier incubation probe grouped before its nearby-preparation timer.
No production algorithm, setting, SDK or embedded payload changed.

## Protocol and validity

`retirement-detail-{cold,warm}-sampled-wheel-masonry`: Downloads, 186 entries,
Masonry density 152, wheel events every 30 ms, two four-second moving windows
per capture. Same process 76736, engine `15cff8`, GUI thread 64592, render
thread 73544, window 2194×1186, actual/primary display 160 Hz, DPR 1.75.
First pass is a fresh Qt process with warm disk cache; second pass reuses
that process and heap. Extent changes from 4101.142857 to 4118.142857.
Caption measurements and model/snapshot change signals are zero. No UIA,
QML profiler, tests, builds or offline analysis ran during acquisition.

Scopes use thread-local bounded native records. Kind 4 is retirement, 5 is
retained-vector selection, 6 is release, 7 is pixel-grid flush, 8 is grid
registration. The shared header uses inline thread-local storage across the
two translation units; only the commit TU exports the JSON accessor.
Property-check clocks wrap the existing component lookup/comparison; they
do not alter its result. Native record capacity is 131,072. No records hit
capacity. GC and incubation remain independently instrumented.

- cold: 37,715 cumulative native records; 47,911 matched switches, 900 ns QPC offset spread; 7 displayed intervals above 9.375 ms.
- warm: 67,499 cumulative native records; 53,312 matched switches, 900 ns QPC offset spread; 24 displayed intervals above 9.375 ms.

ETW lost-event/buffer checks pass in the scheduling analyzer. These are
observer-enabled diagnostic runs, not clean before/after comparisons.

## Measured scopes

Each cell contains count / inclusive wall milliseconds / maximum milliseconds.
Nested scopes must not be summed. A call inside `updateProperties` can include
retirement, release or GC. Wall duration includes preemption and instrumentation.

| Scope | First pass | Warm pass |
| --- | --- | --- |
| nearby | 385 / 438.7560 / 6.9307 | 367 / 523.9694 / 7.3210 |
| properties | 636 / 366.7734 / 7.7056 | 586 / 476.5058 / 3.0583 |
| retirement | 1,020 / 127.4975 / 0.4309 | 953 / 202.1360 / 0.7658 |
| vectorSelection | 1,860 / 44.7616 / 0.1639 | 1,744 / 86.2272 / 0.3551 |
| release | 988 / 67.8269 / 5.5792 | 922 / 102.7051 / 5.5419 |
| gridFlush | 1,272 / 116.3090 / 0.8892 | 1,256 / 158.4638 / 1.3210 |
| gridAdd | 3,046 / 84.0492 / 0.1886 | 2,646 / 92.9018 / 0.2960 |

## Findings and next action

- cold: 107,262 component checks consume 90.5077 ms in aggregate; the longest release call is 5.5792 ms, including 5.2698 ms GC.
- warm: 102,914 component checks consume 138.4652 ms in aggregate; the longest release call is 5.5419 ms, including 5.1336 ms GC.

Component identity checks repeatedly call `QObject::property` and convert
a QVariant despite the pool already retaining a native component-key map.
This establishes a recurring cost worth removing. It does not prove that
all remaining spikes, or the earlier 3.3705 ms pre-timer outlier, come from
those checks. In these captures retirement maxima are below 0.8 ms; the
largest pre-timer delays include release-triggered GC. Per-check timing
includes the observer clock overhead. No overhead subtraction is claimed.

Next implement a native component-key accessor and a native internal vector
LRU stamp, preserving component compatibility, retained-scene ordering,
16-vector retention, spare limits and scene-space clip checks. Add regression
coverage for reuse across components and for eliminating private dynamic
property changes during vector scrolling. Then repeat clean interleaved
scrolling captures; do not infer a frame-rate improvement from these costs
alone. Release/GC and grid re-registration remain separate targets.

The normal portable worktree app was restored before analysis. Its host hash
remains `cf591bc8a0745c0daf0dfcbab06dd0873c48feb8526511dd52d021f65308d42c`;
launcher hash remains `7e19827aedbc681b08a9406048999eb64a60031b3ebbe0d5bd802ad99dcdfdbc`.
Diagnostic source, raw native JSON, attribution, selected stacks, process
identity and SHA-256 manifest are preserved under
`profiler/experiments/retirement-bookkeeping-detail-2026-10-19/`.
Raw ETL/CSV and compiled outputs are excluded from that archive.

AI assistance has been used to create this output.
