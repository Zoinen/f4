# Renderer preparation phases after vector retention

Date: 2026-10-21. Branch `zoin_branch/cached-pixel-grid`.
The original goal is constant 160 Hz without spikes. It remains unachieved.
No production implementation, SDK library, embedded payload or running normal
application was replaced in this investigation. This investigation narrows the remaining costs with per-phase timing.

## Gate constructor: a bounded cost, not the observed preparation tail

The previous CPU profile identified `F4VectorGateFrame` as a visible aggregate
cost, but a sample count is not a measurement of its duration within a particular
slow frame. A diagnostic now records six timestamps per constructor call,
covering registry enumeration, changed-batch membership validation, owner/current
map construction, warmed-batch pruning/comparison, and local cleanup. It performs
no per-node timing calls. Storage is bounded to 16384 records and exports only
completed records. Capture validation rejects overflow.

Five probe tests passed, including early returns, incomplete-scope exclusion,
completion publication, and overflow. The initial reset test found stale timestamps
when a slot was reused after resetting the counter. Clearing marks/counts on
reservation fixed that before any live input capture. The failing test log is
preserved alongside the final passing result. The source probe changes restore
to the original renderer when removed; no owner-cache prototype is present.

Cold and warm rapid-wheel and middle-scroll cases passed native 160 Hz, actual
DPR 1.75, Downloads path, 187 entries, density 152, bounded recorder, and focus
checks. Each case has two ten-second blocks. Input uses the existing 10 ms rapid
wheel and quarter-window-height middle displacement. Foreground changes are
recorded by a bounded WinEvent hook; no UI Automation polling, build or offline
analysis ran during input. Import-isolation tests (four passed) and the static
Windows DLL audit passed for each diagnostic host.

| Input / stage | Block | Gate median / p99 / max ms | Largest total preparation ms | Gate time in that frame ms |
| --- | ---: | --- | ---: | ---: |
| rapid / cold | 0 | 0.0518 / 0.1554 / 0.2071 | 4.8404 | 0.0640 |
| rapid / cold | 1 | 0.0527 / 0.1226 / 0.1902 | 2.3901 | 0.0742 |
| rapid / warm | 0 | 0.0516 / 0.1235 / 0.4122 | 3.3479 | 0.1192 |
| rapid / warm | 1 | 0.0563 / 0.1322 / 0.2374 | 3.3889 | 0.1469 |
| middle / cold | 0 | 0.0458 / 0.1221 / 0.1975 | 3.5904 | 0.1636 |
| middle / cold | 1 | 0.0472 / 0.1005 / 0.1437 | 1.8930 | 0.0533 |
| middle / warm | 0 | 0.0474 / 0.1112 / 0.1790 | 2.1820 | 0.1103 |
| middle / warm | 1 | 0.0474 / 0.1119 / 0.2007 | 2.2549 | 0.1827 |

The typical constructor takes about 0.05 ms. In the slowest rapid frame it took
0.0640 ms of 4.8404 ms preparation. Owner/current classification dominates this
small constructor cost, but optimizing it cannot explain or remove most of that
observed tail. The shared-owner hash cache remains rejected; a raw-pointer variant
was not implemented merely to improve this small subtotal.

`verify-gate-frame-stage-analysis.py` recomputes every phase statistic from the
raw records and checks it against `gate-frame-stages-analysis.json`; it passed.
Constructor rows are matched to their enclosing render-preparation span, and
incomplete constructor phases remain absent rather than being assigned durations.

## Surrounding renderer phases

A second diagnostic applies the existing reversible 13-mark preparation probe
to the current retention implementation. Its phases cover resource-batch creation,
render lists, cleanup, batching, sorting, gate setup, uploads/admission, opaque
and alpha material preparation, visualization, resource update, and final gate
filtering. Node-added/removed/geometry/blocked counters are recorded at the same
marks. The original renderer source hash is checked before modification, and
reversing all probe replacements must exactly restore that source.

The slowest cold rapid frame was 4.6261 ms preparation: uploads/admission took
2.7438 ms, alpha-material preparation 0.8845 ms, batching 0.4480 ms, and render
lists 0.2780 ms. It had 95 added nodes, 44 removed, two geometry changes and eight
blocked notifications. Gate setup was only 0.0515 ms. These are wall-clock phase
durations; the scheduler attribution from another capture must not be transferred
to this frame. In particular, this trace alone does not establish GPU waiting.

The slowest initial middle frame took 4.0473 ms: 2.5698 ms uploads/admission and
0.5288 ms alpha-material work. Another middle block reached 5.1107 ms with costs
spread among pre-body work, render lists, uploads, and alpha material preparation.
The spike mechanism is not confined to a single gate-constructor operation.

The rapid capture also differs from middle scrolling in full rebuild frequency:

| Input / stage | Block | Full rebuild requests observed at preparation entry |
| --- | ---: | ---: |
| rapid / cold | 0 | 15 |
| rapid / cold | 1 | 12 |
| rapid / warm | 0 | 21 |
| rapid / warm | 1 | 16 |
| middle / cold | 0 | 0 |
| middle / cold | 1 | 0 |
| middle / warm | 0 | 0 |
| middle / warm | 1 | 0 |

Qt's private enum defines FullRebuild as 0xffff. In the warm rapid blocks, the
largest preparation frames started with that flag and had node additions/removals.
The observation does **not** prove that a particular removal requested the rebuild,
or that all rebuilds can safely be avoided. The existing source has distinct full
rebuild sites for opacity, root funding, root removal, render-node depth behavior,
and material blending changes. The next step is a bounded event probe at those
sites, before considering a targeted fix. Do not suppress FullRebuild globally.

Cold uploads are a separate next target. The present phase groups ordinary and
pending vector uploads; it cannot identify which individual batch accounts for
2.74 ms. Previous upload-route evidence and this new source need to be correlated
in the same capture before changing admission/chunking behavior. Existing atomic
vector readiness and viewer priority contracts must remain intact.

## Evidence and status

- Constructor prefixes: `real-scroll-gate-stages-{rapid,middle}-20261021`.
- Overall prefixes: `real-scroll-retention-preparation-{rapid,middle}-20261021`.
- `gate-frame-stages-analysis.json` and `verify-gate-frame-stage-analysis.py`.
- `retention-preparation-{rapid,middle}-analysis.json`.
- `analyze-retention-preparation-probe.py`.
- Corresponding build receipts, stage headers, generated renderer/main/profile
  sources, capture scripts, focus logs, import results, dependency audits, and
  raw-data hash records are archived in
  `profiler/experiments/renderer-preparation-phases-2026-10-21`.

The deployed normal app still uses the verified retention host SHA-256
`c0133127a7060938dc42e0b7906eec2b6cb316855e865027539a7d088a79ffac`.
No cache experiment was promoted, no thresholds or animation clock were changed,
and no commit or push was performed. Displayed motion and the full 160 Hz goal
remain subject to the existing strict verification gates.

AI assistance has been used to create this output.
