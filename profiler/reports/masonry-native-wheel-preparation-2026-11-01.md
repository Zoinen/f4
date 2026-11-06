# Native wheel preparation attribution — 2026-11-01

The expensive retarget calls synchronously create missing delegates for the upcoming camera region. The largest measured call spends **8.3221 ms preparing rows out of 8.7647 ms total**. Identity, visual, and layout phases total only 0.1450 ms in that call. These are instrumented elapsed times, not clean production frame times.

This analysis correlates the retained cold-t35 Qt QML trace and its native GUI census. There are 53 target-preparation calls inside the 1,366.4994 ms input-and-settling interval. The QML/native clock mapping is cross-checked by the navigation/input marker interval with 1 microsecond disagreement. No new capture or product change was needed.

## Largest target calls

| Input-relative start ms | Target ms | Commit ms | Prepare ms | Identity ms | Visual ms | Layout ms | Finalize ms | Fully contained creation union ms |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 36.2205 | 8.7647 | 8.7623 | 8.3221 | 0.0850 | 0.0377 | 0.0223 | 0.2193 | 4.3462 |
| 198.1821 | 6.5392 | 6.5366 | 5.5016 | 0.0915 | 0.0481 | 0.0304 | 0.1325 | 3.0221 |
| 237.7974 | 5.9975 | 5.9944 | 4.9540 | 0.0915 | 0.0489 | 0.0304 | 0.1241 | 2.6322 |
| 87.3197 | 5.9545 | 5.9509 | 5.2613 | 0.0719 | 0.0298 | 0.0824 | 0.1063 | 2.7904 |
| 130.9560 | 5.8896 | 5.8874 | 5.1385 | 0.0701 | 0.0295 | 0.0818 | 0.1037 | 2.7397 |
| 105.5670 | 5.5221 | 5.5194 | 4.7016 | 0.0788 | 0.0425 | 0.0809 | 0.0964 | 2.4792 |
| 451.3297 | 5.4370 | 5.4341 | 0.1311 | 0.0626 | 2.3523 | 1.7246 | 0.0113 | 0.2853 |
| 178.0579 | 5.3415 | 5.3393 | 4.7108 | 0.0680 | 0.0316 | 0.0206 | 0.1058 | 2.4396 |
| 22.5514 | 5.0514 | 5.0484 | 1.9772 | 0.1564 | 1.1782 | 1.3228 | 0.1308 | 0.9423 |
| 575.4972 | 3.9682 | 3.9666 | 0.2213 | 0.1025 | 1.8455 | 0.7258 | 0.0151 | 0.0774 |

Target preparation calls [prepareViewportMotionTarget](D:/Code/f4-zoin/third_party/ZoinGallery/MasonryLayoutNavigationPaging.cpp:79), compares the old/new delegate index windows, and invokes updateProperties when the window changes. [updateProperties](D:/Code/f4-zoin/third_party/ZoinGallery/MasonryLayoutDelegateCommit.cpp:580) prepares all required rows synchronously. Missing rows pass through prepareDelegateRow and acquire a QML root before binding/layout/finalization. The worst call contains repeated sub-millisecond base-delegate and visual-host creation ranges rather than one large identity or geometry write.

The native prepare phase includes acquisition and readiness work beyond QML Creating ranges. The fully contained creation union is **4.3462 ms** in the worst target call, leaving substantial additional preparation time. Neither that union nor the difference is an exclusive CPU measure. No percentage CPU conclusion follows from these elapsed intervals.

## Correct interpretation of the apparent shader spike

The largest ShaderEffect Creating range begins +124.8173 ms after input start and lasts 20.4575 ms. A separate wheel restart handler begins and ends inside it and occupies 5.8932 ms. The asynchronous component host can suspend and resume creation across ordinary GUI work. Therefore the large Creating range does **not** prove a 20 ms synchronous shader compiler stall. Warming or replacing shaders solely on that number would be unjustified.

The analyzer reports both all overlapping creation ranges and ranges fully contained in each target call. A long suspended range spanning the target otherwise makes the entire target appear to be construction. Tests explicitly cover this trap. Even fully contained ranges remain elapsed observations, not scheduler-residency measurements.

## Next optimization boundary

Reduce the cost of constructing and initializing a newly required row and its mode-specific visual hosts. Measure changes at that boundary with identical inputs and verify normal pixels and source ownership. Merely suppressing identity or geometry notifications cannot remove the dominant part of these bursts. Prior anonymous-spare and caption-metrics experiments did not establish a cadence win; increasing those constants is not justified by this result.

Keep target-leading preparation and its existing metadata budget. Deferring the same work until a camera frame already needs the rows, shrinking the prepared region, or counting only already-loaded pictures would hide work at the expense of coverage. A candidate must preserve the missing-row regression in GalleryLayoutEngineTest::motionTargetReusesTrailingBudgetForFastScroll and be evaluated with coverage holds and physical motion still visible in the results.

## Verification and reproducibility

Analyzer: [qml_native_attribution.py](D:/Code/f4-zoin/profiler/qml_native_attribution.py). Four regression tests pass, covering interval clipping/nesting, owner and boundary matching, aggregate phase payloads versus marker durations, and suspended creation ranges. The CLI requires successful capture exit codes, verifies the raw trace SHA-256, rejects census overflow, and requires the validated mapping discrepancy to be at most 10 microseconds.

```powershell
C:/Dev/Python/python.exe -m unittest discover -s profiler -p test_qml_native_attribution.py
C:/Dev/Python/python.exe profiler/qml_native_attribution.py --trace P:/f4-profiler-traces-f4-zoin-20261101/cold-t35/creation-bindings.qtd --capture C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/cold-t35 --mapping artifacts/entry-qml-flush-20261101/scoped-summary.json --output artifacts/entry-qml-flush-20261101/native-creation-attribution.json
```

The full cold/warm/middle motion acceptance remains open. The measured QML interval has profiler overhead; no new 160 Hz success or performance improvement is claimed. The normal app is unchanged and both diagnostic processes are closed.

> AI assistance has been used to create this output.
