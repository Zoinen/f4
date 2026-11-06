# Masonry: collector isolation and renderer waits

Constant 160 Hz remains unachieved. Removing PresentMon does not eliminate
long compositor cycles. The same capture exposes a concrete application-side
stall: the experimental buffer-prewarming worker contends with the render
thread inside D3D11/NVIDIA calls. This is evidence about the experimental host,
not attribution of the production application's stalls to that worker.

## Capture and limits

Case `real-scroll-nopmon-b1-20261031-wheel-masonry` uses the pinned diagnostic
host SHA256 `631b87d7976664b5f55b3400d0ef23cc48576a58b85d7aa788000b8f4c608a11`.
It preserves the preceding control's renderer flags, including asynchronous
buffer prewarming and inline dependency sets. Actual Downloads contains 192
entries including the parent item; Masonry density is 152, output 3840×2160,
refresh 160 Hz, actual DPR 1.75. Two ten-second wheel windows deliver 1694 input
events with zero inactive inputs. The process starts with an isolated empty
gallery cache. The second window is not a verified-warm workload.

PresentMon is absent. Before input, provider enumeration verifies exactly one
added Dwm-Core logger with any-keyword mask `0x89`, all-keyword mask zero.
Kernel profiling and the narrow graphics collector remain intrusive. There is
no display-performance CSV and no displayed-frame or motion acceptance result.
No screenshots, UI Automation polling, builds or offline analysis run during
the gestures. Trace event/buffer losses and decoder errors are zero.

## All-cycle control

The comparison includes every target-composition cycle inside each input
window, rather than selecting cycles already known to miss delivery. Thread
ownership and scheduler coverage are checked; unknown scheduling time is zero.
This is one pair, not a replicated estimate of observer overhead.

| Collector/window | Cycles | DWM scheduled median / p95 / p99 / max (ms) | Cycle wall time >6.25 ms | App preparation max (ms) |
| --- | ---: | --- | ---: | ---: |
| PresentMon + narrow logger / 0 | 1594 | 2.018 / 4.1072 / 6.4397 / 9.9505 | 20 | 7.1465 |
| PresentMon + narrow logger / 1 | 1595 | 1.8669 / 4.3795 / 6.6686 / 10.3629 | 20 | 7.5372 |
| Narrow logger only / 0 | 1595 | 2.1167 / 4.0524 / 5.9013 / 10.0868 | 13 | 10.1466 |
| Narrow logger only / 1 | 1597 | 1.9001 / 3.8721 / 6.1328 / 8.9101 | 15 | 6.2519 |

Cycle wall time above 6.25 ms is **not** a missed-display count. Loading and
membership churn differ between runs. The data does not justify attributing
the remaining compositor cost to PresentMon or claiming an improvement.

## Render-thread attribution

The worst preparation takes 10.1466 ms: 6.8071 ms scheduled CPU, 0.0235 ms
runnable, 3.316 ms blocked, zero unknown. Upload preparation consumes 3.9149 ms
scheduled CPU; alpha-material preparation contains the entire 3.316 ms wait.
The next-worst preparation takes 8.8073 ms entirely on scheduled CPU. Eliminating
the identified wait alone therefore cannot establish the original acceptance
target.

The blocked render thread is 73800 in host process 52248. Its resumed user
stack goes through ntdll, NVIDIA and D3D11, then:

```
QD3D11Buffer::destroy
QD3D11Buffer::create
QSGBatchRenderer::Renderer::prepareRenderUnmergedBatch
QSGBatchRenderer::Renderer::prepareRenderPass
QSGBatchRenderer::Renderer::render
```

The ReadyThread event identifies thread 54652 in the same host as the readier.
That thread's user stack goes through ntdll, NVIDIA and D3D11, then:

```
F4RhiCalls::create<D3D11_BUFFER_DESC, ID3D11Buffer, ...>
F4RhiCalls::fromProducer<...>
F4AsyncGeometry::created(ID3D11Device*)::<worker lambda>
std::thread::_Invoke<...>
```

The experiment's worker calls `ID3D11Device::CreateBuffer` under producer
`buffer.prewarm`. Its final counters report 1492 creates, 1392 adoptions, 235
misses and zero creation errors. A driver advertising concurrent creation does
not guarantee that creation cannot contend with buffer destruction. The trace
identifies the participating call paths; it does not identify a private NVIDIA
lock implementation.

Symbols come from a map-only relink of the captured host's exact inputs. The
entire executable code section and unwind section match byte for byte. That
analysis executable was never launched. The original host and running normal
application remain untouched.

## Next experiment

Disable only asynchronous buffer prewarming in the same diagnostic host, keep
the workload and remaining flags unchanged, and repeat the scheduling capture.
Compare preparation CPU/waits as well as the lost pool hits. Retain the
original full cold-wheel, verified-warm tour/wheel and quarter-height middle
scrolling acceptance gates before promoting any renderer change. Do not infer
a passing display result from a capture without display measurements.

Evidence is frozen under
`profiler/experiments/masonry-collector-isolation-and-render-waits-2026-10-31`.
Raw ETLs and large decoded packets are external files with hash receipts.
No production source, portable payload, commit or push changed in this turn.

AI assistance has been used to create this output.
