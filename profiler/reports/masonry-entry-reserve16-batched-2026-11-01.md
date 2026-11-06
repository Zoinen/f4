# Masonry: prewarmed roots through the ordinary row transaction

Status: isolated candidate; not promoted. Constant 160 Hz remains unachieved.

## Change and hypothesis

The preceding sixteen-spare prototype supplied roots but assigned their initial properties one by one inside `createComponent`. This variant leaves `MasonryLayout.cpp` unmodified and takes an anonymous spare in the ordinary delegate-pool factory. Normal `prepareDelegateRow`/identity/visual/layout/finalization then perform assignment inside the existing visual-update transaction, as for a recycled slot. The synchronous factory remains the fallback. The existing reserve capacity (sixteen ready roots and one pending construction per layout), cancellation, renderer visibility policy and look-ahead bounds are unchanged.

Only copied diagnostic sources and separately named binaries change. There is no production, installed SDK, Go protocol or embedded payload rollout. The new host SHA-256 is `15251ff2debb7ea6487517fe024b73956ee934bdaba6c0c2025e44b7e983a686`. Control remains `41b6a314d36c2262c21ce85b1dcc03efc5ed5abf7902396184f35ca78c29b79f`, built from the same current source without reserve transforms. Both use the same input/counter recorder. Static Windows import and GUI-subsystem checks pass.

## Integration verification

A new native QtTest fills the real panel's reserve, jumps into a 2,000-row catalog and requires actual spare consumption. It verifies every visible delegate's view/source indexes, entry identity, completed identity transaction and caption text. At actual DPR 1.75 it maps each visible caption's origin and unit vectors into window coordinates and checks whole physical-pixel positions with an unscaled transform. It saves rendered output for inspection. The run consumes eighteen prewarmed delegates and checks twelve visible captions; all three QtTest results (including setup/cleanup) pass. The image was inspected, including the small filenames. The unused white area belongs to the test window outside its fixed-height panel.

The unchanged control fails the explicit reserve-availability assertion, confirming that the integration check requires the candidate path rather than merely rendering an ordinary panel. Its expected negative result is retained separately.

The current native Masonry suite selection passes eleven results with no failures/skips, including folder-preview pixels, nearby preparation, recycling, retained vector scenes, renderer retirement and viewport/brick alignment. These counts include fixture/data rows. The unrelated Details physical-grid failures reproduced by the preceding unchanged control remain unresolved; no all-modes or full-suite pass is claimed.

## Alternating live result

Four fresh processes run A/B/B/A, each with 30 seconds of rapid wheel input after initialized empty-panel entry into actual Downloads (192 rows). Masonry density is 152, actual DPR 1.75 and refresh 160 Hz, default native GC budget 5,000 microseconds. Cache roots are empty before QML/navigation; input starts with incomplete loading, without requiring the reserve to be full. OS cache coldness is not claimed. No builds/tests/offline analysis overlap capture. The normal app remains alive. The temporary refresh lease was released and the prior 60 Hz mode restored without a registry change.

| Case | Arm | Cached / pending at input | Display gaps >9.375 ms | Drops | Maximum display gap | Coverage holds | Spares taken during input | Largest early preparation |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| cold-p21 | A | 25 / 18 | 2 | 0 | 18.7471 ms | 1 | 0 | 5.6605 ms |
| cold-p22 | B | 19 / 18 | 2 | 0 | 12.4887 ms | 0 | 29 | 5.3984 ms |
| cold-p23 | B | 18 / 18 | 2 | 0 | 24.9925 ms | 0 | 32 | 5.7480 ms |
| cold-p24 | A | 14 / 18 | 2 | 0 | 18.7520 ms | 0 | 0 | 6.4376 ms |

Early means commits beginning in the first 250 ms, including every phase-probe commit rather than only those above a preparation threshold. Candidate early complete commits peak at 6.2501 and 7.7614 ms, versus 6.0499 and 7.0113 ms in controls. Later complete commit maxima remain 10.8981/11.4478 ms for candidates and 11.4464/10.4887 ms for controls. These are inclusive elapsed timings, not exclusive CPU costs.

Every display outlier, drop, unmatched/excluded pair and the one control coverage hold remains in the original analysis. Physical wheel error still has zero admitted samples; approximately 4,792–4,794 pairs per case are excluded. All reserve construction-failure counters are zero and maximum ready is sixteen, but before-input aggregate ready-minus-taken counts are only five and four. The reserve is exercised without being forced full at the benchmark boundary.

## Decision

The normal transaction fixes the prototype's assignment mechanism and passes direct integration checks, but it does not establish a consistent cadence win. Keep it isolated. Repeatedly increasing reserve capacity is not justified by these results, particularly because prewarming adds objects and competes for incubation time. The full startup, warm-tour, rapid wheel, quarter-height middle-scroll and physical-motion requirements remain open.

The next investigation should reduce per-delegate construction itself rather than add another reserve. One concrete source candidate is identical `FontMetrics` objects repeated for every entry/folder preview despite their shared panel font. Its geometry must be proven unchanged and its allocation/cadence effect measured before adoption; it is not implemented or credited as a fix here.
