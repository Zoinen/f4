# Masonry: shared caption font metrics

Status: isolated prototype; not promoted. Constant 160 Hz remains unachieved.

## Implementation and build

The control has a `FontMetrics` object in every `GalleryEntryDelegateBase` and every materialized folder-preview shell, all using the same panel icon-label font. The candidate declares one shared object on `GalleryPanel`, exposes its height, and reuses that height in the entry caption reserve and folder Grid caption calculation. Existing arithmetic is unchanged. Source copies remain under the owned temporary directory; production QML is unchanged.

Both versions regenerate the three QML compilation units and their embedded source-resource bundle with Qt 6.11.1's qmlcachegen/rcc, retaining original resource URLs. They use the same compiler flags, base host graph and input recorder, without the rejected delegate reserve. No source interceptor or interpreted-source comparison is used. The installed Qt libraries, archived build inputs and embedded production payload are not replaced.

Host SHA-256:

- Control: `d1c4d1bd81f1cf734c300f971df775dc4fa64f69c154261b336c99184369ac97`
- Candidate: `54506c2d5248b815c475e823cd21a8f6a6910be16fedc74614b263c29e323ca8`

Windows GUI-subsystem/static-import audits pass for both. The candidate's `qmlImportsWithoutInstalledQt` gate passes using resource-only import paths. Its existing minimal-controller warning about `cursorPixelGridViewportOrigin` is retained; that binding is unrelated to font metrics. The first isolation-test link lacked the test controller/moc objects; its failed log is retained separately from the repaired link and passing run.

## Native layout and rendering checks

A shared native test exercises all five modes, changing panel font size through 13, 17 and 24 in each window. It verifies live font-height updates against QFontMetricsF, caption reserves, actual static Text/image leaf origins at DPR 1.75, unscaled unit transforms, geometry records and rendered output. Each version reports seven passing QtTest results, including setup/cleanup and the five data rows. This represents fifteen font/mode states per version, not fifteen distinct QtTest methods.

The candidate removes all directly parented entry FontMetrics objects in this fixture; control counts range from nine to sixty depending on mode/materialization. The panel's shared-height property is present only in the candidate. Folder-preview Grid/pixel checks also pass in all three relevant modes for both versions (five results per run including fixtures).

Masonry, Grid and Icons have identical recorded geometry/leaves and identical capture pixels at all three sizes. The smallest Masonry filenames were visually inspected. Columns captures are also pixel-identical, but the first two font states differ by approximately 1e-13 in one partially visible row's recorded extent and by one whole physical pixel in an off-screen icon origin. Both versions pass the physical-grid/unit-transform assertions. The cause of that off-screen neighboring-pixel choice is not established; full raw geometry equivalence is not claimed.

Details geometry/leaves match after sorting traversal order, while one RGB pixel at (59,39) differs in each font capture. A fresh unchanged control run reproduces a difference at that same pixel with identical geometry. Thus the tiny Details capture difference is within observed baseline variation, not evidence attributed to the candidate.

The first comparison used RGBA ImageChops.getbbox, which can hide RGB differences when the difference alpha channel is zero. That result is rejected. The corrected comparison explicitly checks RGB and alpha channels. `profiler/render_image_compare.py` has four passing tests, including unchanged-alpha RGB differences, alpha-only changes and dimension mismatch. Corrected results and the original rejected output are both preserved; no all-fifteen-images-identical claim is made.

## Alternating cold-entry captures

Four fresh processes run A/B/B/A, each with initialized empty-panel entry into actual Downloads (192 rows), Masonry density 152 and 30 seconds of rapid wheel input. Actual display conditions are DPR 1.75/160 Hz. The native default GC budget is verified at 5,000 microseconds. Cache roots are empty before QML/navigation; first input does not wait for previews. OS file-cache coldness is not claimed. No builds/tests/offline analysis overlap capture. The normal app remains alive, and the bounded temporary display lease restores the prior 60 Hz mode without a registry change.

| Case | Arm | Cached / pending at input | Display gaps >9.375 ms | Drops | Maximum display gap | Largest early preparation | Largest later commit |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| cold-s21 | A | 15 / 28 | 1 | 0 | 12.4932 ms | 5.4260 ms | 10.3275 ms |
| cold-s22 | B | 12 / 0 | 3 | 0 | 75.0024 ms | 5.7156 ms | 9.4850 ms |
| cold-s23 | B | 18 / 18 | 2 | 0 | 25.0019 ms | 6.1019 ms | 10.2344 ms |
| cold-s24 | A | 26 / 14 | 3 | 1 | 18.7572 ms | 6.4564 ms | 11.1279 ms |

Early means commits beginning within 250 ms of input; later means after that. Timings are inclusive elapsed durations, not exclusive CPU costs. All outliers, including boundary intervals, and the control's dropped row remain in the evidence. Coverage holds are zero in all runs. Physical wheel-error verification still has zero admitted samples and 4,791–4,794 excluded pairs per case. Cache states differ, so four captures do not establish a precise regression magnitude.

## Decision and next action

The candidate removes redundant objects and preserves the tested caption behavior, but does not establish a consistent cadence gain. Keep it isolated rather than adopting an object-count reduction as proof of progress toward spike-free rendering. The existing full startup/warm-tour/wheel/middle-scroll, physical-motion and broader pixel-grid requirements remain open.

The proposed hidden-debug-overlay follow-up was checked against current source and the October 18/19 reports: `GalleryLayoutDebugZone` is already a Loader with `active: visible` and conditional URL resolution. Do not repeat that completed optimization. The next measurement should use a fresh QML creation/binding trace to attribute the remaining per-delegate cost directly. The matching SDK qmlprofiler is present; its CLI help was verified. On this Windows GUI-subsystem tool, `QT_COMMAND_LINE_PARSER_NO_GUI_MESSAGE_BOXES=1` routes help to captured output; the initial owned help-dialog process was closed and its receipt retained. No capture was restarted because of that help inspection.
