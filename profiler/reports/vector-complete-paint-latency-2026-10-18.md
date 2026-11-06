# Vector complete-paint latency — 2026-10-18

## Measurement

The current-source upload-admission/span-retention/bounded-order renderer reduces the large upload stalls seen during scrolling. This separate experiment measures its latency cost instead of treating deferred drawing as proof of faster rendering.

Fresh processes open the same Downloads panel at offset zero and settle for two seconds, then reveal offset 1150. Four newly visible SVGs are required: FileIcons_Dark+_nb_pro_1 (1).svg, FileIcons_Dark+_nb_pro_1 (2).svg, FileIcons_Dark+_nb_pro_1.svg, and FileIcons_Dark+.svg. Entry identities match in every final run. All windows are active at actual DPR 1.75 on the 160 Hz screen; target offset remains 1150.

The probe descends through the viewport's intermediate scene containers until it reaches delegate rows. It tests actual viewport intersection, reads the shared preview's thumbnailReady property, and separately reads each retained GalleryVectorImage's cpuSourceStatus. The candidate confirms nativeAvailable on all four surfaces; the normal renderer reports none. The timer observes every 4 ms, retains surface pointers, and records polling overhead.

The final v5 probe requires a scene synchronization after the GUI observes complete readiness, then records the following frameSwapped timestamp on the render thread. This avoids counting a previously synchronized frame that happened to swap after the GUI read readiness. The final screenshot is grabbed only after measurement, and its SVG content was inspected. This is a readiness-plus-swap measurement, not optical timing or a complete per-pixel GPU trace. Polling and frame quantization make it an estimate; it is not used as a clean cadence benchmark.

## Final v5 A/B/B/A results

| Run | All CPU-ready ms | First complete synchronized swap ms | Post-CPU wait ms | Max / total probe polling ms |
| --- | ---: | ---: | ---: | ---: |
| Normal A1 | 129.5981 | 149.0621 | 19.4640 | 0.8830 / 3.7893 |
| Candidate B1 | 125.2018 | 185.0397 | 59.8379 | 0.9373 / 5.6788 |
| Candidate B2 | 131.5035 | 178.4574 | 46.9539 | 0.7754 / 4.9043 |
| Normal A2 | 200.2176 | 227.3980 | 27.1804 | 1.4893 / 6.1876 |

The candidate adds approximately 20–40 ms after CPU readiness relative to these controls. It does eventually paint every required SVG, so its lower upload stalls are not achieved by permanently omitting previews. Complete reveal time is not uniformly worse: normal A2 has a CPU-preparation outlier and remains in the results. Do not discard it or claim CPU preparation is constant.

## Earlier probe versions and limitations

The first version incorrectly assumed delegates were direct viewport children. It found only the two scene containers and correctly failed its nonempty-vector assertion; those timeout results are not latency evidence. A later read encountered Windows default-codepage decoding on non-ASCII filenames; the JSON was intact and the reader was corrected to UTF-8.

V3 identified the right four rows and observed complete swap times A1/B1/B2/A2 of 166.6561/237.9776/187.7975/155.2584 ms. V4 added CPU-stage measurements: CPU-ready 136.4662/140.6896/132.5711/157.4365 ms and observed swaps 157.9721/193.1527/186.1207/173.1891 ms. Those early versions did not enforce a post-readiness synchronization before counting the swap, so they are retained as preliminary measurements, not the authoritative completion times. V5 closes that race.

This is one cold reveal containing four complex SVGs. It does not establish general viewer latency, all folders, all graphics backends or smooth 160 Hz. Source reads/component preparation and initial delegate creation are included in reveal-to-paint time. The first polling callback itself occurs about 50 ms after reveal in these captures, reflecting work before sampling; it must not be misreported as probe traversal time. Future diagnosis should instrument that reveal work separately.

## Decision and next work

The latency gap is measured and explicit: the upload budget trades a few additional refresh intervals of cold-preview completion for lower rendering bursts. Keep it experimental until current-source native/gallery correctness checks and normal portable packaging gates pass. Do not hide this tradeoff behind the cadence improvement, and do not infer a hard frame guarantee from its soft upload budget.

The next rollout work should reuse the already verified renderer patch composition, rerun current-source root/depth/overlap/lifetime/admission tests and gallery vector rendering/navigation tests, integrate the patch into the dependency recipe with regression coverage, then audit/embed/restart only this worktree's binary. After that, target the independently sampled GUI delegate/GC stalls. The original constant-160-Hz goal is still unmet.

Measurement source, raw per-run JSON, hash manifest and final captures are archived in profiler/experiments/vector-complete-paint-latency-2026-10-18. No production source edit, commit, push or experimental renderer deployment occurred in this turn. The normal embedded worktree app is restored.

AI assistance has been used to create this output.
