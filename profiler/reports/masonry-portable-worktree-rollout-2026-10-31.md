# Portable worktree rollout after the mapping integration

## Outcome and goal status

The actual worktree application has been rebuilt and restarted from its newly
embedded Qt payload. The normal CMake graph now contains the source mapping
optimization and the running-thumbnail ownership fix; it does not inject the
later diagnostic object overrides. The previous working Go executable and
payload are preserved. No commit or push was made.

Constant 160 Hz without spikes remains unachieved/unverified. This is a build,
packaging and runtime integration result, not a new scrolling performance pass.
The preceding diagnostic graph still had failed display/motion gates; the
newly deployed production graph needs its own full cold/warm/middle measurements.
Some later low-level experiments remain outside that production graph.

The preceding turn made progress through complete driver evidence and source
mapping integration. This turn makes progress by replacing the old running
application with a tested single-file build and fixing two defects exposed by
the real build path rather than the diagnostic linker recipes.

## Real build graph

The existing Release /static-MSVC CMake graph is used, with Qt 6.11.1 and its
verified locally patched Quick/Qml archives. No installed Qt SDK files were
modified. The normal graph already includes the precise animation-clock
capability; it was not missing merely because an initial directory search did
not find the ignored/untracked policy file. Explicit source inspection confirms
the recipe and imported archive selection.

Fresh CMake targets include the host, Quick View import tests, gallery motion
tests, gallery session tests and Windows application-icon tests. No profiling
controller/main/renderer object is inserted into this application link.

## Build defects found and fixed

### MSVC response-file handling

The real Quick View test target failed with a truncated object filename.
The final response-file line contains 33,284 characters. The first reproduction
accidentally rewrote its original copy through text-mode output, converting LF
to CRLF; both copies then linked. That failed hypothesis/receipt remains.

A byte-controlled repeat distinguishes the cause without changing arguments:

| Response form | Maximum line | Link return code |
| --- | ---: | ---: |
| Original bytes, LF | 33,284 | 1181 |
| Same arguments and line length, CRLF | 33,284 | 0 |
| One argument per line, LF | 194 | 0 |

`ci/msvc_link_response.py` now runs as the MSVC C++ linker launcher. It normalizes
temporary response copies to CRLF, preserves argument content/order and quoted
paths, forwards missing inputs and child exit codes, and removes temporary files.
It preserves UTF-8 and both UTF-16 byte orders. CMake-generated response files
are not rewritten. The old host-only PRE_LINK splitter is no longer used; its
historical source remains. Six helper regression tests pass.

The Windows icon test also exposed a pre-existing incomplete dependency closure:
non-player static tests still link Qt Multimedia when video is enabled, but
omitted the FFmpeg targets' iconv dependency. The common static-test helper now
carries those existing codec targets, rather than naming a private library.
The actual target then links successfully. Its initial failed log is retained.

### Extra Windows console

The first packaged smoke candidate rendered the UI but exposed a second native
window: its Qt host was a console-subsystem executable. A new test in
`F4WindowsApplicationIconTests` reads the actual host's PE header. It fails
before the fix with Actual=3, Expected=2.

The host target now uses `qt_add_executable(... MANUAL_FINALIZATION ...)`, keeps
the existing explicit Qt finalization, selects WIN32_EXECUTABLE and retains
`main(argc,argv)` through mainCRTStartup on MSVC. The regression passes after
the rebuild. The new single-file application exposes one main window, with no
additional host console. Existing QML resource aliases remain unchanged.

`docs/PORTABLE_BUILD_POLICY.md` documents these build/runtime checks. No display,
animation, rounding, acceptance threshold or OS scheduling policy changed.

## Verification of the final native and Go artifacts

- Actual CMake executables pass 11 gallery motion/mapping, five thumbnail
  cancellation, three isolated-resource import and three GUI-subsystem QtTest
  results, with no failures or skips in the selected cases.
- The five rendered motion PNGs remain byte-identical to the prior integration
  at actual DPR 1.75. The source change does not alter a static UI hierarchy.
- The actual host passes the Windows system-DLL/static-runtime audit.
- The compressed payload is regenerated from the final host; decompression
  hashes to that exact native executable.
- The tagged Go candidate is built with CGO_ENABLED=0 and the system Go cache.
  The parent D:/Code/go.work excludes this worktree, so GOWORK=off is scoped to
  these build/test child processes. The parent workspace file is unchanged.
  The final recorded toolchain is go1.26.6 and cache is
  C:/Users/xs/AppData/Local/go-build.
- Five Go extraction tests pass, covering the generated payload, reuse, eight
  concurrent first-launch callers, corrupt data, upgrade separation and preference
  for embedded hosts over development builds.

Native/runtime tests are local Windows checks. They do not constitute a new
Linux/macOS/Windows-version release-matrix certification.

## Packaged launch checks

A copied single Go executable launches with --gui=qt from a fresh test directory.
There is no sibling f4-qt-host.exe and F4_EXT_UI_PATH is absent. Its own portable
Go settings profile keeps this smoke run separate from the user's normal core
profile. The host extracts into a fresh, explicitly recorded cache and hashes
to the rebuilt native host. Native window observations confirm the actual UI,
including both Downloads panels and the runtime worktree branch title.

The second launch reuses the same host without changing its mtime. Its private
cache-generation qt.conf points Qt's lookup prefix at an absent SDK directory;
Qt/Conan paths are removed from PATH and inherited import/plugin overrides are
cleared. The UI still renders and the dedicated import test independently uses
qrc-only import paths. This is an installed-tree-isolation check, not a fresh VM
or proof from an empty log: import-debug environment flags reached the child,
but its stderr log was empty, so no zero-file-read claim is made from that log.
No installed SDK or capture protection was changed.

The fresh smoke profile offered an ordinary application update. It was not
installed. No external software was downloaded or used as the tested build.
The first console-subsystem candidate, its screenshots and the failed
regression remain separate from the final GUI candidate.

## Normal worktree restart

Only the owned tree for D:/Code/f4-zoin/f4-zoin.exe was terminated, after the
replacement passed validation. D:/Code/f4/f4.exe and other worktrees were not
touched. The previous running binary was copied and hash-checked first.

Current normal application:

- Go PID 103648: D:/Code/f4-zoin/f4-zoin.exe.
- Qt PID 17748: the default user-cache generation
  b46c77f26c0eec51bc9d1e132bedebae393cc1ab61c7fb28d86276498fd98126.
- Go SHA256: 9faee5d2444e1a81b6b3a4a2dcfb3f253b7ad2b34c7ab808737d4c9a30ad7976.
- Native host SHA256: cd01b63ff07e92951d2ffcebe65e04a7ecb946f9c539d54c9195a4f0478eb63b.
- No explicit host override, private test qt.conf, profiling flags or debug
  import flags are used in this normal launch.

The native main-window observation confirms restored tabs, Downloads on the
left, the existing P: photo directory on the right, and the dynamic worktree
branch title. The app is left running and active. The display probe reports
the local MSI MAG322UPF at 160 Hz /DPR 1.75, with no remote session.

The new authoritative process receipt is artifacts/normal-qt-current-identity.json.
Older diagnostic scripts hard-code the previous PIDs; they must not be blindly
reused for cleanup after this restart. The previous PIDs 23488/84780 are terminal.

## Evidence and remaining work

Artifacts are under portable-parent-mapping-rebuild-20261031, with the final
GUI build and runtime receipts in gui-v2. Original and corrected response
reproductions, failed links, the workspace-module build failure, baseline
subsystem failure, all actual tests, packaging hashes and screenshots remain.
Only completed owned diagnostic binaries were compressed to restore space;
hash equality is recorded in separate c3/c4 storage receipts. Evidence was not
pruned and external workloads were not stopped.

Next: minimally instrument this actual source/build graph and repeat the
original full-scope cold wheel, verified all-entry warm wheel and quarter-height
middle motion checks. Build/launch correctness is now established locally;
160 Hz frame delivery, two-pixel motion error and zero coverage holds still
need to pass together on this graph. No earlier failed interval is reclassified.

AI assistance has been used to create this output.
