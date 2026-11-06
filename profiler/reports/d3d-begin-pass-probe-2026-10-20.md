# Separate D3D11 begin-pass diagnostic

The restored-display control contained a 6.2253 ms renderer begin-pass scope,
while batch preparation took only 0.4540 ms. This is an attribution gap, not
evidence that the GPU blocked for 6.2253 ms.

Inspection of the cached Qt 6.11.1 D3D11 implementation shows that an ordinary
swap-chain beginPass records reset, target and clear commands, then resets cached
state. Native GPU clears occur later during command execution. Resource updates
and texture render targets can take other paths. Command-list growth can allocate,
copy and release storage. Wall time alone cannot separate work from descheduling.

An artifact-only diagnostic records wall time and thread CPU cycles around the
D3D beginPass and renderer recording callback, with six nested parts: resource
updates, target state, three command acquisitions, and cached-state reset. Command
acquisition also records element counts and whether backing storage changed.
Raw CPU cycles are not converted to milliseconds. Nested scopes overlap.

The recorder pre-touches 32,768 slots before application creation, publishes each
completed immutable record atomically, and rejects capture overflow. A same-exe
environment switch disables its timers. The disabled wrapper still has stack and
metadata overhead; it is not identical to uninstrumented Qt.

The generator reverses its insertions and checks the original beginPass body.
It does not alter SDK headers or the QtGui archive. A separate executable and
receipt preserve the existing comparison host and all 52 pinned build inputs.
No production source or running normal application was replaced.

Verification completed:

- Recorder enabled: 4 tests passed; disabled: 3 tests passed. Tests cover actual
  Qt command-list growth, reference identity, preserved values, nested ownership,
  CPU-counter validity and bounded overflow. These are GUI-less tests.
- Embedded import isolation and compiled-host QML loading: 4 passed, no failures
  or skips, using offscreen/software rendering.
- Windows portable import audit passed with system dependencies only.

Native D3D rendering, probe overhead and live scope attribution remain unverified.
Successful linking and software import checks do not establish those properties.
The host is diagnostic only; no embedded payload was rebuilt or deployed.

Sources and receipts are in artifacts/D3DBeginPassProbe.h,
artifacts/prepare-d3d-begin-pass-probe.py,
artifacts/build-d3d-begin-pass-host.py and artifacts/d3d-pass-build-receipt.json.
