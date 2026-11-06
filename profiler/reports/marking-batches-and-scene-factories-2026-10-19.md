# Marking batches, retained roots, and the next scene target — 2026-10-19

## What changed in this investigation

The preceding preview-resumption turn made progress: a redundant size-role
notification was removed, tested, deployed, and current misses were attributed.
This turn adds bounded native observers inside the actual matching Qt 6.11.1
translation units, then measures the remaining marking work. No collector
policy or ownership change is retained in production. Constant 160 Hz is not
achieved.

The timed mark stack loop retains its original 10,241-object deadline boundary.
Observers time 128-object subgroups, count marked vtable classes with a bounded
native table, and record original batches and remaining deadlines. No clock or
log call is inserted around each individual object. A second full-source
qv4qobjectwrapper replacement counts and names recursive QObject-root visits.
Archive members are removed/replaced directly; no duplicate-symbol override or
/FORCE:MULTIPLE is used. SDK packages remain untouched.

Both cold/warm pairs use the same process within each pair, 186-entry Downloads,
density 152, DPR 1.75, primary/output 160 Hz, 5 ms GC budget, and two four-second
wheel windows. Native arrays stay below limits. Recording perturbs execution;
these runs diagnose work and are not clean frame-cadence comparisons. Builds,
tests, UI Automation and offline analysis do not overlap captures.

## Batch and scheduler evidence

Cold active windows visit 398,212 heap objects across 40 original batches and
3,143 subgroups; warm visits 142,174 across 14 batches and 1,119 subgroups.
Class counts account for every recorded visit (zero unclassified entries).
Cold top classes: String 124,560; QQmlContextWrapper 50,961; QmlContext 50,961;
QQuickItemWrapper 32,836; MemberData 31,032; VariantObject 24,685. These are
visits across collections, not unique allocations or distinct live objects.

Longest 128-object group: 2.518 ms cold and 0.9409 ms warm. Raw QPC context
switches prove all of both intervals are scheduled GUI execution. The top cold
group has two full available samples in markChildQObjectsRecursively ->
QObjectWrapper::markObjects -> MarkStack::drain -> markDrain -> transition ->
onEventLoop. Warm's top group samples the same recursive walk. This proves an
indivisible recursive traversal is present inside marking; the group timer alone
does not assign all 128-object time to a single object.

Longest original batches are 4.7227 ms cold and 1.9743 ms warm in this capture.
The preceding 7.0347 ms MarkDrain does not recur here. A cold 2.998 ms batch starts
with only 0.2751 ms of deadline remaining; a warm 1.9743 ms batch starts with
0.9438 ms. Coarse batches can overshoot; existing negative smaller-batch and
GC-budget experiments still prevent promoting another blind constant change.

## Root attribution and rejected ownership experiment

The full-source root observer identifies retainedGalleryPanel-* Repeater Loaders
with visual parents but no QObject parents. Largest original warm loader:
9,197 descendants and 1.1399 ms. Original warm root walks total 27,924 descendant
visits in 2.9416 ms. This corroborates the older root finding in
`qml-gc-and-render-stalls-2026-10-18.md`; it is not a newly discovered Qt defect.
The earlier all-root/window adoption experiment was already mixed and rejected.

A narrowly scoped new experiment parents only retained loaders to a per-content
QObject owner. Its proposed parent assertion fails on the original code and
passes with the owner. However, the candidate consolidates the tree into the
floating top-level window: warm still visits 27,875 descendants in 2.6948 ms,
and its largest uninterrupted root walk grows to 22,820 descendants / 2.2717 ms.
Cold totals are 74,661 visits / 7.0995 ms original, versus 75,667 / 7.494 ms
candidate. Different captures/collection states prevent claiming a speed delta.
The descendant counts show that the work largely moved, and the larger root
scan is undesirable. No clean A/B is warranted for this rejected approach.

The experiment and its test changes were removed exactly; candidate sources are
saved separately. The candidate workspace test passed ownership assertions but
later failed `leaves > 0`. The same leaf failure reproduces after restoring the
original test/implementation, without the new forced GC. Therefore that failure
does not establish a candidate-specific regression. Do not describe this test
as green or use it to justify an ownership change. Its fixture also emits
missing TestGallery reportMetadataVisibleRange and font/type warnings.

## Concrete next target: per-entry factories

A separate native census runs only during serialization after the scrolling
windows; it adds no per-frame scene observer. QObject and visual child graphs
are deduplicated. It sees 28,554 QObjects. Counts by QML context URL include
5,385 from GalleryEntryPreview.qml, 2,655 from GalleryEntryDelegateBase.qml,
1,242 from MasonryEntryDelegate.qml, and 1,180 from FilePanelChrome.qml.
Context URL is creation-context attribution, not exclusive allocation cost.

Of 1,426 QQmlComponent objects, 1,180 have GalleryEntryPreview as their QObject
parent and GalleryEntryPreview.qml as their URL. Four inline factories remain
in that file: aspectRasterComponent, tintedRasterComponent,
sourceColorIconComponent and fallbackMarkerComponent. The existing
GalleryEntryVisualFactories.qml already provides per-panel factories for mode
delegates and three thumbnail renderers. The next implementation should extend
that verified pattern to these four remaining per-preview factories, preserving
typed instance context, source replacement, tint/aspect behavior, hover/selection,
staged readiness and 175% leaf geometry/render checks. Count the live factories
and allocation/marking work afterward, then measure clean display cadence.
Do not enlarge retention caches or change ownership to hide the traversal.

Other census counts: 2,331 Translate, 2,143 native PixelGridAlignment, 1,231
GalleryLayoutDebugZone, 1,167 ScenePixelAlignment. They are future inspection
candidates, not proof that they can be removed safely. Static alignment remains
required. The native census records all types and creation-context counts.

## Restored build and verification

All ownership production/test additions are removed. The original normal host
was rebuilt and its system-DLL import audit passed. Selected native layout/mode/
session/embedded-import suites pass on both Direct3D11 and OpenGL at 175%:
27 + 25 + 7 + 3 QtTest cases per backend, including setup/cleanup. The separate
workspace test remains a reproduced baseline failure as described above.
The embedded payload was regenerated; extraction/materialization Go tests passed
with CGO_ENABLED=0, GOWORK=off and the system Go cache. Only this worktree's own
launcher was replaced, directly with --gui=qt. Its extracted child matches the
normal host, staged launcher equality is verified, and startup stderr is empty.
No canonical process was touched. No commit or push was made.

Archive: `profiler/experiments/marking-batches-and-scene-factories-2026-10-19/`.
It retains compact native captures (all GC/root records and identities), exact
QPC alignment/scheduling/sample summaries for original observers, source/build/
analysis scripts, rejected candidate snapshots, test results and hashes. Large
nearby-preparation/incubation arrays are omitted from compact captures; original
capture hashes and observer counts are preserved. Executables, raw ETW/CSV,
libraries and build outputs are excluded. The full goal remains active.

AI assistance has been used to create this output.
