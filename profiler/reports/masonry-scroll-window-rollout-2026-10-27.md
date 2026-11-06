# Masonry look-ahead: motion verification and portable rollout

## Current result

The bounded directional look-ahead change is now in the normal worktree app,
`D:\Code\f4-zoin\f4-zoin.exe`, launched directly with `--gui=qt`. The app freshly
extracted the matching embedded host into the normal user cache. No diagnostic
controller override, development host override, Qt path override, scale override
or renderer override was used for that launch. The extracted Qt window is
responsive. This is progress toward constant 160 Hz, not completion of the full
goal across cold loading, longer runs and broader workloads.

The previous turn was progress: it reproduced an actual camera hold, added a
failing-before regression, implemented conserved-budget preparation, and obtained
passing rapid-wheel and quarter-height middle-scroll captures. This turn
revalidated the current state, closed the saved middle-motion analysis gap,
captured it again on the actual 160 Hz / DPR 1.75 desktop, and rebuilt and
launched the coherent portable worktree build.

## Middle-scroll displacement

The new analyzer matches each native motion sample to its render-frame bracket
and the PresentMon event inside that frame's render-end/swap interval. It uses
the recorded continuous-job velocity and fixed-displacement reversal revisions.
Revision counts are checked against actual recorded turns. Consecutive samples
must have consecutive presentation events. It compares actual rendered camera
displacement with both elapsed displayed time and elapsed native animation time.
Only pairs crossing a deliberate reversal or changing composite geometry are
excluded; coverage holds remain independently counted and prohibited.

The strict gates were unchanged: zero coverage holds, no displayed gap above
9.375 ms and maximum displayed motion error at most two physical pixels. The
native velocity consistency check additionally requires error at most 0.002 px.
This is camera-transform evidence; it is not a physical monitor scanout capture.

| Quarter-height middle capture | Display gap p99 | Display gap max | Displayed velocity error max | Native velocity error max | Holds / missed refreshes |
| --- | ---: | ---: | ---: | ---: | --- |
| Oct 23, block 0 | 6.2963 ms | 6.5111 ms | 0.74374 px | 0.00073 px | 0 / 0 |
| Oct 23, block 1 | 6.2837 ms | 6.3737 ms | 0.35228 px | 0.00055 px | 0 / 0 |
| Oct 27, block 0 | 6.2798 ms | 6.3982 ms | 0.42444 px | 0.00055 px | 0 / 0 |
| Oct 27, block 1 | 6.2719 ms | 6.3675 ms | 0.33455 px | 0.00055 px | 0 / 0 |

Both captures use 296.5 logical pixels of pointer displacement for an
1186-high window and include active reversals near the directory ends.
The new capture is `real-scroll-coverage-leading-quarter-20261027-middle-masonry`.
It confirms actual display refresh 160 Hz and DPR 1.75, not a forced scale or
hard-coded result. The analyzer validates active-window/input checks and uses
the existing source path and bounded records. No builds, tests, UIA polling or
offline analysis ran during its input blocks. The diagnostic was deliberately
launched because the process inventory showed no surviving prior F4 process;
there was no timeout-triggered restart.

The four earlier rapid-wheel candidate blocks retain their strict verification
from [the prepared-window report](masonry-prepared-window-holds-2026-10-23.md).
This turn did not reinterpret missing PresentMon files as successful captures.

## Coherent production build and checks

The normal CMake build rebuilt/linked `f4-qt-host` against the current archives
and pinned production Qt libraries. Its actual expanded link inputs were
inspected and recorded. The expected Qt6Quick and Qt6Qml archive hashes match
the validated production patches; no diagnostic coverage/profile objects were
inserted into this host.

Checks after building:

- Layout engine: 31 passed, no failures or skips, linked against the core
  archive **without** the diagnostic materializer object override. This
  confirms the measured-row regression is fixed in the actual archive.
- Native scene motion/retention: 7 passed on D3D11 and 7 on OpenGL.
- Component publication/staging: 5 passed on D3D11 and 5 on OpenGL.
- Required `F4QuickViewSurfaceTests` import-isolation/compiled-host checks:
  4 passed, no skips, testing the newly built production host.
- Windows dependency audit: only system DLL imports.
- Embedded payload: decompressed hash equals the built host hash.
- Go build: `CGO_ENABLED=0`, `f4_embedded_qt_host`, Windows GUI subsystem,
  system cache `C:\Users\xs\AppData\Local\go-build`.
- Payload/extraction tests: generated payload integrity, extraction/reuse,
  concurrent first launch, corrupt payload rejection, and embedded-host lookup
  priority passed in `internal/plughost`.
- Parent and submodule `git diff --check` pass; only line-ending warnings.

The known Windows response-file single-line truncation affected the surface
test target. Its exact generated arguments were split across lines and linked
again; the required tests above ran from that resulting executable. The layout
test relink still adds the existing missing system Uniscribe dependency for
static HarfBuzz. No portable runtime was replaced with a directory bundle.

The packaging receipt initially contained case-variant duplicate Windows SDK
paths (`Dwmapi.lib`/`dwmapi.lib`), which Windows PowerShell rejects as JSON keys.
All paths/hashes were reverified, equal duplicates collapsed, and the receipt
producer corrected. No binary changed during that metadata correction. The
final receipt pins 448 distinct input paths.

## Live portable identity

- Launcher PID: 43936; extracted Qt PID: 119460.
- Launcher SHA256:
  `5023071d267153cc2bb1ee1a7ac7119f47a6849fd586c9de93a968eac97bdc31`.
- Qt host SHA256:
  `3dc3a3473dfd84fea61016b0deb4df45073e4c5fb4fa8f8ba591aabc62358354`.
- Fresh cache generation:
  `690c41f0a25eec5f68faafc84cb21d1082971fee44373392bbe705e353e7c68b`.
- Extracted host:
  `C:\Users\xs\AppData\Local\f4\qt-host\690c41f0a25eec5f68faafc84cb21d1082971fee44373392bbe705e353e7c68b\f4-qt-host.exe`.
- Previous working launcher retained as
  `artifacts/f4-zoin-before-scroll-window-20261027.exe`, SHA256
  `26eb402bce3628bbc34a80ad62932eec3ca1e7244c09c076dbb3e1b0e47720ef`.

The restart checks the complete build receipt, verifies the extracted host's
hash/path and a responsive shown window, and has a rollback to the saved
launcher if startup fails. It stopped only the owned diagnostic after the
normal replacement was verified. It did not touch `D:\Code\f4\f4.exe` or
another checkout. Startup stderr is empty. No commit or push was made.

## Correction to the previous build description

The Oct 23 report described the diagnostic candidate as a materializer object
override ahead of an unchanged core archive. Inspection of the actual build log
shows that CMake also rebuilt the core archive during that test build. The
candidate did explicitly link the new object, but the archive was no longer
the old hash. Therefore the old nested baseline receipt alone does not prove
an unchanged archive. This turn verifies the coherent archive directly,
records the normal host's expanded link inputs, and verifies the regression
without any object override. The saved measurements remain observations of
their pinned executable; do not use the inaccurate archive description for
causal attribution.

## Remaining scope

The normal app now includes the current native publication/staging changes as
well as the look-ahead fix. Earlier measurements identified a steady-work
tradeoff in staging. The successful captures establish that these tested
workloads meet their frame/motion gates; they do not prove that the tradeoff
is harmless on every cold or more complex workload. Existing unrelated full
UI-suite failures are not claimed fixed or covered by the focused green tests.

Next verification should include reversed run order, longer rapid/middle runs,
larger/heavier directory workloads, startup scrolling while decoding, and cold
SVG preparation. The screenshot/leaf alignment checks from the pixel-grid
work remain applicable; this fix changes request-window arithmetic, not static
visual transforms or dimensions. Physical presentation and general no-spike
completion remain unproven outside the measured cases. The full goal stays
active.

Evidence snapshot:
`profiler/experiments/masonry-scroll-window-rollout-2026-10-27/`, with source,
raw new capture, analysis, native/import test logs, build/package/extraction
receipts, live identity and a verified SHA256 manifest. Executables, object
files, generated payload and SDK archives are excluded from the snapshot and
identified by hashes instead.

> AI assistance has been used to create this output.
