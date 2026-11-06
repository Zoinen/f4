# SVG upload CPU attribution and static-buffer experiment, 2026-10-20

The current production worktree build remains the curve-preprocess build. This
investigation changes diagnostic artifacts only. Constant 160 Hz without spikes
is still unproven. The static-buffer candidate described below is rejected and
has not been added to the portable Qt recipe or running application.

## Final production-library attribution

real-scroll-curve-cpu-middle uses the final 8082fc3f Qt Quick library, production
scrolling defaults and production batch-renderer source with bounded probes.
The same 175%/160 Hz Downloads Masonry scenario uses a quarter-window middle
pointer offset and four 10-second input windows. There were no builds, tests,
offline analysis or UIA observation during input. Independent PresentMon data
was correlated to renderer frames. CPU probes use QueryThreadCycleTime; cycle
counts are not converted to milliseconds or treated as a fixed CPU frequency.

All four strict middle-motion windows passed with zero missed display intervals.
Display maxima were 6.3454/6.4042/6.3473/6.3473 ms. This finite capture is not proof
of all-workload smoothness. Public preparation maxima remained 4.9936/3.0915 ms
cold and 3.5186/2.8921 ms warm.

The largest cold gated admission consumed 4.3144 ms and 14,359,560 CPU cycles.
It uploaded one unmerged QSGCurveStrokeMaterial batch: 1,344,420 vertex bytes and
122,220 index bytes. The upload lasted 4.3069 ms, with 2.2497 ms in buffer creation
and 1.9087 ms in full dynamic updates. These subscopes are nested within upload
and admission; do not sum parent and child durations. This was substantial CPU
execution rather than attributing the entire wall-time spike to descheduling.

The outer batch-preparation maxima were 4.7873 ms/15,787,900 cycles cold and
3.1963 ms/9,892,980 cycles warm. Other tails differ: a warm recording event lasted
1.9672 ms for only 2,502,400 cycles, versus a 1.135 ms event with 3,842,340 cycles.
That is evidence that wall-time tails cannot all be explained by one uniform
amount of CPU work. It is not a scheduler attribution without a scheduling trace.

## Static-buffer candidate and correctness

The candidate keeps large (>512 KiB), unmerged geometry belonging to SVG upload
gates on Qt's static upload path. It suppresses automatic promotion to Dynamic
and converts recycled Dynamic buffers to Static for these uploads. Other batches
retain Qt's normal policy. It does not chunk uploads, add a CPU geometry shadow,
change upload budgets, change frame pacing or change geometry fidelity.

The diagnostic flag F4_PROFILE_DISABLE_VECTOR_STATIC selects the unmodified
policy in the same executable. It is not a product setting. Both capture
identities pin identical successful-link receipts and record the flag separately.

A native fixture changes visible geometry over 16 updates, checks the rendered
pixels each time, and inspects actual QRhiBuffer types after rendering. Large
gated geometry remains static in the candidate; ungated large and gated small
geometry retain normal dynamic promotion. Scene origin and unit transforms are
checked at DPR 1.75. The original renderer behavior is explicitly exercised with
the candidate disabled. Five fixture checks pass per backend in that control;
47 full renderer checks pass per backend with the candidate enabled, D3D11 and
OpenGL. This verifies correctness, not performance suitability.

## Same-executable scrolling experiment

real-scroll-vector-static-control and real-scroll-vector-static-enabled use the
same middle-button scenario and capture discipline as above. Both pass all four
strict motion windows and have zero missed display intervals. No diagnostic
source, executable or link receipt changed between the two runs.

| Public preparation p99 | Control | Static candidate |
|---|---:|---:|
| Cold block 0 | 2.0426 ms | 2.1296 ms |
| Cold block 1 | 1.6821 ms | 1.8392 ms |
| Warm block 0 | 1.7871 ms | 1.7808 ms |
| Warm block 1 | 1.7196 ms | 1.8134 ms |

Three of four preparation p99 values worsened. Cold maximum fell from 4.8051 to
4.4719 ms, but warm maxima were mixed (3.5254/2.7964 versus 3.3838/3.2495 ms).
Among recorded large gated upload samples, warm median increased from 0.2298 to
0.72155 ms. These medians are conditional: the probe records only uploads lasting
at least 100 us, and the sample counts differ (67 versus 48). They are not the
median of every upload. Warm recorded large-upload p99 also worsened, 0.8971 to
1.0110 ms. The slower routine uploads and lack of a consistent frame-time gain
do not justify promoting this policy.

Lifecycle data shows that the control can select small retired buffers for large
SVG requests (2,054,448 vertex bytes selected a 1,040-byte buffer), causing growth.
Warm slow samples also include correctly sized buffers with no recreation whose
dynamic copy alone costs around 1 ms. Thus allocation growth is not the sole
remaining problem. Making all large buffers static replaces the tail with more
routine static-upload cost; it does not solve the workload.

## Next action and limits

Keep the production buffer policy. The next candidate should bound occasional
large allocation/update work without making the routine upload path slower.
The existing chunking prototype remains separate and unpromoted; its source
eligibility/priority latency and total-frame budget still need verification.
Rapid-wheel display-phase error is also unresolved. No full-goal completion,
cross-platform performance claim, commit or push is made.

Evidence includes curve-cpu-scopes-summary.json, curve-cpu-largest-admission.json,
vector-static-comparison.json, strict motion logs, full per-case captures and
successful-link receipts. curve-cpu-before-static/ preserves the final production
probe sources before the static prototype was generated. Older owned profiling
CSVs were losslessly compressed and hash-verified to recover disk space.
