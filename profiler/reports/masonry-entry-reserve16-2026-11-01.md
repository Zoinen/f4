# Masonry: sixteen-spare delegate reserve trial

Status: measured prototype, not promoted. Constant 160 Hz remains unachieved.

## Candidate and control

The October 19 two-spare experiment did not establish a cadence win. The current cold-entry traces instead show seven to ten missing-row preparations in one commit. This trial changes that prototype's bounded ready capacity to sixteen and reapplies its narrow factory/acquisition changes to the current Masonry sources. It tests whether the larger burst can be supplied. It does not presume the earlier negative result has been overturned.

Only non-contained Masonry uses the reserve. One asynchronous construction may be pending per layout; completed, unassigned roots are bounded at sixteen per layout, outside the ordinary pool. Each has the existing component, viewport context and renderer visibility policy. Component/viewport changes cancel obsolete work; destruction releases the sidecar; definitive construction failure suppresses retries. Empty reserves retain synchronous visible-row creation. Viewport, motion-target and decode windows are unchanged. This is an explicit additional retained-object cost.

Control and candidate compile the two affected files from the same current sources with the same compiler flags. Control removes the reserve transforms. Both use the same new main object and input recorder, including counter snapshots before/after input. All new links use `ci/msvc_link_response.py`. Their Windows GUI-subsystem and static import audits pass. No production source, installed Qt dependency or embedded payload changes.

Host SHA-256:

- Control: `41b6a314d36c2262c21ce85b1dcc03efc5ed5abf7902396184f35ca78c29b79f`
- Candidate: `1aa629d1497fcb015dcc210b9def5a78cd9d20c36a41c9d7303477a61f97195b`

## Verification

The lifecycle executable passes: deferred creation, required-property initialization, completion before use, anonymous identity, sixteen-ready bound, replenishment, component replacement, taken-child lifetime, pending cancellation, registry cleanup and suppression of retries after intentionally invalid QML. Its reported error text belongs to that intentional negative case; `passed` is true and the final cumulative ready count is 33.

Native Direct3D mode checks report 11 passing QtTest results and one failure. Passing checks cover folder-preview pixels, nearby preparation, recycling, retained vector scenes, renderer visibility and the 175% viewport/brick grid. Counts include fixture setup/cleanup and data rows, not eleven distinct test methods.

The failure is `detailsFileFieldLeavesStayOnPhysicalPixelGridAt175Percent`, including `gallerySourceColorIcon-13` at physical x=16.75. An unchanged control run of the same sequence reproduces that coordinate (and other source-color icons). A control run in isolation also reveals fractional Details group/text origins. This remains unresolved; the suite is not reported green. The initial default-style run failed because the portable host embeds Basic rather than Windows controls; its log is retained. A software run and the correctly configured native Basic run are separately retained.

## Alternating live captures

Four fresh processes run A/B/B/A using initialized-app empty-panel entry into actual Downloads (192 rows), Masonry density 152, DPR 1.75 and 160 Hz. Each has 30 seconds of rapid wheel input. Cache roots are empty before QML and before navigation; input begins without waiting for previews. OS file-cache coldness is not claimed. No builds/tests/offline analysis overlap capture. The normal app stays alive. A bounded temporary refresh-mode lease was released, restoring the prior 60 Hz mode without a registry change.

| Case | Arm | Cached / pending at input | Display gaps >9.375 ms | Dropped rows | Maximum display gap | Spares consumed during input | Largest early preparation phase |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| cold-m11 | A | 25 / 18 | 2 | 0 | 18.7684 ms | 0 | 6.3890 ms |
| cold-m12 | B | 24 / 10 | 4 | 1 | 12.4989 ms | 49 | 7.6165 ms |
| cold-m13 | B | 16 / 18 | 2 | 0 | 68.7455 ms | 18 | 8.7566 ms |
| cold-m14 | A | 9 / 18 | 2 | 0 | 24.9728 ms | 0 | 6.7479 ms |

All runs have zero coverage holds; every miss/drop, including boundary misses, remains retained. All physical wheel-error pairs remain excluded by the existing analyzer, so no physical-motion acceptance is claimed. Candidate native counters report maximum ready sixteen and zero construction failures. Before-input cumulative ready/taken counts are 54/41 and 40/40: the reserve is exercised, but not necessarily full when first input starts. Cumulative counts span both panels and are not a per-layout occupancy sample.

The larger reserve does not establish a cadence improvement. It is not promoted. Lower later commit maxima in these runs are insufficient to outweigh the missing full acceptance and worse candidate outliers. Cache states differ, and four runs do not establish a general regression magnitude either.

## Follow-up variant

The prototype consumes a spare in `createComponent` and writes initial properties individually before returning. The normal recycled-slot path instead assigns identity, visual snapshot and geometry through the existing row transaction. A separate source-only variant moves reserve consumption into the ordinary pool factory and leaves that transaction responsible for assignment; the synchronous component factory remains its fallback. Its intended graph uses the unmodified control `MasonryLayout.cpp` object, avoiding the old property-by-property handoff. It is not yet compiled or validated. This is a distinct hypothesis, not a claim that the failed trial is fixed.
