# Empty-overlay pointer traversal experiment

## Status

Experimental diagnostic build only. No production dependency, embedded payload,
or normal running application was replaced. No commit or push. Constant 160 Hz
without spikes remains unverified. The user authorized unattended captures.

## Evidence and change

The preceding GUI CPU attribution (`artifacts/preprocess-incremental-bounded-20261029/prior-gui-wheel-attribution.json`)
contains 223 `QQuickDeliveryAgentPrivate::eventTargets` self samples: 147 through
the overlay modal filter and 76 through normal pointer delivery. Those are
samples from the prior renderer trace, not a measurement of this candidate.

An overlay can remain visible because it contains ordinary application children,
even when `stackingOrderPopups()` is empty. Qt 6.11.1's modal filter still calls
`pointerTargets()` before discovering that there are no stacked popups.

The native regression constructs that hierarchy, counts background `contains()`
calls, and invokes the actual overlay wheel filter. The installed baseline makes
one unnecessary call and fails the zero-call assertion. With the fast path
enabled, it makes zero calls and passes. Modal and modeless popup cases preserve
outside-wheel blocking and inside-wheel passage.

The diagnostic replacement returns early only when no stacked popup exists.
It retains the original nonempty-popup path, including its later popup-list
lookup after hit testing. `F4_PROFILE_OVERLAY_EMPTY_FASTPATH=1` selects the new
path; the environment setting is cached once per process. The same executable
is used for the control and candidate. The bounded preprocessing dependency
cache is enabled in both arms.

Source: [Qt 6.11.1 qquickoverlay.cpp](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quicktemplates/qquickoverlay.cpp).
The pinned reference and its hash are retained with the preceding experiment.
No overlay patch was found in the project's dependency patch script. The
installed Conan source tree no longer exists, so the diagnostic rebuild uses
the pinned version source and installed headers. This is a limitation of the
replacement experiment; it has not been promoted into dependency packaging.

## Checks and build corrections

- Installed baseline: modal/modeless cases pass; empty-overlay regression fails
  with one background hit test, as intended.
- Final replacement: five native QtTest results pass, zero fail. Feature-off
  still reproduces the regression.
- Embedded-only QML import isolation: three results pass, zero fail.
- Windows dependency audit passes with system DLL imports only.
- No static product layout, pixel alignment, image quality, or frame acceptance
  threshold was changed.

The first source compilation lacked the private-header include directory. The
first moc regeneration omitted configured QML class metadata; the import gate
caught the resulting unavailable attached object. Both failures were preserved.
The final build uses the original configured Qt moc output with only the
removed source-header include path remapped to the installed header. The final
native tests and import-isolation gate were rerun successfully.

## Live captures: rejected, no speedup claim

Both recorded cases use actual DPR 1.75, 160 Hz, cold isolated caches, and the
existing Downloads/Masonry workload. Original acceptance gates were retained.

| Case | Fast path | Wheel events | Qt inactive events | Native foreground during both windows | Result |
| --- | --- | ---: | ---: | --- | --- |
| overlay-a1 | Off | 1376 | 0 | Target Qt process, stable | Rejected: fewer than 1500 wheel events |
| overlay-b2 | On | 1448 | 0 | ShellExperienceHost, not target Qt process | Rejected: insufficient events and foreground mismatch |

The native observer covers both measurement windows of each case, with zero
foreground callbacks inside them. In B2, Qt's recorded active flag disagrees
with the native foreground owner. That capture cannot establish performance.
The wrapper process returning zero does not override the recorded child capture
failure. No aggregate timing comparison or improvement percentage is reported.

The attempted B1 run stopped at the disk-space preflight and produced no gesture
packet. Completed files and three inactive old diagnostic executables were
losslessly NTFS-compressed, with SHA-256 checks before and after. No trace was
deleted and no acceptance guard was relaxed.

## Artifacts and next work

Frozen compact evidence: `profiler/experiments/masonry-overlay-empty-fastpath-2026-10-29/`
contains 48 files and a manifest identifying 13 external binaries/raw outputs.
The native sources, build scripts, capture scripts, failures, final results,
source/build receipts, compression receipts, foreground records and validity
analysis are preserved. Large packets, executable files and captures remain at
their original artifact paths and are hash-pinned by the manifest.

The diagnostic launcher was stopped by its exact executable path. The normal
worktree application at `D:\Code\f4-zoin\f4-zoin.exe` remains alive (PID 43936).

Next: resolve the diagnostic foreground disagreement without relying only on
Qt's active flag, then obtain accepted same-host control/candidate captures.
Broaden popup behavior coverage before any production integration. This
experiment demonstrates redundant-work removal, not attainment of the
zero-spike scrolling goal.
