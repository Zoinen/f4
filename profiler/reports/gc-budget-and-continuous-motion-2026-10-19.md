# Continuous masonry scrolling: GC and observer attribution

Date: 2026-10-19. Goal: sustained 160 Hz without spikes. **Not achieved.**

## Workload and measurement

Downloads, 186 entries, density 152, 160 Hz, actual DPR 1.75, wheel delta 120 every 30 ms. All cases retain the staged renderer implementation and combined QtQuick library. Each fresh process records two four-second blocks. No UI Automation observer, build, or tests run during clean intervals. No captures are excluded from the tables below. Caption measurements are zero.

The older single-pass workload reached a folder boundary early: one representative run moved for approximately 1.25/1.51 seconds out of 4.15/4.14 seconds. Bulk percentiles therefore included substantial stationary time. The new boundary-bounce workload sends actual wheel events and reverses near either end; it never assigns contentY directly. Block 0 and block 1 no longer mean down/up. Full-recorder GUI trajectories move at least one physical pixel on 93–97% of steps. In the recorder comparison, both arms move between every 30 ms input sample, with two trajectory reversals per block; that coarser sampling does not prove every rendered frame moves.

Display metrics require both display endpoints and the current present inside the measurement window. Producer interval metrics also exclude leading idle intervals but retain long internal stalls. Producer intervals are not native frame CPU durations.

## GC budget trials

A = 5 ms, B = 1 ms, order A1/B1/B2/A2/A3/B3. The native GC budget field verifies the requested value. The original timed-drain chunk of 10241 is retained. Earlier chunk trials predated staged creation and used a different workload; these new trials isolate the budget on the staged implementation. The result is mixed: do not promote the smaller budget.

### Original single-pass workload

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 12.5124 / 18.7621 | 22 | 6.3767 / 12.5123 | 2 |
| B1 | 6.3324 / 12.5079 | 4 | 6.3122 / 12.4814 | 1 |
| B2 | 6.3090 / 12.4995 | 1 | 6.3056 / 6.3592 | 0 |
| A2 | 6.3295 / 12.5158 | 3 | 6.2913 / 12.4899 | 1 |
| A3 | 6.3306 / 12.4993 | 2 | 6.2728 / 6.3553 | 0 |
| B3 | 6.2926 / 12.5076 | 2 | 6.2800 / 6.3794 | 0 |

All blocks combined: A 30, B 8 long display intervals.

The A1 outlier dominates this apparent difference; retain it, but do not infer a reliable benefit.

### Continuous movement workload

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.2946 / 12.5052 | 3 | 6.3272 / 12.4932 | 2 |
| B1 | 6.4045 / 12.5346 | 6 | 6.2871 / 12.4722 | 1 |
| B2 | 6.2812 / 12.4828 | 1 | 6.2807 / 12.4993 | 1 |
| A2 | 6.2855 / 12.5272 | 2 | 6.3108 / 18.7470 | 2 |
| A3 | 12.4943 / 12.5294 | 11 | 12.2603 / 18.7499 | 7 |
| B3 | 12.4929 / 18.7431 | 13 | 6.3650 / 12.4838 | 2 |

All blocks combined: A 27, B 24 long display intervals.

No reliable improvement. Both budgets retain missed refreshes. Largest render peaks do not cluster around wheel reversals; the trace does not support attributing them to repeated logical motion start/stop.

## Recorder ablation

A = full recorder; B = minimal recorder. Both use the same binary, fixed 5 ms budget, and continuous wheel input. Minimal mode disables all repeated GUI/render sampling and vsync callbacks. A single-shot hook captures the render thread identity; input samples remain at 30 ms. Command-file polling is stopped during both arms. Minimal raw frame arrays remain empty, and absent timing fields are null rather than fake zeroes.

### Matched observer comparison

| Run | Block 0 p99 / max (ms) | >9.375 ms | Block 1 p99 / max (ms) | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 12.4886 / 12.5237 | 9 | 6.3368 / 12.5146 | 3 |
| B1 | 6.3281 / 12.5016 | 4 | 6.3328 / 18.8384 | 2 |
| B2 | 6.3382 / 18.7460 | 2 | 6.3074 / 12.4972 | 2 |
| A2 | 6.3177 / 12.5081 | 3 | 6.3101 / 12.5130 | 3 |
| A3 | 6.3412 / 24.9741 | 3 | 6.2925 / 12.4952 | 2 |
| B3 | 6.3421 / 12.5058 | 5 | 6.2892 / 12.4965 | 1 |

All blocks combined: A 23, B 16 long display intervals.

The observer may contribute overhead, but missed refreshes remain with it disabled. Pairwise results are mixed; no precise overhead reduction is established. This is a diagnostic harness on the application path, not a claim that an entirely uninstrumented release binary was measured.

## Low-observer native attribution

`minimal-continuous-sampled-wheel-masonry` uses the minimal recorder, 1 ms budget, native GC phase scopes, PresentMon, and WPR sampling. No QML profiler is attached. The ETW raw-QPC join matches 47,610 switches with 900 ns offset spread at 10 MHz. Zero events/buffers are lost. Host symbols use the offline linker map; ICF aliases and nearest-symbol limitations remain recorded. Unknown DLL or inlined frames are not inferred.

The third-longest producer interval is 11.1444 ms. GC-state overlap is 8.8694 ms; DoSweep is 7.3182 ms, of which BlockAllocator::sweep is 7.1362 ms. This is atomic work despite the 1 ms budget. The GUI is scheduled for 10.8544 ms during that interval. Four CPU samples show QQmlValueTypeWrapper::virtualDestroy with QObject signal removal or QQmlNotifierEndpoint disconnection in the sweep. The phase log contains 2,388 records, below its 32,768 cap.

The two longest producer intervals, 12.9292/11.9806 ms, have no GC overlap. They contain GUI delegate preparation and incubation plus render batch/material work. These are separate targets. Ready-delay maxima are only 0.251 ms (GUI) and 0.107 ms (render), which does not support general scheduler starvation. A context-switch successor process is not evidence of the reason for a wait.

## Allocation census

A separate instrumented QtQml translation unit aggregates type, QML source line, owner property, and create route. It covers three QQmlValueTypeWrapper::create overloads and detached(), and records destruction by type/connection. It retains no QObject or GC-heap pointers. Native string/hash aggregation adds overhead: **use its counts, not timings**. Counts begin immediately before the first wheel block and finish after the second; they include the inter-block gap. Pre-existing objects can be destroyed during the capture. The 4096-key cap did not overflow.

Created 23,708 wrappers; destroyed 21,938.

| Value type | Created |
| --- | ---: |
| QRectF | 11,040 |
| QColor | 6,200 |
| QSizeF | 3,176 |
| QPointF | 1,869 |
| QPoint | 1,072 |
| QFont | 209 |
| QSize | 117 |
| QMatrix4x4 | 25 |

Highest individual site: `GalleryCursorController.qml:89`, an eagerly maintained viewportSceneOrigin point, 1,312 creations. `GalleryEntryDelegateBase.qml` accounts for 8,104; `GalleryEntryPreview.qml` for 6,638; `GalleryEntrySelectionSurface.qml` for 4,193. Four hidden debug-zone rectangle bindings each account for 840 reads. Destroyed connected wrappers include 4,167 QColor, 1,680 QRectF, and 944 QPointF. The census identifies types and source locations; it does not establish the precise time saved by changing any site.

## Next implementation

1. Stop maintaining the cursor viewport-origin value when no alignment operation needs it. Preserve the exact native resting origin and the animation exception.
2. Replace repeated preview-rectangle component reads with scalar geometry, retaining the existing rect interface for consumers that need it. Disable hidden diagnostic geometry bindings.
3. Investigate connected QColor churn in selection/foreground bindings using the recorded owner properties.
4. Re-run continuous matched clean display captures and the native attribution capture after each meaningful change. Test cursor/selection/preview transitions and actual visual-leaf geometry at 175%; do not trade sharpness or complete paint for a better timing counter.

No GC default, production dependency, core implementation, or portable payload was changed in this experiment. The normal worktree app was restored using its embedded host; binary identities and empty stderr are verified by the experiment archive script. The 160 Hz goal remains active.

## Reproduction and evidence

Compact JSON, analysis scripts, diagnostic sources, build commands, tests, and a SHA256 manifest are archived in `profiler/experiments/gc-budget-and-continuous-motion-2026-10-19/`. Executables, libraries, raw ETL/CSV/QTD, linker maps, and build outputs are excluded. Raw captures remain under `artifacts/modes-profile/` and `profiler/traces/` locally.

> AI assistance has been used to create this output.
