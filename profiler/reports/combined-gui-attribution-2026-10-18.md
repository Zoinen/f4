# Combined renderer: remaining GUI work and scrollbar fixes — 2026-10-18

## Scheduler evidence

The preceding combined-renderer experiment improved some tails but did not prove sustained 160 Hz. This capture profiles that same combined QtQuick archive with symbols, real 30 ms wheel events, the default 5000 us GC budget, Downloads 184 entries, actual DPR 1.75 and an active 160 Hz window. Native frame timing under instrumentation is attribution evidence, not a clean comparison.

No ETW events or buffers were lost. Raw-QPC alignment matches 60,456 context-switch identities with 900 ns offset spread. WPR stopped and reports no active recording.

Two long intervals identify the next critical path:

| Native period ms | GUI scheduled execution ms | Render scope ms | Relevant GUI samples |
| ---: | ---: | ---: | --- |
| 12.7907 | 12.2717 | 2.2690 | 8 GC transitions, 6 runGC; delegate creation also present |
| 12.7202 | 11.1912 | 2.2252 | 7 GC transitions, 6 runGC |

Samples are inclusive/nested observations, not exclusive time. Scheduled execution includes kernel work, and GUI/render execution can overlap. The worst render scope in this capture is 4.6994 ms, all scheduled execution. Other long GUI intervals include nearby preparation, component creation and binding updates. GUI ready-delay p99/max is 0.019/0.171 ms; render is 0.009/0.104 ms. The evidence does not support raising priority as the next fix.

A single swapchain reports Composed: Flip. Present and display intervals are retained separately; none of the irregular display intervals are discarded as an environmental artifact. PresentMon defines these as different pipeline measurements ([official metric documentation](https://github.com/GameTechDev/PresentMon/blob/main/README-ConsoleApplication.md)). No optical display-timing claim is made.

## QML attribution

The SDK qmlprofiler --help invocation displayed its usage in a Windows dialog. That dialog was inspected with the computer-use skill and dismissed; the command exited 0. The tool was then attached through its documented interactive interface with memory/creating/binding/JavaScript/signal features. Recording and flush commands completed; the client exited 0 and produced a complete 70,290,494-byte XML trace.

The whole trace contains setup and 512,732 small-object allocations / 52,273,024 allocated bytes. It must not be treated as scrolling-only cost. A separate filtered input retains ranges wholly inside two sustained contentY-change clusters. This proxy excludes setup and some stationary endpoint time; it is not exact QPC alignment. The filtered windows contain 86,948 allocation events / 7,043,680 bytes and 10,008 Creating range events. Creating ranges are not synonymous with distinct created QObjects.

The largest binding/handler ranges include scrollbar position updates and preview renderer exchanges. A temporal allocation attribution proxy identifies iconPixelOffset, the group-header model expression, viewport intersection and shape readiness as further sites to investigate. It is not a C++ allocation stack. Details, source locations and interpretation are in [the standalone QML report](profile-report-combined-qml-scroll-2026-10-18.md).

## Production fixes

Masonry's horizontal scrollbar was invisible but its position binding still reevaluated with contentY: 1,068 evaluations / 55.42 inclusive ms in the filtered trace. Its position binding now short-circuits when invisible. A regression records seven hidden position-change signals before the change and zero afterward, verifies immediate position response when Columns becomes visible, and checks the binding matches the current viewport after settling.

The stricter resting check exposed an existing OpenGL thumb-origin error at physical x=380.333333. The visual thumb leaf now uses the existing cached PixelGridAlignment service for its complete scene-space translation. Resting dimensions use the shared grid context; fitting is disabled during scroll animation and dragging. No scene-wide enumeration or per-frame ancestor rounding pass is introduced. The track remains under the existing scope.

At DPR 1.75 both Direct3D and OpenGL pass ten results: hidden-update regression, thumb/track origins and dimensions/unit transforms, visible scrollbar interaction/layout checks, and wheel gesture/motion cases for all five modes. The new thumb check explicitly asserts fitting disables while scrolling and re-enables at rest. A rendered screenshot was inspected, including small Segoe UI captions. These tests establish removal of the unused updates and alignment correctness, not a sustained-160-Hz gain.

The normal portable host was rebuilt against the production buffer-fit QtQuick archive, not the combined experimental archive. Import isolation, system-DLL audit, generated payload/extraction/reuse/concurrency/corruption/lookup checks passed. The compressed payload and CGO_ENABLED=0 launcher were rebuilt and the worktree app restarted directly with --gui=qt. Only this worktree's process tree was stopped; the canonical binary was untouched. No commit/push was requested or performed.

## Next work and state

Constant 160 Hz without spikes remains unachieved. The combined upload policy stays experimental. Next investigate repeated managed allocations and exact GC phase durations in that combined engine, with source-level hooks rather than changing GC constants again. The group-header model is reread during scrolling even for empty groups; repeated point/URL wrappers are another concrete candidate. Both need regressions and allocation-counter verification before claiming a reduction.

Compact scheduler/QML summaries, scripts, focused test logs and the rendered capture are archived in profiler/experiments/combined-gui-attribution-2026-10-18. Raw ETW/CSV remains in artifacts/modes-profile; raw QML traces remain in profiler/traces and are not part of the checkpoint source set. Runtime identity is artifacts/scrollbar-final-normal-identity.json.

AI assistance has been used to create this output.
