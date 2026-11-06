# Masonry render-list sequence snapshots

## Outcome

A tested prototype avoids indexing merged/render-node/orphan elements for
unmerged-batch retention, while retaining every intervening element in the
ordered sequence. It removes about 60% of indexed entries in the real workload.
It does not establish constant 160 Hz or a repeatable whole-view improvement.
Keep it experimental: no installed Qt package, production recipe, embedded
payload, or ordinary application was replaced. No commit or push.

## Algorithm and safety

The original `f4UnmergedPositions` snapshots every surviving element into both
an ordered list and a position hash. The hash is queried only by eligible
unmerged batches. The candidate counts eligible elements and indexes just
those, preserving the complete ordered list so merged outsiders still prevent
retention after insertion, removal or reordering. If no unmerged batch exists,
the snapshot is empty; a newly unmerged batch falls back to ordinary rebuilding.

A bounded 16-element sample selects the original complete-map implementation
for dense lists. Sampling affects representation and cost only, never whether
an ordering check is skipped. There is no persistent cache, new raw-pointer
lifetime, raster fallback, resolution reduction, change to material evaluation,
or weakening of source/residency checks.

The first version regressed an all-unmerged microbenchmark by about 12%.
A sampled path with the original loop embedded still regressed about 6.5%.
The accepted prototype calls the unchanged complete-snapshot helper before
constructing sparse-path temporaries. The final helper run measures dense
lists at 209.609 versus 209.566 us per 8192-element snapshot, essentially equal.
These rejected variants and raw benchmark outputs remain distinguishable.

## Tests and isolated measurements

Seven helper QtTest results pass, including initialization/cleanup. Cases cover
null, removed, orphaned and render-node elements; exact all-unmerged snapshot
equivalence; merged-to-unmerged fallback; and 64 combinations of outsider
insertion/removal, changed order and upload invalidation. Retention decisions
match the original in those mixed-span cases, with both accepted and rejected
outcomes so an always-reject implementation cannot pass.

The final microbenchmark alternates original/candidate execution order over 16
samples each, with 64 snapshots per sample. It includes snapshot destruction:

| Unmerged share | Original snapshot | Candidate snapshot |
| --- | ---: | ---: |
| 0% | 287.271 us | 7.606 us |
| 10% | 227.748 us | 37.329 us |
| 25% | 214.873 us | 51.708 us |
| 50% | 214.109 us | 78.949 us |
| 100% | 209.609 us | 209.566 us |

These are snapshot costs, not whole-frame savings. Empty-map optimization
explains the large 0% difference; dense snapshots retain the original work.

All 44 native renderer QtTest results pass with the prototype disabled, and all
44 pass enabled. The fixture asserts actual DPR 1.75 and uses native D3D11.
Coverage includes alpha interleaving, full/partial order changes, clipped
branches, culling, render nodes, retained batches, insertion/deletion and upload
admission. All nine corresponding rendered PNGs have identical hashes between
the arms. A representative retained-alpha capture was visually inspected.

An initial invocation omitted `ZOIN_TEST_NATIVE_RENDERER` and skipped 42 cases.
Those logs are preserved as rejected coverage; they are not counted as passing
native validation. The corrected invocation rejects any skipped case.

The static host and import-isolation test were rebuilt from the complete
existing plugin/resource graph. Three import QtTest results pass; the system-DLL
dependency audit passes. Build setup initially selected a stale generated moc
include and then inconsistent copied diagnostic headers. Both were corrected
by retaining the verified moc and canonical original-header paths. Those setup
issues are not product or Qt API defects.

## Live A/B/B/A comparison

Cases: `real-scroll-sparsesequence-{a1,b1,b2,a2}-20261030-wheel-masonry`.
Every arm uses the same new host binary, with only
`F4_PROFILE_SPARSE_SEQUENCE` changing. Both include the preceding PixelGrid
boundary optimization, bounded preprocessing cache, overlay fast path and
frame-latency limit 2. The snapshot counters add the same small atomic-recording
path in both arms; no per-snapshot clocks are added.

Actual Downloads folder, 192 entries, Masonry density 152, 3840×2160,
160 Hz/DPR 1.75, fresh isolated caches, and two ten-second wheel windows.
No screenshots or accessibility observation run during gestures. Native focus
is stable in all eight windows; inactive wheel events are zero. All four original
capture-validity checks pass. PresentMon rows are 3426/3420/3414/3398; wheel
events 1709/1710/1755/1722. Delivered counts and traversal distance differ despite
matched input-generation policy and duration. Second-direction blocks are not
substitutes for verified-warm-tour coverage.

Whole-capture counters, including preparation before the timed windows:

| Arm | Snapshot calls | Input elements | Indexed elements | Empty maps |
| --- | ---: | ---: | ---: | ---: |
| A1 | 4656 | 1875210 | 1826335 | 2 |
| B1 | 4444 | 1794968 | 692490 | 2224 |
| B2 | 4556 | 1836511 | 710058 | 2280 |
| A2 | 4628 | 1871275 | 1825165 | 2 |

Candidate indexed/input ratios are about 38.6%, versus 97.4–97.5% in the
controls. This proves feature use and less bookkeeping, not saved frame time.

| Arm/window | Intervals >9.375 ms | Maximum interval | Maximum motion error |
| --- | ---: | ---: | ---: |
| A1/0 | 55 | 12.7391 ms | 126.13 px |
| A1/1 | 2 | 12.5866 ms | 62.42 px |
| B1/0 | 43 | 12.8123 ms | 124.49 px |
| B1/1 | 42 | 12.9221 ms | 127.25 px |
| B2/0 | 37 | 18.7092 ms | 120.68 px |
| B2/1 | 45 | 13.0147 ms | 120.38 px |
| A2/0 | 31 | 12.6409 ms | 127.49 px |
| A2/1 | 48 | 12.7766 ms | 130.19 px |

Coverage holds are zero throughout. Every window fails display and maximum
motion gates. Candidate cold-block render-list p99 values are .5341/.6079 ms,
controls .7698/.5753 ms; maxima overlap and B2 reaches 1.5662 ms. Preparation
p99 and display-miss counts do not support a robust whole-view improvement.
Median present-to-display latency remains similar at 16.74–17.32 ms controls,
16.77–17.18 ms candidates. A1's favorable second block does not repeat in A2.

## Preservation and next target

Sources, prototypes, helpers, capture/analysis scripts, validation, native logs
and external hashes are frozen in
`profiler/experiments/masonry-sparse-sequence-2026-10-30/`.
Large packets, CSVs, executables and PNGs remain at original paths, hash-pinned.
The owned display hold and diagnostic launcher were stopped. The ordinary
worktree app at PID 43936 remains alive.

The next target is per-batch material/uniform preparation and uploads, which
remain larger measured costs. Preserve native resource-frame requirements,
image quality, full painter ordering and alignment. The separate image
texture-provider connection mutex wait also remains unresolved. The overall
goal still requires original cold, verified-warm and quarter-height-middle
display/motion/coverage gates; it is not complete.
