# Masonry frame identity and actual pixel motion — 2026-10-29

## Status

The constant-160-Hz goal remains unproven. This is an intrusive diagnostic
capture, rejected by the existing PresentMon target-sample-count guard. It must
not be counted as a passing performance run or substituted for the cold-wheel,
verified-warm-wheel, and quarter-height-middle acceptance matrix.

The user authorized future captures without readiness or focus questions.

## Method

The current preprocessing-eligibility diagnostic host was extended with a
120×8 physical-pixel CRC frame marker at (16,16). Each marker records the native
motion-sample index immediately before and after rendering. The marker leaves
the renderer, scroll controller, animation clock, resource limits, source folder,
and acceptance thresholds unchanged.

A separate Desktop Duplication process copies the marker and a 32×384 BGRA strip
of the actual left-panel gallery at client physical (640,360), using one staging
copy/map. The raster buffer is bounded at 384 MiB and is written only after
capture. This observer consumes CPU/GPU resources; its timing is intrusive.

An owned DxgKrnl ETW session records driver refresh events concurrently. Before
and after topology agree: MSI MAG322UPF, DISPLAY1, adapter LUID 87088, source 0,
target 200965, 160/1 Hz. The native packet reports actual DPR 1.75 and 191
Downloads entries. The normal application was preserved; only the diagnostic
launcher tree was stopped after capture.

## Verification

Native marker tests: 5 passed, including CRC corruption, external raster
calibration, actual-175% scene placement/unit transforms, rendered marker
inspection, and marker removal. The rendered calibration was inspected.
Import-isolation and Windows import audits passed in the diagnostic build.

The independent pixel matcher searches the full available vertical strip;
camera displacement does not choose the match or restrict its search. Texture,
peak-correlation, and remote-runner-up checks reject flat/repetitive imagery.
Synthetic checks cover five known integer shifts, flat imagery, and periodic
ambiguity. Fractional-shift accuracy has not yet been calibrated independently;
subpixel results below are estimates, not calibrated metrology.

## Observations

- 3,609 published marker records, within capacity; every before/after native
  motion-sample index agrees.
- 3,674 raster rows, 180,584,448 bytes, within the 402,653,184-byte bound;
  observer status zero, no overflow. Raw pixels were saved after capture.
- The two 10-second measurement windows contain 1,598 and 1,599 raster frames.
  All decoded markers are valid; all accumulated-frame counts are one; every
  adjacent marker ID advances by one.
- Independently identifiable image pairs: 1,104 and 1,107. Difference from the
  recorded native camera displacement: median 0.0865/0.0847 physical px,
  p99 0.7448/0.7891 px, maximum 0.8823/0.9149 px.
- Unmeasurable pairs remain explicit: weak matches 29/25, ambiguous matches
  94/85, low-texture pairs 362/372, carried poses 8/9. These are not passes.
- Driver ETW decoding: 1,301,930 events, 130,560 exported rows; zero lost events,
  lost buffers, property errors, information errors, or ProcessTrace errors.
  All 8,105 output refresh counters are consecutive. Driver-reported refresh
  periods in the windows: p99 6.2606/6.2606 ms, maximum 6.2727/6.3100 ms;
  none exceed 9.375 ms. Refresh cadence alone does not establish application
  frame delivery or physical scanout.
- PresentMon produced only nine target-process records instead of the required
  >3,000. The capture therefore failed its existing acceptance guard. The cause
  of this observer combination's missing PresentMon records is unresolved.

The pixels support the marker-to-native-camera association where independently
identifiable. They do not prove physical photon timing, resolve the excluded
image pairs, or retrospectively reclassify earlier motion-error failures.

## Evidence and remaining work

Compact evidence is frozen under
`profiler/experiments/masonry-frame-pixel-identity-2026-10-29`; large ETL, decoded
events, pixels, and executables remain at hashed external paths in its manifest.
The capture wrappers were subsequently hardened so an observer watchdog does not
skip cleanup of the owned PresentMon session; that failure path was not exercised
in this successful observer run. The frozen executed-wrapper variant retains
the original statement.

Next: independently calibrate fractional matching, associate captured IDs with
driver refresh ordinals without fitting away timing error, investigate the
PresentMon/duplication interaction, and rerun the unchanged full acceptance
matrix. No production optimization or portable payload was promoted by this
experiment. No commit or push was made.
