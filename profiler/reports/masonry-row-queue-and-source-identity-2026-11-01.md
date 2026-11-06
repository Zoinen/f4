# Masonry row queue: lifecycle trace and source identity — 2026-11-01

The trace identifies completed work discarded by the pool and pending work invalidated by presentation updates. A source-identity correction removes most identity replacements, but coverage holds and one refresh miss remain. Neither this candidate nor the preceding asynchronous integration is promoted to the normal application.

## New trace and analyzer

The frozen diagnostic adds a serial number to each actual requested row and GUI-only census events: 64=request, 65=construction start, 66=ready/error, 67=take, 68=cancel, 69=ready notification delivery, 70=first publication. Cancellation reasons distinguish identity replacement, removal from plan, preemption, satisfaction by an existing pool slot and bulk cancellation. Bulk cancellation does not identify its individual reset/hide/destruction cause.

Publication is recorded only on the first claim of an asynchronously constructed root to keep the bounded census from overflowing. It is not a trace of every subsequent pooled reuse. Both the root serial and publication marker are diagnostic dynamic properties.

[requested_row_timeline.py](../requested_row_timeline.py) pairs requests by owner/serial, retains orphan and unfinished records, rejects duplicate/reversed/invalid lifecycles, measures elapsed stages separately, and accounts for constructing plus unclaimed-ready roots. Its six focused Python tests pass. Elapsed construction includes suspended incubation and other GUI work; it is not CPU time.

## Initial trace: cold-e61

The native foreground and cold-cache gates pass at Downloads/192 rows, Masonry density 152, 3840×2160, DPR 1.75, 160 Hz and 30 seconds of rapid wheel input. Computer Use is disconnected before release. The census contains 111,696 records with zero overflow.

| Lifecycle result | Count |
| --- | ---: |
| Requested | 762 |
| Construction started | 287 |
| Completed | 273 |
| Taken and first-published | 221 |
| Canceled | 541 |
| Canceled because identity changed | 196 |
| Completed roots discarded because a pool slot satisfied the row | 48 |
| Completed roots discarded because identity changed | 3 |

Most identity replacements occurred while queued: only seven involved started construction, including three already-ready roots. The pool discarded 51 started roots, 48 of which had completed. Do not count all 196 replacements as wasted completed decodes or construction.

Across all phases, queue wait has p50 75.8112 ms / p95 487.3291 ms / max 1190.5093 ms; elapsed construction has p50 8.6543 ms / p95 21.7092 ms / max 443.7863 ms; ready-to-take has p50 3.1329 ms / p95 14.4874 ms / max 19.5989 ms. These aggregate startup, pre-input and input lifetimes and include metadata/look-ahead work. They are not visible-row-only latency percentiles. The 443.8 ms outlier belongs to a background-panel request; the trace does not establish 443.8 ms of active CPU construction.

The render trace has 118 coverage-held samples and no display intervals over 9.375 ms. This again demonstrates why regular presentation alone is insufficient for smooth scrolling. Both owners reach, but never exceed, two constructing/unclaimed-ready roots. No queue lifetimes remain unpaired or unfinished.

The pool behavior follows its actual implementation: `acquire()` looks for a free slot before invoking the factory that can take a ready row. The bridge then cancels the duplicate queued request. That behavior is a concrete next optimization; it has **not** been changed in this experiment.

## Corrected source identity

The former key appended `modelVisualRevision`. `updateVisualRowsForDelta()` increments that revision even for an image-URL-only update, so thumbnail availability could cancel construction for unchanged source bytes.

The diagnostic correction uses a length-prefixed key derived from entry identity and existing in-memory source facts. For external files it includes the descriptor's access key and source-version token, preserving broker/access epoch and content-version changes without using a temporary materialization path. For local files it uses path, source-version token, size and last-modified time. An unmaterialized row has an explicit distinct key. No source file is read to compute the key. No Go wire fields, format handling, queue bound, viewport limit or pixel-rounding code changes.

The new real-Masonry regression suspends incubation, emits an image-URL-only update, changes density to revisit the pending rows, and checks cancellation. The old-key arm fails (10 cancellations versus the previous 6); the new-key arm passes without those four extra cancellations. Additional tests prove that presentation size/orientation/cache/typed-field changes preserve the key, while local size/time/version and external content-version/broker changes invalidate it. Changing an external temporary backing path does not invalidate it.

Final host SHA-256: `a825e348ede7c9fdf1535fc7bb3070413722dc3f6caa4bc521d6e682c7c5a540`. Control and candidate use this same binary and tracing graph. Both enable asynchronous construction; only `F4_REQUESTED_ROW_STABLE_SOURCE` differs. The first trace uses the preceding traced host. No comparison silently substitutes synchronous construction for the source-key control.

## Foreground A/B/B/A comparison

All four captures retain the same workload and native foreground/cold-cache/GC-5000 admission checks. Each begins input with 48 cached frames and zero pending cache reads; this does not imply the full folder is warm. The OS file cache is not claimed cold. No other work ran inside input timing.

| Case | Key | Display p99 ms | Display max ms | >9.375 ms | Drops | Coverage-held samples | Identity replacements | Ready identity discards | Ready pool discards |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| cold-f71 | old | 6.28550 | 6.4364 | 0 | 0 | 93 | 204 | 5 | 42 |
| cold-f72 | source | 6.31230 | 12.5034 | 1 | 0 | 76 | 74 | 0 | 31 |
| cold-f73 | source | 6.29722 | 6.4594 | 0 | 0 | 65 | 76 | 2 | 38 |
| cold-f74 | old | 6.28331 | 6.3946 | 0 | 0 | 76 | 187 | 3 | 44 |

The replacement reduction is repeatable in these two pairs. Coverage-held samples are lower in the candidates but overlap the control range, and one candidate misses a refresh. There is no overall acceptance claim. The counts represent held samples, not distinct visible pause episodes.

Every queue timeline has zero orphan/unfinished lifetimes and a maximum of two constructed roots per owner. Candidate identity replacements are not zero. The implementation can transition from the explicit unmaterialized key to a known source; the trace does not record full key components, so it does not classify all remaining replacements as that transition or as real content changes.

Navigation-to-input attribution and raw pre-input work are retained in `pre-input-attribution.json` for every comparison case. Input is still readiness-driven, so fixed-early-input behavior remains unverified. Wheel-curve pairs remain excluded by the existing motion analyzer, which admits no physical-motion error samples. No ≤2 px physical-error, full-folder warm-tour, or quarter-height middle-scroll acceptance follows from these captures.

## Verification and limitations

The final selected native Qt suite has 15 results: 14 pass, one fails. The new identity and thumbnail-update regressions, suspended-reset and reversal/mode cases, five-mode 175% geometry cases at 13/17/24 caption sizes, parked-bitmap cases and SVG resizing pass. The unresolved existing recycled-preview test again fails its coverage assertion `reopened > 0`; it did not exercise a recycled identity change. It reports no premature-geometry observation in that failure. Do not call the suite green or weaken the assertion to hide the missing coverage.

The named actual Text/Image leaves map to whole physical origins and unit scene transforms at DPR 1.75. Segoe UI caption glyphs are verified in rendered regions (323/566/1250 qualifying pixels at the three sizes); the smallest caption and raster icon were inspected. The earlier MS Sans Serif native-fixture failure and its correction remain documented in the [integration report](masonry-requested-row-integration-2026-11-01.md).

Both newly linked hosts pass embedded-only `qmlImportsWithoutInstalledQt`, the actual-host `executableUsesWindowsGuiSubsystem` test and the Windows system-DLL audit. Links use `ci/msvc_link_response.py`. The first stable-key test compile lacked the census include directory; adding that explicit include fixed the harness build. No failed capture was restarted. This is diagnostic validation, not a packaged embedded-payload release.

All five owned capture pairs are closed. Their closed host binaries and several prior closed test executables were copied to P:, hash-verified and replaced by symlinks to recover scarce workspace/system-drive space. The normal pair 9252/106980 remains alive. Both temporary display leases restored their prior 60 Hz mode with no registry changes. No production rollout, commit or push occurred.

## Next work

Prefer a completed requested row before rebinding a generic free pool slot, while registering its correct component key and preserving ownership and geometry-before-visibility. Add a regression that actually exercises that handoff and pooled reuse. Then repeat request/ready/publication tracing, classify remaining identity transitions, and check coverage at the beginning of rapid input. Preserve the two-root bound and target-leading window while establishing whether this removes the remaining throughput loss before changing any limits.

Sources, scripts, raw-capture references, test outputs, images and hashes are frozen in [the experiment manifest](../experiments/masonry-row-queue-and-source-identity-2026-11-01/manifest.json). The normal application is deliberately unchanged because the full objective remains unverified.

> AI assistance has been used to create this output.
