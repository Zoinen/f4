# Middle-button panel autoscrolling, 2026-10-17

Fixed and running from `D:\Code\f4-zoin\f4-zoin.exe --gui=qt` on branch `zoin`.

Follow-up correction: the small-offset checks below established removal of
pixel quantization, but did not detect backward viewport resets. The later
quarter-screen reproduction found those resets in the earlier fast traces.
The additional restore fix and remaining cold-rendering limitation are recorded
in `middle-autoscroll-first-pass-2026-10-17.md`.

## Cause and change

The gesture already used `FrameAnimation`. The discontinuity happened after its
velocity calculation: the shared viewport rounded every translation to a physical
pixel, and middle-button motion did not suspend the delegates' static alignment.
At low speed the regression recorded 14 stationary painted transitions out of 18,
followed by one-pixel jumps. Columns showed the same problem along its horizontal axis.

The viewport now follows fractional scroll positions while an animation runs.
Middle-button motion participates in the shared alignment suspension, including
the dead-zone lifecycle. Stopping restores scene-space pixel alignment without
rounding the stored analytical scroll position.

Fallback raster icons use linear sampling during motion and their original
sampling at rest. Qt 6.11.1's native glyph shader also rounds positions, independently
of the viewport: `textmask.vert` rounds transformed vertices, and
`qsgdefaultglyphnode_p.cpp` selects nearest filtering. Panel captions and moving
group headings therefore temporarily use Qt's distance-field renderer when their
configured renderer is Native. Qt/Curve preferences are preserved, and Native
returns at rest. No Qt shader or library change was needed for this fix.

## Verification

Regressions drive an actual middle click followed by a 35-logical-pixel pointer
displacement. All five modes, with and without grouping, now have zero stationary
painted transitions. Requested and painted deltas agree within floating-point
precision. Native-renderer checks assert smooth icon filtering and scalable text
during movement. At rest they enumerate visible text/image leaves, test whole
physical-pixel scene origins and unit transforms at DPR 1.75, and capture output.
Small captions, secondary labels, group counts, icons, and clipping were inspected.

Final targeted results:

- Native motion, grouping, caption layout and header alignment: 23 passed.
- Software motion and alignment: 14 passed.
- Renderer preference/creation and gesture lifecycle: 6 passed.
- Native folder previews, Details fields, Columns text, SVG/raster branches and
  Masonry caption/icon checks: 14 passed.
- Isolated QML imports and production-host startup: 4 passed, none skipped.
- Generated embedded-payload/extraction tests and portable Windows dependency audit passed.

Artifacts use the `artifacts/middle-scroll-*` prefix. The broader panel suite was
not claimed as passing; earlier unrelated reorder/budget failures remain outside
this change.

## Live measurement

The diagnostic host uses the actual autoscroll controller and `FrameAnimation`,
with a displaced stationary pointer source; it does not substitute a scroll
NumberAnimation. QTest separately covers the mouse gesture. The folder was
`C:\Users\xs\Downloads`, 183 entries, density 152, actual DPR 1.75 on the 160 Hz
display. Each capture includes four seconds down and four seconds up. No builds,
tests or external UI observer ran during measurement. These are native frame and
PresentMon captures, not a newly collected QML-profiler trace.

Final slow run, `middle-final-low-masonry`: 636 + 637 displayed presents,
zero discarded presents, zero stationary moving transitions in 1,320 sampled
transitions. Display median was 6.25 ms; p95 was 6.2621/6.2619 ms and p99
6.2783/6.8657 ms. No displayed interval exceeded 9.375 ms. Caption measurement
count during the gesture was zero. The first upward motion frame leaves the
resting pixel-grid phase; subsequent deltas track the analytical position.

Final warmed fast run, `middle-final-fast-warm-masonry`: 1,271 displayed presents,
zero discarded presents, zero stationary moving transitions, and one displayed
interval over 9.375 ms. The first fast traversal after startup had 18 such
intervals while content/geometry warmed. This fixes recurring quantization;
it does not establish that every cold-content frame meets the 6.25 ms budget.

## Portable rollout

The normal static host was rebuilt, audited, compressed into the embedded payload,
and included in a CGO-disabled Go build using the system Go cache. The diagnostic
override was removed before the worktree app was restarted. The running extracted
host's SHA-256 matches the newly built host:
`88395dd94ef7c0a4e7749e7d2773a2bc47f1aa123e233ca21b824a9484a24583`.
The gzip payload and runtime cache key are
`7e2f38dd061cfb2dc6d2a5bab3d0b9a2e7bed7acf6c44172f5ccb0f3511c4584`.
Full identity is recorded in `artifacts/middle-scroll-final-build-identity.json`.
The canonical checkout's application was not replaced.

AI assistance has been used to create this output.
