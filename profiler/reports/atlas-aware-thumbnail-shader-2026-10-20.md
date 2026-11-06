# Atlas-aware thumbnail shader: verified rendering, performance still pending

The sustained 160 Hz objective is incomplete. The ordinary worktree application
is unchanged. This continuation prepares an isolated, flag-controlled candidate
for a matched performance comparison; it does not claim a frame-time win.

## Candidate

The existing thumbnail fragment shader samples local 0..1 coordinates and
therefore makes Qt extract an atlas texture into a standalone texture. The
candidate declares Qt's qt_SubRect_source uniform, maps local sample positions
into that rectangle, and clamps sharpening taps to its half-texel inset. The
checkerboard and rounded mask continue to use local coordinates. A one-pixel
source produces coincident minimum/maximum bounds rather than reading neighbors.

This preserves atlas use for the image provider and folder mosaics, unlike the
separate no-atlas provider experiment. It adds shader arithmetic whose GPU cost
has not yet been compared. Both candidates remain unpromoted.

The original source is third_party/ZoinGallery/resources/shader.frag; the
generated candidate is artifacts/atlas-aware-thumbnail/atlas-aware.frag. It was
baked with the project's existing GLSL 150 / GLSL ES 300 targets plus HLSL 50 and
MSL 12. An initial --qt6 bake failed because the existing shader already needs
textureSize, which is unavailable in ESSL 100. No Qt upgrade or target reduction
was introduced.

[Qt's ShaderEffect documentation](https://doc.qt.io/qt-6.11/qml-qtquick-shadereffect.html)
describes the subrectangle uniform mechanism. The exact 6.11.1 source and hashes
are preserved, including the shader sampled-image callback and atlas extraction
implementation; see qt-sampled-image-source/ and qt-texture-churn-source/.

## Rendering and failure-sensitive regression

Native Direct3D11, forced DPR 1.75: 27 passed, zero failed/skipped. This comprises
24 image combinations, one embedded-resource check, initialization and cleanup.
The matrix covers opaque and transparent data, a 1x1 source, non-square images,
up/down scaling, checkerboard on/off, opacity, sharpening, and rounded edges.
A magenta neighbor occupies the atlas to exercise isolation at image edges.

The whole rendered window is compared with the original shader. Every image
combination measured maximum channel difference zero. The fixture also asserts
non-background pixels and rejects shader error status, so two empty surfaces
cannot satisfy the comparison.

A forwarding QSGTexture records removedFromAtlas() calls. The original shader
made two calls during the observed initial frames in each atlas case; the
candidate made zero additional calls, including its first rendered frame.
These are API-call counts, not a claim of two GPU copies: Qt can reuse the
standalone extraction after its first creation.

Source replacement is exercised in the transparent/checker/upscale atlas case.
The replacement occupies a different normalized atlas position, changes the
rendered image, performs no extraction, and still matches the original shader's
rendering of that replacement.

The same executable, forced to retain the original shader as its candidate,
produces exactly 12 expected extraction-assertion failures (15 other checks
pass). Thus the regression rejects the old resource path even when pixels match.

Actual raster leaves are named atlasShaderRasterLeaf and
embeddedAtlasShaderRasterLeaf. Their final scene-space origins are asserted at
whole physical pixels, and both mapped logical unit vectors are identity.
Rendered originals, candidates, replacement, and embedded-resource output are
saved and inspected. This fixture contains no text or captions; none were
changed by this experiment.

OpenGL native execution remains an outstanding gate after the earlier WGL/RHI
initialization failure in the changed display session. Shader baking establishes
target variants, not successful execution on those other graphics backends.

## Test synchronization and rejected resource experiments

The initial test waited for ShaderEffect.Compiled and stalled on later cases.
Requesting a frame alone did not fix it. The inspected Qt 6.11.1
[shader-info cache path](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/items/qquickshadereffect.cpp)
copies metadata on a cache hit without calling prepareShaderCode() on the fresh
status manager. The fixture now checks actual rendered content, errors, pixel
equivalence and extraction counts. The original/candidate failure-sensitive
assertions remain intact. Failed status-wait logs are retained.

Registering a second resource at the same URL did not supersede the original;
the byte-check failed. A distinct resource plus URL-property interceptor did
not change the tested absolute fragmentShader property either; its assertion
failed. Neither approach is used by the final diagnostic.

Instead, startup replaces the exact six-file zoin_gallery_shaders resource pack
when the diagnostic flag is present. Five packages are copied byte-for-byte
from the current build and only shader.frag.qsb changes. All six resource hashes
are checked immediately. Disabled mode retains the original pack. The candidate
pack bytes are embedded in the diagnostic executable, with no external shader
or QML runtime dependency.

The embedded-resource test loads QML using the normal unchanged shader URL,
checks the selected bytes, renders an atlas source, verifies zero extraction
and expected transparency pixels, and checks the final raster leaf transform.
This proves actual resource selection rather than merely recording a flag.

One test invocation collided with the still-finishing linker and did not run;
its file-in-use error is retained. The completed build was then tested normally.

## Exact diagnostic host and gates

The real-scroll host was rebuilt with --atlas-aware-thumbnail and the existing
prototype flags. F4_PROFILE_ATLAS_AWARE_THUMBNAILS selects the candidate at
startup; F4_PROFILE_THUMBNAIL_NO_ATLAS separately selects the provider experiment.
Use one candidate flag at a time for comparisons. The capture identity records
both, and the successful-link receipt pins the shader, pack, generated header,
actual renderer object and linked host inputs.

The candidate host reached phase=ready through the worktree Go launcher. Its
startup identity and SHA-256 are saved. The exact host then passed
compiledHostLoadsItsQmlModule, alongside qmlImportsWithoutInstalledQt: four
passed including initialization/cleanup, zero failed/skipped. The Windows import
audit passed for that host, with only system DLL dependencies. This is diagnostic
startup/dependency verification, not a rebuilt or deployed embedded Go payload.

Only the exact-path diagnostic launcher tree was stopped. Ordinary launcher
11952 and extracted Qt host 70332 remained responsive at the final check. No
canonical binary, user settings, commit, or push was changed.

## Performance blocker and completion audit

Across three consecutive goal continuations, representative display conditions
have remained unavailable. The first live capture reported 60 Hz / DPR 1.0;
the next CPU-context attempt overflowed its phase recorder while generating
39,138 native animation samples in 20 seconds. Current WMI still reports a
120 Hz Parsec virtual adapter and NVIDIA adapter, rather than the target 160 Hz.
The normal capture guards were kept unchanged and the invalid traces rejected.

Rendering correctness and resource-path removal are proved for the tested D3D11
cases. Preparation p99, displayed frame cadence, GPU cost, folder-mosaic
performance, rapid-wheel phase error, and viewer priority/latency are not proved
by these tests. The source-burst prototype, no-atlas provider, and this shader
candidate must not be chosen or promoted on appearance alone.

All currently prepared comparison work needs a restored 160 Hz / 175% display
with normal frame pacing. Once restored, run matched controls with material
instrumentation disabled, then inspect preparation tails and strict motion gates
before integration, portable payload rebuilding, and exact deployed-build tests.
The full objective remains blocked on that external condition, not complete.

Two additional old scheduler CSVs were losslessly compressed and SHA-256 verified,
recovering 786,936,622 bytes net. A transient out-of-space build launch was
retained and the import-check link succeeded on retry. Recovery records are in
compressed-atlas-shader-space-recovery.json.
