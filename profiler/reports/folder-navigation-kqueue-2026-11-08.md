# macOS folder navigation: blocking watcher shutdown

Measured 2026-11-08 on the canonical `zoin` checkout, base `02dad253c`, with the pending title-bar change. Go executable `f4-zoin`, matching Qt host `qt/host/build-dev`, Apple M5 Pro, Qt 6.11.1, native Cocoa/Metal frontend, normal application profile. The target is `/Users/zoin/Documents/f4-zoin` (49 entries before the parent row), its parent Documents has 38 entries. Both panels use Details. The production window is 1512×882 logical pixels at DPR 2. Fractional-scale layout was not changed by these fixes.

## Cause and proof

Directory navigation is asynchronous, but `PanelsFrame.Show` synchronously calls `syncDirWatches`. Moving its kqueue watch calls `Watcher.Close`, which waits for the native source goroutine. That goroutine used a 100 ms `Kevent` timeout without a cancellation wakeup. Thus the UI thread waited for the old directory's idle watch on each transition. The Go execution trace attributes **5.64 seconds** of blocked time to `panelDirWatch.stopLocked → Watcher.Close → newNativeSource` in the baseline run. The implementation originated in `c9d1eebd3`.

The regression test first failed with **85.33 ms average close latency** after letting each watch enter its idle wait. A pipe registered with the same kqueue now wakes it immediately when its writer closes. The goroutine still owns descriptor cleanup, and Close still waits for callbacks and cleanup. This removes the timeout without moving cleanup to unbounded background goroutines or disabling directory notifications.

## Measurements

The built-in Qt navigation runner executes the same `panel.open` actions used by native Enter handling. Each run has 5 warmup cycles and 25 measured enter/leave cycles; no artificial settle interval. Go/Qt JSONL use the shared monotonic clock. Tracing and CPU sampling are enabled, so these are attribution captures, not clean delivery certification. A separate benchmark process overlapped the final mask-run capture; its numbers must not be claimed as a clean frame-budget pass.

| Capture | Enter median / p95 | Leave median / p95 |
|---|---:|---:|
| Baseline | 101.37 / 284.09 ms | 97.43 / 99.55 ms |
| Immediate kqueue shutdown | 108.38 / 127.60 ms | 14.50 / 26.09 ms |
| Plus simple mask matching | 58.26 / 60.03 ms | 12.01 / 26.31 ms |

The runner waits for a non-provisional catalog, viewport placement and a subsequent swapped frame. Its total is stricter than first visible feedback. Baseline enter action-to-Go-start median was **80.87 ms**; after both changes it was **0.36 ms** (leave **0.31 ms**). Directory enumeration itself was sub-millisecond in the baseline.

Once the watcher wait was removed, CPU sampling exposed repeated `SemanticStyle → IconChoices → Predicate.Match → regexp` work. Simple exact/prefix/suffix glob masks now use string comparisons while regexes, escapes, character classes and multiple wildcards retain their old implementation. The equivalence test covers case conversion, Unicode, separators, newlines, escapes, overlap and empty names. The isolated `*.go` benchmark changed from **131.0 ns/op** to **13.28 ns/op** on this machine. No filename/result cache or metadata freshness policy was added.

## Native keyboard control and remaining limitation

A native 4-second held Return injected only one key through the automation API, so it is **not** evidence of OS auto-repeat. A separate sequence of 20 native Return presses delivered 21 `panel.open` actions including that initial press. The API took about 1.13 seconds for those 20 presses. First correlated frame latency was available for 20 actions: median **17.75 ms**, maximum **59.46 ms**, 12 exceeded 16 ms. Input actions and directory chunks were processed promptly; later Qt publication/placement/frame work remains above the requested budget in some transitions. These fixes therefore **do not establish consistent ≤16 ms folder navigation**.

The QML profiler was not required to identify the blocking Go wait. Evidence here consists of correlated native navigation events, Go CPU profiles, the Go blocking trace, and a native sampling capture. A later Go trace capture timed out during setup and was excluded from timing comparisons.

## Validation

- Native directory-change/debounce tests and concurrent/idempotent kqueue close pass.
- The idle-close regression passes ten repetitions after the fix.
- Race tests cover dirwatch, filegroups and theme.
- kqueue package cross-compiles for FreeBSD, OpenBSD, NetBSD and DragonFly.
- File-group and theme tests pass; panel/native presentation regression suites are also run.
- The canonical executable is rebuilt with `CGO_ENABLED=0`, using the system Go cache, and relaunched through `run-f4.command`.

Raw evidence is retained locally in ignored `.codex-build/navigation-2026-11-08/`: `f4-nav-before-{go,qt}.jsonl`, baseline CPU profile, watcher-only trace pair, `f4-nav-fast-{go,qt}.jsonl`, final CPU profile/summary, native-key traces, and `f4-nav-wait.trace` / blocking profile. Analyze runner traces with `tools/analyze_navigation_benchmark.py`; inspect CPU files with `go tool pprof` and the blocking trace with `go tool trace`.
