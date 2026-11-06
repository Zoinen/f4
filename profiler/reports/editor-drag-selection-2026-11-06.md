# Editor drag selection

Reproduced in the actual Qt editor on `D:\test.py`, using the normal worktree
profile. Native mouse drag: (100,300) to (400,950), after one Page Down.
Temporary Go/Qt navigation traces remain outside the repository.

## Findings

- Go pointer processing was small: 0.057 ms for press, 0.071 ms for move.
  The captured move nevertheless reached the Qt patch boundary 29.318 ms after
  outbound packing, and selection painting depended on that reply.
- Ordinary stream selection lacked the immediate local feedback already used
  by Alt-drag rectangular selection.
- The occurrence-marking guard required styled row projection for every active
  selection, including multiline selections that cannot highlight occurrences.
- After relaxing that guard, live verification exposed `invalid_state`
  rejections: the real producer included five rectangular-selection fields
  outside the compact protocol whitelist. The hand-built renderer test did not
  reproduce the real producer's field set.

## Changes

Ordinary left-drag now updates the existing selection overlay immediately from
committed row coordinates. Selection columns clamp at the row's visual extent.
The backend remains authoritative for character boundaries, selection semantics,
and document changes. Pending local feedback survives an older endpoint-row echo
and yields to the authoritative endpoint after release. Alt-drag, word selection,
and source-addressed mouse events retain their existing paths.

The cursor patch guard checks whether an occurrence needle actually exists.
Entering or leaving a qualifying word selection still updates styled rows.
The compact producer now excludes rectangular fields while full document state
retains them.

## Verification

- The new stream-feedback test failed before the fix because it waited for the
  backend. It now exercises 500 alternating row endpoints without backend echoes,
  release, end-of-line clamping, stale echoes, and authoritative column correction.
- Native Windows at DPR 1.75: synchronous mouse-to-local-selection work averaged
  0.211 ms, worst 7.349 ms across 500 moves. This measures component work, not
  input-to-photon latency or sustained refresh rate.
- Native selection/pointer/pixel-grid suite: 9 passed, no failures or skips.
  Scene-space leaf origins/transforms checked and rendered selection captured.
- `go test ./internal/editor ./internal/semantic -count=1` passed.
- Direct renderer cursor-state regression passed. New real-producer regression
  validates the exact compact field set and preservation of full rectangular state.
- Final actual-editor capture: press and move both reported `cursor-state`;
  both compact patches were accepted; release was unchanged. No full row update
  was requested by these pointer events. The capture is a functional sample,
  not a statistical high-rate mouse benchmark.
- Portable host import isolation, GUI subsystem, DLL audit, and embedded-host
  extraction tests passed. Embedded host SHA-256:
  `f051ed1a2411ddc21c7c60986dafa092c4594073f92b33daad72037de548680d`.

AI assistance has been used to create this output.
