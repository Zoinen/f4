# Masonry stable-row transaction experiment — 2026-10-31

The unchanged-row fast path is diagnostic only. It did not consistently improve displayed-frame delivery and has not replaced the running normal build. The 160 Hz goal remains incomplete.

## Scope and method

Four fresh-process captures in control/enabled/enabled/control order used the same diagnostic host, SHA-256 `3890993018d75a65840cab9eb7420830d306ae1671df046a8aaeca937eb5f398`. Only the control environment flag changed. The host contains the GUI notification observer and an observation/candidate override of `MasonryLayoutDelegateCommit.cpp`; all other objects come from the current production graph. Both arms therefore include instrumentation overhead.

Each capture used the actual Downloads folder, 192 entries including parent, Masonry density 152, maximized startup geometry, actual DPR 1.75 and 160 Hz. Each ran 30 seconds of rapid wheel input from the initial viewport. No cold-cache state, complete 192-entry warm tour, physical motion error, or middle-button acceptance was verified. All display outliers and dropped presents remain in the analysis; PresentMon is not optical scanout evidence.

The source already has a whole-window unchanged-index fast path in `MasonryLayoutProperties.cpp`. This experiment does not duplicate it. Instead, after preparing each row, it conservatively skips the visual transaction only when the snapshot, source identity, mode, geometry, preview geometry, row/column, caption and geometry-ready state are unchanged. It still finalizes row visibility. New or changed rows take the original path. No extra rounding or scene mapping was introduced.

## Results

| Run | Commit median / p99 / maximum, ms | Display intervals above 9.375 ms | Dropped presents |
| --- | --- | --- | --- |
| Control a1 | 1.2023 / 5.9540 / 12.0383 | 0 | 0 |
| Enabled b1 | 1.1585 / 5.6687 / 11.5074 | 3 | 1 |
| Enabled b2 | 1.1846 / 5.1608 / 9.7600 | 2 | 0 |
| Control a2 | 1.0111 / 5.2710 / 10.6000 | 2 | 0 |

Enabled b1 reused 147,296 of 150,959 delegate visits; enabled b2 reused 146,659 visits. The high reuse rate confirms the guard was exercised, but the reverse control was faster at the median. These runs do not establish an overall performance win.

An earlier observation-only delegate capture measured 2,283 foreground left-panel commits: median 1.328 ms, p99 6.2895 ms, maximum 9.9903 ms. Row notification and acquisition contain outliers. The existing phase timers omit the RAII `endVisualUpdate()` callback. Subtracting measured non-overlapping phases from the delegate-loop duration leaves occasional 6–7 ms gaps in both candidate and control runs. This remainder includes transaction completion and loop overhead; it does not yet prove which callback consumes that time. Direct timing of transaction completion is the next justified measurement.

Source inspection shows `endVisualUpdate()` restores the visual-identity state, and fallback-icon bindings depend on that state. Some fallback Image components are synchronous. Whether those bindings cause the observed long gaps is an unverified hypothesis; no provider/cache behavior change was made.

## Correctness checks and failed attempts retained

The selected native layout suite passed 12 data cases in each arm at DPR 1.75 using the embedded Basic style. Coverage includes nearby preparation, retained vector scenes, recycled preview readiness, bitmap parking, native camera culling, SVG camera placement and live theme updates.

Five camera output images were byte-identical between arms. The combined scene capture differed at 105 pixels by at most one channel level, entirely within caption bands. Existing native tests document this last-bit native glyph rebatching tolerance. Actual leaf scene-origin/transform assertions passed, and both rendered captures were inspected, including the small captions. This is not reported as byte-identical output.

The first test launch failed because the default Windows Controls style was not embedded in the standalone test executable. That failure and the initial overly strict image-comparison rejection are retained alongside the successful Basic-style results and explicit tolerance verification.

## Evidence and state

- Raw captures, per-run identities, all PresentMon rows, analysis, candidate source and build receipt: `artifacts/stable-row-commit-candidate-20261031/`.
- Comparison generator: `artifacts/compare-stable-row-captures-20261031.py`; output `comparison.json` in that directory.
- Native results: `native-validation.json` and the preserved control/enabled output directories.
- Observation-only capture: `artifacts/delegate-commit-probe-20261031/delegate-phases-a1.json`.

No production source changes, portable payload rollout, commit or push were performed for this experiment. The normal application remains the previously verified ac68c2e7 / 653cbd8a build. The broader cold/warm/middle-motion and physical-motion acceptance remains outstanding.
