# Test Optimization Plan – Updated 2026-08-07

**Goal:** Reduce the test suite wall-clock time from ~23s to ~10s for local development.

**Current baseline:** ~22.9–23.3s (measured after reverting the parallelization attempt; see below).

**Top offenders (still relevant, but note that Colorer caching has been applied):**

| Test | Time (approx.) | Issue |
|------|---------------|-------|
| `TestAllDialogs_LayoutValidation/Settings.Colorer` | ~0.3s (was 2.09s) | **Cached** – now fast |
| `TestAllDialogs_LayoutValidation/Settings.Editor` | ~1.4s | Editor settings dialog (still slow) |
| `TestPanelsFrame_DriveMenuBookmarkKeys` | 1.42s | Uses `t.Setenv` + `t.Parallel` (causes panic) |
| `TestTerminalView_ProcessFar2lInteract_ConcurrentRace` | 1.51s | Artificial delays |
| `TestIssue117_OSC52_Read_SecurityDenial` | 1.53s | Timeouts |
| `TestAsyncBuffer_ContextRace` | 1.05s | Race condition test |

---

## Progress Status

| Step | Description | Status |
|------|-------------|--------|
| 1 | **Cache Colorer schemas** | **DONE** for `TestAllDialogs_LayoutValidation` (added `_ = ListColorerSchemes()` at the start). This reduced `Settings.Colorer` from ~2s to ~0.3s. Still needs to be applied to other tests that use Colorer (e.g., `colorer_plugin_test.go`). |
| 2 | **Parallelize safe tests** | **ATTEMPTED & REVERTED** for `TestAllDialogs_LayoutValidation`. Parallelization caused panics due to global `vtui.FrameManager` state and required complex isolation; after fixing panics, the overall time increased to ~25s (overhead outweighed benefits). The attempt taught us that this test is not a good candidate. Other tests that do not share global state can still be parallelized. |
| 3 | **Replace `time.Sleep` with `assert.Eventually`** | **NOT YET DONE** – planned for the next iteration. Expected savings: ~1–2s. |
| 4 | **Remove `t.Parallel()` from tests using `t.Setenv` / `t.Chdir`** | **NOT YET DONE** – will be addressed when profiling those tests. |
| 5 | **Add `-short` flag** | **NOT YET DONE** – will skip integration tests (Lua, 7z, external processes) in normal runs. |

---

## Lessons Learned from the Parallelization Attempt

- **Global singletons are the enemy of parallelism.** `vtui.FrameManager`, `AppConfig`, `GlobalHotkeysMgr`, etc., are all shared across tests. Isolating them per subtest required copying and overriding, which introduced overhead and race conditions.
- **Creating a fresh `FrameManager` for each subtest is expensive** – it reinitialises screens, panels, and VFS, which adds significant CPU time.
- **`t.Parallel()` does not guarantee speed-up** if the parent test is not parallel and the subtests are not truly independent. In our case, the parent test was sequential, so subtests ran sequentially anyway, but with extra setup cost.
- **Conclusion:** For `TestAllDialogs_LayoutValidation`, the sequential version with Colorer caching is optimal. Parallelization should be reserved for tests that are already fast and do not share heavy resources.

---

## Revised Priorities

1. **Cache Colorer schemas in other tests** – extend the warm-up to `colorer_plugin_test.go` and any other test that loads schemas repeatedly.
2. **Eliminate `time.Sleep`** – replace with `assert.Eventually` in the three async tests listed above.
3. **Add `-short` flag** – skip long integration tests during local development.
4. **Parallelize only isolated tests** – those that do not touch global managers or filesystem outside `t.TempDir()`. This includes many unit tests in sub‑packages (`piecetable`, `textlayout`, `vfs`, etc.).
5. **Refactor global state** (long‑term) – consider using dependency injection or a test‑specific context to make tests more independent.

---

## Next Steps (Detailed)

- **Immediate:** Apply patch to replace `time.Sleep` with `assert.Eventually` in `terminal_view_test.go`, `ansi_parser_test.go`, and `async_buffer_test.go`.
- **Short‑term:** Add `-short` flag to `run_all_tests.sh` and skip integration tests.
- **Medium‑term:** Audit all tests for `t.Setenv` / `t.Chdir` usage and either remove `t.Parallel()` or restructure them to use `t.TempDir()`.
- **Long‑term:** Refactor global singletons to support per‑test instances (e.g., using `sync.Once` with reset functions).

---

## Expected Outcome

After implementing steps 2 and 3, we aim to reduce the total time from ~23s to ~18–20s. Further gains will come from parallelizing independent tests and skipping integration tests, potentially reaching the ~10s goal.

