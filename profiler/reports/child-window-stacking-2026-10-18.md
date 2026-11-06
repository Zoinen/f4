# Native child-window stacking during masonry scrolling

## Diagnosis

A lean ETW scheduling capture, scheduling-cold-wheel-masonry, distinguishes
ready-to-run delay from scheduled execution. Render ready latency p99 is
0.008 ms, maximum 0.025 ms; GUI p99 is 0.023 ms, maximum 0.181 ms. In the worst
15.7289 ms native period, the GUI is scheduled for 15.3099 ms while the render
thread waits for 10.355 ms. Another 11.426 ms render interval is entirely
scheduled-running. These times include kernel/interrupt execution, not just
user CPU. The largest recorded interrupt is 272 us, clock calibration drift
125500 ns, and the trace reports no lost events or buffers. These data do not
support raising thread priorities as a remedy for the multi-ms stalls.

A separate fresh sampled capture, schedule-symbols-cold-wheel-masonry,
resolves local host/Qt symbols: 1902 GUI samples with 1898 matched stacks and
3178 render samples with all stacks matched. GUI inclusive counts include
MasonryLayout::prepareNearbyDelegates 456, Qt's child-window stacking update
165, prepareDelegateRow 119 and popBrickItem 100. These are inclusive samples,
not mutually exclusive timing totals. Several long GUI intervals contain
6–7 samples in the stacking walk. The walker visits every QQuickItem and
casts it to QQuickWindowContainer, even when no native child window exists.

The first sampled capture omitted Loader events and cannot establish symbolic
attribution. A warm repeat has a different extent and is not substituted for
the fresh capture. Remote symbol decoding was stopped after confirming the
owned export process was stalled fetching symbols; the completed export uses
local PDBs from the same saved ETL. Partial export files are not evidence.

## Production change and source audit

Before starting the recursive walk, check QGuiApplication::allWindows() for
any window whose parent is the current Quick window. With none, return. With
any, execute the original recursive walk and stacking behavior unchanged.
This preserves the original contained-window parent condition and avoids
per-item work in ordinary panels. No layout, rounding, thread priority, Qt
version or class layout changes are introduced.

The source patch is embedded in ci/patch-qt-dependencies.py, applied after
Conan source patches, with exact single-anchor validation and idempotent
recipe insertion. Existing CI cache keys already include this script.

A cached source directory proved to contain Qt 6.10.1, so its incompatible
qquickwindow.cpp prototype was rejected. The compiled replacement instead
uses the official Qt 6.11.1 source and matching installed private headers.
The generated recipe reproduces that replacement after newline normalization.
Applying the existing renderer recipe to official 6.11.1 batch-renderer source
also reproduces the integrated renderer after newline normalization. The
rejected build did not replace a working library or app.

## Clean display measurements

A uses the preceding normal stable-interval Qt library; B adds only the
stacking guard. Both use identical gallery sources and harness. Each fresh
process uses Downloads with 184 entries, density 152, extent 4161.142857,
actual DPR 1.75, active window and 160 Hz display. Wheel events are 120 units
every 30 ms, about four seconds each direction. State is restored between
processes. No build, tests, sampler, ETW recording, expensive observer or
diagnostic logging runs during clean captures. Caption measurements are zero.
PresentMon display-change intervals are the headline.

| Run | Down p99 ms | Down maximum ms | Down >9.375 ms | Up p99 ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| A1 | 12.5030 | 18.7472 | 16 | 6.2867 | 2 |
| B1 | 6.3643 | 25.0144 | 6 | 6.3260 | 2 |
| B2 | 6.3205 | 18.7493 | 3 | 6.2842 | 1 |
| A2 | 12.4940 | 24.9982 | 8 | 6.3073 | 3 |

Both pairs improve downward p99 and long-interval counts. Maximum spikes
remain, including a worse B1 maximum than A1. Constant 160 Hz is not achieved.
Local capture stems are child-stacking-{a1,b1,b2,a2}-wheel-masonry; compact
evidence is artifacts/child-stacking-summary.json. Generated traces and binaries
are excluded from the source checkpoint.

## Verification and rollout

- Native regression before: 500 meta-object queries versus expected zero,
  with the native-child control passing (child-stacking-before-175.txt).
- After: 5 results pass on Windows D3D11 and 5 on OpenGL at DPR 1.75.
  Actual native container stacking, origins and unit transforms are checked.
  Offscreen has 4 passes and one expected native-platform skip.
- Recipe tests: 23 pass, including idempotence, composition, source drift and
  ambiguous-anchor rejection without saving.
- Focused SVG checks: 26 pass on normal Qt with explicit upload-gate capability
  absence, including all five panel modes, cache reuse, resize, revision,
  cancellation, animation and swaps (child-stacking-gallery-175.txt).
- Import isolation and compiled-host loading: 4 pass. Windows DLL audit and
  tagged Go payload/extraction/materialization/lookup checks pass with CGO off
  and the system Go cache. Cross-platform release gates were not run here.

The newly embedded normal host includes stable batch retention, idle vector
geometry preservation and the stacking guard. Experimental upload admission
and combined unmerged retention remain disabled. The full project suite is
not claimed green: experimental native rows require the prototype libraries,
as documented in the adjacent experiment reports.

Only this worktree's process tree was replaced; the canonical checkout was
untouched. The app launches directly with --gui=qt, no diagnostic override,
and title f4 [zoin_branch/cached-pixel-grid]. The running extracted host hash
matches the new host; startup stderr is empty. Evidence is
artifacts/child-stacking-normal-identity.json:

- Host SHA256: 774a5fc4add840ec3b826fe5e0aa49caa48fa337ffae14cb505531ed6abe5a3a
- Launcher SHA256: 23c95deca4a70d9ea645194be578497ab7244bf0e6006a3baf581f870d81c724

Next investigation should measure delegate preparation and remaining render
resource work. The larger 160 Hz objective remains unfinished.
