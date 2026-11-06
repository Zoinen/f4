# Requested-row incubation in Masonry — 2026-11-01

The diagnostic integration removes the large cold-entry delegate-commit bursts and produces two wheel captures without refresh misses. It also introduces coverage holds, so it is **not** ready for promotion and does **not** achieve constant 160 Hz motion. The normal application and embedded payload are unchanged.

## Implemented integration

Four diagnostic C++ copies connect the [requested-row prototype](masonry-requested-row-incubation-2026-11-01.md) to Masonry component creation, pool acquisition, row finalization, model replacement/reset/incremental changes and destruction. They preserve the existing native census. No MasonryLayout fields or ABI changed. The same host runs either arm: `F4_REQUESTED_ROW_INTEGRATION=1` enables incubation; the control clears it.

Only non-contained Masonry with a window, QML engine and existing incubation controller uses the new path. Other modes and controller-less operation retain ordinary synchronous construction. The queue's plan orders visible rows, the existing delegate/target-leading range, then the existing metadata range. It does not expand those ranges. At most two constructing or unclaimed-ready roots are retained per panel. When a newer requested row becomes higher priority, the next frame cancels and requeues lower-priority construction even if both rows remain inside the plan.

An unfinished root stays outside `brick.item` and the active-row indexes. A finished root is taken through the normal factory/pool path, rebound with current row data and geometry, and unmasked during finalization. An existing free pool slot cancels the duplicate queued request. Reset/incremental/model-replacement/destruction hooks cancel work before releasing the previous layout state; hide and mode changes cancel pending work too.

Host SHA-256: `949452c96b490c94bce97876c193f4c81e7cd70e281f8c45e02591701a9572b4`. All links use `ci/msvc_link_response.py`. Frozen sources, scripts and evidence hashes accompany [the manifest](../experiments/masonry-requested-row-integration-2026-11-01/manifest.json).

## Native foreground B/A/A/B comparison

The four captures use actual Downloads (192 rows), both panels in Masonry density 152, native 3840×2160, DPR 1.75, 160 Hz, fresh private thumbnail caches and initialized-app navigation from an empty folder. Each exercises 30 seconds of rapid wheel input. The helper is disconnected before timing. Native foreground admission, every sampled input, cache scope and the actual 5000 μs GC budget pass. No build, test, screenshot or analysis ran inside an input capture.

| Case | Arm | Display intervals | p99 ms | Max ms | Intervals >9.375 ms | Dropped presents | Coverage-held samples |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| cold-d51 | candidate | 4801 | 6.28760 | 6.3973 | 0 | 0 | 70 |
| cold-d52 | control | 4799 | 6.30620 | 12.5009 | 2 | 0 | 0 |
| cold-d53 | control | 4799 | 6.30301 | 18.7468 | 2 | 0 | 0 |
| cold-d54 | candidate | 4801 | 6.28450 | 6.3907 | 0 | 0 | 105 |

The zero-miss display result is useful but insufficient: the renderer can present regular frames while holding its camera against the coverage boundary. In cold-d54, for example, recorded samples request Y≈399–425 while the covered/rendered boundary stays at Y≈396. These are real failures of the zero-hold requirement. The sample counts are not counts of separate visible pause episodes.

Early input-relative preparation maxima drop from 6.2041/6.0187 ms in controls to 0.3701/0.5264 ms in candidates. Later commit maxima remain 9.4600/9.9740 ms in candidates, versus 11.5182/12.5935 ms in controls. Elapsed native spans are not CPU-residency measurements, and a GUI span longer than one refresh need not cause a missed render-thread presentation.

## Preserve cold-entry work

| Case | Navigation→input ms | Cached frames at input | Pre-input commit elapsed union ms | Largest contained commit ms | First post-nav submitted present displayed ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| cold-d51 | 436.0133 | 48 | 51.2964 | 3.8227 | 83.6226 |
| cold-d52 | 549.1331 | 10 | 191.3140 | 45.8318 | 135.4145 |
| cold-d53 | 552.5385 | 11 | 191.3040 | 39.4742 | see raw pre-input attribution |
| cold-d54 | 539.4285 | 48 | 53.4670 | 3.7971 | see raw pre-input attribution |

This retains the navigation-to-input interval instead of reporting only already-started input. The same readiness-driven harness admits different cache states: candidates have 48 cached frames at input, controls 10/11 with 18 pending. That difference must remain visible in interpretation. A separate fixed-early-input capture is still required; readiness must not conceal unfinished work. The first submitted/displayed present is not proof of the first new-folder content pixel.

All four motion analyses exclude 4793–4794 wheel-curve pairs and admit no physical-error samples. They do not prove the ≤2 px physical motion requirement. Full-folder warm touring and quarter-height middle scrolling remain outstanding for this candidate.

## Tests and visual verification

The two new real-Masonry tests pass with suspended incubation/reset and rapid reversal/mode changes. A separate queue test now verifies preemption **inside** a retained plan; all ten standalone cases pass on the native renderer at DPR 1.75 (12 QtTest results including fixtures). Both constructing and ready-root counts stay bounded.

The 175% geometry checks cover actual named Text/Image leaves in five modes at caption sizes 13/17/24, including complete scene origins and unit vectors. Parked-bitmap cases and SVG thumbnail resizing pass. The existing recycled-preview geometry check passed in an isolated run, but a later combined run fails its coverage assertion `reopened > 0`: it observed no recycled identity changes. It does not report premature geometry in that failure. This remains unresolved; there is no full-suite-green claim, and the test must exercise actual reuse before rollout.

Rendered inspection caught a test-fixture gap that geometry alone missed. Initial native captures contain no caption glyphs; the same executable with incubation disabled has the same problem. Its default MS Sans Serif font logs DirectWrite creation failures. A pixel check records zero glyph-colored pixels in the caption region for **both** arms. Those initial captures are not valid caption-rendering proof.

The fixture now selects Segoe UI through its existing metrics property. In the rerun, caption glyph pixels are present at all three sizes (323/566/1250 qualifying pixels in the first caption's region in each arm). The three candidate Masonry captures match the corresponding new control captures exactly in RGB **and** alpha. The smallest caption and raster icon were inspected. This is a fixture-font change, not a product-font or pixel-rounding change.

`F4QuickViewSurfaceTests qmlImportsWithoutInstalledQt` passes with embedded-only engine imports. `F4WindowsApplicationIconTests executableUsesWindowsGuiSubsystem` is compiled against this exact diagnostic host and passes. The Windows portable dependency audit passes with only system DLL imports. These checks do not claim an embedded-payload release: no new payload/Go package was produced.

The first integration run failed before panel creation because the standalone test selected the unavailable native Windows Controls style. Subsequent runs explicitly select the host's embedded Basic style. The raw failures remain. An initial analysis invocation using Python `-m` failed an import; running the tool as its documented script succeeded. No capture was restarted for that analysis error.

## Next investigation

Instrument requested row→start→ready→pool claim→coverage publication around the held samples. Determine whether throughput, queued-notification timing, repeated re-prioritization or invalidation causes the missing lead rows before changing the bound. Preserve every target-leading row and keep the same camera trajectory.

One source-level concern is already identified: this prototype's request key appends `modelVisualRevision`. `MasonryLayout::updateVisualRowsForDelta` increments it even for image-URL-only deltas. Thus ordinary thumbnail availability can invalidate pending construction; it is broader than an immutable file-source revision. This is a justified next hypothesis, **not yet proof that it causes the observed holds**. A corrected key must still invalidate changed source content, and finalization must continue applying the newest visual snapshot.

Also make the existing recycling regression exercise actual slot reuse under asynchronous scheduling, keep the rendered-glyph check, add fixed-early input, and repeat the full motion workload. Do not accept regular presentations obtained by holding the camera in place.

The four owned diagnostic pairs are closed. Their per-case host paths preserve hashes through archived symlinks. The temporary display lease restored its prior 60 Hz mode with no registry change. The normal worktree Go/Qt pair (9252/106980) was verified alive after capture cleanup. No commit, push or normal-app rollout was performed.

> AI assistance has been used to create this output.
