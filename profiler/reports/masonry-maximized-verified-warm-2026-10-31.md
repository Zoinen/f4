# Verified warm scrolling with maximized startup

Two 30-second rapid-wheel captures now have verified all-entry warm preparation on the current production graph. One contains a 12.5074 ms display interval and one dropped present; the repeat contains neither. Neither run adds thumbnail-cache misses, stores or frames. Constant 160 Hz without spikes remains unachieved, and physical-motion acceptance is unverified.

## Correct the warm observer before measuring

The initial stricter tour timed out twice at the first viewport, naming JPEG rows 47 and 48. A detailed demand-trace process published both thumbnails and completed its tour. Both files also decoded successfully in an independent PIL check. This initially suggested timing-sensitive publication, but subsequent state inspection contradicted that interpretation.

The untraced inspection records valid decoded URLs in both the catalog and delegate snapshots for those rows. Their `masonryGeometryReady` and `sceneVisible` drawing gates are false. Qt visibility is deliberately retained for native scene reuse, so `QQuickItem::isVisible()` plus rectangle intersection incorrectly admitted two parked delegates. The native visible-index set contains rows 0–46, exactly matching the 47 actually drawn rows; the old scanner admitted 49. Their effective preview sources were empty because their drawing/readiness gates were closed, not because the underlying decode had failed. The captured window was inspected and is retained.

The corrected observer additionally requires the row's actual `sceneVisible` drawing gate. It does not filter out loading drawn rows or change product visibility. Five fixture-based regressions verify the original 49-row false admission, the correct native 47-row set, decoded sources on parked rows, continued rejection of a drawn unready row, and Qt-hidden exclusion. No product bug is established by the original timeouts. The failures, trace and untraced inspection remain preserved.

## Preparation and scope

The diagnostic host replaces **main only** in the current production graph. It has no renderer, GC, cache, decoder, null-string prototype or ongoing GUI-notification/census overrides. Host SHA-256: `2a366f5b405a828b06e2cba2ebfc452f115eec76cc5947d9d61a8704d935f8ec`. The Go launcher remains ac68c2e7. Private settings restore maximized geometry before renderer initialization, preserving the corrected production-sized startup.

Actual Downloads contains 192 entries including parent. The tour moves through the normal panel scrollbar viewport method, in half-viewport increments down and back to zero. It preserves normal content and requires, at each stop:

- actually drawn/intersecting rows, with ready previews or explicit terminal failures;
- settled folder state and no loading nested image/Loader surfaces;
- zero pending shared thumbnail requests;
- the requested viewport position within 0.51 physical pixels;
- a stable identity/source/cache/layout signature for at least 500 ms;
- a subsequent `frameSwapped` after that stable period begins;
- actual active window, DPR 1.75 and 160 Hz.

Each accepted tour covers every index 0–191 across 17 stops. Both untraced tours take approximately 11.17 seconds. Each timed capture is preceded by a separate verified tour and begins at contentY zero. The tour scanner and command-file poll are stopped during timed input; only frame-stage/input recording remains. Cache counters are sampled before and immediately after input. No builds, tests, offline analysis or UI Automation run during those windows.

Eight additional verifier regressions reject incomplete coverage, loading surfaces, missing subsequent frames, drawn unready previews, missed viewport targets and new decoding during or after preparation. The inherited generic analysis hard-codes its warm label to false; its original output is preserved, with separate `verified-analysis.json` and warm-verdict files establishing the corrected scope from actual evidence.

## Results

| Capture | Wheel inputs | Render range p99 / max, ms | Long displayed intervals >9.375 ms | Dropped presents | Maximum display interval, ms | Cache miss / store / frame deltas |
| --- | ---: | --- | ---: | ---: | ---: | --- |
| admitted-warm-wheel-e1 | 2828 | 2.7248 / 4.4869 | 1 | 1 | 12.5074 | 0 / 0 / 0 |
| admitted-warm-wheel-e2 | 2814 | 2.7753 / 4.5580 | 0 | 0 | 6.4823 | 0 / 0 / 0 |

Both captures retain 119 cached frames throughout, zero pending requests before/after input, and unchanged miss/store counters since the final tour stop. These counters establish the thumbnail warm-state condition; they do not prove that every possible metadata read, allocation or GPU upload is absent. Display metrics retain every outlier and drop, including startup. A zero-miss repeat is not proof of the full goal. PresentMon is not optical scanout evidence, and frame-stage envelopes are not exclusive CPU costs.

## Localize the remaining miss

The first run's long display interval is associated with a present about 9.606 seconds after input starts. Its producer interval is 5.9548 ms and its matched Qt render-signal range is 0.8769 ms. The neighboring dropped present occurs at about 9.613 seconds. A later neighboring Qt frame lasts 9.3070 ms, of which its render-signal range is 2.9527 ms; it has no before/after-synchronization signals. Its pre-render envelope includes frame pacing and must not be called GUI CPU time.

The correlation uses exact containing Qt frame intervals with an explicit 1000 ns clock/CSV tolerance, not nearest-frame substitution. All selected neighboring rows match one frame; one partial capture-boundary frame is retained separately. The first run's largest GPU-completion envelope, 7.9745 ms, occurs near 23 ms into input and is not the later displayed miss. No single present, render or GPU metric proves why DWM selected or dropped a particular frame.

This rules out new thumbnail decoding as the explanation for the verified warm miss and narrows the next investigation to native frame pacing/presentation and the surrounding render work. It does not establish a driver/DWM cause or rule out every GUI contribution to earlier producer timing.

## Evidence and state

- Corrected host, full warm tours, captures, raw PresentMon data, all outliers, verifier results and exact frame correlation: `artifacts/maximized-warm-admission-20261031/`.
- Original false-admission timeouts: `artifacts/maximized-verified-warm-20261031/`.
- Opt-in demand trace, complete traced tour and terminal receipt: `artifacts/maximized-warm-demand-trace-20261031/`. Traced timings are not used as performance results.
- Untraced catalog/facade/delegate inspection and PNG: `artifacts/maximized-warm-inspect-20261031/`.
- Observer/verifier code and tests: matching date-suffixed scripts under `artifacts/`.

Disk pressure was handled by hash-verified external copies of explicitly identified closed diagnostic executables, preserving their old paths through file symlinks. No running normal executable, SDK or raw trace was moved. The first history-archive attempt stopped at a wrong third filename after successfully archiving two files; a follow-up verified those existing links, completed the correct third file and saved the receipt. Normal application sources and payload are unchanged, with no commit/push.

The full objective still requires cold rapid wheel, verified warm rapid wheel without misses, quarter-height middle-button motion, zero coverage holds, the physical-motion limits, normal-window behavior and 175% sharpness. This report verifies the preparation and records two warm baselines; it does not substitute those for the remaining acceptance requirements.

AI assistance has been used to create this output.
