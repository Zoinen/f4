# Early SVG clip/branch provisioning: rejected experiment

## Hypothesis and regressions

The previous trace associated remaining full rebuilds with clip insertion.
The experiment provisions a Masonry SVG preview's clip and vector scene branch
while its source is still loading, retaining both through loading-to-ready.
Definitive failures restore an unclipped ordinary file icon. Local catalog
snapshots supply the same vectorState field already used by external catalogs.

Before the implementation, the new pending-preview assertion fails in Masonry,
and the native branch test shows the row moving to galleryMasonryRasterBranch
while pending. Afterward both tests pass at DPR 1.75, including the existing
rendered/leaf-alignment checks; the pending-to-ready transitions produce no
clipChanged or parentChanged signals. A local malformed-SVG fixture also checks
failed and ready snapshot states and passes.

These checks establish the proposed transitions, not a scrolling improvement.

## Corrected harness

The artifact-only wheel harness now sends its first wheel event immediately.
It no longer repositions contentY to an old initial extent before the return
sweep. This removes the previously identified artificial jump and 30 ms idle
boundary. It still records all frames during each sweep; it does not discard
slow movement frames or suppress visible SVG work. The JSON now records actual
renderer drawing-order count. Analysis explicitly reads JSON as UTF-8.

## Controlled capture

Two fresh-process hosts use the same corrected harness, Qt opacity guard,
current icon/fallback changes and normal four-order reservation. Only early
clip/branch provisioning differs. The baseline build temporarily removes the
owned provisioning hunks and restores them in a finally block. Existing icon
fallback changes are preserved.

Both captures confirm Downloads, 184 entries, density 152, initial extent
4161.142857, active window, actual DPR 1.75 and 160 Hz. Wheel delta is 120 every
30 ms, four seconds per direction. Detailed cause instrumentation is enabled,
so wall timings include observer overhead; no UI Automation or CPU sampler runs.

Artifacts:

- artifacts/modes-profile/early-roots-baseline160-wheel-masonry.json
- artifacts/modes-profile/early-vector-roots160-wheel-masonry.json
- Their PresentMon CSVs and present-summary.json files
- artifacts/modes-profile/early-vector-roots160-comparison.txt
- artifacts/early-vector-roots-prototype.patch

| Metric | Baseline | Candidate |
|---|---:|---:|
| Complete rebuild frames | 15 | 18 |
| Frames first triggered by clip insertion | 7 | 7 |
| Frames first triggered by exhausted reservations | 8 | 11 |
| Unmap vertex/index upload bytes | 204,336,108 | 218,653,428 |
| Buffer creations | 9,509 | 10,103 |
| Maximum drawing-order count | 2,538 | 2,638 |
| Down display p99 | 18.7543 ms | 18.7487 ms |
| Up display p99 | 6.3504 ms | 6.3138 ms |

Each down sweep has 28 display intervals over 9.375 ms. The candidate's smaller
single maximum gap does not establish an improvement: rebuilds and upload work
increase and the display p99 remains effectively unchanged. This is one pair,
not a distribution of repeated trials; the regression in work counters and
unchanged clip-triggered frame count are sufficient to reject this mechanism.

## Outcome and next evidence needed

All provisioning product changes and their temporary behavioral tests are
reverted. The previous Qt opacity guard, existing icon/fallback work and normal
portable executable are preserved. Prototype sources and captures remain local.

Inspecting fifteen recently prepared scene files finds no literal clip property.
That does not identify which Qt-created clip nodes caused the trace, nor prove
all authored SVG clipping is absent. Before another lifecycle change, map the
actual inserted clip nodes to their QQuickItem owners, clip rectangles and
readiness transitions, rather than infer ownership from a shared ancestor.
The metadata runner also publishes vector dimensions only after full scene
source preparation; whether that delays physical clip creation needs a trace.

## Follow-up: actual clip ownership

The artifact-only observer now maps QQuickItemPrivate::clipNode directly and
refreshes ownership after synchronization, when newly created nodes exist.
The accessor is read-only. The capture
artifacts/modes-profile/clip-owners160-wheel-masonry.json and its summary
clip-owners160-summary.txt identify the inserted clips as GalleryEntryPreview,
not an unidentified shared ancestor. Examples include rows 96–100 with clips
around 154 by 154 logical pixels. Rows 118 and 137 also show remove/add pairs
under the raster and vector branches; row 137's final clip is approximately
1057.71 by 11.43 logical pixels.

This lookup traverses the entire item tree after each synchronization. It has
substantial overhead (25.56 ms median native interval in this run) and is only
an ownership probe. Its frame times must not be used as production performance.
There is no evidence here that generated SVG scenes introduced those particular
clips.

The distinction for the next implementation is physical scene lifetime:
unchanged QML clip/parent properties do not prove that their QSG nodes survive.
The existing native drawing gate removes a blocked row's renderer subtree,
including its clip; unblocking can reinsert that clip without clipChanged.
The successful property-level prototype checks therefore did not establish
the lifecycle needed to avoid Qt's full rebuild.

Investigate keeping a bounded set of SVG clipping roots outside the culled
drawing subtree, while leaving raster tiles on their existing batched path.
Any implementation needs native node-lifetime and rendered clipping tests,
viewport/prefetch bounds and a reservation/depth-range check. This is a proposed
next mechanism, not an implemented or verified optimization.

Constant 160 Hz remains unfinished. The corrected harness can now support that
next investigation without the artificial return-scroll reposition.

## Restored build validation

The restored source passes 26 focused vector checks at actual DPR 1.75 and four
import/startup fixture/test results. Windows system-DLL and tagged embedded-host
extraction/reuse gates also pass. An initial restore run linked a stale prototype
test object because a separate target's link-response failure stopped Ninja
before that object compiled. Its two obsolete prototype-assertion failures are
preserved in restored-vector-roots-175.txt. Explicitly rebuilding the session
object and relinking yields the clean 26-result run in
restored-vector-roots-rechecked-175.txt.

The normal portable launcher is rebuilt with the restored Qt host, whose SHA256
is 6956b07e3c0949cb8512affa06612a6c9c6bd9c58f221906fae303eb686033a3.
It retains the previously verified opacity fix and unrelated icon/fallback
changes. The early-root experiment, detailed observer and reservation override
are excluded.

AI assistance has been used to create this output.
