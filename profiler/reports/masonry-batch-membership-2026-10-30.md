# Masonry text batch membership

## Finding

Most sampled native-text batches contain only one geometry node, although
the main groups share viewport-sized clips, glyph cache and color. This rules
out a separate clip per caption as the explanation for those groups. It also
does not support removing vector roots: no sampled text batch uses a forced
vector root. The previous global root-threshold experiment did not improve
batch counts or satisfy the scrolling gates.

The next bounded probe should attribute exits from `prepareAlphaBatches()`:
intervening root/render-node boundaries, incompatible state and overlap/order
barriers. Shared root and text material do not imply safe reordering across
intervening content. No caption reparenting or drawing-order change was made.

## Observer and validation

Exact Qt 6.11.1 diagnostic host, derived from the uniform-material observer.
Every 256th uniform update per material class copies batch membership, root
and clip identity, root SG type, forced-vector-root membership, geometry index
type/position attribute, material flags, bounds status and text glyph-cache /
color into a fixed 8192-slot table. The member walk stops at 64 and records
truncation. Full capacity returns before dereferencing arguments. The table is
touched before application startup; output serialization follows the gesture.
No node pointers are retained for later dereference. Deferred uniform updates,
shaders, geometry, resource ordering and graphics synchronization are unchanged.

Eight helper QtTest results pass, including bounded member walk and capacity
guard. All 44 native rendering results pass, zero skips, with actual DPR 1.75
and D3D11. Three static import-isolation results and the Windows DLL audit pass.
The observer host is diagnostic only; the embedded portable payload is not
promoted. Native tests do not establish a new caption-layout regression because
no caption-layout modification is present.

## Live capture

Case: `real-scroll-batchmembership-a1-20261030-wheel-masonry`.
Actual Downloads, 192 entries, Masonry density 152, 3840×2160, 160 Hz, DPR 1.75,
fresh process and empty isolated cache, two ten-second wheel windows. Frame
latency remains 2; sparse sequence mode is off. The second direction is not
claimed to be verified warm. No screenshot or accessibility polling runs during
the gestures. Native foreground ownership is stable; all 1781 wheel events
arrive active. PresentMon yields 3469 target rows. Original capture validity
checks pass. Finite display hold is explicitly released afterward.

3674 records fit in the 8192-slot table; all publish successfully. The two
windows contain 1693 / 1762 samples, all on the recorded render thread inside
the preparation scope. Deterministic sampling may correlate with draw order;
these are sample-weighted observations, not unbiased population counts or CPU
time by class. Pointer identities across frames do not count simultaneous live
objects. This attribution run is not an A/B performance comparison.

| Native text samples | First window | Second window |
| --- | ---: | ---: |
| Samples | 468 | 471 |
| Merged batches | 468 | 471 |
| One-member batches | 402 | 393 |
| Root SG node equals clip node | 442 | 435 |
| Forced vector root | 0 | 0 |

All sampled text uses unsigned-short indices (5123), position attribute 0,
material flags 1 and bounds within float range. "Merged" is Qt's geometry
upload path and can still contain only one member. Some larger member chains
are truncated at 64; their full sizes are not inferred.

The two most frequent root/clip pairs have identical viewport extents
1097.142857×1035.428571 logical pixels. They account for 308 / 284 text samples;
308 / 282 of those have one member. 305 / 281 share the same glyph-cache and
color pair. Thus the observed fragmentation persists within shared viewport
clips and material parameters. Other compatibility fields and intervening
draw-list barriers still need attribution.

## Gates remain unmet

| Metric | First window | Second window |
| --- | ---: | ---: |
| Display intervals above 9.375 ms | 26 | 11 |
| Maximum display interval, ms | 13.6081 | 12.7398 |
| Maximum motion error, physical px | 133.0019 | 129.5945 |
| Preparation median, ms | 0.5773 | 0.4613 |
| Preparation maximum, ms | 7.4828 | 4.4827 |

No claim of constant 160 Hz or performance improvement follows from this probe.
No production renderer change, commit or push is performed. Evidence and
external artifact hashes are preserved under
`profiler/experiments/masonry-batch-membership-2026-10-30/`.
