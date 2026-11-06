# Masonry material and texture preparation attribution

Goal remains sustained 160 Hz scrolling without spikes. This experiment does
not establish completion. No production deployment, commit, or push was made.

## Measured attribution

The earlier source-burst prototype improved SVG eligibility latency, but did
not improve preparation p99. Additional diagnostic scopes divide material work
into uniform callbacks, uniform writes, sampled-image callbacks, and shader
resource binding work. Samples are bounded and pre-touched. Slow per-call
metadata is recorded only at >=100 microseconds; per-frame sums include all
calls. Nested scopes must not be added as independent work.

The observer-off/on pair used the same executable and quarter-window middle
scrolling, with four ten-second blocks per phase. Both passed the strict motion
gate without display misses. Median public preparation was about 0.30–0.32 ms
with the material observer disabled and 0.39–0.40 ms enabled. The difference
shows material observer overhead; temporal noise prevents treating it as an
exact correction. Later micro-instrumentation adds further uncalibrated cost.

In the micro capture's cold phase, ten >=100 microsecond
QSGOpaqueTextureMaterial calls consumed 3.3934 ms in total. Of that, 3.1807 ms
was inside sampled-image callbacks, versus 0.1020 ms in uniform writes and
0.0018 ms in SRB work. The largest call was 0.6749 ms, including 0.6737 ms in
the sampled-image callback; its existing SRB was unchanged.

Thirty-one slow QSGRhiShaderEffectMaterial calls consumed 4.2521 ms, including
4.1758 ms in sampled-image callbacks. This is a conditional set of slow calls,
not the distribution of all calls. Cold alpha sampled-image per-frame p99/max
were 0.1121/0.6919 ms; warm values were 0.0336/0.0798 ms. The capture passed
strict middle-scroll motion validation with no display misses. It does not
prove spike-free behavior beyond its finite windows.

These results direct the next experiment toward texture preparation rather
than uniform copies or an assumed shader-cache problem. They do not yet prove
that atlas allocation is the specific expensive operation inside the callback.

## Diagnostic standalone-thumbnail experiment

An artifact-only provider wrapper delegates image conversion, size, byte count,
and pixels to the ordinary Qt texture factory. Only managed zg-thumb- sources
use the alternate createTexture call, and only when the diagnostic environment
flag is present. Normal viewer images and interface icons retain their factory.

Qt 6.11.1's standard factory requests TextureCanUseAtlas. The wrapper omits
that flag; this is a hypothesis to test, not a promoted optimization.
[Qt source](https://raw.githubusercontent.com/qt/qtdeclarative/v6.11.1/src/quick/util/qquickpixmapcache.cpp)

Provider compilation and diagnostic host linking passed. At forced DPR 1.75,
Direct3D passed eight selected native checks, including bounded factory
residency, folder-preview rendered pixels in three modes, repeated navigation,
and pending-thumbnail presentation. This is selected regression coverage,
not an assertion that every factory callback or full application mode is covered.

OpenGL passed initialization and the cache-only check, then could not create
its WGL/RHI context. Its log reports missing opengl32sw, failed WGL resolution,
and failed RHI creation. The bounded run timed out after 180 seconds. This is
an unresolved environment/verification gap; it is not recorded as a pass or
as proof of a provider regression.

The first live control capture reported 60 Hz and DPR 1.0, versus the prior
160 Hz / DPR 1.75 environment. The validation guard rejected it. Its partial
cold trace is retained, but it must not be compared with the prior captures
or used as 160 Hz evidence. No enabled/control performance result is claimed.

## Evidence and remaining work

Analysis: material-parts-analysis.json, material-details-analysis.json,
material-micro-analysis.json. Capture prefixes: real-scroll-material-observer-*,
real-scroll-material-details-*, real-scroll-material-micro-*. Pre-change source
snapshots are material-observer-before-details/ and material-details-before-micro/.

The standalone-thumbnail build inputs, successful-link receipt, native logs,
and rejected live capture are preserved alongside the source. The ordinary
worktree application was kept alive; only diagnostic launchers were replaced.

Next: restore the representative display environment, compare both paths in
the same executable with material instrumentation disabled, verify folder-mosaic
batching and texture retention, and repeat rapid-wheel motion validation. Reject
the hypothesis if it fails to improve tail times or harms batching. Continue
the broader SVG feature, viewer-priority, portable packaging, and exact deployed
build gates before promoting any pending prototype.
