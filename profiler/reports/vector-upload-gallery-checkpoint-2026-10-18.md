# Renderer and gallery checkpoint, 2026-10-18

This checkpoint preserves the tested stable-interval improvements and the
unfinished next experiment. It does not claim constant 160 Hz scrolling.

## Integrated work

The normal Qt dependency recipe includes owned drawing-order intervals,
reservation return and retention of unchanged batches. Native-package cache
keys include the replacement data. See retained-renderer-integration and
retained-renderer-lifecycle reports for measurements, lifecycle regressions,
portable gates and the already-running host identity. Downward display p99
remains about 12.5 ms. The rejected cold-upload policy experiments are kept
as patches and reports, not activated in the dependency recipe.

## Experimental work saved

The gallery now has a native GalleryVectorUploadGate item, registered in its
runtime, and a versioned optional render-thread protocol. CPU source readiness
and native readiness are separate. Pending viewer swaps can prepare geometry
without drawing over the outgoing image. Native priority is 0 for viewer
surfaces, 100 for viewport previews and 200 for overscan. The capability-absent
path retains opacity-based presentation.

The v2 Qt patch is preserved under
profiler/experiments/vector-upload-admission-v2-2026-10-18. It forces isolated
transform batch roots without adding clipping, admits pending uploads under
soft byte/count/time limits, and reports completion, pending work and failure.
The recipe does not activate that patch. The current native fixture includes
experimental tests that deliberately fail their capability assertion against
the normal library when native testing is enabled. This is an experimental
checkpoint, not a claim that the complete normal native suite is green.

## Verified evidence

- Initial gallery SVG selection: 26 passed, 0 failed at DPR 1.75 with the
  experimental library (vector-upload-gallery-first-175.txt).
- V2 isolation and composite admission selection: 5 passed, 0 failed at DPR
  1.75 (vector-upload-isolation-v2-175.txt). The small transform isolation case
  failed before the fix because it shared a batch with an ordinary sibling.
- After rebuilding the latest gallery test object, the viewport and catalog
  swap cases explicitly require nativeKnown, nativeAvailable and nativeReady
  with ZOIN_EXPECT_UPLOAD_GATE=1: 4 passed, 0 failed including init/cleanup
  (vector-upload-gallery-capability-checkpoint-175.txt).
- Swap timing messages measure request to the first observed target-color
  capture. They include capture/test overhead and are not clean scrolling
  performance measurements or proof that all frames meet 6.25 ms.
- Both repositories pass git diff --check before staging.

Logs and generated captures remain local build artifacts, excluded from Git.
Earlier v1 all-native/OpenGL results are documented separately; they are not
claimed as a full v2 rerun.

## Remaining work

- Measure clean guarded Downloads scrolling with identical gallery sources
  against normal and v2 Qt libraries, plus source-to-complete-paint latency.
- Registering/removing transform batch roots currently triggers Qt full
  rebuilds. Establish regression evidence before attempting interval ownership
  and reservation return for these roots.
- Finish retry policy: native allocation/shader failure currently maps to an
  image error; transient errors must not become permanent source failures.
- Run explicit capability-absent assertions, full v2 native/OpenGL coverage,
  lifecycle/cancellation coverage and actual rendered leaf checks at 175%.
  Existing MS Sans Serif fixture warnings mean geometry assertions alone do
  not establish small-caption sharpness.
- Integrate the optional patch into the reproducible dependency recipe only
  after correctness and performance evidence, then complete portable audits,
  payload embedding and worktree-only restart.

The running portable application is unchanged by this checkpoint and still
uses the integrated retention host recorded in the preceding report. No new
experimental payload has been deployed. No push is requested.
