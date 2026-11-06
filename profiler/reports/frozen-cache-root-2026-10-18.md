# Resolve the active cache root once

## Diagnosis and change

After late video metadata was coalesced, thumbnail planning still blocked the
GUI. The `thumbnail-planning-probe160-wheel-masonry` capture measures up to
10.172 ms in ExternalCatalogThumbnailPlanner::submit, almost all inside
DecodeManager::decodeImages rather than metadata demand or cache admission.

The deeper `cache-lookup-probe160-wheel-masonry` capture isolates constructors'
beginLookup/joinLookup calls. Their key generation calls cacheFilePath,
cacheDirectoryPath, then StorageLocations::cacheRoot. In the active workload,
96 GUI calls to cacheRoot total 98.128 ms and peak at 2.894 ms. The method
holds the storage mutex while repeatedly resolving the Windows temporary path.

Storage is already process-scoped and forbids changing a root after use.
cacheRoot now resolves and retains the default or configured root on its first
call, under the existing mutex. Repeated reads return that retained QString.
The explicit-namespace helper remains independent, and override acceptance
rules remain unchanged. No decode priorities, resource ownership, cache schema,
UI geometry or raster/vector presentation policy changes.

## Regression

activeCacheRootDoesNotFollowEnvironmentChanges redirects TMP/TEMP/TMPDIR to one
temporary directory, starts the cache, then redirects them to another. The
namespace helper proves the changed environment is observable; the active
root must stay fixed. It fails against the old implementation and passes after
the change. All 15 selection-cache results pass. The test restores environment
variables and uses only fixture directories.

Evidence: `artifacts/storage-root-before.txt`, `storage-root-after.txt`.

## Instrumented call costs

Both diagnostic workloads use Downloads with 184 entries, density 152,
initial extent 4161.142857, actual DPR 1.75 and a 160 Hz window. Wheel delta
120 repeats every 30 ms for four seconds in each direction. Source scopes and
GUI event observation add overhead; these are call attribution, not production
frame-rate claims. Worker-thread scope records are excluded.

| GUI operation | Before | After |
|---|---:|---:|
| cacheRoot calls | 96 | 96 |
| cacheRoot total | 98.128 ms | 0.0088 ms |
| cacheRoot maximum | 2.894 ms | 0.0006 ms |
| beginLookup total, 48 calls | 53.047 ms | 0.5282 ms |
| joinLookup total, 48 calls | 46.336 ms | 0.2493 ms |
| request submit maximum | 10.5286 ms | 0.2386 ms |
| viewport thumbnail plan maximum | over 10 ms | 0.4254 ms |
| visual snapshot application maximum | 5.4147 ms in preceding probe | 2.6655 ms |

The after capture is `frozen-root-fixed-probe160-wheel-masonry` under
`artifacts/modes-profile`. Before scopes retained only calls over 0.05 ms;
after scopes explicitly retain every cacheRoot/beginLookup/joinLookup call to
measure their new small costs. Outer timings remain inclusive and are not
additive. The snapshot maxima come from separate instrumented captures and
do not establish an isolated effect for every snapshot update.

## Clean frame measurements

The first attempted comparison attached to the previous tab during activation:
32 entries, density 150 and zero scroll extent. The
`frozen-root-baseline-a1-wheel-masonry` capture is invalid and excluded.
The harness now waits for the requested session path before retaining a panel;
the runner also rejects a wrong path or zero scroll extent.

The valid pair builds share that guarded harness and the committed normal Qt
renderer. The baseline overrides only StorageLocations with its pre-fix source;
the candidate uses the new implementation. Both use the workload above with
no source scopes, UI Automation observer, CPU sampler, compilation or tests
during capture. The unshipped interval renderer is absent from these pairs.

| Run | Down display p99 | Down maximum | Down intervals >9.375 ms | Up display p99 | Up intervals >9.375 ms |
|---|---:|---:|---:|---:|---:|
| Baseline A1 | 24.8923 ms | 31.2774 ms | 30 | 6.3695 ms | 4 |
| Candidate B1 | 25.0031 ms | 35.7978 ms | 27 | 12.5065 ms | 14 |
| Baseline A2 | 18.7825 ms | 31.2488 ms | 35 | 12.4915 ms | 10 |
| Candidate B2 | 24.9936 ms | 31.2749 ms | 29 | 12.4866 ms | 10 |

Capture stems are `frozen-root-valid-a1`, `-b1`, `-a2`, `-b2`, each followed
by `-wheel-masonry`. Matching metadata confirms actual Downloads, density,
extent, active window and DPR. Removing the measured OS work does not establish
a reliable end-to-end improvement in these pairs. The retained change fixes
root stability and removes a demonstrated GUI cost; constant 160 Hz remains
unfinished. SVG uploads, full render-list rebuilds and visual snapshot work
remain separate costs.

## Verification and rollout

The derived-cache suite has 31 passing results, one skipped DNG-fixture case,
and one failing JPEG-writing fixture. jpegWithoutOrientationRetainsMetadata
fails at QImage::save before exercising metadata retrieval. A separately linked
old-StorageLocations control reproduces the same failure. It is not reported
as a passing full suite. Logs: `frozen-root-derived-cache.txt` and
`frozen-root-jpeg-baseline.txt` under artifacts.

The focused session suite passes nine results at 175%, resource-only imports
and compiled-host startup pass four results, tagged embedded-host tests pass,
and the Windows system-DLL import audit succeeds. The rebuilt normal host has
SHA256 `39de3aae0377420abedcdaeb0aa420bfce2afb27ffdaa8dcc94d03ad8451eb82`.
The payload is regenerated and the Go launcher is built with CGO disabled and
the system Go cache. The worktree app is replaced and restarted directly with
--gui=qt and no profiling, host-path or order-padding override. Its extracted
host has that exact hash, the title identifies zoin_branch/cached-pixel-grid,
and startup stderr is empty. Identity is recorded in
`artifacts/frozen-root-portable-identity.json`. Cross-platform release validation
is not claimed.

AI assistance has been used to create this output.
