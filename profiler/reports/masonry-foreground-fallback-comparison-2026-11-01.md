# Foreground-validated fallback comparison and cold-entry work — 2026-11-01

The fallback-resolution candidate does not establish a repeatable scrolling improvement and is not promoted. Four fresh foreground-validated runs retain all missed refreshes, drops and motion exclusions. Including the pre-input window exposes a larger problem: initial delegate commits take 35–44 ms and were outside the previous input-only headline.

## Matched diagnostic builds

Control SHA-256: `c2cafbf4057e30e262820d085b50c0b31026f7abaa7c11ac32babfb3b1b45edb`. Candidate: `db0da491503f25bba924ec6a57fda7db767e2a26631a82f4a7b8ba6322e16257`. Both link the same native foreground-gated main object and native census graph. Control/candidate QML override objects and raw resource bundles are inherited from the previously tested fallback-guard prototype. The candidate adds only the thumbnail-expected icon-resolution guard and the matching marker-renderer guard. Neither product source nor the normal app payload is changed.

The operation is initialized-app cold entry from a private empty directory into actual Downloads, both panels, 192 rows, Masonry density 152, maximized 3840×2160 desktop, actual DPR 1.75 and 160 Hz, followed by 30 seconds of rapid wheel input. Each process has its own empty thumbnail cache before QML and navigation; the OS file cache is not claimed cold. Native Windows foreground ownership is confirmed at admission, navigation, and every input sample. The Computer Use helper is reset before the collector releases navigation and absent in both observer checks. No builds, tests, screenshots, QML profiler or CPU/scheduler trace run during input. Native census instrumentation is present equally in both arms, so these are matched diagnostic timings rather than a final uninstrumented product audit.

## Input-window results

| Case | Arm | Display intervals | p95 ms | p99 ms | Max ms | >9.375 ms | Drops | Coverage holds | Excluded wheel pairs |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| cold-z21 | control | 4798 | 6.26452 | 6.30750 | 18.7468 | 3 | 0 | 0 | 4792 |
| cold-z22 | candidate | 4800 | 6.26390 | 6.30041 | 12.5089 | 2 | 0 | 0 | 4795 |
| cold-z23 | candidate | 4800 | 6.26361 | 6.29600 | 12.5025 | 1 | 0 | 0 | 4795 |
| cold-z24 | control | 4800 | 6.26390 | 6.29482 | 12.4973 | 1 | 0 | 0 | 4794 |

The candidates miss three refreshes across the two runs; the controls miss four. This small count difference, overlapping phase costs, and unequal async loading at input start do not establish a causal speedup. Both candidates still miss refreshes. Later maximum delegate commits are 12.2975 and 11.2857 ms versus 11.0974 and 11.3753 ms in controls. The dominant work has not been removed.

## Preserve work before the first wheel event

The recorder starts input only after the initial viewport becomes available. Reporting only that window omits the largest cold-entry delegate commits. The new [pre_input_attribution.py](D:/Code/f4-zoin/profiler/pre_input_attribution.py) analyzes navigation-to-input separately. It unions nested native commit intervals, reports straddling ranges explicitly, and retains pre-input dropped presents. It excludes submissions made before navigation, even when their eventual display follows navigation. First displayed post-navigation submission is a timing bound on a present, **not proof that its pixels contain the new folder**. Static loading can legitimately have idle gaps, so these gaps are not relabeled as continuous-animation dropped frames.

| Case | Navigation→input ms | Cached frames at input | Pre-input commit union ms | Largest pre-input commit ms | Submitted presents | Dropped | First post-nav submission displayed ms |
|---|---:|---:|---:|---:|---:|---:|---:|
| cold-z21 | 548.6831 | 11 | 187.4151 | 43.5333 | 11 | 0 | 132.1803 |
| cold-z22 | 654.1350 | 20 | 199.2906 | 39.0922 | 12 | 0 | 185.3000 |
| cold-z23 | 650.9610 | 44 | 192.8311 | 38.3774 | 14 | 0 | 188.4330 |
| cold-z24 | 654.1234 | 28 | 187.3944 | 37.3610 | 13 | 0 | 210.8354 |

These unions are elapsed native ranges, not measured CPU residency. They must not be added to inclusive QML ranges or nested callback totals. The underlying raw census has no overflow. All four foreground admissions pass. Loading state at the first input varies substantially, so counting an earlier cache population as a scrolling win would be misleading.

The two largest initial commits in each run process roughly 47–48 row updates. Phase attribution shows their prepare phases dominating: the worst control call spends 38.0442 of 43.5333 ms in preparation, while a candidate call spends 33.3987 of 39.0922 ms there. The existing visual-phase tool and the new interval report are complementary; aggregate phase payloads are not inferred from tiny marker spans.

## Next construction experiment

[MasonryLayout::createComponent](D:/Code/f4-zoin/third_party/ZoinGallery/MasonryLayout.cpp:326) calls createWithInitialProperties and only afterwards sets the new delegate parentItem to the viewport. The root therefore completes construction before final viewport attachment. Several real delegate bindings depend on Window.window and attached pixel-grid state. The next small experiment should install the final parent as an initial property and keep the new root hidden until the normal row commit.

First add a regression observing actual parent and window availability during component completion, and verify the old path fails it. Then test the parent-at-construction candidate against the same baseline. Verify actual leaf origins, unit transforms and rendered small captions at 175%, source requests, visibility, cancellation/ownership and all modes. Preserve target-leading coverage. This is a hypothesis to test; current evidence does not prove how much constructor time a second binding pass consumes. Do not substitute a larger anonymous spare pool or untested asynchronous row creation for this experiment.

## Verification, failures and lifecycle

Five new analyzer regressions pass: pre-input burst retention, clipping/nesting, pre-navigation submission exclusion with drop retention, reversed windows, and malformed native intervals. The exact raw captures, stage binaries, object/response files and results are preserved. Both new host PEs use the GUI subsystem and pass system-DLL audits; the candidate QML graph has the prior five-mode behavior and embedded-import tests. This does not replace portable payload rebuilding or final extraction verification.

One post-activation helper refresh requested neither screenshot nor text and the API rejected that empty request. No second input was injected. The helper was reset; independent Windows foreground ownership and helper absence were then checked before release, and every input sample passed the native foreground gate. That failed pre-timing observation is not used as screenshot or performance evidence.

Workspace publication reused one diagnostic executable path only after the preceding owned process pair closed. Per-case admission hashes and immutable C-stage/P-archive hashes are reconciled in publication-history.json; historical receipt entries for the reused workspace alias are snapshots at admission, not claims that its current contents still represent every old arm. No capture case was restarted.

All four owned Go/Qt pairs are terminal; normal Go PID 9252 and Qt PID 106980 remain alive. The bounded display lease restored the prior 60 Hz mode with no registry change. No normal payload rollout, commit or push occurred.

The full 192-entry/17-stop warm tour, quarter-height middle-button motion, uninstrumented cadence validation and portable rollout remain open. The wheel analyzer still produces zero accepted physical-error samples; excluded wheel pairs are retained above. Stable 160 Hz is not achieved.

> AI assistance has been used to create this output.
