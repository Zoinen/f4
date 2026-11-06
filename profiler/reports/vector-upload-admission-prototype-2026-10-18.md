# Native vector upload admission: working correctness prototype

## Mechanism implemented

The prototype adds a named, optional QSGRendererInterface resource capability:
`f4.vectorUploadGate.v1`. Its function table supports configure, remove and
generation-specific readiness queries, all on the render thread. A standard
unpatched Qt build returns null for the name, so an eventual gallery consumer
can preserve its current path when the capability is absent. There is no new
Go wire field or Qt class-layout change.

A registered group has an isolated scene root, source generation, CPU-complete
flag, presentation flag, and integer priority. Initial geometry uploads are
admitted by priority, then group age and drawing order. Admitted batches also
run native material/clip preparation, but their prepared draw records are
withheld. Unadmitted batches do neither. Once every current batch is prepared
and CPU completion is true, the group becomes ready after the resource update
has been submitted. The presentation flag remains separate: readiness alone
does not expose the replacement over the old image or file icon.

The admission limits are an estimated 1 MiB, 32 batches, or two milliseconds
of upload-loop wall time per frame. The first batch may exceed those limits
to ensure progress. Material preparation follows admission and is not included
in that upload-loop timer. A single driver call can still exceed a frame.
Therefore these are soft work limits, not a guarantee of 6.25 ms frames.

Ready groups use the ordinary update path. Unmanaged scenes retain their
normal behavior. Generation changes clear readiness; removed nodes cancel
their registrations. Prepared membership records include batch and element
identity so recycling a batch object does not automatically prepare another
element. Render-node callbacks are also withheld until presentation; they
must not escape an otherwise hidden vector group.

The caller currently must supply an isolating root/clip. The fixture does so.
The native prototype does not yet enforce isolation for an arbitrary item
transform, and there is no QML consumer yet.

## Regression and native evidence

The new fixture first fails against the normal renderer at the absent
capability assertion: two passes and one failure including init/cleanup.
The prototype makes it pass. It uses 24 independent, large unmerged geometry
batches, each with 16,384 vertices and six 32-bit indices, plus a render node.
Only four vertices are referenced, avoiding irrelevant overdraw while testing
real buffer upload work.

The test verifies:

- Uploads are spread across frames: five admission captures and at most five
  geometry batches uploaded in a frame on both tested backends.
- The entire captured background stays unchanged through loading and after
  readiness while presentation remains disabled.
- Render-node draw callbacks remain uncalled during preparation.
- Enabling presentation paints all 24 expected cells, with correct colors.
- Actual DPR is 1.75; every affected geometry leaf has a stable name, whole
  physical-pixel scene origin, and unit scene transform.
- A generation change cannot reuse the previous generation's readiness.
- CPU completion is independently required even when old buffers are resident.
- Removing a completed group removes its readiness registration.
- A later viewer-priority group overtakes an unfinished panel-priority group.
- Cancelling the unfinished panel group and removing the viewer group leaves
  the background intact on subsequent rendering.
- An individual allocation larger than the soft byte limit still completes.
- A valid empty group completes without upload work.

The final native suite passes all eleven results on the default Windows
renderer and on OpenGL. This includes the preceding retention, clipping,
opacity and lifetime fixtures. Logs under artifacts are
`vector-upload-gate-membership-native-175.txt` and
`vector-upload-gate-membership-opengl-175.txt`; the new test alone is also
recorded in `vector-upload-gate-expanded-175.txt` and
`vector-upload-gate-composite-175.txt` during development. The completed capture
`vector-upload-gate-complete-175.png` was inspected: all expected cells are
present on the blue background. No text or raster-image leaves are introduced
by this fixture.

No synthetic fixture timing is presented as Downloads or Android scrolling
performance. No claim is made that source-to-complete-paint latency improves
in the application yet.

## Reproduction and state

The self-contained Qt patch and native regression are archived in
`profiler/experiments/vector-upload-admission-2026-10-18`. Applying the Qt patch
to copies of both base sources reproduces the compiled prototype after newline
normalization. The renderer base is the preceding integrated stable-interval
source, SHA256
`9240208618920966c429802866de0b53f656a4ba7e0ce9134b708134e264af53`.

The prototype library is `artifacts/qt-vector-upload-gate/Qt6Quick.lib`.
It replaces both the batch-renderer and renderer-interface objects in an
isolated local archive. The normal imported Qt library, host executable and
embedded payload remain unchanged. No normal application restart is required
for this test-only experiment.

Automatic approval review rejected a shell command that included deleting the
temporary protocol header, with the stated reason `blocked by policy`. The
normal source was restored using a command that retains the unused header.
The test patch's CPP portion passes apply-check against the restored source;
the existing header can be excluded when reapplying the patch in this checkout.
All ten normal native results pass again at DPR 1.75 in
`artifacts/vector-upload-gate-normal-restored-175.txt`. A fresh process-identity
check confirms the worktree app still uses the preceding integrated host,
SHA256 `a6a5aa0c5951a58621278e55de0396ec85fdddb8f2583ae74ac4ff2e749629b4`, with
the correct worktree title. Evidence is
`artifacts/vector-upload-gate-normal-identity.json`.

## Required gallery integration

The next implementation must connect real vector surfaces to this mechanism
and then measure both frame tails and time to the first complete paint. It
must not replace those measurements with this synthetic test.

1. Introduce a native gallery gate using the optional capability. Query the
   interface and register/query nodes only on the render thread. Keep status
   transport and owner lifetime safe across GUI deletion, scene invalidation,
   window changes and cancellation. Preserve the standard-Qt/software path.
2. Provide batch isolation without silently changing SVG clipping. A new clip
   around the gallery wrapper can alter overflow/zoom behavior. Prefer checking
   whether the registered transform can become an explicit Qt batch root;
   otherwise prove any clipping matches the existing presentation. The current
   prototype's first-element ownership lookup assumes isolation.
3. Keep generation-aware CPU and native readiness separate in
   GalleryVectorImage. Keep the ordinary panel icon until native readiness.
   In GalleryVectorSwap, let pending hardware surfaces participate in native
   preparation instead of remaining under opacity zero, while holding their
   presentation flag false until the GUI commits the slot. Preserve the legacy
   opacity path when the optional capability is absent.
4. Add an explicit pending-work/status query before wiring automatic frame
   requests. Deferred native work must request more frames, but waiting for CPU
   or VFS work must not create an indefinite render loop. Add failure status
   and a deliberate retry/error policy for native allocation or shader failures;
   the current boolean readiness API is insufficient for a production consumer.
5. Prioritize the active viewer, then its ordered neighbors, then current
   viewport previews and overscan. Reevaluate priorities as the viewport or
   viewer request changes. Do not let a detached source hold native admission.
6. Verify every panel mode, swaps, transparency, animation, malformed documents,
   authored image elements, source replacement, cancellation and failure/skip
   behavior. Check actual leaves and rendered output at DPR 1.75. Test both
   patched and capability-absent paths.
7. Compare the guarded heavy-folder workload in clean repeated captures, with
   no compilation or diagnostic scopes during measurement. Record upload work,
   image latency and cancellation independently. Then integrate the reproducible
   Qt patch, native-package cache identity, portable gates and embedded host.

The 160 Hz goal remains active. This turn establishes an executable native
mechanism and its correctness boundary; real-surface integration, performance
measurement and rollout are still unfinished.

AI assistance has been used to create this output.
