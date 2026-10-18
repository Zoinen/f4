# Vector upload gate v2 checkpoint

This is unfinished experimental work, separate from the integrated stable
interval recipe and from the running portable application's renderer.

`qt-upload-gate.patch` is a self-contained diff against Qt 6.11.1 after the
stable interval recipe. Apply it from the qtdeclarative source root. It changes
qsgbatchrenderer.cpp and qsgrendererinterface.cpp, embedding its implementation
and protocol without relying on files under artifacts. The renderer base is
the same SHA256 recorded in the v1 experiment. Do not apply both gate patches.

The optional `f4.vectorUploadGate.v2` interface adds pending/failure queries,
version/size validation and explicit previously-prepared state. Registered
transform nodes become batch roots without introducing an SVG overflow clip.
The current gallery source contains the matching protocol and QML consumer;
the parent native fixture contains v2 regressions.

This patch is NOT applied by ci/patch-qt-dependencies.py. Native upload-gate
tests require an executable linked against this experimental Qt library in
addition to ZOIN_TEST_NATIVE_RENDERER=1. Against normal Qt their capability
assertions fail deliberately. Ordinary gallery rendering uses its fallback
when the capability is absent. Set ZOIN_EXPECT_UPLOAD_GATE=1 for the gallery
swap test to assert that the native consumer actually uses this capability;
set it to 0 when checking the capability-absent path.

Verification and remaining work are recorded in
[the checkpoint report](../../reports/vector-upload-gallery-checkpoint-2026-10-18.md).
