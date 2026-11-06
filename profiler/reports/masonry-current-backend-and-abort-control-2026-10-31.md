# Current backend checks and rejected D3D12 control, 2026-10-31

## Result

Do not switch the default backend. The corrected renderer graph passes 46
native tests on each actual D3D11/D3D12 backend at DPR 1.75, and all 25 output
PNGs match byte-for-byte. However, the D3D12-requested full-folder live run
aborts in Qt after its verified warm tour. It produces no final profile packet
and cannot support a throughput, motion, backend-identity or full-acceptance
claim. Constant 160 Hz remains unachieved.

The preceding goal turn made progress by repeating cold/warm/middle captures,
confirming native camera precision and rejecting a queue-depth-one control.
This turn checks another configuration against the current corrected graph,
identifies backend-specific diagnostic assumptions and resolves the abort
instruction against an exact code match. No production source, installed Qt
SDK, default backend, normal app or portable payload is changed.

## Dirty-region feasibility

The current Qt D3D11 implementation uses a flip-discard swapchain and calls
`Present()` after rendering the full target. Adding a partial-damage hint alone
would not be a correct optimization. Microsoft requires an app using dirty
rectangles to confine writes to those rectangles and keep the entire declared
region current. A valid implementation would need corresponding rendering and
buffer-content tracking, rather than merely changing the present call.
[Microsoft Present1 contract](https://learn.microsoft.com/en-us/windows/win32/api/dxgi1_2/nf-dxgi1_2-idxgiswapchain1-present1),
[incremental buffer tracking](https://learn.microsoft.com/en-us/windows/win32/direct3ddxgi/dxgi-1-2-presentation-improvements).
No dirty-rectangle or swap-effect patch is applied.

Earlier backend controls used an older cold workload. The present control
instead uses the current running-thumbnail preservation fix and a verified
warm route. This is an exploratory backend configuration control, not an
isolation of one driver/compositor function: backend-specific optimizations
can take their defined fallback paths.

## Native tests and counter expectations

The artifact copy of the current native test source adds an explicit
`QSGRendererInterface::Direct3D12` assertion alongside existing D3D11/OpenGL
assertions. It does not change product tests or rendering code.

The initial D3D12 execution has 44 passes and two failures, both diagnostic
assumptions: shared-uniform cache hits must be nonzero and dependency-cache
entries must be built. Source inspection establishes that
`SharedMergedUniformWordHash20261031.h::begin()` and the preprocessing supported
scope intentionally accept only `QRhi::D3D11`. Thus D3D12 must use the original
paths with zero shared-uniform hits/updates and no allocated dependency graph.

Two intermediate copies also contain incorrect fallback-counter expectations:
one expects a bypass counter, and another expects zero scope passes. The scope
returns before allocating state, but its destructor still counts each pass
and original node walk; bypass count remains zero. All failed versions and
outputs are retained. The final v4 copy asserts those exact source-defined
conditions without changing geometry, native leaf transforms, small-text,
image, clipping, ordering, mutation or rendered-pixel checks.

Final results: 46 passes / zero failures / zero skips on each actual backend.
Each produces 25 PNGs; all pairs are byte-identical. The inherited 175% leaf
origin/unit-vector assertions remain in force. The live control's diagnostic
guards similarly distinguish inactive D3D11-only counters from quality and
performance requirements. Actual API enum 8 remains mandatory for accepting a
D3D12 profile packet.

## Live attempts

Real Downloads, 192 entries, Masonry density 152, 3840x2160 desktop, actual
160 Hz / DPR 1.75, ordinary window behavior and four decode workers. No
background app, VM, process priority or external affinity change is made.

### B1: wrong backend, rejected

`real-scroll-postsync-currentd3d12warm-b1-20261031-wheel-masonry` produces a
complete packet with `graphicsApi == 4`. The shared launcher removes
`QSG_RHI_BACKEND`, discarding the control's outer environment setting.
The capture fails its diagnostic guard and is explicitly rejected. Its full
packet is preserved and hash-verified; it is not repurposed as a favorable
D3D12 result. Only its exact diagnostic process tree is stopped.

### B2: requested backend retained, Qt abort

The dedicated launcher sets `QSG_RHI_BACKEND=d3d12` after sanitization. A
read-only check before timed input confirms that Qt child PID **22104** retains
that value. Its complete warm-tour receipt visits all 192 indices, preserves
normal visibility and reports `all-entries-displayed-and-quiescent`, with no
pending previews. The final profile packet is never published; the last state
is phase `up`.

The wrapper reaches its 140-second observation deadline. Current process
inspection then confirms the Qt process, diagnostic Go launcher, PresentMon
and matching Python capture processes are terminal; the timeout alone is not
used to infer termination. Windows Application events 1000/1001 independently
record the abort of PID **0x5658 = 22104**, exact executable path and report ID
`91626a31-83f0-4314-8f29-8d9029da8717`. They report:

- Original module timestamp `0x6ae5bbf2`.
- Exception `0xc0000409`, recorded parameter 7.
- Fault RVA `0x8d3688` in the diagnostic host.

The WER report remains available; its temporary minidump is absent. No call
stack or driver cause is claimed from the fault code. The requested-backend
environment is not substituted for the missing final packet's actual-API
assertion. There is no accepted full D3D12 scrolling result.

## Exact code mapping

The current host is relinked with the same response graph solely to produce a
symbol map; the relink is not launched. Before using it, the original and
relinked `.text` and `.pdata` bytes, RVAs and sizes are required to match:

- `.text`: 42,244,096 bytes, SHA-256
  `c20ea843b02f7dbe426ce29a719e97d33b8c79036d557de87b3291495eacf62b`.
- `.pdata`: 1,650,688 bytes, SHA-256
  `dcc5149149c7ae53f8cb65ab39c989f376edf44a66aceefb3bb53dd2b6dd755d`.

The fault falls inside the unique runtime-function interval
`0x8d3670..0x8d369f`, at `qAbort()+24`, from `Qt6Core:qassert.cpp.obj`.
This establishes the abort instruction, not its caller. A map from an earlier
host is not used. The next diagnostic should capture the fatal Qt message and
caller before abort, using a fatal-only sink with no per-frame file logging;
it should also persist the actual API before timed input. Do not interpret
this failed configuration as an optimization or silently narrow its workload.

## Storage and final state

The completed native-test folder is copied from its physical C: archive to a
workspace storage directory. All 273 files are hash-checked, and both original
aliases are preserved through checked junctions before retiring the verified
old physical copy. An initial Windows PowerShell/.NET compatibility failure
occurs before any source move; the existing copied destination is resumed and
fully verified, rather than recopied or deleted blindly. A completed October
17 ETL is transparently compressed with identical logical SHA-256, preserving
it while reclaiming 385,716,224 allocated bytes. No unrelated files are removed.

All owned live diagnostics are terminal. The normal Go PID 23488 and Qt PID
84780 remain alive at their original paths and hashes. Current product source
still matches the running-thumbnail candidate receipt. No commit/push or
portable rollout occurs. The goal stays active.

Compact evidence is preserved in
`profiler/experiments/masonry-current-backend-and-abort-control-2026-10-31/`:
325 source/script/result/image files plus 22 verified external receipts.

> AI assistance has been used to create this output.
