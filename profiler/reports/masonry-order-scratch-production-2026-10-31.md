# Bounded ordering snapshots deployed, 2026-10-31

The production renderer now reuses ordering-snapshot storage. This removes a
measured allocation source; it does not complete the constant-160-Hz goal.

## Implementation

The existing unmerged-batch retention check builds old and new opaque/alpha
snapshots for each render-list rebuild. Its temporary QList/QHash allocations
were observed blocking for 3.5333 ms and 1.8956 ms in the production wait trace.

The new F4OrderScratch storage has four leases per render thread, each serving
up to 16384 pointer identities. Capacity grows geometrically and remains bounded.
Keys are never dereferenced by the scratch index. Generation tags invalidate
previous contents, including wraparound; duplicate keys retain their final
position as before. Move-only leases prevent simultaneous snapshots and nested
rebuilds from overwriting one another. Oversized snapshots or exhausted leases
take the original Qt-container path. No GPU buffer is overwritten or reused by
this change.

The painter-order, membership, material, depth, removed-element and interleaved
outsider checks are unchanged. No persistent scene result is cached. On this
64-bit build the four maximum-size scratch slots fit within 2.5 MiB per render
thread; smaller scenes retain less. Storage is released with its render thread.

The implementation is embedded into the portable Qt source patch, so generated
Conan recipes do not depend on this checkout at runtime/build-source execution.
All six portable Conan cache identities now include the new header. The local
production CMake graph selects a hash-verified owned Qt archive containing only
the replacement batch-renderer object. The installed SDK remains unchanged.

Sources: ci/qt_quick_order_scratch.h, ci/qt_quick_upload_policy.py,
ci/tests/qt_quick_order_scratch_test.cpp, ci/test_patch_qt_dependencies.py and
the six cache-key entries in .github/workflows/build.yml. No Go protocol or
product setting was added.

## Regression and packaging checks

The source-patch regression failed before integration. The cache-key regression
also failed with the header omitted, then passed after correction. A first
cache test used the wrong Windows text encoding; its error is retained, and
the corrected missing-header baseline fails for the intended reason.

All 40 portable-recipe tests pass. The compiled C++ test checks 1048576 lookups
against a standard unordered-map reference, duplicate keys, bounds, empty lists,
generation wrap, move/move-assignment, nested lease exhaustion and thread
isolation. After warming all four maximum-size slots, 4000 snapshots perform
zero heap allocations. These are component-level allocation results, not proof
that an entire live frame makes no allocations.

The original native renderer passes 44 tests. The changed production archive
also passes all 44, plus 11 motion/mapping tests, with no skips. Nine native
renderer PNGs and five motion PNGs at actual DPR 1.75 are byte-identical to their
baselines. Isolated QML imports, the Windows GUI PE subsystem regression and
the system-DLL audit pass. No static UI layout or pixel-rounding code changed.

An initial Qt object compile failed on a stale generated-header include inside
the old Conan build tree. The failed log is retained. Copying the existing
workspace-owned generated moc header into the new owned source directory fixed
the build; no installed SDK file was modified.

The compressed Qt payload was regenerated, then embedded in a CGO_ENABLED=0 Go
build with GOWORK=off and the system Go cache. Five extraction/materialization
tests pass, including reuse, concurrent first launch and corrupt-payload
handling. The packaged executable opened both panels on first extraction and
on cache reuse with an absent-SDK qt.conf and Qt/Conan paths removed from PATH.
No explicit host override or sibling host was supplied. This is local isolation
evidence, not a clean-VM release certification.

## Scrolling observations: no full performance claim

All captures use the actual Downloads folder with 192 entries, Masonry density
152, the local 3840 by 2160 / 160 Hz / DPR 1.75 display, and a 30-second wheel
input interval. Every input verifies density, mode, entry count, DPR, refresh,
acceptance and foreground state. Fresh first captures begin at contentY zero.
Both test panels contain Downloads. Input is requested every 10 ms, but actual
delivery remains irregular; no exact 100 Hz delivery claim is made.

| Run, in capture order | Display intervals above 9.375 ms | Maximum display interval | Rendering range p99 / max |
| --- | ---: | ---: | ---: |
| Candidate first b1 | 0 | 6.4553 ms | 2.7549 / 5.8524 ms |
| Candidate same-process repeat b2 | 0 | 6.5453 ms | 2.6599 / 3.8183 ms |
| Original renderer reverse control a7 | 1 | 12.4844 ms | 3.0687 / 9.7865 ms |
| Candidate fresh-process repeat b3 | 6 | 12.5320 ms | 3.2979 / 9.5997 ms |

The mixed reverse-order result prevents a reliable end-to-end improvement
claim. The deterministic allocation regression passes, but GPU-resource waits,
presentation behavior, decode/cache state and scheduling still affect these
runs. The old control executable was NTFS-compressed during earlier owned-file
storage recovery, while the new recorder was uncompressed for its captures;
this is another reason not to treat the initial/reverse comparison as a clean
causal performance estimate. Original failed runs and every outlier remain.

These are initial/repeat baselines, not verified cold-cache or all-192-entry
quiescent warm-tour runs. Physical motion error, zero coverage holds and
quarter-height middle-button velocity remain unverified on this build. The
original 9.375 ms display threshold, 2 px maximum motion error, zero coverage
holds and .002 px middle-velocity tolerance remain unchanged. A pair of passing
display captures does not satisfy those other gates.

## Running build and remaining work

The normal worktree app was replaced only after build, native, packaging and
actual-window checks. It runs directly with --gui=qt and the new default
embedded-host cache generation, without diagnostic or explicit-host flags.

- Go PID 9252, D:/Code/f4-zoin/f4-zoin.exe, SHA256
  ac68c2e7c2fd259b760f91fc7c001be26d77ec2b33c86c36d1f22893a53fe613.
- Qt PID 106980, default cache generation
  db229e662ec9aaabb4204bb28cfcacf925afbc9767112ea2d12ece39124ec30b,
  host SHA256 653cbd8a9e6b416e1d2af88f88e34e353edcb4a18e7a3fb9d7ce32d6152a949b.

The actual window restores the user's tabs, Downloads left and the existing P:
photo directory right, with the runtime branch title. The previous working
Go binary and previous payload are preserved for rollback. Only owned
diagnostic trees were stopped. Only closed owned files were NTFS-compressed,
with byte hashes checked before and after. External workloads, VMs, affinity
and desktop settings were not changed. No commit or push was performed.

Next, identify the source and dimensions of textures created during first
scrolling, then remove proven redundant extraction/resource creation. Geometry
buffer renaming and per-draw binding copies are separate measured targets.
Repeat the complete original cold/warm/middle/motion gates on the actual
production graph. Constant 160 Hz with no spikes remains unproven.

Working evidence is under artifacts/order-scratch-production-20261031 and
artifacts/order-scratch-recorder-20261031; reverse-control data is retained in
artifacts/production-cpu-recorder-20261031. Compact evidence and external
binary/archive hash references are frozen beside this report.

AI assistance has been used to create this output.
