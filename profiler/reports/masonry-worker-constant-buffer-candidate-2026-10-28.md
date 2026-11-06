# Fresh constant-buffer preparation implemented; representative capture pending

The worker reserve now includes fresh constant buffers, addressing the 1024-byte
12.2859 ms creation observed in the preceding geometry-only experiment. Data,
range binding, QRhi integration and queued-read safety checks pass. This is a
candidate implementation, not evidence that the measured stall or whole-scene
spikes are eliminated. No live comparison has been run for this variant.

## Reserve and binding contract

The worker retains full geometry size coverage (4 bytes–4 MiB), but reduces
each 4 MiB vertex/index class from two slots to one. It adds two constant-buffer
slots for each power-of-two capacity from 256 bytes through 64 KiB. The 100
slots sum to 25,427,440 bytes, about 24.25 MiB, below the unchanged 32 MiB
per-device reserve capacity bound. This does not bound driver overhead,
adopted active buffers or total process memory. At most 32 device records remain.

Only exclusive constant binding, DYNAMIC usage, CPU_WRITE, zero misc/stride,
256-byte minimum, 64 KiB maximum and 16-byte-aligned requested sizes qualify.
Geometry descriptor rules stay unchanged. Adoption requires capacity at least
the requested size and strictly less than twice it. Unsupported combinations,
missing slots, unsupported concurrency or other threads keep the original path.

These are fresh native resources, not recycled buffers still in GPU use. The
existing ownership, GPU-completion and DISCARD/NO_OVERWRITE policies remain
unchanged. The worker only creates resources; context operations stay on their
original threads. Shutdown joins the worker and releases reserve resources.
Logical QRhi/shadow sizes, update lengths, binding offsets and draw sizes stay
unchanged. The legacy diagnostic feature flag and snapshot key still use the
geometry names; per-device uniformHits/uniformMisses distinguish constant use.
The capture helper requires actual constant-buffer hits before accepting an
enabled run. Worker resource/producer labels now use buffer.prewarm.

## Verification achieved

- Nine direct D3D11/QRhi checks pass, including setup/cleanup. Actual QRhi
  allocation of a logical 1536-byte uniform buffer adopts a native 2048-byte
  resource and increments the uniform-hit counter. Byte copies/readbacks cover
  geometry and constant buffers, descriptor bounds, reserve bounds, wrong
  threads and shutdown.
- A compute shader reads two values from a prepared constant buffer through
  CSSetConstantBuffers1 with a 256-byte starting offset and 1024-byte range.
  It returns the expected 1111 and 2222 values. This tests native binding
  semantics, not just a CPU copy of allocation bytes.
- The previous geometry-only implementation fails constant-descriptor
  eligibility, with its own old header/ABI and calls object. Baseline result:
  two pass, one expected failure.
- Four queued-read safety checks pass using prepared constant resources and
  the existing reuse guard. At least two actual constant-pool adoptions are
  required. Earlier same-frame reads retain old bytes; unrelated updates and
  unknown/untracked/unsupported cases preserve fallback behavior.
- Embedded QML import isolation passes four checks. The Windows system-DLL
  dependency audit passes. No UI layout changed.

## Display change and limits of rendering evidence

Unforced native scene/shader checks now observe QWindow DPR 1.25, rather than
the required 1.75. Windows reports 3840x2160 at 59 Hz for the NVIDIA controller;
the Parsec virtual adapter has no current mode in that query. The failed test
logs are preserved (native: 10 pass/37 fail; shader: 3 pass/24 fail). Their
environmental failures do not establish native rendering correctness at 175%.

With QT_SCALE_FACTOR=1.4 over the current 1.25 environment, the window reports
1.75 and all 47 native scene checks and 27 rendered shader checks pass, with
zero skips and failed reuse maps. These are explicitly forced-DPR static
rendering checks, not an actual restored 175% / 160 Hz desktop and not a
performance comparison. They do not replace representative hardware capture.
No display setting was changed and no 160 Hz timing value was fabricated.

The user has been asked to restore the measurement display configuration;
the response is pending. Live enabled/control/control/enabled captures remain
required, including true 160 Hz / unforced DPR 1.75, constant-buffer feature use,
display/motion gates, loading and memory. The candidate is not promoted and
the original constant-160-Hz objective remains active and incomplete.

## Workspace and evidence preservation

Disk cleanup removed eight inactive duplicate Go diagnostic launchers only
after exact artifact-directory, inactive-path and SHA256 checks against the
preserved working f4-zoin.exe. It recovered 1,312,972,800 bytes; paths/hashes are
recorded in artifacts/removed-duplicate-launchers-coverage-20261028.json.
Native hosts, objects, sources and capture packets were retained.

The normal app remains unchanged (Go PID 43936 and embedded host PID 119460).
No embedded payload replacement, commit or push occurred. The matching
experiment snapshot contains sources, build hashes, old/forced-DPR test logs,
binding/safety evidence, current display query and external binary references.
