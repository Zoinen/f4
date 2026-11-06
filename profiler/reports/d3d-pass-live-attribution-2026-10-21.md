# Live render-pass attribution and remaining scheduling gap

Three accepted diagnostic runs completed at 160 Hz and DPR 1.75, retaining focus
through cold and warm inputs: detailed begin-pass probe enabled, same-executable
probe disabled, and a separately linked host with an outer-scope CPU counter.
Both texture candidates were disabled. All recorders remained bounded.

## Detailed probe

In the measured windows the native D3D beginPass always used target type 5,
with no resource update batch and no command-storage growth. Its maximum wall
time by cold/warm block was 0.0420, 0.0445, 0.0199 and 0.0208 ms. This excludes
command-list growth as the cause of a multi-millisecond delay in those observed
passes, not in every possible application state.

The renderer recording callback maxima were 0.1522, 0.2057, 0.3240 and 0.2986 ms.
One native beginPass and one callback record fit within each outer renderer pass;
thread IDs, CPU counter validity and containment checks pass for every measured
record. The warm packet is cumulative, so each analysis filters the actual warm
measurement windows instead of recounting cold samples.

Typical outer-pass median wall time was approximately 0.0073-0.0075 ms with the
detailed probe, versus 0.0035-0.0043 ms with it disabled. This temporal comparison
is consistent with several microseconds of probe overhead, but is not an exact
calibration or an overhead correction for unrelated tails.

The old long stall did not recur in the enabled run. A 15.9453 ms outer-pass
scope did recur in disabled cold block 0, within a frame whose preparation was
16.6019 ms and producer interval 25.1606 ms. Batch preparation in that frame was
only 0.3829 ms. The disabled arm has no inner CPU counters, so the cause of that
specific long outer scope remains unidentified. Do not infer GPU blocking from
its wall time alone. The disabled cold block also has 16 long displayed intervals
and fails strict motion. Enabled first blocks also fail the 2 px displayed-motion
ceiling despite zero counted >9.375 ms display intervals.

## Outer CPU measurement

A new artifact-only host adds CPU-cycle recording to existing native scope 7,
even when detailed RHI and callback probes are disabled. All other renderer and
application sources, including the native RHI object, are reused byte-for-byte.
Original comparison and detailed-probe receipts remain valid.

The instrumentation regression fails before the change because cpuValid is false
(2 passed, 1 expected failure) and passes afterward (3 passed, no failures or
skips). Embedded import isolation and compiled-host QML loading pass (4 tests).
The Windows dependency audit passes. The normal application is untouched.

The outer-CPU host's live cold/warm captures pass the new validity check requiring
every scope-7 record to contain a valid positive CPU-cycle count. Across the
cumulative warm packet, the largest outer-pass wall time is 0.8992 ms with
159,120 CPU cycles; the cold maximum is 0.5572 ms with 99,280 cycles. The previous
16 ms stall did not reproduce. Raw cycles are not converted to elapsed time.

The outer run still fails strict displayed-motion requirements: cold block 0 has
one >9.375 ms displayed interval and 17.5932 px maximum displayed-step error.
Its other three blocks have zero counted misses and maximum error below 0.689 px.
This is useful attribution evidence, not constant-160-Hz verification.

Next evidence should correlate thread scheduling with the affected scopes.
CPU cycles and wall-time distributions alone do not identify whether a delay is
blocking, preemption, interrupts or a longer code path. No speculative renderer
optimization has been deployed based on this gap.

Sources and outputs: artifacts/analyze-d3d-pass.py,
artifacts/d3d-pass-attribution.json, artifacts/d3d-pass-phases.json,
artifacts/d3d-pass-motion.json, artifacts/pass-outer-phases.json,
artifacts/pass-outer-motion.json, artifacts/build-pass-outer-host.py,
artifacts/PassOuterCpuTest.cpp and their build receipts/logs. Accepted per-case
inputs, screenshots and PresentMon data use prefixes real-scroll-d3d-pass- and
real-scroll-pass-outer- under artifacts/modes-profile.
