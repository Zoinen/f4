# Complete driver captures and gallery mapping integration

## Status

Constant 160 Hz without spikes remains unachieved. This continuation completes
the driver trace that was truncated in the preceding turn, obtains a separate
capture without Desktop Duplication, and integrates the previously measured
parent-mapping optimization into the gallery source. No animation-clock or
acceptance-threshold change, commit or push was made. The normal application
and its embedded payload remain the last working build; the new integration
host is diagnostic, not a portable rollout.

## Complete image/driver repeat

`real-scroll-postsync-presentationdriverpixels-b2-20261031-wheel-masonry` uses
the unchanged current marked host and full normal-window workload: actual
Downloads, 192 entries, all-entry warm tour, Masonry density 152, two 30-second
rapid-wheel windows, actual 3840×2160 /160 Hz /DPR 1.75.

The new sequential trace has a 1 GiB cap on C: with preflight headroom, rather
than the old 256 MiB cap on D:. It completes at 419,692,544 bytes. All 3,674,020
events decode with zero lost events, lost buffers, metadata errors, property
errors or ProcessTrace errors. All 19,441 selected driver refresh numbers are
consecutive; each measurement window contains 4,800 consecutive refresh records.
Before/after display-path queries agree. The old failed/truncated run remains.

| Image/driver quantity | Window 0 | Window 1 |
| --- | ---: | ---: |
| Produced markers / unique native-frame submission joins | 4,800 | 4,801 |
| Captured image rows | 4,800 | 4,800 |
| Identifiable image pairs | 2,845 | 2,827 |
| Image vs native displacement median px | .09364 | .09837 |
| p99 px | .79530 | .78471 |
| Maximum px | 1.03872 | .98722 |
| Original display-tracked target rows inside window | 1 | 0 |

All unmeasurable pairs remain counted: window 0 has two without a motion sample,
78 weak, 1,673 low-texture, 167 ambiguous and 34 carried-pose pairs; window 1
has 1,677 low-texture, 180 ambiguous, 87 weak and 28 carried-pose pairs. The
existing 16-pixel synthetic fractional calibration remains the reference;
it is not an error bound for arbitrary gallery content.

The temporal rule associating desktop ImageQpc with the preceding driver frame
timestamp produces eight non-unit refresh associations. All eight corresponding
captured image-ID steps are one. This reinforces that temporal association
alone cannot establish which physical refresh contains a captured image.
Separate image-ID anomalies remain: window 0 has one repeat and one step of two;
window 1 has consecutive IDs throughout. They are retained, not excused away.

Every capture row has AccumulatedFrames=1. Desktop-global masked-content flags
remain one. The narrow visible strip, its valid CRC and measured content do not
prove that every application pixel is visible or establish optical scanout.
Copy/map medians are 1.49035 /1.49275 ms, maxima 2.4330 /2.7466 ms. The observer
is intrusive. The original display sample guard still fails; neither submission
records nor the partial set of identifiable pixel pairs become a passing gate.

## Separate capture without desktop readback

`real-scroll-postsync-presentationdriverclean-a1-20261031-wheel-masonry` retains
the same full workload, marked host, settings, window mode and frame latency 2,
but does not launch Desktop Duplication. It records a complete 303,300,608-byte
driver ETL with zero loss/decoding errors and 19,171 consecutive refresh counters.
The standard display-tracked PresentMon stream recovers 10,084 target records;
the normal capture-validity checks pass. The strict performance check fails.

| Original acceptance metric | Window 0 | Window 1 |
| --- | ---: | ---: |
| Preparation p99 ms | 1.6862 | 1.8165 |
| Display interval maximum ms | 6.4623 | 12.5047 |
| Display intervals over 9.375 ms | 0 | 2 |
| Motion-error maximum physical px | 3.59214 | 3.52862 |
| Coverage holds | 0 | 0 |

Every measured reported display timestamp associates with a selected DPC
event-header timestamp within 10 microseconds. The two over-budget intervals
in window 1 also advance driver refresh ordinals by two. They remain genuine
failures of the original display gate; the repeated driver's refresh itself
continues normally. No physical scanout claim is made.

At 19,000.296 ms and 21,212.7826 ms into window 1, reported render-completion
times lead the preceding driver's refresh timestamp plus one nominal 6.25 ms
period by 12.0679 /11.9677 ms. Native submission finishes 17.7682 /17.0488 ms
before that nominal refresh. Preparation takes .3518 /.5976 ms and recording
.2819 /.4949 ms. This is reporting/phase attribution, not a directly observed
DWM latch deadline or a new performance pass. It does not identify the exact
compositor scheduling cause in this trace.

For the largest motion error in window 0, the reported display step is 6.4473 ms
while its associated driver frame timestamps differ by 6.2504 ms; window 1's
largest error has 6.4860 ms versus 6.2520 ms. Both animation steps are 6.25 ms.
The original motion errors stay failed. The evidence argues against changing
the animation clock to follow notification jitter. It does not reclassify all
previous misses or make an easier driver-time gate the acceptance criterion.

## Source integration and checks

`GalleryItemMappingScope.h` now implements the tested bounded, per-pass mapping
reuse in the gallery itself. `GalleryScrollAnimation::postSync()` uses it for
delegate and vector-gate bounds. It reuses up to 32 parent mappings among
translation-only siblings; other leaf transforms, incompatible windows and
capacity overflow use Qt's full mapping. No pointer, geometry or transform
cache survives the GUI-locked synchronization pass. No new rounding, layout,
source-decoding or animation-timing behavior is introduced.

The helper is enabled directly, with no profiling environment variable or
global atomic counters. Regression coverage lives in the existing actual
`GallerySceneItemMotionTest.cpp`: 150 siblings need one ancestor mapping;
transformed ancestors and leaves match Qt; a new pass refreshes geometry and
reparenting; overflow stays bounded and preserves geometry. The original
prototype had already failed the repeated-mapping budget with reuse disabled;
its live component measurements are in the per-pass parent-mapping report.
This source port has not yet been live-benchmarked as a replacement portable app.

Verification:

- The actual production scroll-controller source compiles with the new helper.
- A diagnostic controller retaining the existing probes uses that same helper.
- 46 native QtTest results and 11 motion/mapping QtTest results pass without
  failures or skips, plus three isolated QML-resource import results.
- All 30 rendered native/motion PNGs are byte-identical to the prior tested
  integration at 175%. The small-caption/thin-stroke image was inspected.
- The newly linked marked diagnostic host passes the Windows system-DLL audit.
- The first diagnostic-controller compile found a mismatched old probe header
  through include-path lookup. Copying the two exact base-local probe headers
  fixes that diagnostic assembly. The first failed log is retained. Actual
  production source compilation had already succeeded before that correction.

The source/test files already existed as untracked gallery work in this large
checkout. They remain unstaged; no empty git-diff result is used as proof of
their validation. The new helper is also untracked. Existing unrelated edits
are preserved. `docs/PIXEL_GRID.md` describes the scoped mapping lifetime; its
pre-existing byte encoding was preserved during the small ASCII insertion.

## Remaining work and final state

The clean warm result still fails the original display and two-pixel motion
requirements. Cold loading and quarter-height middle motion have not acquired
a new full pass. The source integration still needs portable-host/payload
rollout and repeated full-scope verification. Its small measured synchronization
saving is useful headroom, not a demonstrated cure for the two early-ready
delivery misses.

All owned capture applications, foreground/desktop helpers and named ETW
sessions are terminal. The normal Go PID 23488 and Qt PID 84780 remain alive.
No external workload, affinity, OS memory/storage policy or installed Qt SDK
was changed. Twenty additional closed owned diagnostic hosts were compressed
with byte hashes unchanged, preserving the first storage receipt separately.
The new regression executables ran; the newly linked diagnostic host was not
launched as the user's normal app. No portable binary is claimed complete.

Evidence is under `artifacts/presentation-driver-pixels-b2-20261031`,
`artifacts/presentation-driver-clean-a1-20261031`,
`artifacts/presentation-driver-clean-20261031/a1` and
`artifacts/gallery-parent-mapping-port-20261031`. Complete driver ETLs stay at
their recorded owned C: paths, with hash-preserving compression receipts;
complete packets and CSVs stay under `artifacts/modes-profile`.

AI assistance has been used to create this output.
