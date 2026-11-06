# Masonry resource identities and cold-cache controls

The dominant slow upload in the fresh-cache resource probe targets a
4096 x 4096 BGRA texture. The update has a 4-byte row pitch and takes
13.2906 ms. Other slow uploads target the same resource. Dynamic uniform,
vertex and index buffer maps also pause, as do allocations of small standalone
textures. Thus neither upload payload size nor a single buffer category alone
explains the stalls.

The diagnostic records descriptions at successful resource creation, retaining
pointer, timestamp, native thread, dimensions, format, usage and binding flags.
Slow Map/Unmap/UpdateSubresource calls retain resource pointer, subresource,
map type and row pitch. No GetDesc call, hash-map lookup, allocation or JSON
serialization is introduced into each upload/map. Offline attribution selects
the latest creation at or before the call end, retaining pointer reuse rather
than assuming a pointer is one immutable resource. Resource destruction is not
recorded; pointers created outside this instrumented RHI remain unidentified.

Two fixed 65,536-slot buffers separately bound calls and creation records;
release/acquire publication prevents reading incomplete slots. All three
captures remain within their recording bounds. The empty-cache configuration
and cold-input overlap follow the preceding cold-preparation report. The
application cache is fresh, but OS disk cache and Go metadata are not flushed.

## Verification

- Resource recorder: nine passes including setup/cleanup. Tests cover preserved
  return values/reference/void, exactly-once invocation, descriptions, pointer
  reuse, failed creation, upload arguments and both buffer bounds.
- Disabled recorder: three passes including setup/cleanup. Both creation and
  resource-call wrappers preserve execution without publishing metadata.
- Native D3D11 rendering: 44 passed, zero failed/skipped at actual DPR 1.75.
- Import isolation/compiled host: four passed; system-DLL audit passed.
- An initial native test invocation incorrectly set QT_SCALE_FACTOR=1.75 on
  the already-scaled desktop, causing 26 failures at the wrong effective DPR.
  The corrected invocation removes that override and asserts actual DPR 1.75.
  Both logs are retained; the wrong-scale run is not a rendering regression.

The preceding RHI baseline's 412 matching executable sections and reversible
102-call wrapping checks still apply. Only diagnostic artifacts changed;
production sources, dependency archives and embedded payload remain unchanged.

## Cold captures

Each row is a separate fresh process with a unique empty application cache,
the actual Downloads folder and two ten-second rapid-wheel blocks, at 160 Hz
and DPR 1.75. The first arm records resource metadata; the disabled arm uses
the identical host with both recorders disabled. The third changes atlas
dimensions through child-only environment variables. No unrelated workload is
disabled, and no builds, tests, UI Automation observation or offline analysis
run during input.

| Arm / Qt PID | Block | Display gaps >9.375 ms | Maximum display interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: |
| Default atlas, metadata enabled / 39496 | 0 | 10 | 18.7504 ms | 6 |
| Default atlas, metadata enabled / 39496 | 1 | 1 | 12.5120 ms | 0 |
| Default atlas, recorder disabled / 5600 | 0 | 7 | 12.5246 ms | 0 |
| Default atlas, recorder disabled / 5600 | 1 | 1 | 12.4999 ms | 0 |
| 1024 x 1024 atlas, metadata enabled / 120172 | 0 | 4 | 12.5249 ms | 0 |
| 1024 x 1024 atlas, metadata enabled / 120172 | 1 | 1 | 12.6011 ms | 0 |

The disabled arm demonstrates stalls without the recorder. It does not prove
zero overhead or quantify the recorder's effect on tail latency: this single
sequential pair is not a randomized repeated overhead benchmark.
Smaller-atlas cadence is insufficient for constant 160 Hz, and no atlas-size
change is promoted. A smaller maximum or fewer misses in this temporal sample
is not a causal performance claim.

Default metadata capture recorded 2,905 creations. A single 4096 x 4096 BGRA
resource was created approximately 634 ms before input. It was not repeatedly
reallocated throughout the gesture. Its largest update cost is 13.2906 ms;
other long operations include:

| Operation | Resource | Wall time |
| --- | --- | ---: |
| Map, recording | 5,632-byte dynamic constant buffer | 4.4758 ms |
| CreateTexture2D | 114 x 249 BGRA shader resource | 4.3899 ms |
| Map, preparation | 68,796-byte dynamic index buffer | 4.0318 ms |
| Map, preparation | 394,680-byte dynamic vertex buffer | 3.5902 ms |
| UpdateSubresource | 4096 x 4096 BGRA, row pitch 1,780 | 3.4313 ms |

The 1024 control explicitly preserves QSG_ATLAS_SIZE_LIMIT=2048, matching
the default 4096 atlas eligibility threshold. Packing failures can still send
more images to standalone textures; changing page size is not an isolated
GPU-memory-size intervention. Resource metadata confirms an actual 1024 atlas.
Its slowest atlas updates are 5.1463 ms in block 0 and 7.4070 ms in block 1;
a 447 x 250 standalone texture allocation takes 6.3643 ms. Hence merely reducing
atlas dimensions does not remove resource waits.

These captures have no scheduler trace. Their operation wall times must not
be labeled blocked time by importing the preceding capture's scheduling data.
The earlier paired trace independently establishes blocked intervals inside
the same API call sites, not the wait owner of these particular resources.

## Next candidate

[Qt 6.11.1 atlas source](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/quick/scenegraph/util/qsgrhiatlastexture.cpp)
enqueues image-body and four edge-padding updates separately. Its column
updates match the observed four-byte row pitch. A useful next candidate packs
the same padded pixels into one upload, retaining atlas coordinates, filtering
and transparency. It must first pass padded-pixel/stride regressions and native
render comparisons, then matched fresh-cache captures. It is not implemented
or promoted in this step. One larger upload may still wait, and added CPU copy
cost must be measured. Do not infer that all other stalls disappear with it.

The normal portable worktree app remains running (launcher 43936 / Qt 119460).
Only the exact resource diagnostic launcher tree was closed. No commit/push.
Constant 160 Hz without spikes remains unverified.

Evidence: `profiler/experiments/masonry-resource-identity-controls-2026-10-27/`.

> AI assistance has been used to create this output.
