# Scalar cached icon alignment — 2026-10-18

## Change

PixelGridAlignment exposes scalar offsetX/offsetY getters backed by the same cached QPointF and offsetChanged notification. The existing point API remains unchanged. Gallery delegates retain the attached alignment QObject, returning null when inactive, rather than creating point values and zero points in iconPixelOffset. Translate bindings consume the primitive scalar properties. Details fallback previews, Columns, Grid, Icons, Masonry, raster thumbnails, vector thumbnails and the video badge use the same path. No additional alignment pass, ancestor walk, rounding, transform policy, or native object was introduced.

## Geometry and rendering verification

- physicalGridCachesLeavesAcrossMotionAndUnrelatedFrames now exercises scalar access on actual native Text, TextInput, TextEdit (including a small secondary line), and raster Image leaves. It compares scalar getters with the unchanged point getter, checks scene origins and unit vectors, exercises motion, ancestor changes, reparenting and hide/show, and asserts no per-leaf fitting work during shared-scope motion. Both D3D11 and OpenGL at actual DPR 1.75: 3 passed, 0 failed including init/cleanup. Capture pixel-grid-cached-leaves-175.png inspected, including small secondary text.
- All five wheel-burst modes, alignment-sleep/rest checks for Grid/Masonry, and Columns text/icon scene checks: 10 passed, 0 failed on both D3D11 and OpenGL at DPR 1.75. Logs: artifacts/scalar-icon-final-{d3d11,opengl}-175.txt. Actual leaf origins/unit vectors and resting captures checked; Columns three-column OpenGL capture inspected.
- The old Columns fixture failed on both point and scalar implementations. It tested animation flags before queued rewrap/polish started animations and used an unavailable legacy font. Baseline evidence: artifacts/scalar-icon-columns-baseline-d3d11-175.txt (e.g. icon physical x=29.5 and text y=23.75). The fixture now installs the same Segoe UI fixture used by other grid tests and lets queued work start before waiting for settlement. The corrected fixture passed the original point-based implementation (artifacts/scalar-icon-columns-settled-baseline-175.txt) and the scalar implementation. Alignment thresholds remain unchanged; no production geometry correction was added for this fixture issue.
- Import isolation: artifacts/scalar-icon-import-isolation.txt: 3 passed, 0 failed. Static Windows system-DLL audit: artifacts/scalar-icon-portable-audit.txt: passed after providing the MSVC dumpbin path. Embedded host materialization/payload checks: artifacts/scalar-icon-embedded-tests.txt: passed.

## QML allocation experiment

Before: cursor-allocation-qml-memory-2026-10-18.qtd. After: scalar-icon-qml-memory-2026-10-18.qtd (20,183,828 bytes, complete XML, profiler client exit 0). Normal renderer, Downloads with 184 entries, active 160 Hz window, DPR 1.75, Masonry density 152, default GC budget 5,000 us, actual wheel input every 30 ms.

Sustained contentY-change clusters exclude startup and stationary tails. They are an activity proxy, not exact QPC windows. Nested profiler intervals overlap; inclusive durations cannot be summed as wall-clock cost.

| Helper range | Before | After |
| --- | ---: | ---: |
| Calls | 2,725 | 2,574 |
| Inclusive time | 18.9243 ms | 3.3746 ms |
| Temporally attributed small allocations | 3,968 / 468,128 bytes | 1,090 / 104,640 bytes |

Allocation attribution is the earliest-ending active QML range proxy, not a native allocation stack. Total positive small-object events in the filtered captures were 99,040 / 7,799,104 bytes before and 94,809 / 7,376,128 after. These are observed capture totals, not an exact isolated memory-saving guarantee. Data: artifacts/scalar-icon-qml-comparison.json.

## Clean ABBA cadence experiment

No QML profiling client, UI Automation observer, sampler or compilation during these captures. A is the prior cursor-allocation normal diagnostic host; B is the scalar icon normal diagnostic host. Both use the normal renderer, not the experimental combined engine. Actual input is 30 ms wheel events, 267 events for A1 and 268 for B1/B2/A2. All captures confirm active window, Downloads/184 entries, DPR 1.75, 160 Hz, default GC budget 5,000 us. PresentMon display intervals include only intervals whose two endpoints lie inside the same scrolling-direction observation window.

| Run | Display p99 down / up ms | Display max down / up ms | Worst native frame period ms |
| --- | ---: | ---: | ---: |
| A1 | 6.3219 / 6.2852 | 18.7513 / 12.5006 | 16.1425 |
| B1 | 6.3841 / 6.3171 | 18.7541 / 12.4906 | 20.8088 |
| B2 | 6.3456 / 6.3506 | 18.8756 / 12.5042 | 17.6005 |
| A2 | 6.3593 / 6.3375 | 18.7535 / 12.4851 | 14.9549 |

No consistent cadence improvement is established; B also has larger worst native periods in these runs. The change reduces measured helper execution/allocation pressure but does not eliminate spikes. Do not claim steady 160 Hz or discard the unfavorable samples. PresentMon display timing is distinct from Qt frame periods and does not prove optical presentation timing.

Data: artifacts/scalar-icon-clean-comparison.json and artifacts/modes-profile/scalar-clean-*-wheel-masonry-present-window-summary.json. Next investigation should sample the remaining long frames directly, especially recurring downward-scroll stalls (both variants still exhibit approximately three refresh intervals for the worst display gap). Prior traces identified GC/delegate creation and initial vector uploads; fresh attribution is required after these changes before choosing the next fix.
