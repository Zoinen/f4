# Active-only D3D11 snapshot copy: do not promote

This candidate remains artifact-only. It passes its native correctness checks
but has no repeatable end-to-end scrolling benefit. The normal app continues
to use the previously deployed QML signal-mask fix. The 160 Hz goal is unmet.

## Candidate and native evidence

The actual D3D11 setShaderResources recording path is changed to construct an
independent default snapshot in place and copy only finalized binding lists for
present shader stages. It omits QRhiBatchedBindings builder accumulators, which
the execution path does not read. All command indices and per-command dynamic
offsets remain independent. It uses the original SDK types and layouts; SDK
headers and archives are unchanged. No cache across commands or frames is used.

F4_PROFILE_DISABLE_ACTIVE_BATCH_COPY selects the stock retain operation inside
the same executable. This is a diagnostic switch, not a user setting.

The stock implementation passes 19 initial command checks and fails the new
ownership check: an otherwise released finished builder still shares a retained
resource array. The candidate passes that check. Expanded tests pass 21 checks,
including each uniform/sampler/UAV stage category, sparse and combined masks,
pool growth, independent snapshots after source mutation, reset and a throughput
benchmark. Disabling the copy change in the final executable reproduces the
ownership failure while the other 20 checks pass.

Actual D3D11 graphics readback checks every pixel of three offscreen frames with
independent vertex/fragment uniform offsets, alternating SRBs and textures.
Compute readback checks independent uniform offsets and UAV storage ranges.
Stock and candidate each pass four GPU tests, with no failures or skips.
The isolated host passes four import/host-loading tests and its system-DLL audit.

A single 1000-bind benchmark reports approximately .16 ms stock and .15 ms
candidate. That small synthetic difference is not sufficient evidence of an
application-level benefit.

## Matched rapid-wheel captures

Both run orders use the same candidate executable and link receipt, changing
only the diagnostic switch. All eight cold/warm case recordings pass focus,
160 Hz / DPR 1.75, input, geometry, phase-buffer, caption-measurement and motion
coverage checks. The folder has 187 entries and density 152. Other renderer,
texture and vector experiments are absent from the production-based harness.

Render-to-frame-end p99 in ms, stock -> candidate:

| Run order | Cold 0 | Cold 1 | Warm 0 | Warm 1 |
| --- | ---: | ---: | ---: | ---: |
| Stock then candidate | 3.0484 -> 3.2684 | 2.6889 -> 2.6115 | 2.9137 -> 3.0069 | 2.8960 -> 2.7569 |
| Candidate then stock | 3.0333 -> 3.3402 | 2.8023 -> 2.7524 | 2.8110 -> 2.9556 | 2.9538 -> 2.9220 |

Only four of eight blocks improve. The first pair has one counted displayed
miss in stock and three in the candidate; the reversed pair has none in either
arm. Motion errors still exceed the unchanged 2 px ceiling. Leave this change
out of production; correctness and a small microbenchmark gain do not justify
claiming smoother scrolling.

## Refined preparation attribution

The verified production CPU stacks were reclassified by their actual frame
phase instead of aggregating all render-thread instructions. Binding snapshot
copies appear during recording. The separate preparation phase contains all
194 cold and 168 warm isNodeBlocked samples, plus buffer-data assignment,
material preparation, bounds computation and batch work.

The slowest preparation frames in this sampled trace are 3.7626 ms cold and
4.7213 ms warm, with only four and five CPU samples respectively. They did not
reproduce the earlier 8.917 ms scheduler event. The warm frame includes two
Element::computeBounds samples and batch upload/unmap stacks; the cold frame
includes tagged-root rebuilding, sorting, decompression and driver work.
These sparse samples guide the next instrumentation, not an exact cost model.

The current Element::computeBounds implementation scans geometry vertices and
then maps the resulting local rectangle through the node matrix. Geometry and
matrix changes both invalidate its transformed bounds. Investigate whether
unchanged large geometry is being rescanned after transforms, measuring scans
and their duration before designing a local-bounds cache. Correct invalidation
must include geometry changes, replacement, removal and node lifetime.
No bounds cache has been implemented in this experiment.

Evidence: artifacts/D3DActiveBatchCopy.h,
artifacts/D3DActiveBatchCopyTest.cpp, artifacts/D3DActiveBatchGpuTest.cpp,
artifacts/active-copy-build-receipt.json,
artifacts/active-copy-pairs-analysis.json,
artifacts/production-preparation-stack-attribution.json, native test logs and
matching real-scroll-active-copy cases under artifacts/modes-profile.
