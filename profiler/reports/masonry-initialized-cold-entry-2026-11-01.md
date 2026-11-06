# Masonry: initialized application, cold folder entry

Status: measurement progress; constant 160 Hz is not achieved. No additional product change was made for this experiment.

## Scope and acquisition

Diagnostic host SHA-256: `0665e79574fe133e47c62abfa4b1ac37662ca8c0feb48369aca97932e57e0a78`.
Both panels initially show an empty directory. The gate observes at least three rendered frames, waits at least two seconds, verifies zero cached frames/stores/pending requests, and sends the ordinary `panel.navigatePath` action to Downloads. Existing first-viewport wheel input begins without waiting for thumbnails to finish. Downloads has 192 rows, Masonry density 152, DPR 1.75, 3840×2160 at 160 Hz. The native default GC budget remains 5000 microseconds.

The desktop had reverted to 60 Hz. Each successful capture used a bounded temporary 160 Hz display lease; the previous 60 Hz mode was restored afterward, without changing the registry setting. These are temporary-mode captures, not evidence of an unchanged desktop refresh setting.

Raw evidence: `C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/`.

| Case | Result | Cached/pending at input | Display gaps above 9.375 ms | Drops |
| --- | --- | --- | --- | --- |
| cold-j11 | Rejected: display guard terminated diagnostic at 60 Hz | — | No accepted measurement | — |
| cold-j12 | Initialized-entry scope accepted | 4 / 12 | 118.7513, 25.0024 ms | 0 |
| cold-j13 | Initialized-entry scope accepted; early scheduler trace | 14 / 19 | 24.9899, 12.4952, 12.5147 ms | 0 |

For j12 the empty baseline rendered 84 frames; navigation preceded input by 483.0393 ms. The cache was empty before navigation, not necessarily at first input. OS file-cache coldness is not claimed.

## Findings

Settling the application before entry does not remove the first-scroll stalls. All j13 display outliers correspond to presents within 34 ms of input start. The first interval crosses input start and is retained. The next two presentation intervals are 12.5598 and 13.4747 ms; GUI-thread scheduled time is respectively 12.0488 and 12.5922 ms. Renderer scheduled time is 3.8626 and 3.1388 ms. Thread times can overlap and must not be added. This narrows investigation toward GUI work during entry/first input; it does not identify the responsible function.

The j13 scheduler decode reports 951904 events, zero events/buffers lost and zero schema errors. Its early trace does not cover the full input. Profile samples use an unverified prefix fallback; no function attribution is claimed from them, and maps from other diagnostic binaries must not be reused.

J12 bounded visual-phase census has 94814 events and zero overflow. Its largest post-input commit is 14.7745 ms at +189.4614 ms, including 8.6448 ms layout and 4.2211 ms preparation. Later completion/binding spikes remain, although no later display-threshold misses were recorded in this run. These are descriptive phase timings, not proof of causation for the initial display gap.

Both runs have zero coverage holds. J13 retains one unmatched motion commit. The wheel analyzer has zero valid physical-error samples and excludes its motion pairs: physical motion acceptance remains unproven. No full performance acceptance is claimed.

## Verification and next action

Six initialized-entry admission tests pass, covering initialized baseline, settlement, navigation ordering, process identity, empty pre-navigation cache and display/input constraints. J11 is preserved as rejected: dump confirms intentional fatal display-guard exit (`0xc0000409`, fatal-app-exit subcode 7), not decoder attribution.

Retain startup boundary misses and later GUI spikes in final acceptance; this narrower entry experiment does not replace the full wheel/middle-scroll/physical-motion suite. The normal production app and canonical executable were preserved.

## Follow-up: exact symbols and independently verified samples

The symbol-only link uses the initialized-entry response file through `ci/msvc_link_response.py`. Its `.text` and `.pdata` sections (addresses, sizes and bytes) match the captured host exactly. The generated map's preferred image base also matches. Evidence: `artifacts/initialized-entry-symbol-map-verification-20261101.json`. The symbol-only binary is never launched.

Independent xperf output matches all 411 selected GUI/render profile samples by thread, instruction pointer and timestamp, with zero mismatches. This validates the profile prefix fallback for this trace. `profile-prefix-verification.json` pins both trace and reference hashes. The previous warning about unverified prefix samples is resolved for j13 only.

Verified first-miss stacks include `ExternalCatalogModel::handleImageInfos`/metadata publication, QML notification chains and resource lookup. Subsequent intervals include thumbnail publication, `MasonryLayout::createComponent`, `GalleryDelegatePool::acquire`, and QML object finalization. Incubation-controller stacks occur in 4/21, 1/11 and 5/13 GUI samples in the three intervals respectively. These inclusive sampled counts overlap; they are not exact durations or an exhaustive distribution. Missing/multiple stack matches remain recorded; fragments are preserved separately.

The bounded visual census records 25, 11 and 25 delegate commits starting in those three intervals, respectively. Many owners have zero to four rows, consistent with contained folder-preview layouts; owner identity alone does not prove which folder each belongs to. The two larger layout owners have up to 47–68 rows. Largest commits starting in the second and third intervals are 4.7819 and 6.0441 ms, respectively. Commit times are inclusive and must not be added across nested layouts. Thus no single 12-ms main-layout commit explains all the GUI time: publication, incremental component creation and nested layouts share the interval.

Next probe: distinguish the incubation-controller callback budget from its synchronous component-finalization work and nearby preparation. Instrument complete callbacks before testing a scheduling change; lowering a timer alone cannot preempt a synchronous QML finalizer. Preserve visible preview latency and viewport coverage when evaluating any candidate. This evidence does not support another blind GC-budget change.
