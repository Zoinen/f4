# Small-upload buffer selection, 2026-10-20

A measured buffer-selection improvement is implemented, tested, packaged, and running.
Constant 160 Hz without spikes remains incomplete. This change addresses tiny uploads
that previously acquired large pooled buffers; large SVG upload costs remain open.

## Production change

When assigning a pooled vertex/index buffer to a batch with no owned buffer, requests
up to 8 KiB consider only candidates up to max(4 KiB, 8 times the requested size).
If none qualifies, Qt allocates a small buffer and leaves large candidates in the pool.
Best fit among eligible buffers is preserved, including growing an undersized small
candidate. Requests above 8 KiB retain the existing selection policy. Owned buffers
are unchanged. The bounded retirement budget remains 16 MiB per category.

This is in the portable Qt source-patch payload, with strict source anchors and
existing serialized-recipe refresh behavior. No runtime setting, pool-budget override,
new per-frame QML work, draw-order change, vector rasterization, or rounding traversal.
The diagnostic on/off environment flag is absent from production source.

## Rejected owned-buffer exchange

First tested exchanging an already-owned oversized buffer for a smaller existing
pooled buffer, returning the large one to the pool. Focused tests passed on D3D11
and OpenGL, including budget accounting and rendered 175% placement. Real scrolling
regressed: warm-snapshot create delta rose from 15 to 6,489, and a 12.4168 ms display
gap produced a failed motion gate (43.53 px maximum step error). Returning large
buffers crowded the retirement pool and increased churn. The experiment was rejected
and is absent from production. Its code, failing capture, and passing unit tests
are retained rather than treating unit success as performance success.

## Workloads and measurement

Downloads, 186 entries, Masonry density 152, DPR 1.75, 160 Hz screen, 30 ms bouncing
wheel events. Every complete capture has four 10-second input windows. Native scrolling
uses the production default, finite OutSine curve, and precise animation clock. Default
GC budget is 5000 us; diagnostic pixmap cache is 2 MiB. Bounded pre-touched renderer
probes and independent PresentMon record upload and presentation timing. No build,
test, offline analysis, UIA observer, or scheduler tracing runs during input windows.

Small-upload statistics below cover only uploadBatch samples >=100 us with combined
requested vertex/index size <8192 bytes. They are thresholded diagnostic totals, not
all upload time or all small allocations. Warm snapshot lifecycle deltas include the
inter-capture gap as well as warm input windows. Inclusive timing parts are not added.

| Run | Cold tiny-map total ms | Warm tiny-map total ms | Warm create delta | Oversized warm slow samples |
|---|---:|---:|---:|---:|
| Previous split-update baseline | 21.8466 | 23.5704 | 15 | 155 |
| First small-buffer candidate | 8.6262 | 5.7327 | 35 | 0 |
| Same-binary rule disabled | 17.4872 | 14.3137 | 21 | 93 |
| Same-binary rule enabled | 4.9261 | 5.1726 | 25 | 0 |
| Exact production-source instrumentation | 6.4587 | 4.5734 | 49 | 0 |

The disabled/enabled pair has identical successful-link receipts and differs only by
a diagnostic selection override. Warm thresholded map time falls about 64%, while
create counts remain low. Separate first and production-source runs repeat the smaller
mapping cost. Total frame preparation changes are smaller: same-binary warm internal
prepareRenderPass p99 falls from 1.9089/1.9557 to 1.7990/1.8654 ms. Maxima vary;
no universal tail-latency guarantee follows from these samples.

## Presentation failures retained

The owned-exchange capture fails the refresh gate as described above.
The same-binary disabled capture reports 3,157 of 3,268 cold presents dropped, leaving
only 20 comparable motion pairs in its first window; the motion analyzer correctly
rejects insufficient displayed coverage. Regular render-thread motion and low upload
timings do not explain that presentation failure. It is unresolved and is not relabeled
as a passing or irrelevant run. Upload-service records remain usable for their limited
purpose, but this is not a clean paired presentation comparison.

A disabled repeat loses window focus during cold input and fails the harness activity
assertion; no warm stage runs. That repeat is not accepted as a controlled input test.
The ordinary running app remains responsive. Further desktop/presentation attribution
is needed; no unsupported claim about another application or compositor cause is made.

## Exact production-source capture

The final diagnostic renderer is generated from the same uninstrumented source used
in the new portable Quick archive. It does not reapply the experimental algorithm or
retain the diagnostic disabling flag. Successful-link receipt pins that source, the
production library, policy module, and existing startup controller source.

All four windows pass complete presentation brackets, zero missed-refresh checks,
motion p95 <=0.5 px, and maximum <=2 px. Retarget/geometry seams are excluded by the
existing motion analyzer; this guard is not the definition of the complete goal.

| Window | Max display gap ms | Motion p95 physical px | Max motion error px |
|---|---:|---:|---:|
| Cold 0 | 6.4315 | 0.1088 | 1.2991 |
| Cold 1 | 6.3431 | 0.1117 | 0.7028 |
| Warm 0 | 6.4150 | 0.1001 | 1.1651 |
| Warm 1 | 6.3661 | 0.0973 | 0.7918 |

Public preparation maxima are 3.0198, 3.2460, 2.7698, and 2.7448 ms. There are two
producer intervals above 9.375 ms but zero display misses; deliberate pacing and
actual displayed cadence must remain separate. The earlier 18.734 ms failure is
still retained in the preceding report and is not erased by this passing run.

## Verification

Before the change, four new production regression rows fail against the old library
while the original eight selection rows pass. They cover both vertex/index buffers,
small undersized candidates alongside large adequate ones, and all-oversized pools.
After the change:

- 35 dependency-patch tests and 2 animation-clock recipe tests pass.
- Production renderer: 42 checks/backend pass on D3D11 and OpenGL, including setup
  and cleanup, pool bounds, ordering, retained batches, and complete presentation.
- Native camera: 30 checks/backend pass, including startup registration, finite
  motion, identical retarget deadlines, interruption, suspension, bounds, and SVGs.
- SVG session subset: 14 checks/backend pass, including cache re-entry, background-free
  swaps, all five panel modes, gradients/alpha/images, authored animation and resize.
- Expanded isolated candidate fixture: 24 checks/backend pass with actual API asserts,
  empty/mixed pools, both threshold edges, large requests, cost accounting, and 175%
  scene-space origins/unit vectors. The rendered red-leaf image is inspected.
- Production panel/renderer tests verify physical placement at DPR1.75; rendered
  Masonry caption/vector output is inspected. No UI geometry was changed.
- Embedded-only QML import test: 3 checks pass. Windows system-DLL audit passes.
  CGO_ENABLED=0 embedded-host extraction tests pass using the system Go cache.
  Host payload is regenerated and embedded in the staged launcher.

The initial SVG verification runner used artifacts/ as its working directory, so two
otherwise-passing swap tests failed saving artifacts/... captures. The runner is fixed,
those tests and remaining backend suites pass from the repository root, and the failed
logs are retained. Existing font/undefined-QPointF and duplicate-plugin warnings remain.

Three old large CSVs were losslessly compressed to recover build space. Decompressed
SHA256 equality and copied archive equality were verified before unlinking originals;
explicit path and recovery receipts are retained. No evidence was discarded.

## Deployment and remaining work

Only the exact D:/Code/f4-zoin/f4-zoin.exe process tree was replaced after all gates
passed. Its freshly extracted Qt child is responsive and matches the rebuilt host.
No host, frame-latency, or buffer-budget override is active; the canonical checkout
was untouched. The prior working launcher is retained. Nothing was committed or pushed.

- Quick library: 5aa56ec1105aa9ce9b35a5f5c9ab104bf262d05597661ae863dbd617f43bed0c
- Host: f795016a40de6fa761bc35e1958f458062e2dbc5291f38066143eb112b20a9b7
- Launcher: 8dbf629cf96283be49ecfd80edbc93d88109ace97c959d60acdcf28ad7b8f8cf

Next reduce repeated large static SVG GPU uploads without reintroducing the rejected
visited-root or owned-buffer retention experiments. Investigate the rare ordinary
upload/presentation failures with a reproduced scheduler trace, preserve velocity
continuity/reversal/bounds/deadline behavior, and extend input/workload verification.
The original constant-160 Hz objective remains active and unproven.

Verified source, build, test, raw capture, failure, rendered-output and deployment
snapshots: ../experiments/small-upload-buffer-fit-2026-10-20/manifest.json.
AI assistance has been used to create this output.
