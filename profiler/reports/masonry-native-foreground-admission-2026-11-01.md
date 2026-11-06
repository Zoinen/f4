# Native Windows foreground admission — 2026-11-01

A diagnostic Qt window can report isActive=true while another process owns the Windows foreground window. In the independent unchanged-host check, Windows foreground stayed on Parsec in all 32 observations containing the diagnostic window; the window was visible, not minimized and not DWM-cloaked. The capture still recorded Qt active=true. Native focus ownership must be checked independently before using these captures for cadence decisions.

## Evidence

The first fallback-guard comparison retained all four runs:

| Case | Arm | Displayed intervals in input | Drops | Largest display interval ms | Coverage holds |
|---|---|---:|---:|---:|---:|
| cold-u11 | control | 39 | 4718 | 1031.2294 | 0 |
| cold-u12 | candidate | 40 | 4757 | 1006.2530 | 0 |
| cold-u13 | candidate | 42 | 4757 | 1018.7404 | 5 |
| cold-u14 | control | 42 | 4756 | 1024.9866 | 0 |

These runs are excluded from the optimization comparison. They have no native foreground evidence, and almost all presents were discarded. No per-run Windows foreground observation was collected for cold-u11 through cold-u14; the independent cold-v11 control established the foreground mismatch and reproduced the same presentation symptom. This is evidence for a capture confounder, not proof of which foreground window existed at every instant of the earlier runs.

cold-v11 used the previously frozen host with SHA-256 `9a74cacc3bb67bfd7027508b409f026c3e68735e00fc2c1d139dc30fcdb0cbd9`, without the fallback-guard QML overrides. Its 1 Hz read-only native observer found foreground=Parsec and diagnostic foreground=false in every observed interval. Remote-session flag was false; the active primary display adapter was NVIDIA RTX 4090, while Parsec virtual outputs were detached. No monitor-power explanation was proved or applied.

## Corrected diagnostic gate

The new artifact-only host uses a native GetForegroundWindow/GetWindowThreadProcessId check. Its empty-folder gate waits for external release and native foreground ownership. The capture starts PresentMon and releases navigation only after the Computer Use helper has been reset and the native owner matches the exact diagnostic Qt PID. Input packets record nativeForeground separately from Qt active. The generic [capture_foreground.py](D:/Code/f4-zoin/profiler/capture_foreground.py) verifier requires owned native foreground at admission, no helper process, native foreground at navigation, nonempty input, and native foreground at every input sample.

The diagnostic was selected by its returned Computer Use process path and window handle, activated, then the Computer Use session was reset. Its helper was confirmed absent before timing. No Computer Use observer, screenshot, tests, or build ran during timing. The native gate observes foreground at admission/navigation/input samples; it does not claim continuous monitoring between samples.

## Foreground-validated baseline cold-w11

The Gallery implementation is the frozen baseline, with diagnostic-only native admission and input fields. Exact new host SHA-256: `66a97206f09d3373c5aff36a146cd889edabcdb18ede7ac1b0f968050fd92b66`. Initialized-app entry into Downloads, 192 rows, Masonry density 152, DPR 1.75 and 160 Hz were preserved. The thumbnail cache was empty before QML and folder navigation. The OS file cache was not claimed cold.

| Metric | Result |
|---|---:|
| Displayed intervals | 4799 |
| Display p50 ms | 6.2499 |
| Display p95 ms | 6.26261 |
| Display p99 ms | 6.286519999999999 |
| Largest display interval ms | 12.5013 |
| Intervals >9.375 ms | 1 |
| Dropped presents | 0 |
| Coverage holds | 0 |
| Native-foreground input samples | 2816 |
| Input samples without native foreground | 0 |

This restores representative displayed delivery; it is not a product optimization or a candidate/control performance comparison. Input event counts also differ from the occluded runs, so their CPU totals are not an equal-workload comparison. One missed refresh remains. The motion analyzer still excludes 4,793 wheel pairs and produces zero physical wheel-error samples; one motion commit is unmatched. Full warm-tour and middle-button acceptance were not run. Stable 160 Hz remains unproved.

## Tests and lifecycle

Seven admission regressions pass: valid native evidence, Qt-active/native-inactive rejection, old captures without native fields, empty input, wrong/missing PID, contradictory helper state and missing native navigation foreground. The original false Qt-active condition is explicitly covered. The fresh host passes GUI-subsystem and system-DLL audits. This diagnostic host is not a rebuilt portable product payload.

All owned diagnostics are closed; normal Go PID 9252 and Qt PID 106980 remained alive. Temporary refresh-rate leases restored the previous 60 Hz mode without registry changes. Closed diagnostic binaries were copied and hash-verified to the P archive before replacing their workspace files with symlinks to recover worktree disk space. No product rollout, commit or push occurred.

## Next comparison

Link the existing fallback-guard control/candidate QML override objects onto the new foreground-entry host response graph. Publish a fresh worktree diagnostic for each arm, and repeat A/B/B/A using unused case IDs. For each window, wait at awaiting-foreground.json, select the exact returned process/window, activate it, reset the helper, verify no helper process and native foreground, then create allow-capture.json. The capture starts its collector and releases the native gate itself. Keep all misses, drops, coverage holds and excluded motion pairs. Do not reuse the rejected cold-u cases or treat the single validated baseline as a candidate win.

> AI assistance has been used to create this output.
