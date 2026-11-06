# QML signal-mask production integration

The normal worktree app now uses the wider conservative QML signal mask. This
promotes the repeatedly measured notification reduction, not the rejected atlas,
standalone-texture or D3D binding-reuse candidates. Constant 160 Hz without spikes
is still unverified. Nothing was committed or pushed.

## Source and dependency integration

ci/qt_qml_signal_policy.py applies six strict, reversible changes to Qt 6.11.1's
qqmlengine.cpp. Its private allocation extends NotifyList with eight relaxed
atomic mask words while preserving the SDK prefix and existing mask. Allocation,
typed deletion, retained-list clearing and connectedness lookup change together.
Modulo-512 collisions remain conservative; there is no unsafe cross-thread
walk of exact notifier endpoints. The production patch has no diagnostic switch.

ci/patch-qt-dependencies.py embeds the patch into the Conan recipe, validates all
anchors before saving the modified source, and refreshes an existing recipe
block idempotently. All six portable dependency cache keys include this policy,
the existing upload policy and the animation-clock policy.

For the local build, a separate Qt6Qml.lib replaces precisely the original
qqmlengine implementation member. Installed SDK headers and archives remain
unchanged. A SHA-checked CMake override selects this archive alongside the
previous production Qt Quick library. The CI source patch is the reproducible
build path; no local artifact path is embedded into the Conan recipe.

## Validation

- 45 Python recipe/cache tests pass.
- F4QtQmlSignalMaskTests passes nine cases, with no failures or skips: cross-thread
  arguments and bindings, C++ receivers, 64/512 collisions, all 513 generated
  signal indices, repeated engine destruction, and retained-list reconnection.
- Drawing-order and curve native CTests pass.
- Import isolation and compiled production-host loading pass four cases, with
  no failures or skips. The Windows system-DLL import audit passes.
- The viewer/panel stock-QML and patched controls both report 156 passes,
  18 failures and three skips across the same 177 cases. No new failing assertions
  or skips appear. Existing failures include tab/breadcrumb pixel placement and
  pointer interactions; these are not declared fixed or ignored as passing.
- The dialog suite reports 59 passes, five failures and six skips at DPR 1.
  Each of the five failed cases reproduces at the same assertion when linked
  with stock Qt QML and otherwise identical application objects.

The two broader suites therefore remain red. Their stock-QML controls isolate
the observed failures from this change. They do not replace fixing the existing
issues, prove all UI behavior, or prove constant-160-Hz performance. Suite elapsed
times are not scrolling benchmarks.

Two test executables initially hit a linker response-file line-length failure
that truncated object paths. Relinking the same inputs with one argument per
line succeeds; the original and normalized response files and receipts are kept.
No source fix was inferred from that linker failure.

## Embedded build and restart

The deterministic compressed host was regenerated and byte-hash checked after
decompression. A CGO_ENABLED=0 launcher was built using the system Go cache.
Generated-payload, extraction/reuse, concurrent first-launch, corrupt-payload
and lookup-priority tests pass.

The first restart encountered an executable file lock; rollback restored the
previous app. The corrected restart waits for exact-path process termination
and retries the copy. The new app has a responsive native window and runs its
verified host from the content-addressed cache with no F4_EXT_UI_PATH override.
The canonical D:\Code\f4\f4.exe was not touched.

Deployed launcher SHA-256:
093384cb51580023067727b38b3a657047ad1e3da1a89c651ace5beb4656d7f7

Deployed Qt host SHA-256:
eaa67261d463d4cccaa85a7b1a95136528c32c4e082be753b353cfd80fa7cb22

Host payload/cache key:
7b174e9b10d38e96fe6f7712e4598396dbaf12634e389fe4cdb6b4b0aeed50fb

The earlier live comparisons establish removal of tens of thousands of
unnecessary proxy deliveries and lower outer-pass p99 in both run orders.
Fresh production scrolling measurements and remaining cadence/CPU investigation
are still needed. Linux/macOS runtime and release-matrix verification were not
performed in this Windows worktree; no cross-platform release claim is made.

Evidence: artifacts/qml-signal-production-live-identity.json,
artifacts/qml-signal-portable-build-receipt.json,
artifacts/qt-qml-signal-production/build-receipt.json,
artifacts/qml-signal-ui-coverage-verification.json,
artifacts/qml-signal-view-surface-comparison.json,
artifacts/qml-signal-production-native-final.txt,
artifacts/qml-signal-production-imports.txt,
artifacts/qml-signal-policy-python-final.log and per-stage build/test logs.
