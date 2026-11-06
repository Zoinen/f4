# Standalone thumbnails: first valid restored-display pair

Both arms completed at 160 Hz, DPR 1.75, with focus retained and bounded native
recorders. The same executable and build receipt were used; material micro-probes
and the atlas-aware shader were disabled. Only the managed-thumbnail standalone
texture flag changed. Corresponding cold and warm layouts match between arms.

Each arm contains two ten-second cold and two ten-second warm middle-button
boundary-bounce inputs in Downloads. This is a single temporal control/enabled
pair, not a causal conclusion or a production promotion.

| Stage/block | Preparation p99 control / enabled, ms | Recording p99 control / enabled, ms |
| --- | ---: | ---: |
| Cold 0 | 2.4438 / 2.4333 | 0.8196 / 0.6648 |
| Cold 1 | 1.9314 / 2.0902 | 0.8473 / 0.7225 |
| Warm 0 | 1.9627 / 1.8334 | 0.7611 / 0.6695 |
| Warm 1 | 2.2141 / 1.7849 | 1.1965 / 0.7045 |

Displayed intervals above 9.375 ms were 52, 57, 51, 21 in control, and zero in
all enabled blocks. This is different from the producer long-present metric.
Nevertheless enabled cold 0 and warm 0 had maximum displayed-step errors of
5.4813 and 5.6628 physical pixels, exceeding the unchanged 2 px limit. Their
interval maxima were 8.1735 and 8.2383 ms. Thus zero counted misses does not mean
uniform motion. The other enabled blocks stayed below 0.566 px maximum error.
Commit-step p95 remained about 0.0003011 px in all blocks.

The strict motion command failed on the first control block. A separate
diagnostic invocation without gate arguments retained every block; it is not a
passing strict result. Neither arm proves the constant-160-Hz objective.

Recording p99 improved in four of four blocks; preparation improved in three.
The large temporal difference in displayed cadence needs a reversed-order
repeat before attributing it to textures. No new optimization was deployed.

Evidence: artifacts/standalone-ready-phases.json,
artifacts/standalone-ready-motion.json, and per-case JSON, build identities,
screenshots and PresentMon CSVs under artifacts/modes-profile with prefix
real-scroll-standalone-ready-. Previous lost-focus attempts are excluded.
