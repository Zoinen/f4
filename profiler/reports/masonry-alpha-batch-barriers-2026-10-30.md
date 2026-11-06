# Masonry alpha-batch barriers

## Finding

Qt's alpha batching is stopping primarily to preserve drawing order across
overlapping geometry, rather than because captions have separate clips or
incompatible glyph caches. In the two measured windows, 2624 of 2703 sampled
text-batch searches (97.08%) stop on overlap; the other 79 stop at a different
batch root. Of the overlap blockers, 1513 use QSGSmoothColorMaterial and 1111
use QSGRhiShaderEffectMaterial. All overlap blockers share the seed's root.

Those material classes identify rounded-color and shader geometry, not an
exact QML objectName. The source contains caption backgrounds and thumbnail
shaders in these branches, but this probe does not independently map each SG
node to its owning QML item. Do not claim an exact item identity from material
class alone.

Removing the overlap check would be incorrect: Qt is considering moving the
next compatible caption ahead of intervening content, which can cover it.
Global caption reparenting is also not established safe. The current layout
explicitly preserves row order for captions extending above thin panoramas.
No drawing order or scene hierarchy was changed.

The next optimization should either group only geometry proven independent
without changing overlap order, or reduce repeated state/upload work while
keeping the draw sequence. Sharing identical native-text uniform data is a
candidate requiring a content census and explicit frame/resource-lifetime
tests; shared glyph cache and color alone do not prove identical uniform bytes.

## Probe and checks

Diagnostic host based on the preceding batch-membership observer and the
unchanged Qt 6.11.1 alpha-batching algorithm. Every sixteenth new native-text
batch search records its stop reason, scan/join counts, first failed
compatibility fields and the actual intervening element found by the original
`checkOverlap()` walk. It does not add another overlap walk or alter the
decision. The first-failure classifier repeats comparisons only for sampled
incompatible candidates. This is intrusive attribution, not a performance A/B.

The 8192-slot table is touched before startup; records publish atomically after
their scope completes. It copies identities and computed bounds, never retains
nodes for later dereference. Render-node boundaries do not read uncomputed
bounds. Fixed capacity returns before reading arguments. Uniform update,
ordering, geometry and resource synchronization paths retain their behavior.

Eleven helper QtTest results pass, including scope publication/restoration,
capacity guard and first-failure classification. All 44 native renderer
results pass, zero skips, at actual DPR 1.75 with D3D11. All nine saved native
render images exactly match the preceding observer by SHA-256. Three static
import-isolation results and the Windows system-DLL audit pass. No portable
embedded payload or production renderer is promoted.

## Capture validity and limitations

Case: `real-scroll-alphabarrier-a1-20261030-wheel-masonry`.
Actual Downloads, 192 entries, density 152, 3840×2160, 160 Hz/DPR 1.75,
fresh process and empty isolated cache, two ten-second wheel windows. Default
frame latency 2; sparse strategy disabled. The second direction is not claimed
to be verified warm. Original capture-validity checks pass. All 1705 wheel
events are active; native foreground ownership is stable across both windows.
No screenshots/accessibility polling occur during gestures. The display hold
is released and only the owned diagnostic launcher tree is stopped afterward;
the normal worktree application remains running.

Whole capture: 46389 text seeds, 2900 published samples, zero capacity overflow.
The measured windows contain 1385 / 1318 samples; each timestamp is on the
recorded render thread inside preparation. These samples describe newly built
batches, whereas the preceding uniform observer samples later draw preparation
including retained batches. Their denominators must not be interchanged.
Deterministic sampling may correlate with scene order. Counts do not measure
CPU time or establish the maximum possible whole-view speedup.

| Sampled text-batch searches | First window | Second window |
| --- | ---: | ---: |
| Samples | 1385 | 1318 |
| Stop on overlap | 1342 | 1282 |
| Stop at different root | 43 | 36 |
| Finish with one member | 1372 | 1313 |
| Overlap blocker: SmoothColor | 844 | 669 |
| Overlap blocker: ShaderEffect | 498 | 613 |
| Elements visited by original overlap walk | 2823 | 2517 |

Sampled incompatibility checks first fail on drawing mode 3032 / 2978 times,
or material comparison 272 / 202 times. No sampled first failure is clip,
attribute layout, index type, opacity, material type or view count. Multiple
incompatible candidates can occur before one terminating overlap, so these
counts are not mutually exclusive batch outcomes.

## Performance gates remain unmet

| Metric | First window | Second window |
| --- | ---: | ---: |
| Display intervals above 9.375 ms | 16 | 39 |
| Maximum display interval, ms | 12.5339 | 12.9980 |
| Maximum motion error, physical px | 110.3124 | 113.2375 |
| Coverage holds | 0 | 0 |
| Preparation median, ms | 0.5436 | 0.5424 |
| Preparation maximum, ms | 7.2672 | 5.9791 |

These are observations of an instrumented run, not evidence of improvement.
Constant 160 Hz has not been achieved. No commit or push is performed.

The D: drive filled after the capture and analysis completed. An attempted
NTFS compression of diagnostic link outputs reclaimed no space. A hash-verified
duplicate `alpha-barrier-20261030/host.exe` was then removed; the identical,
audited live-host executable remains at
`artifacts/f4-qt-host-alpha-barrier-20261030.exe`. Sources, raw capture, tests,
logs and cleanup receipt are retained. This cleanup does not change executable
contents or the running normal application.

To allow further builds, three inactive diagnostic intermediate directories
(`uniform-material-20261030`, `batch-membership-20261030`, and
`alpha-barrier-20261030`) were archived under
`C:/Users/xs/AppData/Local/Temp/f4-profile-build-archive-20261030/`.
All 56 / 57 / 58 files were checked by SHA-256 before their old copies were
removed. Directory junctions preserve the original paths and contents;
the live-host executables and raw captures remain at their original locations.
The relocation receipt and script are included in the evidence snapshot.
The first archival attempt stopped before any source rename because Windows
PowerShell lacks `Path.GetRelativePath`; the corrected run verified the
existing copy before proceeding. About 746 MB was free after relocation.

Evidence: `profiler/experiments/masonry-alpha-batch-barriers-2026-10-30/`.

> AI assistance has been used to create this output.
