# Masonry: bounded raw-reference frame-owner memo

The 160-Hz goal remains unmet. This experiment is not promoted. Production
sources, the embedded payload and the preserved normal application were not
changed. No commit or push was performed.

## Control CPU evidence

The rotation-disabled control completes the CPU comparison requested by
`masonry-uniform-rotation-live-cpu-2026-10-28.md`:
`real-scroll-uniform-rotation-control-cpu-20261028-wheel-masonry`.
It passes capture gates with 1648 inputs, actual DPR 1.75 and 160 Hz. The trace
has zero lost events/buffers or schema errors. The updated stack reader decodes
18387 stacks and 17512 profiles. Its sampled-profile prefix is independently
verified against xperf: 472/472 reference records match, zero errors. The exact
host code and runtime-function-table symbol-map verification remains applicable.

Combined preparation self samples, rotation enabled / disabled:

| Function | Enabled | Disabled |
| --- | ---: | ---: |
| `QSGNodeUpdater::isNodeBlocked` | 431 | 395 |
| `F4VectorGateFrame` constructor | 232 | 214 |
| `Updater::visitNode` | 152 | 192 |
| `Renderer::updateMaterialDynamicData` | 148 | 144 |
| `QRhiBufferData::assign` | 106 | 124 |

Recording samples for `StageUniformBufferBatches` copy construction are 362 / 370;
`ResourceBatches` copy construction is 134 / 128. These are sampled counts, not
exact function durations or proof of a causal difference. They do not support
blaming the rotation prototype for these existing scene/binding costs. Rotation
remains experimental for the separate cadence reasons already recorded.

The earlier `preparation-cpu-and-frame-gate-lookups-2026-10-21.md` experiment had
mixed/worse warm results using an allocated QHash of shared owners. It was read
before implementing this different scratch representation. The older active
binding-copy experiment was also reviewed and is not being promoted on these
new aggregate counts.

## Candidate and lifetime boundary

`RawFrameGateOwners.h` memoizes nearest owner references only during one
`F4VectorGateFrame` constructor. It uses a fixed 1024-entry open-address table,
at most 16 probes per lookup/insertion, and a fixed 64-node scratch path.
Entries point to stable shared-owner values in the unchanged gate registry;
the memo owns neither nodes nor gates. No memo entry survives the constructor.
The registry and topology are immutable during this scope. Gate readiness fields
can still change as before. Full/colliding tables and deeper paths fall back to
the exact ancestor traversal. No persistent invalidation protocol is introduced.

The renderer's two classification loops use the memo; admission budgets,
readiness, ordering, clipping, upload policy, source loading, and animation
clocks are unchanged. The diagnostic runtime switch selects memo or reference
walk in the same executable. Bulk counters merge once per constructor rather
than adding atomic operations for each lookup. The control still uses this
instrumented helper, so it is not a byte-identical stock-production timing run.
Fixed scratch initialization and counter overhead must not be ignored when
interpreting absolute costs.

## Regression and host checks

- The bypass baseline fails four helper regressions (3 passed / 4 failed).
- Candidate helper: 7 passed, zero failure/skip. Covers nearest/nested owners,
  negative branches, reparenting between passes, gate replacement/removal,
  lifetime release, tiny-table saturation, 320-node paths, null input and siblings.
- Repeated-sibling registry searches: 421 candidate versus 8800 reference.
- Native renderer: 48 passed at actual DPR 1.75, zero failure/skip, including
  drawing order, blocked branches, vector admission and later publication.
- Shader/readback checks: 27 passed, zero failure/skip.
- Static import isolation and compiled-host QML loading: 4 passed.
- Static Windows dependency audit: passed.

The initial native invocation omitted required suite flags and produced two
failures / 42 skips; the initial shader invocation omitted the uniform-reuse
flag and failed its cleanup assertion after 26 passes. Both original logs are
retained separately. The correctly configured runs above are the passing evidence.
No source layout was changed, and existing actual-175% rendered regressions ran
against the candidate renderer. These checks do not establish scrolling smoothness.

## Reversed-order live runs

Downloads, 191 entries, Masonry density 152, actual 3840x2160 / 160 Hz / DPR 1.75,
two 10-second boundary-bouncing wheel blocks, nominal input interval 10 ms.
Each run starts with a separate empty gallery cache. Both arms use the same
diagnostic host and worker-prepared buffer/atlas stack, with uniform backing
rotation disabled. No build, offline analysis or UIA observation runs during input.

Prefix `real-scroll-raw-frame-gate-`, suffix `-20261029-wheel-masonry`:

| Arm | Inputs | Display misses, blocks 0 / 1 | Preparation p99 ms, 0 / 1 | Render-to-end p99 ms, 0 / 1 |
| --- | ---: | ---: | ---: | ---: |
| on | 1695 | 9 / 4 | 3.5383 / 3.7460 | 6.1692 / 6.2224 |
| off | 1703 | 4 / 6 | 3.3216 / 3.5588 | 5.3173 / 6.2006 |
| off-r2 | 1723 | 10 / 0 | 3.1557 / 2.4489 | 6.2137 / 4.2389 |
| on-r2 | 1727 | 5 / 8 | 3.5095 / 3.7480 | 5.9931 / 6.5829 |

All four individual capture guards pass: correct build/feature use, refresh/DPR,
focus, input count, source and bounded recorders. Every block fails the unchanged
maximum-two-physical-pixel motion-error gate. The apparent zero-miss off-r2 block
still has 4.0135 px maximum error and is not a passing 160-Hz result.

The memo reduces total registry searches from about 20.2/20.5 million to
4.36/4.25 million, about 4.7x. Enabled hit counts are 848514 / 826786. Around
3.17/3.05 million insertions fill almost all 1024 slots per constructor on average;
1.19/1.20 million insertion attempts fall back at the probe limit. These are
failed insertion attempts, not lost owners or failed image decodes. The table
bound trades registry walks for table probing/initialization; fewer registry
searches do not prove less total CPU work.

These are cold/loading-plus-reverse-scroll blocks, not guaranteed fully warmed
scenes. Final preparation activity still reports pending work in three arms
(18 / 29 / 0 / 3). The harness's `scrollExtent` variable, captured before input,
is 3497.5714 in the enabled arms and 3542.5714 in controls. It is not a final
geometry comparison. Decoder/source discovery and admission therefore differ
across runs; no exact same-layout causal speedup or slowdown is claimed. Two
control blocks also record coverage holds (5 / 2 frames), another failed goal
gate. All evidence is retained rather than selecting only favorable runs.

No consistent end-to-end benefit is established, so the candidate stays isolated.
Do not turn a high cache-hit rate or a single warm zero-miss block into a promotion.

## Next work and retained state

Return to same-frame phase/cause attribution of the large preparation spikes;
do not pursue registry-search counts alone as the explanation for 7–12 ms tails.
Separate cold UX captures from a verified fully warmed comparison, retain source
and geometry/loading-state evidence, and keep all existing fidelity/motion gates.
The older detailed preparation probes and their source anchors can guide a new
bounded current-source probe without suppressing rebuilds or vector admission.

Only the exact raw-frame diagnostic launcher tree was stopped after capture.
The normal launcher PID 43936 and extracted host PID 119460 remain alive. Build
receipts, sources, tests, control CPU analysis, capture data and derived metrics
are snapshotted; binaries and raw traces remain hashed external artifacts.
