# Masonry: automatic batch-root threshold control

## Outcome

Raising Qt's automatic batch-root thresholds did not meaningfully reduce
uniform updates or consistently improve scrolling. Retain the defaults. This
rejects this specific global tuning candidate; it does not establish which
caption roots, clips, materials or overlap checks prevent further merging.
No UI hierarchy, default setting, Qt package, CI policy or embedded payload changed.

## Matched experiment

A/B/B/A uses the exact same uniform-material observer host in all arms.
Frame latency is 2, sparse sequence strategy disabled, and the preceding
PixelGrid boundary cache, bounded preprocessing and overlay fast path remain
enabled. Both standard media workers and authored vector geometry remain intact.

| Setting | Control | Candidate |
| --- | ---: | ---: |
| QSG_RENDERER_BATCH_NODE_THRESHOLD | 64 | 256 |
| QSG_RENDERER_BATCH_VERTEX_THRESHOLD | 1024 | 1000000000 |

Qt's renderer constructor reads these settings, and its automatic promotion
checks subtree node/vertex counts. The high candidate vertex limit suppresses
that heuristic in this workload; forced vector-upload roots remain unchanged.
The experiment does not move labels above images or alter painter ordering.

Native tests directly read back the private renderer fields on its render
thread and verify the expected values. All 44 renderer QtTest results pass
with control settings, and all 44 pass with candidate settings, zero skips,
actual DPR 1.75 and native D3D11. These primarily geometric renderer tests do
not establish every possible native-text visual invariant of a threshold change.
An earlier native run also toggled the sparse-sequence feature; its logs remain
marked confounded. The accepted rerun keeps sparse sequence disabled in both
arms and adds direct threshold readback.

Capture activation checks inspect only the selected Qt process's two threshold
environment values, preserving a per-case receipt. The renderer source and
native readback establish how this exact host consumes those values.

Cases: `real-scroll-batchthreshold-{a1,b1,b2,a2}-20261030-wheel-masonry`.
Actual Downloads folder, 192 entries, Masonry density 152, 3840×2160/160 Hz,
actual DPR 1.75, new processes and empty isolated caches, two ten-second wheel
windows. The second direction is not a verified-warm-tour substitute. No UI
Automation or screenshots run during gestures. All eight native foreground
windows stay stable, zero inactive events. All original capture-validity
checks pass. PresentMon rows are 3452/3434/3401/3413; wheel events
1781/1804/1708/1798. The policy and duration match, not delivered counts or
traversal distance. Both finite display hold and owned diagnostic launcher are
stopped afterward; the ordinary worktree app at PID 43936 remains alive.

## Update counts

Counts cover the whole process observation, including startup/preparation.
They are not direct CPU timings or per-window update counts.

| Arm | Total uniform updates | Text-mask updates | New backing slots |
| --- | ---: | ---: | ---: |
| A1 | 936015 | 259309 | 2136 |
| B1 | 932753 | 258184 | 2136 |
| B2 | 919928 | 255667 | 2136 |
| A2 | 926863 | 256007 | 2136 |

The update mix remains similar. Detachments and large-data growth are zero;
material-table overflow is zero; class call/byte totals exactly match their
aggregate counters. There is no large reduction supporting the proposed
automatic-root explanation under these chosen thresholds.

## Display and motion

| Arm/window | Intervals >9.375 ms | Maximum display interval | Maximum motion error |
| --- | ---: | ---: | ---: |
| A1/0 | 29 | 13.0833 ms | 114.52 px |
| A1/1 | 1 | 12.4298 ms | 83.22 px |
| B1/0 | 32 | 12.6486 ms | 112.42 px |
| B1/1 | 1 | 12.2266 ms | 46.86 px |
| B2/0 | 41 | 12.6669 ms | 127.65 px |
| B2/1 | 44 | 12.7532 ms | 117.42 px |
| A2/0 | 30 | 14.0907 ms | 125.35 px |
| A2/1 | 1 | 12.2989 ms | 47.47 px |

Coverage holds are zero throughout. Every window fails the original display
and maximum-motion gates. The favorable second block of B1 does not repeat
in B2; controls also show favorable second blocks. No consistent benefit is
established. Constant 160 Hz remains unachieved.

## Source cross-check and next work

Qt's native glyph geometry uses unsigned-short indices. Its base text-mask
material enables blending; material comparison depends on glyph cache and
color. These source facts do not prove all captions share compatible state.
Clip identity, batch root, overlap checks, actual batch membership and geometry
safety still constrain merging. The renderer's merge predicate also checks
position attributes, index type, material flags and safe transforms/ranges.

The existing aggregate material counter cannot identify a caption's actual
root or explain why its batch remains separate. Inspect those nodes directly
with bounded sampling before redesigning caption ownership or removing clips.
Preserve native text's pixel behavior, thin-image overlap order, selection and
layout-animation fallback. A global threshold increase is not the fix supported
by this evidence.

Sources, scripts, final isolated native tests, confounded first-run logs,
capture/analysis outputs, glyph-source cross-checks and external hashes are
frozen in `profiler/experiments/masonry-batch-threshold-2026-10-30/`.
Large packets, CSVs, native PNGs and executable remain at original paths,
hash-pinned. No commit or push. Original cold, verified-warm wheel and
quarter-height middle gates remain outstanding.
