# F4 viewer rendering snapshot

`ViewerResampleEffect.h` and `.cpp` are byte-for-byte copies of the files in
`third_party/ZoinGallery` in the working checkout on 2026-09-26. The corresponding
`resources/viewer_resample.vert` is also unchanged. Keeping this small snapshot
inside the proof makes the demo self-contained and leaves the F4 checkout and
its running binaries untouched.

The demo supplies a different `resources/viewer_resample.frag` at the original
resource URL. It retains the same uniform layout but procedurally draws
one-pixel strokes instead of sampling an image. Source selection, half-size
pyramids and production image filters are not under test here. The material,
scene-graph geometry, render-target uniforms and vertex alignment are real F4
code. `ShaderDiagnosticScene.qml` uses the original `pixelAligned` property and
disables it only while the shared ancestor animation is running.

Snapshot SHA256:

- Header: `8FFB6B7FAFD3ED21C1A58153680D1ABEE9E788D69E3BDE8A6F402B1C4FED9112`
- C++: `8C62E39212C56134175FDD2828215BB6A00DAC1608F15AAFBE9B66925E6C8282`
- Vertex shader: `7D8DB142C0EF5603EF7FD56B0D1EC9CE2000FD10D0A16D0C22EDD81237FB8007`
