# Native scroll thumb, 2026-10-20

Fixed a native-motion rollout gap: panel contents advanced on the render thread
while the scrollbar thumb remained at its last GUI position during a GUI pause.
The vertical thumb now follows the same camera. Pressing it commits the latest
camera and stops animation before the control handles the press. The verified
embedded worktree build is running; native motion remains opt-in. Constant
160 Hz, startup, and the remaining lifecycle rollout are not declared complete.

## Regression before correction

At DPR 1.75, a 120 ms GUI pause allowed the native camera to advance while the
thumb's actual QSG transform moved 0 physical pixels instead of 10.5538. The
test failed on Direct3D 11 and OpenGL before the implementation change.

After adding camera-following, a second regression showed that pressing the
moving thumb could leave native motion running. It failed on both backends
before the event-filter correction. The tests measure rendered transform nodes,
not only GUI properties that are intentionally stale while GUI delivery stops.

## Implementation

GalleryScrollAnimation accepts the vertical scrollBar control. During postSync,
while GUI state is locked, it captures the thumb node, its ordinary transform,
the logical camera anchor, and Qt's logical-to-visual thumb mapping. The mapping
accounts for minimumSize through visualSize, availableHeight, and logical size.
Render frames reuse that matrix and ratio without reading GUI properties,
measuring captions, traversing layout ancestors, or adding rounding operations.

Stopping dirties the thumb's ordinary item transform so Qt restores normal
positioning at the next sync. Mouse press and touch begin first stop native
motion with the latest camera committed, before Qt evaluates the control's
logical thumb geometry. Mode/source/size changes keep the existing cancellation
and fallback paths. The QML panel supplies its existing vertical control; no new
visual wrapper, source file, module, or setting is introduced.

This fixes thumb rendering and press handoff. It does not eliminate all GUI
contentY publication or prove the whole native lifecycle ready for default use.

## Verification

- Camera suite: 12 results/backend, zero failures/skips on D3D11 and OpenGL.
  Includes proportional and minimum-size thumbs, stop-on-press, hit testing,
  metadata rewrap, shrinking geometry, retargeting, resize/mode/source fallback,
  SVG/raster culling, and independent scene branches.
- Final D3D11 thumb deltas: 11.2135 physical pixels proportional and 2.14424
  minimum-size, matching their expected camera-derived movement. OpenGL also
  matches its expected deltas.
- At rest the actual galleryScrollBarHandle leaf has whole physical-pixel
  scene origins and dimensions, and scene unit vectors remain unit vectors.
  Rendered rest images equal ordinary GUI-positioned reference images at 175%.
  Captures were inspected; the actual thumb leaf is checked, not only a wrapper.
- Session suite: 12 results/backend, zero failures/skips, including all five
  panel modes at 175%, vector reentry, background-free viewer swaps, gradients,
  transparency, and embedded images.
- Software fallback: 3 results, zero failures/skips.
- Resource-only QML import isolation, Windows system-DLL audit, and embedded
  host extraction tests pass. Go uses CGO_ENABLED=0 and the system build cache.

A first build-helper edit happened while cmd was still reading that helper;
the command exited unsuccessfully. The stable helper was rerun to completion
before any changed binary was tested. The normal working application stayed
alive throughout builds and tests. No failed build was used for timings.

## Scrolling measurements after correction

Two fresh-process diagnostic captures, each 40 seconds of input: Downloads,
186 entries, Masonry density 152, 160 Hz/DPR 1.75, wheel every 30 ms, boundary
bounce, default 5000 us GC budget and compiled 16 MiB geometry pools. Native
motion is explicitly enabled for measurement, diagnostic pixmap cache is 2 MiB.
The ordinary collector is used, with no GC phase replacement in these captures.
No builds, tests, UIA, platform tracing, or offline analysis run during input.

Long means greater than 9.375 ms, not the stricter 6.25 ms nominal frame period.
Producer and displayed intervals are separate measurements.

| Run | Input seconds | Long producer intervals | Long displayed intervals | Max display gap |
| --- | ---: | ---: | ---: | ---: |
| First, cold | 20 | 7 | 1 | 12.4787 ms |
| First, warm | 20 | 0 | 0 | 6.4157 ms |
| Repeat, cold | 20 | 4 | 0 | 6.4376 ms |
| Repeat, warm | 20 | 0 | 0 | 6.3855 ms |

The first cold miss occurs about 734 ms into scrolling, not at the window
boundary. It is retained. Camera coverage stalls are absent in the first
capture's 6399 within-window adjacent motion pairs. A single repeated run with
zero misses does not establish constant 160 Hz or quantify the isolated cost
of thumb following versus system variation. Native startup and clean alternating
longer comparisons remain necessary before default rollout.

## Deployment and evidence

Rebuilt Qt host SHA256:
`42532f5f47cc488131e9cebe267039cb591974c59fdfbc4e34032886d0ddaccc`.
Rebuilt launcher SHA256:
`76e899cb2e4796ddab8353309b0a7d1eca6b36401ec16e68765417f64c43a871`.
The live content-addressed child matches the host and its window is responsive.
No host override is used. Only exact-path worktree/diagnostic launcher trees
were terminated; the canonical checkout executable was untouched.

The original implementation is preserved in the earlier prepared-SVG archive.
The accompanying experiment manifest retains current sources, regressions,
before/after logs, rendered images, actual-link identities, native input data,
PresentMon timing, analysis, and deployment receipt. An older large CSV was
losslessly compressed and hash-verified for disk space before new builds.
No commit or push was made. The goal remains active.

AI assistance has been used to create this output.
