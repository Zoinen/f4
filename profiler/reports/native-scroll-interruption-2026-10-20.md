# Native scrolling interruption lifecycle — 2026-10-20

## Reproduction and correction

The native camera can advance while GUI synchronization is delayed. Hiding a
panel/window or changing its scrollbar previously called cancelNative(false),
discarding that progress. A new data-driven regression pauses GUI delivery for
120 ms during an 800 ms linear scroll, interrupts it, checks that the current
position is committed, restores visibility, verifies the position stays stable,
and starts another scroll to completion.

Before correction, all three cases failed on D3D11 and OpenGL. Example D3D11
positions were 50.4 -> 50.4 (panel hide), 27.9 -> 27.9 (window hide), and
27.9 -> 27.9 (scrollbar detach): no rendered progress was preserved.

Visibility and scrollbar changes now stop with commitPose=true. Model, geometry,
and document replacement cancellation retain their existing separate behavior.
After correction, D3D11 positions were 33.3 -> 162.9, 27.9 -> 157.5, and
27.9 -> 157.5. There are no added per-frame calculations or visual wrappers.

## Verification

- Both backends: 11 passed, zero failed/skipped. Includes three interruption
  cases, two scrollbar cases with actual 175% leaf geometry and rendered-image
  comparisons, retargeting, geometry shrink, metadata rewrap, and fallback.
- Before/after logs: artifacts/native-interruption-{before,fixed}-{backend}.log.
- Build initially exposed QObject fixture typing in the test; corrected to use
  the fixture's visible property before running the failing baseline.
- No new frame-rate measurements in this change. Earlier cold timing still
  includes a displayed miss; constant 160 Hz is not established.
- Native motion remains disabled by default pending rollout verification.

AI assistance has been used to create this output.

## Portable deployment

Import isolation, system-DLL audit, embedded extraction tests and packaging
passed. The worktree launcher was restarted directly with --gui=qt; the cached
child hash matches the rebuilt host and its window responds. No host override
or canonical-build replacement was used.

Host SHA256: `c4e9f15092808de44b33bb82c95501b3cefd28c73ee85bf2117a24ed6745b32d`.
Launcher SHA256: `c7e98fbaf5a7b2b41c8293298bfc2b9a12d939dc44a4a425a23a21b71edcf244`.
