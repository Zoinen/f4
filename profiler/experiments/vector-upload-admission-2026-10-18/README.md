# Vector upload admission prototype

This is an opt-in native prototype, not the running application's renderer.
It is the next mechanism after the rejected cold-upload policy experiments.

`qt-upload-gate.patch` applies with ordinary `git apply` from qtdeclarative
after the integrated stable-interval patch on Qt 6.11.1. It modifies
qsgbatchrenderer.cpp and qsgrendererinterface.cpp and is self-contained: its
protocol and implementation are embedded in the renderer source. No artifact
headers or exported include tree are needed to compile the patched sources.
The archive was applied to copies of both base files and reproduced the two
compiled prototype sources after newline normalization.

`native-regression.patch` applies after the retained-renderer lifecycle native
fixture. Its base CPP has SHA256
`560eeb473cc635f0a27ba2b7311e30ce61c7f90e7039e9d6268aeb2b3e110661`.
It adds the optional protocol header and one composite native test. In this
working copy the header is already present as an unused untracked file; use
`git apply --exclude=qt/host/tests/VectorUploadGateProtocol.h` for the test patch
there. A fresh checkout without that header can apply the whole patch.

The named capability `f4.vectorUploadGate.v1` returns a render-thread-only
function table. Callers register an isolated scene root, source generation,
CPU-completion flag, presentation flag and integer priority. Lower priorities
run first. The current prototype requires the caller to provide an isolating
clip/root; the native fixture uses clips. It does not yet enforce isolation
for arbitrary nonclipped item roots.

Initial uploads and material/clip preparation are spread over frames. No
prepared draw record is published until the whole group is ready, and the
separate presentation flag remains responsible for the GUI commit. A byte
estimate, batch count and elapsed upload time limit admission. One oversized
batch can proceed alone. These are soft work limits, not a hard frame deadline.

Readiness bookkeeping uses both batch and element identity. Source-generation
changes discard readiness. Removal cancels registered work. Unmanaged scenes
keep the normal renderer path. The prototype also withholds render-node draw
callbacks while preparation is pending.

The native test fails on the normal renderer because the capability is absent.
All eleven prototype native results pass on the default Windows renderer and
OpenGL at actual DPR 1.75. The new fixture's 24 large geometry batches complete
in five captures, with at most five uploaded batches per frame, while the old
background remains pixel-identical until explicit presentation. This is
correctness evidence, not a real-folder performance result.

See [the implementation report](../../reports/vector-upload-admission-prototype-2026-10-18.md)
for the integration boundary and required next work. No QML consumer, portable
payload change, or 160 Hz success claim is included.
