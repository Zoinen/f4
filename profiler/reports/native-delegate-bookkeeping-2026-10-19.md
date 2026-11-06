# Native delegate bookkeeping — 2026-10-19

Goal: constant 160 Hz without spikes. The goal remains incomplete.

## Change and rationale

Retirement now reads the existing GalleryDelegatePool component-key map
instead of looking up `_zoinGalleryDelegateComponent` on each QML delegate.
Creation no longer publishes that private dynamic property. The vector LRU
stamp is a native quint64 on GalleryDelegateItem; its reads/writes no longer
use QVariant or emit `_zoinGalleryVectorSceneUse` property-change events.
The stamp survives slot recycling just as the old property did.

The earlier detailed probe measured roughly 100,000 component checks per
eight-second scroll pass, totaling 90–138 ms inclusive of its timing
observer. This change removes those string/metaobject/variant operations.
It does not establish a proportional CPU or frame-time improvement.

Component compatibility, the 16-vector retention limit, scene-space clip
checks, ordering, retirement debt limits, spare limits and GC settings remain
the same. There are no QML layout, alignment, wire-protocol or settings changes.

ZoinGalleryCore also declares its existing QuickControls2Impl library
dependency explicitly, alongside the private-header target. A standalone
pool test exposed unresolved QQuickIconImage methods without that library;
the QML host previously obtained it through scanned QML dependencies.

## Regression and build evidence

The real SVG scrolling/settling regression installs a dynamic-property
observer before panel construction. The original implementation fails with
**104 component events and 90 vector-use events**. The changed implementation
passes the zero-event assertion, while retaining bounded clipped SVG scenes
and settling its nearby-preparation timer. The native pool regression checks
cross-component isolation, release/reacquire, reset/adopt, zero initial stamp,
stamp preservation and final trimming.

Direct3D and OpenGL each pass QtTest totals of 27 layout/pool tests, 15 mode
tests at actual DPR 1.75, and 3 static QML import tests. The mode selection
covers switching all five modes, native retirement, SVG/raster isolation,
sentinel states, motion/rest alignment, Columns text leaves, and Masonry
icon leaf origins/unit transforms/raster pixels. Captured Columns captions
and extensions and the Masonry icon/captions were visually inspected.

The first standalone test link failed on the undeclared icon implementation
dependency. After declaring it, a manual relink still used an old response
file because only the test object had been requested from Ninja. Building
the executable target regenerates that response file; the final build and
tests pass. All three build logs are preserved.

## Clean display comparison

Fresh-process A1/B1/B2/A2/A3/B3 order: A is the preceding implementation, B
adds native bookkeeping and its link dependency. Downloads has 186 entries,
Masonry density 152, actual display 160 Hz, DPR 1.75. Each run uses two
four-second moving blocks, 120-unit wheel events every 30 ms, boundary bounce,
and the original 5 ms GC budget. Caption measurements/model/snapshot signals
are zero. Both display endpoints must fall inside the input window. All runs
are retained. No build/test/WPR/QML-profiler/UIA/census work overlaps captures.

After A1 completed, validation requested `primaryScreenHz`, a field emitted
only by the incubation-specific build. The clean build has actual `screenHz`
instead. The validator was corrected and resumed with the existing A1 raw
capture; A1 was neither discarded nor overwritten. All normal workload and
display checks were applied before continuing the other five planned runs.

| Run | Block 0 p99 / max ms | >9.375 ms | Block 1 p99 / max ms | >9.375 ms |
| --- | ---: | ---: | ---: | ---: |
| A1 | 6.4001 / 12.6015 | 4 | 6.3204 / 12.5054 | 1 |
| B1 | 6.3332 / 12.5127 | 3 | 6.2927 / 12.4970 | 1 |
| B2 | 6.2979 / 12.4923 | 2 | 6.2747 / 12.4840 | 1 |
| A2 | 6.3072 / 12.4967 | 2 | 6.3024 / 12.4959 | 1 |
| A3 | 6.3020 / 12.5085 | 2 | 6.3263 / 12.5075 | 2 |
| B3 | 6.3001 / 12.5125 | 2 | 6.3007 / 12.4954 | 1 |

Across 24 seconds of input per version, A has 12 long displayed intervals and B has 10. This small difference is not evidence of a reliable cadence improvement. Both versions still miss refreshes; typical missed-refresh intervals are approximately 12.5 ms. The source and regression prove removal of unnecessary bookkeeping; constant 160 Hz is not achieved.

## Rollout and next work

The Windows system-DLL audit and installed-tree-independent import tests pass.
The host payload is regenerated and embedded in a CGO_ENABLED=0 Go launcher
using the system Go cache. The extraction suite covers payload agreement,
first extraction/reuse, concurrent extraction, corrupt payload rejection,
development-path precedence and changed-payload cache separation. The normal
worktree launcher is restarted directly with --gui=qt and the matching cached
Qt child is verified against the rebuilt host. Startup stderr is empty.

Host SHA-256: `f88bd5d5b2fbc21e13099db3d8c98b9eb16938579ffcea8e14c6c44bb5483f12`.
Launcher SHA-256: `c131c68cfb01cb6395ebadeb74c7e7cf6f99c129152c08a9b2ad2be1ab090078`.

Next reattribute remaining misses on this implementation, particularly
release-triggered GC and pixel-grid re-registration. Diagnostic core sources
must be regenerated from the current commit implementation: the old copied
GuiPreparationCommitProbe.cpp still uses the removed component property and
must not be linked against current creation code or the changed item layout.
Do not change prefetch limits or claim an incubation-budget fix from these
results. No commit or push was performed.

Compact source, tests, captures, summaries, tooling and hashes are archived at
`profiler/experiments/native-delegate-bookkeeping-2026-10-19/`. Executables,
libraries, raw ETL/CSV and build outputs are excluded.

AI assistance has been used to create this output.
