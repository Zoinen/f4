# Shared pixel-grid boundary discovery

## Change

Pixel-grid watched nodes now cache boundary identity and moving-owner discovery.
Registering an additional leaf reuses its ancestors' cached information. Dirty
traversal and nearest-scope selection use the same watched-node data. Scope
installation refreshes boundary identity and rebuilds affected chains;
reparenting invalidates moving-owner discovery. There is no global type cache,
new rounding, change to alignment arithmetic, or animation-frame fitting.

Implementation: `third_party/ZoinGallery/src/pixelgrid/PixelGrid.cpp`.
Regression: `physicalGridSharesBoundaryDiscovery` in
`third_party/ZoinGallery/tests/GalleryQmlInteractionTest.cpp`.
The documentation describes the invalidation contract and `boundaryChecks` counter.

## Targeted validation

At actual window DPR 1.75, all six QtTest results pass: initialization, integer
grid metrics, retained motion/static visual leaves, shared boundary discovery,
Qt-owned Flickable content, and cleanup. The new test registers 128 leaves under
shared ancestors and verifies exactly 128 boundary discoveries. It installs an
attached scope after descendant registration, reparents the caption across
boundaries, and checks final scene-space origins and unit vectors.

Rendered checks include visible small native Text, TextInput, TextEdit and a
raster image in the existing fixture, plus the new small-caption fixture.
Both captures were visually inspected. A blank white-on-white first version
of the new capture was caught during inspection; the fixture now sets a dark
background and asserts rendered pixels as well as capture existence.

The first test invocation accidentally multiplied the actual 1.75 desktop
scale by an additional `QT_SCALE_FACTOR=1.75`, reporting DPR 3.0625. That result
is retained as rejected double-scale evidence. The accepted final invocation
removes the override and reports DPR 1.75. No geometry tolerance was relaxed.

A/B/B/A registration microbenchmarks use the original linked core versus the
new PixelGrid object, with identical other objects. Each process collects 48
post-warmup samples, creating/alignment-registering/flushing 128 leaves under
12 shared ancestors. Registration medians per leaf are:

| Run | Baseline | Candidate |
| --- | ---: | ---: |
| First | 13.061 us | 5.527 us |
| Repeat | 13.263 us | 5.404 us |

Pooled median is 13.159 versus 5.470 us, a 58.4% reduction for this registration
workload. This is not a claim of a 58% whole-frame improvement.

The statically linked candidate host and import test were rebuilt using the
existing complete resource/plugin graph. Three installed-tree-isolated import
test results pass; the Windows dependency audit contains only system DLLs.
The existing GalleryPanel undefined-QPointF import warning remains in the log.
The ordinary embedded Go payload was not regenerated or promoted in this
diagnostic experiment; this is not a portable release completion claim.

## Live scrolling comparison

Cases: `real-scroll-pixelboundary-{a1,b1,b2,a2}-20261030-wheel-masonry`.
Both arms use frame-latency limit 2, bounded preprocessing cache and overlay
fast path. B changes the PixelGrid object. Actual 4K/160 Hz/DPR 1.75,
Downloads, 192 catalog entries, density 152, fresh isolated caches, and two
ten-second wheel windows are retained. No UI Automation or screenshots run
during gestures. Native foreground is stable in all eight windows, zero
inactive inputs. All four original capture-validity checks pass.

Target PresentMon rows are 3420/3410/3421/3418; delivered wheel events are
1682/1725/1745/1729. Input generation is matched, not delivered counts or
traversal distance. B1's initial native guard still expected the baseline host
path and rejected the candidate before any gesture. The guard was corrected
to verify the exact candidate path, then released the same running capture;
its startup handshake therefore took longer. B2 is a separate repeat. This
startup difference must not be hidden as identical preparation timing.

| Arm/window | Intervals >9.375 ms | Maximum interval | Maximum motion error |
| --- | ---: | ---: | ---: |
| A1/0 | 46 | 25.0126 ms | 276.60 px |
| A1/1 | 50 | 18.7127 ms | 138.68 px |
| B1/0 | 42 | 12.7494 ms | 129.28 px |
| B1/1 | 40 | 18.8123 ms | 113.43 px |
| B2/0 | 39 | 12.7362 ms | 122.66 px |
| B2/1 | 49 | 25.0251 ms | 109.14 px |
| A2/0 | 49 | 18.7383 ms | 132.47 px |
| A2/1 | 47 | 12.7095 ms | 114.88 px |

All coverage holds are zero. Every window fails the original display and
maximum-motion gates. Median present-to-display latency remains similar:
16.79–17.15 ms baseline, 16.73–17.22 ms candidate. B2 also has a 10.5982-ms
preparation outlier, so this optimization does not bound scene-preparation
cost. Do not infer a robust whole-view improvement from pooled miss counts.
These are cold launches; the second direction is not verified-warm coverage.

## State and next work

The source optimization and regression remain in the worktree; there is no
commit or push. Existing dirty changes were preserved. Compact sources,
scripts, logs, validation and hashes are frozen in
`profiler/experiments/masonry-pixel-boundary-cache-2026-10-30/`.
Large packets, CSVs, PNGs and executables remain at their original paths,
pinned by the manifest. The owned display hold was explicitly released.

Constant 160 Hz remains unachieved. Next optimization targets the measured
render-list sequence/hash construction and material/upload costs. The separate
image connection-lock wait still requires object/readier attribution before
a safe fix. Verified-warm wheel and quarter-height middle coverage remain
required, along with the original cold gates.
