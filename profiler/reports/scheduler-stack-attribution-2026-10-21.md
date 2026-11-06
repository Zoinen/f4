# Scheduler and stack attribution for Masonry

The user authorized unattended future captures on 2026-10-21. Further captures
do not require another readiness question. Focus, DPR, refresh rate, recorder
bounds and trace-integrity gates still apply.

## Evidence and decoder validation

Accepted basic scheduler captures use real-scroll-pass-outer-scheduler, and
sampled CPU/context-switch stack captures use real-scroll-pass-stacks. Both
retain 160 Hz, DPR 1.75 and focus through the cold/warm input windows. Their ETW
readers report zero lost events, lost buffers, schema errors or trace-processing
errors. Stack collection adds substantial observation work; stack-run frame
timings are attribution data, not a matched performance comparison.

The stack reader validates the published StackWalk prefix with TDH. The current
Windows build does not expose SampledProfile v2 metadata through TDH. Its
documented 64-bit layout is therefore used only with independent xperf checks:
318 cold and 253 warm reference samples match thread ID, instruction address
and timestamp, with no mismatches in those reference slices. The output keeps
the fallback marker rather than presenting it as a successful TDH lookup.
This validates the exercised format, not arbitrary future schemas.

References: [StackWalk event format](https://learn.microsoft.com/en-us/windows/win32/etw/stackwalk-event),
[sampled-profile format](https://learn.microsoft.com/en-us/windows/win32/etw/sampledprofile).

A separately linked map has byte-identical executable code (42,099,242 bytes)
and runtime-function table to the captured host. Native symbols are resolved
with ASLR module bases from the same trace. Chained x64 unwind entries are
followed to their primary procedure; unrelated nearest symbols are not used to
fill gaps. See [Microsoft's chained-unwind description](https://learn.microsoft.com/en-us/cpp/build/exception-handling-x64?view=msvc-170#chained-unwind-info-structures).

## Resource wait identified

The basic trace's largest cold outer begin-pass scope is 0.4633 ms: 0.4273 ms
blocked, 0.0087 ms runnable and 0.0273 ms scheduled. Its wait reason is 27,
WrResource. These are scheduling states, not a decomposition of GPU time.
[Microsoft wait-reason definitions](https://learn.microsoft.com/en-us/dotnet/api/microsoft.windows.eventtracing.cpu.kernelwaitreason?view=trace-processor-dotnet-1.0)
identify the numeric reason; the number alone does not identify the resource.

In these captures the relevant stacks are associated with switch-in, not the
outgoing switch timestamp. The analysis pairs a blocked interval with the first
switch-in after its ReadyThread event and labels the stack source explicitly.
The largest cold stack-capture wait is 0.9725 ms inside a 1.0963 ms pass. The
resume stack resolves through:

QSG renderer beginRenderPass -> QMetaObject activation -> QQmlData::signalEmitted
-> QCoreApplication::postEvent -> QEventDispatcherWin32::wakeUp -> user32/win32k.

This identifies queued QML signal forwarding and its native dispatcher wake-up
as the captured wait path. It does not prove that the earlier, separately
captured 15.9453 ms scope had the same cause. That older event has no stacks.
The initial outgoing-timestamp join found no stacks and was corrected before
attribution; missing stacks were not invented or matched by proximity.

## CPU work remains

Render-thread CPU samples in the input windows total 8,234 cold and 6,812 warm;
two and one respectively lack matching stack fragments. Repeated self/inclusive
hotspots include D3D11 shader-resource batch copying/destruction,
QSGNodeUpdater::isNodeBlocked, material dynamic-data preparation and binding.
The large preparation samples are mostly scheduled CPU time; the isolated
kernel wake-up wait is a separate issue.

An unrelated build_release.py process was present in a read-only process
inventory. It was not terminated. Background work and stack-collection overhead
must not be mistaken for application work or used as an exact overhead correction.

Sources and outputs: artifacts/analyze-pass-outer-scheduler.py,
artifacts/pass-outer-scheduler-attribution.json, artifacts/StackTraceReader.cpp,
artifacts/verify-stack-profile-prefix.py,
artifacts/stack-profile-prefix-verification.json,
artifacts/analyze-pass-stack-symbols.py and
artifacts/pass-stack-symbol-attribution.json. Raw compressed ETLs, metadata,
native snapshots and filtered stack records remain under artifacts/modes-profile.
