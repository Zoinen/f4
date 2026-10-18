# Correcting Qt Quick buffer-pool selection

## Reproduced cause

The preceding continuation produced new correlated timing evidence and rejected
two unsupported rollouts, so it counts as progress. This continuation targets
resource recreation in the remaining render upload stalls.

Qt 6.11.1's Renderer::unmap searches a pool for a reusable vertex/index buffer.
When its first entry is undersized, the existing comparison can never replace
that candidate with an adequate one: the adequate candidate must also be smaller
than the selected undersized buffer. Instead it chooses the largest undersized
entry and grows/recreates it, leaving sufficient pooled capacity unused.

For a 64-byte upload and pool capacities [16, 128, 64], the old code chooses
the 16-byte object and resizes it instead of taking the existing 64-byte object.
The fix adds one precedence condition: an adequate candidate beats an
undersized selection. The existing search then retains smallest adequate or,
when none fits, largest undersized behavior. Pool limits, buffer types, upload
contents, ownership and retirement rules are unchanged. This is not a new
buffer handoff/cache experiment.

## Native regression and source audit

The fixture renders an actual unmerged geometry batch through Qt's normal
render path, with 64-byte vertex and index payloads. A render-thread callback
seeds known real QRhi buffer capacities; after rendering it checks the selected
buffer identity and remaining pool cost/count. Qt retains ordinary ownership of
the selected buffer and remaining pooled buffers. There is no mock selection
algorithm or direct call to a copied unmap implementation.

Eight rows cover vertex/index buffers, undersized entries before exact fits,
undersized entries before the best adequate fit, an adequate entry first, and
the largest-undersized fallback. Before: 6 results pass and 4 fail, precisely
the undersized-first cases for both buffer kinds. After: all eight rows pass.
The final checks also verify actual native backend, DPR 1.75, rendered red
pixels and scene-space origin/unit transforms of the fixture leaf.

The production recipe adds a separate idempotent source patch with strict
single-anchor validation. Unknown/ambiguous sources fail before saving. The
exported recipe is self-contained; it does not read this checkout's artifact
directory. All six CI native-cache keys already hash the modified patch script.

Applying the order, blocked-opacity and stable-interval replacements to
official Qt 6.11.1 source reproduces the existing compiled renderer. Executing
the actual exported buffer-fit patch then reproduces the compiled candidate
exactly after newline normalization. The candidate library also retains the
integrated native child-window stacking fix. No ABI/header layout or Qt upgrade
is involved. Evidence: artifacts/audit-buffer-pool-recipe.py.

## Incidence in actual scrolling

A separate observer preserves the original policy and records only cases in
which an undersized buffer was selected despite adequate pooled capacity. It
records 1890 events across the process, of which 1076 are within the actual
scroll direction intervals: 559 vertex and 517 index events. The largest
requested payload in those events is 1344420 bytes. This establishes the bug
occurs in the measured workload, rather than only in the synthetic fixture.

With the fit correction, the same observer records zero missed-fit events in
the same guarded workload. The log still records normal initialization and
the metadata confirms DPR 1.75, 184 entries, extent 4161.142857, default 5000 us
GC budget and zero QObject-root adoptions.

These are logical missed-fit events before allocation, not isolated driver
timings or a direct count of all saved native allocations. For example, a
buffer-type promotion can still require recreation even when capacity fits.
Logging changes cadence substantially; observer frame timings are excluded.
The other 814 events in the first capture occur outside the direction windows
and must not all be labeled startup work.

Local evidence: buffer-pool-misses{,-fixed}-wheel-masonry.json under
artifacts/modes-profile, corresponding buffer-pool-misses{,-fixed}.log, and
artifacts/buffer-pool-misses-summary.json. Raw logs/traces and binaries remain
outside the source checkpoint.

## Clean display measurements

A is the preceding normal renderer; B adds only the fit correction. Both
hosts use identical current gallery sources and diagnostic harness. Each
fresh process uses Downloads, 184 entries, density 152, extent 4161.142857,
actual DPR 1.75, active window and 160 Hz. Wheel delta 120 repeats every 30 ms,
about four seconds per direction. State is restored between processes.
No build, tests, observer, sampler, ETW recording or debug logging runs during
these captures. GC and ownership experiments are disabled; caption measurement
counts remain zero. PresentMon display-change intervals are the headline.

| Run | Down p99 ms | Down max ms | Down >9.375 ms | Up p99 ms | Up max ms | Up >9.375 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1 | 6.3307 | 18.7519 | 4 | 6.2725 | 12.4917 | 1 |
| B1 | 6.3501 | 18.7421 | 4 | 6.2912 | 12.4786 | 1 |
| B2 | 6.3390 | 24.9886 | 6 | 12.4962 | 12.5173 | 8 |
| A2 | 12.4963 | 25.0222 | 12 | 6.3890 | 12.5086 | 6 |

The first pair ties on long counts. In the reverse pair, B improves the
downward tail but regresses upward. No reliable overall scrolling gain is
established and no run is excluded on an unverified environmental explanation.
The correction removes a real recurring allocation mistake; it does not prove
constant 160 Hz or eliminate the remaining upload/driver and GUI stalls.
Local stems are buffer-fit-{a1,b1,b2,a2}-wheel-masonry; compact evidence is
artifacts/buffer-fit-summary.json.

## Verification and remaining scope

- Dependency recipe tests: 27 pass, including composition/idempotence,
  source drift and ambiguous-anchor rejection.
- Native rendering selection: 17 results pass on Windows default RHI and
  17 on OpenGL at 175%, including pool selection, clip lifecycle, opacity,
  order reservations and the normal retained-batch cases. Focused reruns
  explicitly assert the actual D3D11/OpenGL backend rather than assuming the
  environment selected it.
- Focused SVG selection: 26 results pass with native upload-gate capability
  explicitly absent, including all five panel modes, cache/source replacement,
  cancellation, resize, gradients/alpha, authored animation and viewer swaps.
  The resize capture's small caption and thin blue stroke were inspected.
- Import isolation and compiled-host QML loading: 4 results pass; Windows DLL
  audit succeeds. Tagged Go embedded payload/extraction/reuse/concurrency/lookup
  checks pass with CGO disabled, GOWORK off and the system Go cache (0.622 s).

The full native suite is not claimed green: experimental upload-admission and
unmerged-retention rows still require their prototype libraries. Cross-platform
release gates were not run here. The next investigation must reattribute the
remaining large resource stalls after this fix, rather than infer they all
disappeared from the missed-fit counter reaching zero. The full 160 Hz goal
remains active and unfinished.

## Portable worktree rollout

The normal host now links the tested buffer-fit Qt library, retaining the
previous stacking/stable-interval corrections. Its compiled source matches the
exported recipe. The library contains neither the missed-fit observer nor the
GC/ownership/upload-admission experiments. The normal host passed import/DLL
checks, its compressed payload was regenerated, and the staged CGO-free Go
launcher passed the tagged extraction checks before replacement.

Only the process tree whose executable resolves to this worktree's f4-zoin.exe
was terminated. The staged launcher was copied, hashed, and launched directly
with --gui=qt after clearing diagnostic overrides. The actual extracted child
matches the freshly built host, title is f4 [zoin_branch/cached-pixel-grid], and
startup stderr is empty. Canonical D:/Code/f4/f4.exe was untouched. Evidence:
artifacts/buffer-pool-normal-identity.json.

- Host SHA256: 549f063f2f8e3252f5784f0b017d6500b719102ff799fb49b279964e44383bed
- Launcher SHA256: b9c479436f6fbc954f0eabccf67594844a1e35fd962a3c7eab40eb6e4e7f9d58

No new commit or push is made. Earlier uncommitted icon-coalescing work remains
intact. The full goal remains open; the display measurements above retain the
spikes rather than redefine success around the allocation counter.
