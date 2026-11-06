# Thumbnail atlas policy: focused regression and display-context rejection

The sustained 160 Hz goal is incomplete. All changes in this continuation are
diagnostic artifacts and reports; the running ordinary application is unchanged.
The atlas policy is not promoted and has no accepted performance comparison yet.

## Actual factory behavior

ThumbnailFactoryPolicyTest exercises AsyncImageResponse's actual worker/store,
crop parser, completion, textureFactory(), and render-thread createTexture().
The fixture is a non-square 63x37 image, optionally cropped to 43x23. It tests
RGBA8888, ARGB32 premultiplied, and RGB32, each as managed thumbnail and ordinary
viewer source. Factory image format, pixels, extent, and byte count are compared
with Qt's standard factory. The rendered result checks transparency over black
and the green half; the raster leaf is named factoryRasterLeaf.

At forced DPR 1.75, final scene origin is exactly (12,18) physical pixels and
both mapped logical unit vectors are identity. The saved rendered window was
inspected: hard red/green boundaries and the non-square footprint are intact.
There is no text or caption in this focused fixture.

Same test object, two linked implementations:

| Executable | Diagnostic policy | Result |
| --- | --- | --- |
| Ordinary provider | requested standalone thumbnails | 8 passed, 6 failed |
| Experimental provider | disabled | 14 passed, 0 failed, 0 skipped |
| Experimental provider | enabled | 14 passed, 0 failed, 0 skipped |

All six expected failures are precisely the managed-thumbnail isAtlasTexture
assertion: the ordinary provider still produces an atlas texture. In the
experimental provider, managed thumbnails become standalone while ordinary
viewer textures remain atlas textures. Thus the check distinguishes the actual
policy from a wrapper whose output merely looks correct.

Compilation initially failed on ambiguous mapToItem(..., {}); using the explicit
QPointF overload fixed it. The failed compiler log is retained. No production
code was changed to accommodate the test.

## Source-level explanation, not a timing conclusion

The fetched Qt 6.11.1 sources show that
[ordinary texture materials](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/scenegraph/util/qsgtexturematerial.cpp)
call commitTextureOperations() in updateSampledImage(). The
[atlas implementation](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/scenegraph/util/qsgrhiatlastexture.cpp)
flushes its pending uploads, including padding preparation, in that call.

For ShaderEffect, the
[sampled-image callback](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/scenegraph/qsgrhishadereffectnode.cpp)
first commits the source texture, then removes an atlas texture when the effect
does not use atlas geometry or a subrectangle uniform. Our current thumbnail
shader uses local 0..1 coordinates and has neither facility. Atlas removal can
extract the image into a standalone texture through a GPU copy after the atlas
has released its CPU image. This is a concrete extra resource path; it is not
evidence of a second CPU decode or an extra full CPU image upload.

The standalone provider experiment avoids that atlas path but may affect
folder-mosaic batching because those use the same managed thumbnails directly.
An atlas-aware shader is another possible approach, but its sharpening samples
must clamp at each image's atlas subrectangle and its checkerboard/mask must
retain local coordinates. Neither approach has yet earned promotion.

Fetched source URLs and SHA-256 hashes are in qt-sampled-image-source/sources.json;
the previously fetched atlas source provenance remains in
qt-texture-churn-source/sources.json. One initial fetch had a DNS error, and an
incorrect shader-node filename returned 404 before the actual source was found.

## Live display context remains unsuitable

The earlier validation rejected 60 Hz / DPR 1.0. WMI now reports 3840x2160 at
120 Hz for both the Parsec virtual adapter and NVIDIA adapter, while the actual
diagnostic QQuickWindow still reports 60 Hz / DPR 1.0. Adapter information does
not substitute for the application's screen and captured present timing.

A separately labeled CPU-context-only capture retained the normal 160 Hz
capture script unchanged and explicitly required 60 Hz / DPR 1.0. It failed the
existing bounded-recording guard: scrollPhases recorded 282,814 events against
capacity 262,144. Native animation recorded 39,138 precise-clock samples during
two ten-second inputs, far above display cadence. The trace is retained but
rejected; no enabled comparison or meaningful per-frame timing delta is claimed.
Increasing buffer capacity would not make this environment representative.

The diagnostic launcher was stopped by its exact executable path afterward;
the ordinary launcher and extracted host were preserved. Representative testing
needs the 160 Hz / 175% display context restored with normal frame pacing.

## Storage and next steps

Two explicitly named old scheduler CSVs were losslessly compressed and verified
against SHA-256 before removing their raw copies, recovering about 797 MB net.
The recovery manifest is compressed-material-next-space-recovery.json.

Next: matched control/enabled captures with material instrumentation disabled,
first on the restored display environment; check preparation tails, source
eligibility latency, folder-mosaic batching, and rapid-wheel phase error. Keep
the existing 160 Hz, recorder-capacity, and motion gates unchanged. Full viewer,
SVG, packaging, and exact deployed-build verification remain outstanding.
