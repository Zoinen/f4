# Null-string reuse on the current production graph

The prototype passes the selected correctness checks but does not consistently improve scrolling. It remains diagnostic only. Constant 160 Hz without spikes is not achieved.

## Correctness and runtime control

The original and candidate paths run in one executable, selected once by `F4_PROFILE_DISABLE_NULL_STRING_REUSE`. The candidate reuses the existing per-engine null-QString empty value; non-null empty QString follows the original path. It changes neither GC scheduling nor rendering policy. The measured host uses the current production objects/libraries except the main frame/input observer and the candidate `qv4engine.cpp` translation unit. No allocation census or GUI-notification observer is active.

Four standalone QML runs, control/enabled with both JIT and forced interpreter, each pass 8,192 binding checks. All four produce identical observable-output SHA-256 `81ef040bc1c0c7ec6e33644ba08db1c32af2c92520e31decdb7d754dfbfc0787`. Coverage includes reactive string/var bindings, alias writes, null/non-null/Unicode replacement, JSON, equality, concatenation, empty property keys, explicit GC with live/deleted QML objects and engine teardown. The output also records QString nullness and notification counts, rather than checking only displayed text.

A second regression links the exact engine object used in the measured host. Control reports 32,768 fresh null-string objects; enabled reports zero. Both report zero semantic failures. This independently verifies that the runtime flag selects different allocation paths.

The selected native gallery suite passes 12 data cases in each arm at actual DPR 1.75 using D3D11 and embedded Basic controls. It covers vector/raster independence, nearby preparation, parked bitmaps, recycled-row readiness, retained vectors, camera culling and live theme updates. Existing actual-leaf scene-origin/unit-transform assertions execute. Five camera PNGs are byte-identical. The combined scene differs at 149 pixels by at most one channel level, confined to caption bands under the existing native-text tolerance. Both images were inspected, including small captions. This is not reported as byte-identical output.

## Scrolling comparison

Four fresh processes run in control/enabled/enabled/control order with the same host, SHA-256 `aa1f906bf78f0843bf753f27219011017369b7e4f4653280026aa6264ccb0995`. Actual Downloads contains 192 entries including parent; each capture preserves Masonry density 152, DPR 1.75 and 160 Hz. Every first input starts at contentY zero. The private profile restores maximized geometry before renderer initialization, preserving the corrected production-sized startup.

Each capture contains 30 seconds of rapid wheel input. No builds, tests, offline analysis or UI Automation run during timed input. The observer records main frame stages and input only. These are fresh-process initial-viewport runs: a cold cache and a complete quiescent all-entry warm tour were not verified. Physical motion and middle-button acceptance are also unverified. PresentMon is not optical scanout evidence.

| Run | Inputs | Render p99 / max, ms | Input handler p99 / max, ms | Display intervals >9.375 ms | Dropped presents | Max display interval, ms |
| --- | ---: | --- | --- | ---: | ---: | ---: |
| Control a1 | 2766 | 2.8447 / 6.2335 | 3.3655 / 7.5181 | 0 | 0 | 6.4851 |
| Enabled b1 | 2791 | 2.9618 / 8.6115 | 3.1507 / 8.6940 | 0 | 0 | 6.4393 |
| Enabled b2 | 2811 | 2.9296 / 6.5383 | 2.6090 / 8.3334 | 7 | 6 | 18.7434 |
| Control a2 | 2816 | 2.8585 / 5.9712 | 2.8203 / 7.2396 | 4 | 2 | 18.7555 |

All misses and dropped rows remain in the evidence. Candidate b2 misses occur about 17 ms, 5.20 s, 6.75 s and 16.21–17.18 s after input starts. Reverse-control misses occur about 1 ms, 18 ms and 23.61–23.62 s after input starts. Early misses are retained, not excluded as startup artifacts. Frame-stage ranges are instrumented wall time, not exclusive CPU cost.

These results do not establish a consistent benefit. Both initial runs have zero long displayed intervals; both repeats have misses. The enabled render p99 is slightly worse in both comparisons. Eliminating one observed allocation class therefore does not justify a production dependency patch or predict a proportional GC-time reduction. GC frequency/duration was deliberately not instrumented in these comparison runs.

## Failures and evidence preservation

The first host relink reproduced MSVC's leading-character truncation on a long LF-only response line. The failed log and input remain preserved. The successful relink uses `ci/msvc_link_response.py` without recompiling or repeating already passed QML tests; future builds use that helper too.

The first enabled-window inventory occurred before its window was created. The capture rejected stale readiness before starting PresentMon or input. The same live PID was revalidated, activated and captured without restart. A pre-input attempt receipt preserves the event. The launcher now waits for readiness from its own live PID. No completed performance run was discarded.

Four explicitly named closed diagnostic host binaries were copied to an external temporary archive, hash-verified, then atomically replaced by file symlinks at their former paths. The copies and relocation receipts retain all bytes and original hashes. No running binary, SDK, normal app, raw trace or capture was relocated. This recovered space without deleting evidence. Do not relaunch historical archived probes as new worktree captures.

## Outcome and next work

No production source, dependency default, embedded payload or normal application changed. No commit/push. The normal ac68c2e7 / 653cbd8a worktree app remains the verified running build.

The candidate and its regression stay available for future allocation work, but are not promoted. Next, establish a fully verified all-entry quiescent warm pass using the corrected maximized startup and isolate the remaining misses there. The full goal still requires cold rapid wheel, verified warm rapid wheel, quarter-height middle-button motion, zero coverage holds, the physical-motion limits, normal-window behavior and 175% sharpness. These initial-viewport captures do not substitute for that acceptance scope.

Sources, build/test/capture scripts, raw captures, all outliers, per-run identities, correctness results and comparison data are under `artifacts/null-string-current-graph-20261031/` and matching date-suffixed scripts in `artifacts/`. The closed-binary relocation is recorded in `artifacts/closed-probe-binary-relocation-20261031.json`.

AI assistance has been used to create this output.
