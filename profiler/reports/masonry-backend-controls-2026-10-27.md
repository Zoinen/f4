# Backend controls: D3D12 and OpenGL do not remove cold spikes

The alternate-backend controls do not establish constant 160 Hz. D3D12 still
misses refreshes with default concurrency and full media support. OpenGL also
reports repeated display gaps, and its camera-to-present correlation is rejected
by the existing D3D timing bracket. No backend switch is promoted.

## Intervention and checks

All three arms use the same unchanged cold-profile host, without the earlier
RHI call wrappers, atlas-upload candidate or decode admission limit. The full
input/hash receipt is verified before each launch. The launcher selects only
QSG_RHI_BACKEND in its own child process; saved settings and the normal running
portable app are untouched. The Go launcher runs Qt directly with --gui=qt.

The native batch-renderer suite passes 44 checks on D3D12 at actual DPR 1.75.
An initial invocation lacked an explicit D3D12 assertion, so the test source was
copied into an artifact and extended to assert Direct3D12 where it already
checks other backend identities. That verified copy also passes 44 checks with
the D3D12 assertion enabled, and 44 with the existing OpenGL assertion enabled.
No production test source was changed. This is native batch/rendering coverage,
not a complete UI, viewer, media or cross-machine compatibility certification.

Each live packet independently confirms the actual graphicsApi enum: D3D11=4,
D3D12=8, OpenGL=3. A silent fallback invalidates the capture. The unchanged host's
previous import-isolation/compiled-host and system-DLL checks remain pinned by
its receipt; no new host build or release/packaging claim is made here.

## Cold captures

Order is D3D11, D3D12, OpenGL, one fresh process/cache per arm. All use the actual
Downloads folder (190 entries), density 152, a 2194 x 1186 logical window, four
default decode workers, original atlas behavior, 160 Hz and DPR 1.75. Each has
two ten-second rapid-wheel blocks. OS disk cache and Go metadata are not flushed.
Focus/input-count and recorder bounds pass. No builds/tests, UI Automation
observation or offline analysis run during input.

| API / Qt PID | Block | Display gaps >9.375 ms | p99 interval | Maximum interval |
| --- | ---: | ---: | ---: | ---: |
| D3D11 / 99628 | 0 | 5 | 6.5106 ms | 12.6637 ms |
| D3D11 / 99628 | 1 | 0 | 6.3228 ms | 6.4370 ms |
| D3D12 / 58952 | 0 | 8 | 6.6722 ms | 31.2424 ms |
| D3D12 / 58952 | 1 | 3 | 6.3525 ms | 12.5226 ms |
| OpenGL / 72772, display-only | 0 | 21 | 12.4891 ms | 62.4869 ms |
| OpenGL / 72772, display-only | 1 | 21 | 12.4611 ms | 12.5906 ms |

This single temporal sequence is not a randomized repeated benchmark and does
not support a causal ranking of backend performance. It does show that neither
alternate configuration demonstrated the required end state. There is no
favorable candidate to promote based on these samples.

D3D11 and D3D12 have zero coverage holds in these captures but still exceed the
two-physical-pixel motion ceiling: respective block maxima are 61.025/3.594 px
and 195.054/110.120 px. Diagnostic analysis publishing these results is not a
strict motion pass. Final preparation stores are 214, 185 and 188, respectively;
OpenGL still has 13 requests pending at capture finish. These counters are not
unique visible-file completion counts. The OpenGL preparation observer's
longest callback is 74.4 microseconds, a recording cost rather than frame budget.

## OpenGL observability limit

PresentMon labels all target OpenGL rows Runtime=Other, swapchain address zero,
and Composed: Copy with GPU GDI. The existing analyzer requires a presentation
inside the Qt renderEnd-to-swapped bracket; 385 of 3,155 tested native frames
have no matching presentation in that bracket. Representative events occur
roughly 0.7-1.4 ms after swapped. The motion analyzer correctly rejects the case.
It is not changed to select the nearest event, extend its timing bracket, or
ignore unmatched frames.

The OpenGL table rows above come from a separate target-PID display-only
analysis, using non-dropped events with both previous and current displays
inside the input window. They must not be described as verified camera motion
or independently validated physical scanout. A backend-specific presentation
observer would be required before claiming smoothness from OpenGL. The rejected
correlation and its sample timestamps are preserved as structured evidence.

The combined motion analyzer publishes the completed D3D cases before failing
on OpenGL. Its partial JSON contains no OpenGL result and is not a three-backend
motion success. The separate display-only summary explicitly records each
actual API and presentation mode.

## Decision and next investigation

Keep the production backend unchanged. These controls do not justify dropping
compatibility, changing animation clocks, relaxing presentation gates, or
serializing thumbnails. The prior same-capture symbolized stacks still locate
D3D11 waits in graphics allocation locks and NVIDIA critical sections; the
alternate controls do not explain their owner or show that those exact waits
also occurred under D3D12/OpenGL.

Next discriminate allocation/producer ownership and resource churn on D3D11,
including device/context identity for render-thread and offscreen decoder work.
Avoid another broad backend rollout without a measured candidate that preserves
thumbnail latency and the existing rendering contract.

Only the exact backend-diagnostic launcher tree was closed afterward. Normal
worktree launcher 43936 / Qt 119460 remain running. No production source,
dependency archive or payload changed, and no commit/push was made.

Evidence: `profiler/experiments/masonry-backend-controls-2026-10-27/`.

> AI assistance has been used to create this output.
