# Current Masonry row activation: GC and null-string reuse

Constant 160 Hz without spikes remains unachieved. This investigation accounts for the previously unmeasured row-completion cost and tests an allocation-reduction prototype. No production source, SDK, payload or default changed.

## Callback accounting

Earlier phase timers omitted the RAII `endVisualUpdate()` callback. An observation-only override now measures it, preserving original transactions. Bounded buffers retain every commit total and individual completion callbacks lasting at least 0.2 ms; neither buffer overflowed.

Both fresh-process captures use actual Downloads, 192 entries including parent, Masonry density 152, maximized startup geometry, actual DPR 1.75 and 160 Hz. Each contains 30 seconds of rapid wheel input from the initial viewport. No cold-cache state, complete warm tour or physical-motion acceptance is claimed. Instrumented durations include overhead and preemption.

| Capture | Left commits | Commit median / p99 / max, ms | Completion total per commit p99 / max, ms | Largest single completion, ms | Remaining loop remainder max, ms |
| --- | ---: | --- | --- | ---: | ---: |
| delegate-end-a1 | 2262 | 1.3244 / 6.2064 / 11.4832 | 0.9503 / 5.6094 | 5.2565 | 0.0377 |
| delegate-end-stack-b1 | 2287 | 1.2470 / 5.7049 / 9.6865 | 1.3665 / 6.9367 | 6.4807 | 0.0530 |

The remainder excludes completion and the other non-overlapping row phases. This directly accounts for almost all earlier missing loop time. Host SHA-256: `5f9f7a5491b695e3d00310c912458b983b3b683deb5ee87b58c605aabeac4537`.

The first capture retains three display intervals above 9.375 ms and one dropped present. It is attribution evidence, not a clean performance comparison. No outlier was removed.

## CPU stacks inside completion

A 12-second scheduler/profile trace covers the first eight input seconds of the second capture. It contains 3,293,273 events with zero reported event loss, buffer loss, schema errors or ProcessTrace errors. SHA-256: `48040e9af4f85d0f7295dad7da318ebf23456cbcd3e871a51ffdd8aae3ff9438`.

The sampled-profile reader uses its documented prefix fallback. Independent xperf output verifies all 261 selected reference rows by thread, instruction pointer and timestamp. Selected slow-completion samples have no missing stacks. The broader analysis retains one render and two GUI profiles without stacks.

The 6.4807 ms completion callback for recycled row 18 contains seven GUI CPU samples; all seven contain `QV4::MemoryManager::runGC`. Sampled states include global-object marking, weak-value marking and mark draining, entered from string allocation during QML binding evaluation. A 5.2916 ms callback for row 160 contains six samples, five in the collector. This is exact-time, same-thread containment, not an exclusive CPU percentage or an explanation for every scrolling miss.

Shorter callbacks also contain image-source updates and synchronous Loader creation. The evidence does not support assigning all activation cost to icon rasterization. Earlier broad GC findings remain relevant; this capture ties the current completion gaps to GC in the current production graph. Earlier smaller-GC-budget experiments were inconclusive; no budget change was repeated or promoted.

The symbol-only link has identical `.text` and `.pdata` bytes/RVAs to the captured executable. Hashes and PE ranges are checked before resolution. That link was not substituted into the capture. Unknown/JIT and ICF-ambiguous symbols remain explicit.

## Count the observed allocation route

A separate probe counts `ExecutionEngine::newString(const QString&)` by QML source line and broad value category. Native aggregation allocates: **these are counts only, never frame-time comparisons**. Concatenation, identifier creation and other routes are outside coverage. Unknown QML frames remain unassigned. The 4096-site cap did not overflow.

The first census records 316,751 calls in 30 seconds, including 83,560 empty QString values. A second distinguishes null from non-null empty values:

| Category | Calls |
| --- | ---: |
| Null QString | 71,130 |
| Non-null empty QString | 17,803 |
| Other string | 185,750 |
| Image URL | 19,535 |
| QRC URL | 35,429 |
| Gallery-name prefix | 324 |
| Total | 329,971 |

These are separate workloads, not before/after results. Null QString accounts for about 21.6% of the second capture's observed route. Assigned null sites include `GalleryEntryDelegateBase.qml:218` (18,540 calls) and lines 235/236 (7,988 each). Reading C++ QString properties into JavaScript repeatedly creates managed strings. Other routes and non-empty strings remain significant.

The exact [official Qt 6.11.1 source](https://github.com/qt/qtdeclarative/blob/v6.11.1/src/qml/jsruntime/qv4engine.cpp) is preserved locally, SHA-256 `05221d678d3c8b200b375e9b4e5401dd581fe0b340594be91f9a572cd7c04da8`. The SDK is unchanged.

## Prototype and regression

Qt already roots an empty string initialized from a null QString per engine. The artifact-only prototype returns that existing value for null QString input once the engine string table is initialized. Non-null empty QString retains the original path, preserving the C++ null-versus-empty distinction. No new root, cache, engine-layout change or GC algorithm change is introduced.

The regression ran against the original production Qml library before the prototype existed. Original: `semanticFailures=0 freshNullStrings=32768 calls=32768`, with the expected allocation-regression failure. Prototype: `semanticFailures=0 freshNullStrings=0 calls=32768`, passing.

Checks cover eight engine construction/destruction rounds, scoped values surviving explicit collections, stable root lifetime, null/non-null-empty conversion, JavaScript equality, empty property keys, JSON, concatenation and isolation between two live engines. The first regression compile lacked private-header include paths; that log is retained and the paths were corrected before accepting either result.

This is **not an application performance win yet**. QML binding/lifetime regressions, normal rendering checks and clean matched scrolling captures remain necessary before dependency integration or portable rollout. Allocation reduction does not imply proportionally shorter GC or prove 160 Hz acceptance.

## Evidence and state

- Callback source/captures: `artifacts/delegate-end-probe-20261031/`.
- Trace/readers: `C:/Users/xs/AppData/Local/Temp/f4-delegate-end-wait-20261031/`.
- Symbol link/map: `C:/Users/xs/AppData/Local/Temp/f4-delegate-end-symbols-20261031/`.
- Allocation probes: `artifacts/string-allocation-census-20261031/`, `artifacts/null-string-census-20261031/`.
- Original regression/prototype: `artifacts/null-string-regression-20261031/`.
- Preparation/build/capture/analysis scripts: `artifacts/`, with matching date suffixes.

Closed evidence was losslessly NTFS-compressed with hash checks. Disk exhaustion initially prevented saving this report. A verified external backup preserved a closed regression executable during recovery; the first restore ran out of space, and the final atomic restore verified the original hash. Its recovery receipt and backup are retained. No raw trace was deleted.

Diagnostic trees are stopped by verified executable path. The normal ac68c2e7 / 653cbd8a worktree app is unchanged. No commit or push. Full cold/warm/middle-motion, coverage, physical-motion and display-cadence acceptance remains outstanding.

AI assistance has been used to create this output.
