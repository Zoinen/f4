# Reversed standalone-thumbnail comparison: no reproducible benefit

The user supplied readiness for the live repeat. The first enabled attempt lost
focus during its warm phase and failed the existing validity gate; it is excluded.
A new enabled run followed by a new control run passed all focus, display and
bounded-recorder checks at 160 Hz and DPR 1.75.

Both accepted arms use the same executable and receipt, material micro-probes
disabled, atlas-aware shader disabled, and the existing native input machinery.
The managed-thumbnail standalone texture flag is the only candidate toggle.
The strict motion ceiling was not relaxed.

Downloads now contains 187 entries, compared with 186 in the previous day's
pair. Within today's pair, entry count, density, window size, graphics API and
corresponding stage extents match. Today's cold extent is 4075.142857 logical
pixels and warm extent is 3623.142857. Do not pool the two days as identical
folder-layout repetitions.

| Stage/block | Preparation p99 control / enabled, ms | Recording p99 control / enabled, ms |
| --- | ---: | ---: |
| Cold 0 | 2.1011 / 2.2092 | 0.5403 / 0.8022 |
| Cold 1 | 1.7891 / 1.8002 | 0.7862 / 0.6772 |
| Warm 0 | 1.8504 / 1.7575 | 0.5864 / 0.7473 |
| Warm 1 | 1.7404 / 1.7730 | 0.6505 / 0.8598 |

Both preparation and recording p99 worsen in three of four enabled blocks.
The earlier recording benefit does not reproduce in the reversed temporal order.
The candidate remains artifact-only and is not promoted.

All eight blocks have zero counted display intervals over 9.375 ms, but first
blocks still fail the 2 px displayed-motion ceiling. Enabled cold/warm block 0
maxima are 7.0689 / 7.3006 px; control maxima are 7.5152 / 7.6627 px. Second-block
maxima are below 0.684 px in both arms. Commit-step p95 remains approximately
0.0003011 px. Thus accurate camera increments and no counted refresh misses do
not establish uniform displayed motion. The strict command fails on enabled
cold block 0. Full diagnostic output without gate arguments is retained
separately and is not represented as a strict pass.

The initial enabled cold phase in the rejected attempt passed, but its warm
phase reports windowActive false and 708 inactive checks. It is not spliced into
the accepted pair. Neither rejected nor accepted captures overwrite prior runs.

Next: the separately linked D3D begin-pass diagnostic can distinguish wall time
from thread CPU service and split command acquisition from the recording callback.
Its native rendering and overhead still need live verification. No optimization
was deployed and constant 160 Hz remains unverified.

Evidence: artifacts/standalone-repeat2-phases.json,
artifacts/standalone-repeat2-motion.json,
artifacts/standalone-repeat2-comparison.json,
artifacts/verify-standalone-repeat2.py and per-case files under
artifacts/modes-profile with real-scroll-standalone-repeat2- prefix.
