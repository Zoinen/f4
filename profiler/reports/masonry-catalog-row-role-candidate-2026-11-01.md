# Catalog row notification candidate, 2026-11-01

## Result and rollout decision

An experimental implementation now avoids blanket role invalidation when an
already loaded catalog row is replaced. It removes the five immediate Masonry
rewraps caused by the five 64-row metadata batches in the measured Downloads
startup. **It has not established consistent improvement in displayed frame
intervals and has not replaced the normal portable build.** Constant 160 Hz
without spikes remains unachieved.

The final current-source native test binary passes **31 selected tests**,
including the new visual-only update regressions, sparse catalogs, video
fallback, cached metadata and SVG classification/failure handling. Five panel
mode captures and actual caption/vector scene-origin/unit-transform checks at
DPR 1.75 pass; all five rendered images are pixel-identical to the control.
These are useful bounded regressions, not proof of all rendering formats or
the complete performance objective.

## Implementation

`ExternalCatalogRowsTransaction` previously emitted `dataChanged` with an empty
role list after every row batch. Qt interprets that as every role changing.
Masonry consequently treated display-only changes as changes to image/folder
semantics, names, caption metrics and geometry, and synchronously flushed its
lightweight rewrap path.

The candidate accumulates the union of roles actually changed by replacements
of loaded entries. Facade-backed roles and thumbnail replanning remain
published because replacement still retires the previous facade. Image size,
settled-metadata barriers, folder/image classification, names, paths, source
indexes, versions, selection, size and modification time retain their required
notifications. Installing an absent sparse row still invalidates all roles.
The batch is fully validated and installed before its single notification;
retirement order and decode pipeline resumption are unchanged. No geometry
algorithm, pixel rounding, QML resource, Go wire protocol or SDK is changed.

The final source adds an explicit source-index comparison to the first measured
prototype. The final `cold-s2` capture and current-source native tests use that
version. Do not conflate the prototype's hashes with the final host.

## Scrolling measurements

All captures use the real Downloads folder, 192 entries, Masonry density 152,
3840×2160, 160 Hz and DPR 1.75. Both panels display Downloads. Each has an empty
gallery cache before QML; the OS file cache is not flushed. The counter probe
has no overflow, writes after input ends and measures inclusive elapsed spans,
not exclusive CPU. No Computer Use helper was connected. This remains the
startup/first-scroll stress case, not entry into Downloads in an initialized
application.

Four 30-second captures ran control/candidate/candidate/control, without
compilation or analysis during timing:

| Case | Arm | Frames cached at input start | Display gaps >9.375 ms | Dropped presents | Maximum display interval | Immediate rewraps nested in the five row transactions |
|---|---|---:|---:|---:|---:|---:|
| cold-r1 | control | 3 | 4 | 0 | 50.0102 ms | 5 |
| cold-r2 | prototype | 35 | 0 | 0 | 6.5564 ms | 0 |
| cold-r3 | prototype | 3 | 10 | 0 | 50.0048 ms | 0 |
| cold-r4 | control | 3 | 3 | 0 | 49.9962 ms | 5 |
| cold-s2 | final source | 11 | 4 | 0 | 25.0030 ms | 0 |

All boundary intervals, misses, missing/ambiguous pairs and excluded motion
pairs remain in raw data. Startup admission varies; in particular, the zero-miss
prototype run starts warmer. It cannot establish a win over the controls. The
other prototype has more misses than either control, so promotion would be
premature. The final capture also fails the display interval requirement.

The later offscreen row transaction takes .3868 and .5722 ms in the prototypes,
versus 1.9810 and 1.2090 ms in the controls; its immediate flush is absent. The
final source's corresponding transaction is .5165 ms. This is evidence of
removed synchronous work, not a causal estimate for whole-run display misses.

The final capture retains a **10.2598 ms** transaction before input, despite
having no nested immediate flush. Its post-start transactions take 1.0294,
1.0904 and .5165 ms. Catalog facade/property publication remains capable of
large elapsed costs after the redundant geometry branch is removed. The final
render-signal range reaches 8.6339 ms; these counters do not separate its CPU
work from waits. Native matrix error remains small, but no wheel-curve,
physical-pixel motion or optical acceptance claim is made.

## Test development and rejected experiments

The new selection, display-caption metadata and hidden-state cases reproduce
blanket invalidation on the original implementation. The candidate passes them
without a layout revision or rewrap. Additional tests require real folder/name
changes to retain explicit notifications and keep a retired facade alive
through the synchronous callback. The final source and compiled test source
match byte-for-byte. Baseline has 26 passes and five expected new-test failures;
the final candidate has 31 passes and zero failures.

An attempted additional active-preview test initially demanded immediate
pointer identity/ready-state behavior that conflicts with supported snapshot
presentation transitions. A proposed delegate rebind was removed; the current
`MasonryLayoutModelDelta.cpp` is byte-identical to the input of this
investigation. Later fixture versions also failed to paint their initial
bitmap on both control and candidate, before any update. The saved control
capture shows a blank preview area. Changing the asserted pointer, file
revision and colour predicate did not make that a valid update regression.
Those attempts are preserved and the invalid fixture is kept under artifacts,
outside the product test suite. Its failure does not prove an introduced
product regression, nor does its rejection verify bitmap presentation. That
additional coverage still needs a valid initial rendering fixture.

Storage guards rejected `cold-s1` before application/collector launch. Several
test artifact writes/links also hit disk capacity; these were setup failures,
not performance samples. The candidate link was once started before the
baseline link completed; both handles were then observed to completion before
retrying the failed link. Only closed owned diagnostic binaries were relocated
or compressed, with hash verification. Captured raw data and user files were
not deleted. The normal application was kept alive throughout.

## Evidence and remaining work

[Curated evidence manifest](../experiments/masonry-catalog-row-role-candidate-2026-11-01/manifest.json)
records source/build hashes, the complete measurement summary, test logs,
pixel comparisons, final live identities and raw-data locations. Raw captures
remain below `C:/Users/xs/AppData/Local/Temp/f4-current-cold-motion-20261031/`.
Executables and raw capture buffers are excluded from the curated copy.

The Windows system-DLL and GUI-subsystem audits pass for the final diagnostic
host. It uses a worktree-specific executable and development host override.
No new portable payload, release validation, normal-build rollout, commit or
push is claimed.

Next application-side work should quantify which facade/source fields actually
change during those row batches and whether immutable-source facades can be
retained safely. Avoid reintroducing blanket geometry work or speculative
delegate binding changes. Further acceptance still includes initialized-app
cold folder entry, complete wheel motion/curve checks, rendered-pixel motion
verification and sustained no-spike delivery with all intended visuals.

AI assistance has been used to create this output.
