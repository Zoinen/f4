# Local workspace instructions

## Mandatory portable-build policy

- Before changing, running, debugging, or reviewing any build, packaging,
  release, deployment, or CI workflow, read
  `docs/PORTABLE_BUILD_POLICY.md` completely.
- The platform contracts and verification gates in that document are required;
  do not replace them with a directory bundle on Linux or Windows, and do not
  replace the signed application bundle with runtime extraction on macOS.

## Mandatory build-and-run cycle

- After every change to workspace files, always produce a fresh canonical build of the affected application, including the Go core and QML/Qt frontend when either participates in the result.
- Only after the fresh build succeeds, close every already-running `f4` instance that belongs to this workspace together with its Qt host child, then launch the newly built canonical QML frontend.
- Never leave the previous application instance running after a successful rebuild. Verify that the old processes exited and that the new Go and Qt host processes are alive.
- If the build fails, keep the last working application instance alive, fix the build, and repeat the cycle before handing the task back to the user.

## Mandatory static physical-pixel alignment

- Every non-animated visual item must resolve to the physical pixel grid at
  rest. Validate scene-space positions, layout edges, dimensions, and effective
  ancestor translations with the actual window DPR; an integer logical
  coordinate is not sufficient. Treat 175% (`devicePixelRatio == 1.75`) as a
  mandatory fractional-scale case.
- Snap the result of layout arithmetic, including centering, margins, spacing,
  and container placement. Snapping only a leaf item's local `x`/`y` is not
  sufficient when a parent contributes a fractional scene transform.
- Native-rendered text and raster images must never inherit a fractional static
  translation or scale. When exact mathematical centering falls between two
  physical pixels, choose a neighboring whole physical pixel instead.
- Animation frames are the only exception. Animation start, end, and every
  non-animating/resting state must return to the physical pixel grid.
- Every new or modified static UI layout must have a regression check at 175%
  scale that maps the affected items into scene coordinates and verifies whole
  physical-pixel positions. Include rendered-image verification when text,
  icons, one-pixel strokes, or clipping could regress without a geometry change.
- A pixel-grid check of a wrapper (`Rectangle`, `RowLayout`, `ColumnLayout`,
  control background, and so on) does **not** cover its visual descendants.
  Give every affected static `Text`, `TextInput`, `TextEdit`, `IconLabel`, and
  raster-image leaf a stable `objectName`, then test each leaf's scene-space
  origin after the window is shown and the layout has settled. For a compound
  control, enumerate every line/style, including small captions and secondary
  labels; wrapper-only coverage is forbidden.
- Treat automatic centering by `RowLayout`/`ColumnLayout` as unsafe at a
  fractional DPR. Even when both extents are whole physical pixels, an odd
  physical-pixel difference produces a half-pixel centering offset (for
  example, centering 37 px inside 74 px yields 18.5 px). Choose compatible
  extents or apply a scene-space pixel correction to the shared visual group
  after layout; never accept the layout's implicit centering unverified.
- `snapPx(localValue)` proves only that one local value is rounded. It does not
  prove final alignment when any ancestor, anchor, implicit-size calculation,
  layout distribution, transform, or window offset is fractional. Tests and
  corrective bindings must map the leaf through its complete ancestor chain to
  `QQuickWindow::contentItem` and round that final scene coordinate.
- Static native text and raster icons must map local unit vectors to scene unit
  vectors (no scale, rotation, shear, or layer resampling) as well as land on
  whole physical-pixel origins. A 175% regression must assert both translation
  and effective transform for the actual leaves and grab the rendered window
  when the change affects text or icon sharpness. Inspect the smallest/secondary
  text in that capture; checking only a large bold label is insufficient.
- When a pixel-grid regression is found, first add or extend a test that fails
  on the offending leaf coordinate, record the measured physical coordinate
  that caused it, and only then apply the fix. A test that would have passed the
  broken hierarchy is not an acceptable regression test.

## Canonical f4 binary

- Always build the user-facing Go application binary only as `/Users/zoin/Documents/f4/f4` in the project root.
- Never create, build, copy, or launch alternate `f4` application binaries in subdirectories, temporary directories, or under any other filename.
- Supporting Qt/QML build artifacts may remain in their one canonical configured CMake build directory, but the application must always be launched through the root `/Users/zoin/Documents/f4/f4` binary.
