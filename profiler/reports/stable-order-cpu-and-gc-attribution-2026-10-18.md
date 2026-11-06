# Stable-order CPU attribution and validating GC experiments

## Progress and input-mode correction

The preceding turn created and tested a bounded-reservation prototype, saved
its native/clean measurements and rejected an unsupported default change. That
was progress. This turn separates remaining GUI/render work using CPU samples,
corrects trace alignment, adds input-kind validation, and verifies whether a GC
prototype actually reaches the live collector. The goal remains unfinished.

An audit found that the recent current-fit resource, fit-padding, bounded-orders,
and gc-grain captures have wheelIntervalMs=0 and no wheelEvents. Their suffix
was misleading; these are continuous linear-animation traversals, not wheel
input tests. The two reservation reports and their compact summaries now have
explicit corrections. Raw names/tables are preserved. This does not invalidate
the native tests or the observation of broad uploads during continuous
scrolling, but they cannot establish wheel-handler performance.

The earlier committed buffer-fit, icon-coalescing, GC-slice, GC-ownership and
backend A/B captures are separately audited at 30 ms and 267-268 actual wheel
events; those comparisons are unaffected. A runner guard now rejects a
wheel-named capture when F4_PROFILE_WHEEL_INTERVAL is missing/nonpositive.
Three guard regressions pass. Future captures must verify actual input metadata,
not infer it from filenames or environment changes in another shell process.

## QPC alignment correction

The first sampled capture's before/after wall/QPC relation drifts 1.842650 ms;
the actual old offset error, measured directly, is 0.186550 ms. Drift is not itself
a measured frame stall. A raw ETW reader matches 52428 context-switch identities
(new TID, old TID, CPU) in exactly the same sequence as the xperf export. At
10 MHz, offset spread across the complete trace is 900 ns, consistent with
microsecond export quantization. This replaces the uncertain wall-clock anchor.

A second capture verifies the result independently: 50481 matching switches,
900 ns spread. Its wall calibration drifts 0.941650 ms and old offset error is
0.254250 ms. Raw alignment is used by both sample and scheduling analysis.
Four alignment regressions cover quantization, missing/invalid input, identity/
CPU mismatch and variable offset rejection. An initial test-fixture arithmetic
error was corrected; the final four tests pass. The raw reader uses Microsoft's
[CSwitch schema](https://learn.microsoft.com/en-us/windows/win32/etw/cswitch).

Trace/display correlation includes only display intervals whose two endpoints
belong to the same direction window. Neighboring API intervals cannot cross the
pause between directions. Samples are inclusive counts, not milliseconds, and
scheduler running time is scheduled execution, not pure user-mode CPU time.

## Fresh actual-wheel evidence

bounded-real-sampled-wheel-masonry uses the bounded renderer, Downloads 184
entries, density 152, actual DPR 1.75 and active 160 Hz, default D3D11 and GC
budget 5000 us. It records 268 actual wheel events at 30 ms, not a linear
animation. Scene orders stay exactly 223640; caption measurements are zero.
The ETW trace has zero lost events/buffers. Six interior displayed intervals
exceed 9.375 ms; their values are 12.4982, 12.4988, 12.5006, 12.5013, 12.4985,
and 12.4826 ms. These are attribution results under sampling, not a new clean
performance headline.

- The worst native period is 14.590 ms. Its render wall is 9.2654 ms, with
  9.2634 ms scheduled execution and only 0.002 ms ready time. Render preparation
  is 8.2603 ms. Ten render-thread samples within that render interval include
  prepareRenderPass 9, uploadBatch 7, unmap/direct dynamic update 5 and memcpy 5.
  These nested counts cannot be summed. This supports aggregate upload/copy
  work as a remaining cost, rather than a long scheduler-ready delay.
- Another period is 14.0188 ms with GUI scheduled execution 12.9068 ms and
  render wall 2.4185 ms. A neighboring 12.8899 ms period has GUI execution
  12.0519 ms, with 7 GUI samples under GCStateMachine::transition. The preceding
  neighboring frame has 6 GC samples. These establish material GC activity,
  not a claim that every millisecond of the GUI interval is GC.
- Ready-delay p99 is 0.024 ms GUI and 0.009 ms render; maxima are 0.262/0.052 ms
  for this capture. Raising thread priority is not supported by these results.
  Wait reasons include normal frame synchronization and must not all be called
  external scheduling interference. Two other gaps still need more detailed
  GUI attribution; the capture does not explain every gap with one cause.

The initial bounded-sampled-present capture is explicitly labeled continuous
animation in sampled-summary.json. It also shows real GUI/GC and upload costs,
but is not substituted for the actual-wheel evidence above. Some private C++
code resolves to shared/folded template symbol names; an apparent QHash name is
not treated as proof of a particular hash algorithm bottleneck.

## Timed GC drain prototype and why its live comparison is inconclusive

Official Qt 6.11.1 qv4mm.cpp sets markLoopIterationCount=1024 and the timed drain
loop checks its deadline after i<=markLoopIterationCount*10, up to 10241 pops.
The intended experiment reduces this chunk to 128 while leaving other GC logic
unchanged. It is more specific than changing the time budget alone.

A native synthetic test queues the same valid V4 heap object 4096 times after
warming its references. With an already-expired deadline, original code returns
Complete after all 4096; the replacement returns Ongoing after 128. Both pass a
separate real 50000-object JavaScript allocation/retained-string collection
check. This proves direct-call yield behavior, not full GC correctness or UI
performance. The fixture initially assumed an existing collector mark stack;
that null-stack error was corrected by owning a local test stack. Shared graph
references were then warmed so net queue reduction measures the intended pops.

An A/B/B/A with actual metadata GC budget 1000 us on both builds was captured,
but must not be interpreted as a valid changed-collector comparison. It also
used continuous animation and is labeled as such. The replacement was linked
through /FORCE:MULTIPLE; this only proves external symbol selection.

A live observer with 268 actual wheel events records replacement-call ranges
0->0 in BOTH directions. Inspection of the original Qt6Qml qv4mm MSVC bigobj
then proves inlining: internal markDrain is section 181, 231 bytes, with the
original cmp edi,2800h at byte offset 177, followed by QDeadlineTimer::hasExpired.
There is no relocation/call to the replaceable timed MarkStack::drain symbol.
The old loop is inside the caller itself. Thus the standalone override cannot
test the proposed collector change. The captures are preserved as inconclusive,
not reported as a failed optimization or a performance improvement.

A valid next experiment must rebuild the matching QtQml memory-manager
translation unit so its internal callers change, verify generated code and live
engagement, then compare actual wheel input at the same time budget. The proposed
source delta, direct-call test, live counter and inline proof are archived.
No private symbol override or GC policy enters the normal application.

## Saved evidence and restored state

Sources, compact sampled/scheduling summaries, clock matching, native test
results, input guards and inconclusive trial summaries are saved in
profiler/experiments/stable-order-gc-attribution-2026-10-18. Raw ETL/CSV traces,
objects, disassembly, PDBs and binaries stay outside Git under artifacts.
The tracked production source remains unchanged; no commit or push is made.

The panel state is restored and WPR is no longer recording. Only this worktree's
process tree is replaced. Its normal portable launcher starts directly with
--gui=qt after clearing host, GC, renderer, scale and reservation overrides.
The extracted child matches the normal host build, title identifies the branch
and stderr is empty. Canonical D:/Code/f4/f4.exe is untouched. Identity evidence:
artifacts/stable-order-gc-normal-identity.json.

- Host SHA256: 549f063f2f8e3252f5784f0b017d6500b719102ff799fb49b279964e44383bed
- Launcher SHA256: b9c479436f6fbc954f0eabccf67594844a1e35fd962a3c7eab40eb6e4e7f9d58

Constant 160 Hz without spikes is not achieved. The next actions are now grounded:
correctly rebuild/test the live GC deadline loop and reduce aggregate cold
geometry upload/copy work while preserving complete SVG presentation and bounded
source-to-visible latency. Neither a near-6.25 ms p99 nor an external override's
standalone test is accepted as completion.
