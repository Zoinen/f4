# Masonry: isolate uniform-snapshot observer overhead

The uniform-snapshot observer can now be bypassed without removing any QRhi
upload or changing renderer decisions. The control is verified, but it does
**not** eliminate rare display misses. This is a diagnostic experiment, not a
product change or completion of the constant-160-Hz goal.

## Control and correctness

The preceding [scheduler/counter investigation](masonry-cache-counters-and-isolation-2026-10-31.md)
found statistical CPU samples inside `F4UniformSnapshot::update`. That function
belongs to the diagnostic observer: it records material names, backing-store
state and atomic counters before forwarding `updateDynamicBuffer`.

One host binary now has the switch `F4_PROFILE_SKIP_UNIFORM_SNAPSHOT`.
The default preserves the observer. Setting it to `1` calls the same
`batch->updateDynamicBuffer(buffer, offset, size, data)` operation directly.
Its arguments, order and copy semantics remain unchanged. No content hash, batching, resource reuse, shader,
geometry, text, vector, scrolling or layout policy was modified.

The added regression uses the Null QRhi backend to inspect three queued buffer
updates, including overlap and a large payload. It checks their order, target,
offsets and copied bytes after mutating the original source data. With the
observer disabled, the complete diagnostic snapshot must remain unchanged.

Verification on the same host object graph:

- Helper suite: 16 passed in each mode, no failures/skips.
- Native suite on the actual 175% desktop: 46 passed in each mode, no failures/skips.
- All 25 native rendered PNGs are byte-identical between modes. The small-caption
  and thin-stroke fixture was also inspected visually.
- Import isolation: 3 passed. The portable-host DLL dependency audit passed.
- The new diagnostic host is separately named and built in its own artifact
  directory. No production binary, Go protocol, installed Qt package or SDK
  archive was changed. This is not a rebuilt portable Go payload rollout.

## Same-host A/B/B/A captures

All four captures used the real `C:\Users\xs\Downloads`, all 192 entries,
Masonry density 152, 3840×2160 at 160 Hz and actual DPR 1.75. Each used a fresh
isolated cache and a verified all-entry warm tour, followed by two 30-second
quarter-window-height middle-scroll passes. Native foreground remained stable
through every measurement window. All other prototype flags were held fixed,
including typed scene discovery, frame latency 2 and native vector rendering.

No extra kernel/PMC trace or external-process affinity control ran during
these captures. PresentMon and the existing bounded in-host/foreground probes
remain; bypassing this observer does not imply a zero-overhead measurement.

The original gates remain: display intervals at most 9.375 ms, physical motion
error at most 2 px, native velocity error at most .002 px and no coverage holds.

| Run | Observer | Refresh misses by pass | Largest display interval, ms | Strict middle result |
| --- | --- | --- | --- | --- |
| A1 | enabled | 4 / 1 | 12.4888 | fail: displayed-motion continuity |
| B1 | disabled | 0 / 0 | 6.4767 | pass |
| B2 | disabled | 0 / 1 | 12.4861 | fail: displayed-motion continuity |
| A2 | enabled | 0 / 0 | 6.5528 | pass |

Both B1 and A2 pass all the unchanged middle checks. A1 and B2 fail the original
strict analyzer at nonconsecutive displayed-motion frames; no missing pair was
discarded to manufacture a motion pass. Their supplemental display-gap analysis
is diagnostic only. The enabled captures recorded about 1.03 million observer
calls each, across startup, preparation and measurement. The disabled captures
recorded zero calls, bytes, backing-store counters and material/histogram entries.

## Preparation timings

The values below are elapsed phase timings from all complete measured frames
in both passes, not sampled scheduled CPU time. There are 9,599 frames for A1
and 9,600 for each other run. All measured frames have the expected 13 internal
preparation markers; none was dropped from the upload-stage attribution.

| Run | Preparation median / p99, ms | Upload stage median / p99, ms | Preparation max, ms |
| --- | --- | --- | --- |
| A1 | .3565 / 1.7644 | .0185 / .8281 | 3.4040 |
| B1 | .3637 / 1.6589 | .0183 / .7624 | 3.0548 |
| B2 | .3553 / 1.6025 | .0183 / .6517 | 3.8203 |
| A2 | .3648 / 1.7097 | .0185 / .7754 | 2.8380 |

The disabled runs have lower preparation/upload p99 in this series, while
medians overlap and the largest preparation value occurs in B2. This is useful
for keeping future measurements cleaner; it is not evidence that uniform
observer cost caused every miss, or that its removal guarantees a stable
frame-time improvement. A2 also passes with the observer enabled. The sparse
miss counts do not establish a reliable miss-rate reduction.

## Next step and remaining acceptance

Use the observer-disabled mode for further renderer attribution. The preceding
trace also sampled the serial per-byte hash in shared-uniform content lookup.
That is a separate candidate for a bounded optimization retaining exact byte
equality and collision correctness; it was deliberately not changed in this
control. Compare it against this cleaner baseline rather than assigning
observer work to the product.

The earlier evidence of ready F4 frames missing compositor refreshes remains
unresolved. These clean captures have no scheduler trace and cannot establish
the scheduler/DWM cause of each individual miss. Cold wheel and verified-warm
wheel were not rerun here. Full workload acceptance, normal-window delivery,
motion, coverage, SVG quality and static physical-pixel alignment are still
required before claiming success.

## Preservation and final state

The [experiment manifest](../experiments/masonry-uniform-observer-control-2026-10-31/manifest.json)
contains compact sources, build/test receipts, strict results and hash receipts
for the preserved raw packets and presentation traces. D: raw duplicates were
retired only after verifying their C: copies. No unrelated data was deleted.

All four owned diagnostic app trees were stopped after analysis. Normal F4 Go
PID 23488 and Qt PID 84780 remain alive unchanged. No commit or push was made.
Future captures are authorized without another readiness question.

> AI assistance has been used to create this output.
