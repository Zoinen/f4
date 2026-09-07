# Video in f4

Issue [#186](https://github.com/unxed/f4/issues/186), step 12. `F3` on a video
file plays it. *Where* and *how well* depend on what the screen underneath can
carry, and that sentence is the whole design: one source of frames, four ways
of putting them on screen, and a rung of the ladder for every target f4 builds
for.

This file replaces the first version of the plan, which had one rung and
mistook it for the feature. Section 2 says what that got wrong and why.

## 1. What is there today, and what it actually covers

`tryOpenVideoPlayer` (`actions.go`) opens `VideoView`, which starts mpv with
`--wid` pointed at an `internal/ttyx` overlay window. mpv owns decoding,
timing, seeking and audio; f4 owns the window and drives mpv down its JSON IPC
socket, because the overlay is override-redirect and shaped out of the input
region and therefore has no keyboard of its own.

That much works. What is worth being clear-eyed about is its reach:

```
tryOpenVideoPlayer -> sharedTTYXSession() == nil -> return false
ttyx.Open          -> $DISPLAY empty            -> ErrNoDisplay
```

So video today means **X11, and nothing else**. Not Wayland without XWayland,
not Windows, not macOS, not a bare tty, not ssh, not the built-in terminal.
CI builds thirty-odd `GOOS/GOARCH` cells with `CGO_ENABLED=0`; on almost all of
them `F3` on an `.mp4` already answers no, and it answers no *before* mpv is
ever looked for.

The conclusion that matters for planning: **mpv is not what costs us the
platforms.** Removing it would not gain a single one. The presentation layer
costs us the platforms. "Fewer external dependencies" and "works on more of the
build matrix" are two separate goals and need two separate decisions; conflating
them is how this feature nearly got designed around the wrong problem.

## 2. Correcting the bandwidth argument

The first version of this document argued that video cannot go down a terminal:
twenty five frames of 976x748 RGBA is 730 MB/s, sixel takes a tenth of that and
spends a core doing it, therefore there is nothing to support over ssh.

The arithmetic is right and the conclusion does not follow, because it prices a
picture nobody asked for. Nothing about `F3` on a video requires full
resolution or full frame rate. What a file manager is asked most of the time is
*what is in this file* — and a fifteen frame a second, panel-sized, or even
character-cell picture answers that completely.

Priced honestly, the low rungs are cheap:

- **Terminal graphics** (kitty, far2l, sixel) at the size of the viewer frame
  and ten to fifteen frames a second is an ordinary image stream, and f4
  already ships every part of it for still pictures.
- **Coloured text** is cheaper still. Half-block cells (`▀`, foreground and
  background set separately, so one cell carries two pixels) on an 80x50 grid
  is 4000 cells; a truecolor `SGR` pair plus the character runs about 41 bytes,
  so a whole frame is roughly 160 KB and 25 fps is about 4 MB/s. With the
  screen diff that `ScreenBuf` already does, and a cap around ten frames a
  second, that survives ssh. These are estimates and want measuring, not
  quoting.

So the rule is not "video is local". The rule is **the picture is sized to what
the transport can carry**, and there is a usable answer at every size.

## 3. Video is four layers, not one feature

Kept apart, they have very different costs, and only one of them is hard.

**Demux.** Pure Go, and half the knowledge is already in the tree:
`plugins/mediainfo` parses EBML/Matroska, ISO-BMFF and RIFF today. It is a
metadata prober rather than a packet demuxer, so it needs extending, not
inventing.

**Decode.** The wall. Section 6.

**Presentation.** Already the limiting factor, already written for still
pictures, and the place where the ladder lives. Section 4.

**Audio output.** The layer everyone forgets, because mpv hides it. Section 7.

## 4. The degradation ladder

| Rung | Where it works | Status |
| --- | --- | --- |
| 1. X overlay, mpv | X11 | done |
| 2. Terminal graphics | kitty, far2l, sixel: ssh, Wayland, macOS, f4's terminal | **missing, largest gain** |
| 3. ANSI half-block art | any colour terminal | missing |
| 4. Poster frame plus `mediainfo` | anywhere a still picture shows | missing |

Rung 2 is the biggest win per unit of work: the kitty path already carries
still pictures in f4, and it is the rung that covers ssh, Wayland and macOS at
once.

Rung 3 pays for itself twice. Today `image_view.go` writes *"This backend
cannot display images."* into the middle of the frame when
`ScreenBuf.SupportsGraphics()` is false. The same half-block renderer that
makes video degrade gracefully also makes `F3` on a PNG over ssh in a plain
xterm show the picture. One renderer, two features, and one apology message
deleted.

Sextant and octant cells from the Unicode 13 Legacy Computing block give more
resolution again where the font has them; that is a configuration knob, not a
second renderer.

## 5. mpv is a rung, not a foundation

This is the structural consequence and the reason the current code cannot
simply be extended.

**mpv is a black box that draws itself.** It never hands f4 a frame. So rungs
2, 3 and 4 cannot be built on top of what exists — they have nothing to draw.

The shape the work has to take:

- **A frame source**, yielding `(frame, pts)`. First an ffmpeg pipe decoding to
  rawvideo; later a pure-Go demuxer over `plugins/mediainfo` feeding MJPEG and
  VP8; possibly hardware decode much later (section 6).
- **Four renderers** consuming that source: overlay, terminal graphics, ANSI,
  poster.
- **mpv reduced to the rung 1 fast path** and nothing else. It stays because on
  X11 it is better than anything f4 will write: hardware decode, A/V sync,
  subtitles, watch-later state.

With that split, "no dependencies" stops being a yes-or-no question. MJPEG and
VP8 play with no external binary at all on every cell of the matrix; everything
else degrades to a poster frame with an honest caption; and with ffmpeg on
`PATH`, everything plays.

## 6. Decoding: the reach of pure Go

| Codec | Pure Go |
| --- | --- |
| MJPEG | trivial — `image/jpeg` per frame |
| VP8 | `golang.org/x/image/vp8`, the lossy WebP decoder |
| GIF, APNG, animated WebP | mostly present already |
| **H.264, HEVC, VP9, AV1** | **no, and nothing on the horizon** |

Writing an H.264 decoder means CABAC, deblocking and B-frames, and without SIMD
it would not reach 1080p in Go even once it were correct. Those four rows are
also exactly what is on people's disks. A pure-Go-only policy therefore
converges on *playing the formats nobody has*, which is why the plan is a
ladder and not a purity rule.

## 7. The GPU question, and what is already in the tree

Asked directly: can modern codecs be computed on the GPU, given that f4 already
depends on gogpu?

**Not as compute shaders.** Entropy decoding — CABAC in H.264/HEVC, the
arithmetic coders in VP9 and AV1 — is strictly serial and data-dependent, the
canonical worst case for a GPU. Every practical "GPU decoder" parses the
bitstream on the CPU. Reconstruction maps well (inverse transform, motion
compensation, deblocking, YUV to RGB), and hybrids of exactly that shape have
existed before — DXVA once offered motion-compensation-only and IDCT-only
acceleration levels — but the bottleneck stays on the CPU and the work is of
research scale.

**Vulkan Video, however, is already in the dependency tree, unexposed.** The
WebGPU surface has no decode: neither `gogpu/wgpu` nor `gogpu/gputypes` mentions
video at all. But the Vulkan HAL underneath is generated wholesale from
`vk.xml`, and it contains the fixed-function decode block:

```
VK_KHR_video_queue, VK_KHR_video_decode_queue
VK_KHR_video_decode_h264, _h265, _av1, _vp9
CmdBeginVideoCodingKHR, CmdDecodeVideoKHR, VideoDecodeInfoKHR, ...
```

`github.com/gogpu/wgpu/hal/vulkan/vk` is an ordinary importable Go package —
566 exported functions in `commands_gen.go` alone — reached through goffi
without cgo, and excluded only on `js/wasm`.

The honest price, before anyone gets excited:

- It is driver-frontend work. The bitstream is still parsed on the CPU — SPS,
  PPS, slice headers — the DPB is managed by us, and the `StdVideoDecodeH264*`
  structures are filled by us. ffmpeg's Vulkan decoder took years.
- Reach: Linux (NVIDIA, RADV, ANV) and Windows, yes. macOS, no — MoltenVK does
  not implement Vulkan Video. The BSDs only where Mesa is current. Every
  `noffi` cell is out by construction. AV1 and VP9 driver support is much
  patchier than H.264 and H.265.
- The output is a `VkImage` in NV12, which then has to reach the overlay or be
  read back and scaled for a lower rung — the presentation problem again.

So: architecturally the right long-term answer to "modern codecs without an
external binary", and **more** expensive than the ffmpeg pipe rather than less.
Worth recording, not worth starting.

### The wasm option, for completeness

Running ffmpeg compiled to wasm under wazero is tempting because the pattern is
already established here — `wazero v1.12.0` is a direct dependency and
`ncruces/go-sqlite3-wasm` is in `go.mod` — and it needs neither cgo nor `PATH`.
It fails on the matrix instead: wazero's optimizing compiler has backends under
`backend/isa/<arch>` for amd64 and arm64, and everything else runs the
interpreter, which its own documentation describes as what makes targets such
as riscv64 possible. That is fine for SQLite and meaningless for a video
decoder. Add tens of megabytes to a binary that already links at ~120 MB, and
an unverified SIMD story, and it is not the way in.

## 8. Audio, and why the ladder mostly deletes the problem

Without cgo, audio output is a per-platform argument of its own: the PulseAudio
or PipeWire socket protocol on Linux is reachable in pure Go, WASAPI through
COM on Windows needs no cgo, CoreAudio on macOS needs purego, and illumos,
solaris and dragonfly have nothing. A/V sync on PTS is not a detail either.

The ladder makes most of that go away. Rungs 2 to 4 are watched over ssh, in a
tty, or in a terminal window — **silent by nature, and nobody expects
otherwise**. Audio is only wanted on rung 1, which is precisely where mpv
already provides it. Unless and until f4 decodes locally by itself, there is no
audio problem to solve.

## 9. The focus rule

Playback carries on while the terminal is not on top, the way a player behaves.
The picture goes with the terminal, because an override-redirect window that
stayed up would be over somebody else's application, but the film keeps running
and the sound keeps coming.

`[Video] PauseOnFocusLoss=1` for whoever wants the other behaviour.

## 10. Missing tools

A file manager that answers "cannot play this" is answering the wrong question.
`external_tools.go` finds a tool under any of the names it goes by — ffmpeg is
also `avconv`, the Libav fork, which some distributions still ship — and when
it is missing it says what is missing, what it was for, and the command that
installs it, chosen from the package manager that is actually on the machine
rather than guessed from the operating system.

With the ladder in place this message becomes rarer and more accurate: it is no
longer "no video for you" but "this codec needs ffmpeg; here is the poster
frame meanwhile".

## 11. Far 3 as the reference

The issue names far2l. The request behind it, from the linked discussion, is
narrower and more useful: *plugins for viewing images and video through F3*,
which on Far 3 means **Review** by Max Rusov. Its key map is the reference for
f4's own.

Taken from Review's documentation only — `Review.hlf`, `Review-Macros.lua` and
`Readme.txt`. The Pascal sources were not used. Same discipline as the far2l
rule in `IMAGES_PLAN.md`: behaviour is a legitimate reference, code is not.

**Media control**

| Key | Action |
| --- | --- |
| `Left` / `Right` | seek ∓10 s |
| `Shift+Left` / `Shift+Right` | seek ∓1 s |
| `Ctrl+Left` / `Ctrl+Right` | to start / to end |
| `Up` / `Down` | volume ±10 |
| `Shift+Up` / `Shift+Down` | volume, smaller step |
| `Ctrl+Up` / `Ctrl+Down` | volume maximum / minimum |
| `A`, `Ctrl+A` / `Ctrl+Shift+A` | next / previous audio stream |

**Shared with the picture viewer**

| Key | Action |
| --- | --- |
| `PgDn` / `PgUp` | next / previous file on the panel |
| `Home` / `End` | first / last file |
| `Ins` / `Del` | pick / unpick in the Far panel |
| `Ctrl+F` | full screen |
| `Ctrl+I` | file information |

Review reports a seek as `Seek: HH:MM:SS / HH:MM:SS` and a volume change as
`Volume: N`. In full screen its OSD hides itself after three seconds without
mouse movement.

Two things worth noting rather than copying:

- **Review documents no pause key for media.** f4's `Space` is a local decision
  and stays one.
- **`F4` to `F8` are passed through to Far** from the thumbnail view, so file
  operations keep working while a picture is up. `ImageView` already does the
  `Ins`/`Del` half of this; the rest is unexamined.

Where f4 differs today: volume steps by 5 rather than 10, no modifier variants
on the arrows at all, no position or duration anywhere, and no file stepping,
full screen, picking or audio streams in `VideoView` — all of which `ImageView`
already has in some form (`Step`, `GoTo`, `SetFullScreen`, `SetSelected`,
`imageSiblingPaths`), so the work is mostly making the two frames agree.

## 12. What is left, in order

**V1. The frame source.** `(frame, pts)` over an ffmpeg pipe decoding to
rawvideo, with the size asked for by the consumer rather than the file's own.
This is the piece everything below waits on. Tests: the command line shape and
the frame timing, without ffmpeg present.

**V2. The ANSI renderer, and the still pictures it fixes.** Half-block cells
with truecolor. Wire it into `ImageView` first, where it replaces the "cannot
display images" message and can be looked at without any video at all, and only
then into the video path.

**V3. Rung 2, terminal graphics.** The frame source into the existing kitty
path at frame size and a capped frame rate. Measure before choosing the cap.

**V4. Rung 4, the poster frame.** First frame plus the `plugins/mediainfo`
card, as the answer when no decoder is available at all.

**V5. The Review key map.** Section 11, on top of a `VideoView` that by then
has somewhere to show a position bar. Position and duration come from mpv down
the socket it is already given — but note that `videoPlayer.Command` writes and
never reads, so this needs a held connection with `observe_property` on
`time-pos`, `duration`, `pause` and `volume`, not the per-command dial it does
now.

**V6. Sibling walking, full screen, picking.** Mirror `ImageView`. Generalise
`FileSystemPanel.ImageSiblings` over a predicate rather than copying it.

Not scheduled, recorded so the reasoning is not repeated: pure-Go H.264
(section 6), Vulkan Video (section 7), wasm ffmpeg (section 7), and a Windows
console overlay for rung 1.

## 13. Decisions worth not undoing

- **The gate is the presentation layer, not the tool.** Any future "it does not
  work on X" starts by asking which rung was reached, not whether mpv is
  installed.
- **One frame source, several consumers**, exactly as `ImagePipe` is one
  pipeline with several consumers. A second decoding path bolted to a renderer
  is how this feature becomes unmaintainable.
- **mpv is not the foundation.** It is the top rung, and it is kept because it
  is better than what f4 would write for that rung, not because anything
  depends on it.
- **The picture is sized to the transport.** Never full resolution because the
  file is.
- **Behaviour from Far 3 is a reference; Far and far2l code is not.**
