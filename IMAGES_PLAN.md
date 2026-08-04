# Image and video support in f4 — plan and handover

This file is the entry point for the work on issue #186 (viewing pictures in
f4). It is written so that the work can be continued with nothing but the
repository at hand. Read it first, then, as needed:

- `TERMINAL.md` — the built-in terminal, including the kitty graphics protocol
  it accepts from child processes.
- `../vtui/GRAPHICS.md` — the graphics layer: `ImageSurface`, `ImagePlacement`,
  the protocol backends, cell size negotiation.
- `PLUGIN_PLAN.md` — unrelated work, but the same kind of document, and a good
  example of the shape this one is meant to keep.

## 1. How the work is done

The user is unxed, the author of f4. The conversation is in Russian; **all
code, comments and documentation are in English**. Testing happens on Linux
Mint 22.3 Cinnamon (X11) with `./run_all_tests.sh`; CI builds every OS,
including 32-bit ARM.

Rules, which have not changed since the plugin work:

- Every reply with code carries **tests** and an **English commit message**.
- Split a large task across several replies and start with a plan.
- Nothing from far2l copied verbatim: it is GPL2 and f4 is not.

The user values being told *why* a solution looks the way it does, being shown
the fork in the road and the reason one branch was taken, and being told what
was found along the way. Do not smooth things over.

## 2. Architecture

**One pipeline, several consumers.** `ImagePipe` (`image_pipeline.go`) is the
only thing that turns a path into pixels: it caches decoded surfaces, merges
concurrent requests for the same file, runs a small pool of workers, and
prefetches the neighbours of whatever is on screen. `PreviewSync` answers with
the best picture that can be had without decoding the whole file — a cached
one, a thumbnail seen earlier, or the thumbnail the file carries inside itself.
`LoadSync` decodes properly. Everything that shows a picture — the viewer, the
gallery, and eventually the quick view panel — asks the pipeline and never
touches a decoder directly.

**vtui ships rectangles of pixels, not pictures.** `ImageSurface` is straight
alpha RGBA; `ImagePlacement` says where on the cell grid it goes, which part of
the source to take, and at which z index. Backends (kitty today, iTerm2 and
sixel later, plus the native GUI renderers) know how to put that rectangle on
screen and nothing else. Anything the viewer wants to change about the picture
itself — a turn, a mirror — has to be baked into the pixels first. That is what
`image_transform.go` is for.

**The viewer is one frame with modes.** `ImageView` is a single frame that can
be showing one picture, the thumbnail grid, or a slide show. The grid and the
show are not separate frames because all three share the sibling list, the
pipeline, the graphics key and the set of picked pictures; separate frames
would need callbacks to keep four things in step and would gain nothing.

## 3. File map

In `f4` (package `main`):

- `image_pipeline.go` — the cache, the job queue, the workers, prefetch
- `image_decode.go`, `image_qoi.go`, `image_bmp.go`, `image_preview.go` —
  decoders and the Exif thumbnail path
- `image_external.go` — the converter of last resort: `magick`, `convert` or
  `ffmpeg`, whichever is on the `PATH`
- `image_transform.go` — rotation and mirroring over the RGBA bytes
- `image_view.go` — the viewer frame: layout, zoom, panning, orientation,
  the info overlay, keys
- `image_gallery.go` — the F12 thumbnail grid and the shared selection
- `image_slideshow.go` — the Ctrl+S timer
- `kitty_graphics.go` — accepting the kitty protocol in the built-in terminal
- `actions.go` — `tryOpenImageViewer`, `imageSiblingPaths`, and the wiring of
  the selection between the viewer and the panel
- `file_panel.go` — `ImageSiblings`, `SetSelectedByName`, `IsNameSelected`
- `config.go` — the `[Images]` section

In `vtui`:

- `graphics.go` — `ImageSurface`, `ImagePlacement`, the placement list
- `graphics_kitty.go` — the kitty output backend
- `graphics_scale.go` — `FitInside` and the scalers
- `framemanager.go` — `HideBars`, for a frame that wants the bar rows
- `terminal_env.go` — protocol detection

## 4. Status

Done before the current sequence of work: accepting the kitty protocol in the
built-in terminal (transmission, placement, drawing through
`vtui.GraphicsLayer`, cursor movement per the specification); pixel geometry in
`TIOCSWINSZ` and in the answers to `CSI 14/16 t`; announcing image support to
the child process (`KITTY_WINDOW_ID`, `TERM_PROGRAM`, `TERM=xterm-kitty` under
a terminfo check and the `AnnounceKittyTerm` option); the decoding pipeline;
preview from the Exif thumbnail; QOI and BMP decoders; walking the neighbours
with prefetch; the 100% mode; `Ctrl+R`; errors shown in the title.

Done in the current sequence, listed in the order the work happened rather
than in the order of the numbering below, which the work has already outgrown.
An entry numbered `R` comes from the issue tracker and one numbered `B` is a
defect; the twelve steps below foresaw neither.

**1. Rotation and mirroring.** `image_transform.go` rotates and mirrors a
surface in plain Go. `ImageView` keeps `rotation`, `flipH`, `flipV` and a
`shown` surface; `display()` returns `shown` when the orientation has been
changed and the decoded `surface` when it has not, so an untouched picture is
never copied. Keys: `>` and `.` turn clockwise, `<` and `,` counter-clockwise,
`Alt+>` mirrors across the vertical axis, `Alt+<` across the horizontal one.

**2. Full screen and the info overlay.** `FrameManager.HideBars` in vtui;
`ImageView.ResizeConsole` remembers the console size and gives the key bar row
to the picture in full screen; `F` and `Ctrl+F` switch it, `Close` gives it
back. `Ctrl+I` (and `I`) raises a panel with the name, the size on screen, the
file size, the decoder, the scale and the orientation.

**3. The gallery and the slide show.** `F12` opens a grid of thumbnails;
`Ins` and `Del` pick and unpick, and the choice is shared with the panel in
both directions. `Ctrl+S` runs a slide show with the interval from
`[Images] SlideShowDelay`, five seconds by default.

**4. Quick view on `Ctrl+Q`.** In `quick_view_panel.go`, shows a picture through
the pipeline instead of the hex dump when one is under the cursor. The
placement is computed the way `ImageView.placementFor` does it, but inside the
bounds of the panel.

**5. External decoder.** `image_external.go` registers a decoder at priority
−10 that hands the file to `magick`, `convert` or `ffmpeg` — whichever is on
the `PATH` — and reads PNG back from its standard output. It is registered
only when one of them is there, and it claims only the formats that one can
read, so `IsImageFile` never promises a picture the machine cannot open. The
`[Images]` section gained `ExternalTimeout` and `DecoderPriority`, and
`ImageDecoder` gained an optional context-aware `Decode`.

**6a. Kitty in the built-in terminal: shared memory, resize, alt screen.**
`t=s` resolves the name of a POSIX shared memory object inside `/dev/shm`,
reads it and unlinks it, as the protocol requires. `kittyResizePlacements`
moves the pictures of the main screen by the same shift the reflow gives the
text and drops what left the buffer for good; `kittyRecomputeSpans` works out
again the side of a span the client left to us, both after a resize and after
a cell changes size. Leaving the alternate screen now drops its pictures.

**R1. Picking and walking in the single picture view.** Asked for on the
issue: `Ins` and `Del` pick and unpick the picture on screen and move on to
the next, exactly as they do in the grid, and the title bar turns the colour a
picked tile has and gains an asterisk. The arrow keys walk the directory when
their axis has nothing to pan and pan when it has; `w`, `a`, `s` and `d` pan
whatever happens.

## 5. What is left, in order

Before the numbered work, one defect that is on screen right now:

**B1. A strip of background below the picture, with something flickering in
it.** Under investigation. `ImageView.logGeometry` writes one `IMAGE_GEOM`
line per change of layout under `VTUI_DEBUG=1`, and it has already ruled out
both of the causes that were suspected first. On a console of `153x38` with a
`20x41` cell the frame runs `0,0..152,36` with one row of title bar, and the
placement comes out as `place=45,1 62x36`: the picture covers rows one to
thirty six, which is the whole of its area, so the centring leaves nothing
over. Every line reports `layer=1`, so nothing besides the viewer is drawing
into the graphics layer.

The fault is therefore below the cell grid, in a native renderer, which agrees
with what was observed: it appears on X11 and not in kitty and not on gogpu.
One defect there is proved from the source and fixed in vtui — the X11 frame
buffer covered only the whole cells of the window, leaving `height mod cellH`
pixels, up to forty of them on a cell forty one high, that nothing ever wrote
to and that therefore kept whatever the X server last put in them. That
accounts for a strip *below* the key bar.

Still open: whether the strip that was reported is that one, or lies *between*
the picture and the key bar, which would instead mean that
`vtui.drawNativePlacements` paints a picture smaller than the rectangle the
placement gives it. The new `X11_GFX` line prints the rectangle drawn beside
the rectangle asked for and settles it.

**6b. Kitty polish, what is left.** Unicode placeholders (`U=1` and the
character `U+10EEEE`), and a negative `z`, which needs a change in vtui first:
see the entry in section 8.

**7. iTerm2 and sixel output in vtui.** Add `GraphicsITerm2` (OSC 1337, base64
PNG) and `GraphicsSixel` (DCS, up to 256 palette colours, dithering) to
`graphics.go`; detect them from `TERM_PROGRAM=iTerm.app` and from a `CSI c`
answer containing 4. Test the shape of the sequences, not the pixels.

**8. Accepting iTerm2 and sixel in the built-in terminal.** Symmetrical to the
kitty side: OSC 1337 in `handleOSC`, sixel DCS in the parser, both feeding the
same placement layer.

**9. Fixing the kitty receiver in far2l.** In `far2l/src/vt/vtansi_kitty.cpp`:

- use `GetInt` rather than `GetChar` for `i` and `p` in `a=p` and `a=d`;
- apply `c`, `r`, `X`, `Y` and `z` when placing;
- `d=i` removes the placement but keeps the pixels, `d=I` frees the data;
- `a=d` with no `d`, and `d=a` / `d=A`, remove every visible placement;
- in `AddImage`, `if (rows > 0) { img.cols = cols; }` tests the wrong field —
  it has to test `cols`, and the missing side has to be computed from the
  aspect ratio;
- in `KittyArgs`, replace `if (i + 1 > j && s[j + 1] == '=')` with a test of
  `j + 1 < i`;
- answer `a=q` according to what the backend can actually do, through
  `GetConsoleImageCaps`.

**10. far2l to f4 detection.** far2l sets something like `FAR2L_IMAGES=1` for
its child when `GetConsoleImageCaps` reports RGBA support; take it into account
in `detectGraphicsProtocol` in `vtui/terminal_env.go` so that f4 running inside
far2l turns kitty on.

**11. far2l's own protocol in f4's built-in terminal.** Accept
`FARTTY_INTERACT_IMAGE_*` (see `far2l/WinPort/FarTTY.h`) in `HandleFar2lAPC`,
so that far2l running inside f4 can hand pictures over through its own channel.

**12. Video.** A second source of frames on top of the same placement layer:
decode through an external `ffmpeg` into a stream of RGBA, a frame timer, and
controls from the viewer (`Right`/`Left` for ±10 seconds, `Up`/`Down` for
volume).

Note that steps 9 to 11 exist because **kitty images do not work in either
direction between f4 and far2l today**: not when f4 runs inside far2l, and not
the other way round.

## 6. Decisions worth not undoing

- **The orientation is reset in `open()`, not in `SetImage()`.** `SetImage` is
  also called when the full resolution decode replaces the thumbnail of the
  *same* file. Resetting there would snap a picture back moments after the
  reader turned it.
- **A mirror reverses the direction of a turn.** The state is "rotate by
  `rotation`, then mirror", and for a reflection `R₉₀∘F = F∘R₋₉₀`. So when
  exactly one axis is mirrored, `Rotate` negates the delta; with both axes the
  mirror is a half turn, which commutes, and the sign stands.
- **`HideBars` has to live in the frame manager.** `ScreenObject.Show` forces
  an object visible, so `SetVisible(false)` on the key bar does not survive the
  next frame. The manager hides it — rather than merely skipping the drawing —
  because an invisible bar that still reports itself visible keeps swallowing
  clicks on the bottom row in `dispatchEvent`.
- **The overlay uses a negative z index.** In kitty, a `z` between −1073741824
  and −1 puts the picture under the glyphs but still over the cell background,
  which is what lets the info panel be readable without a box hiding the
  picture. Below −1073741824 the picture would go under the background too.
- **Thumbnails are fetched off the drawing path.** `PreviewSync` is cheap on a
  cached picture but reads the file header on one it has not seen; a screenful
  of tiles would otherwise mean a screenful of reads on every frame.
- **The slide show wraps around, `Step` does not.** Stopping at the ends of the
  directory makes it obvious where the directory ends; a show that stopped
  there would only be a slow way of pressing space.
- **`Stat` for the overlay happens once, lazily, and only when the overlay is
  actually up.** It can be a network round trip on a remote file system.
- **The cell grid is not the window.** A native backend gets a window sized in
  pixels by somebody else, and the whole cells inside it do not reach its
  edges. Everything f4 computes is in cells and stops being the whole story at
  that boundary, which is why an `IMAGE_GEOM` line that looks perfect can sit
  above a defect on screen.
- **The external converter is fed a file, not a pipe.** `heic`, `avif` and
  `jxl` are containers that are read by seeking around them, and both
  ImageMagick's delegates and `ffmpeg` refuse a stream they cannot rewind.
  The bytes are in memory already, so a temporary file costs one write. Its
  name carries an extension guessed from the magic bytes, because ImageMagick
  picks a delegate by the name before it looks inside.
- **The list of formats depends on the converter that was found.** Claiming an
  extension is what makes the panel call a file a picture and the viewer open
  it, so a decoder that claimed `psd` on a machine with only `ffmpeg` would
  turn a hex dump into an error message.
- **`convert` is skipped on Windows.** There, `convert.exe` is the file system
  conversion utility that ships with the system. ImageMagick 7 answers to
  `magick` everywhere, so nothing is lost.
- **Decoder priorities live beside the registry, not in it.** A decoder
  registers with the priority its author chose and the overrides are applied
  when the registry is read, so emptying `DecoderPriority` restores the
  built-in order without a second copy of the built-in numbers.

- **A resize moves pictures but never rescales them.** A placement is a
  rectangle of cells and kitty keeps it that way; `kittyClipPlacement` already
  trims what does not fit at drawing time, so widening the window brings the
  whole picture back. Shrinking `Cols` instead would lose it for good.
- **`WantCols` and `WantRows` are kept beside the computed span.** A side the
  client gave in `c` or `r` is a promise about the layout of the screen and
  stands through everything; only the side we chose for it, and the clamp to
  the size of the screen, are worked out again.
- **A shared memory name is one path component.** `shm_open(3)` allows nothing
  else, and a name with a separator in it would turn `t=s` into a way of
  reading any file on the machine. It is refused before it reaches the file
  system rather than after.

- **An arrow steps only when its axis cannot be panned at all.** Panning to
  the edge of a zoomed picture and then jumping to the next one reads as a
  slip of the finger. A picture that fits has no edge to reach, and that is
  the case the request was about; `w`, `a`, `s` and `d` pan unconditionally,
  so nothing is lost.
- **The title bar carries both a colour and a mark for a picked picture.** The
  colour is the one the grid gives a picked tile, so the two views agree; the
  asterisk survives a terminal whose colours nobody has set up.

## 7. Traps

**A span with one side given follows the shape of the cell, not its size.**
When a client gives `c` and leaves `r` to the terminal, the rows come out of
`srcH * cols * cellW / (srcW * cellH)`, and halving both sides of the cell
leaves that untouched. Only a change of the cell's aspect ratio moves it. A
test that took a cell from ten by twenty to five by ten and expected the
height to change was wrong about the arithmetic, not about the code — the
mistake was carrying an expectation over from the case where both sides are
computed, which does depend on the absolute size.

**`Ctrl+I` is Tab.** On a terminal without an extended keyboard protocol,
`Ctrl+I` arrives as `VK_TAB`, which the viewer uses for the 1:1 fit. Nothing in
f4 can tell them apart — the information is not in the event. Hence the plain
`I` alias. The same class of problem may affect `Alt+>` and `Alt+<`, which are
currently matched by `Char` with the Alt flag set; that has not been confirmed
on a real terminal.

## 8. Open questions and rough edges

None of these are bugs; they are places where a decision was made without
asking, and the user may want a different one.

- `Ctrl+R` goes through `open()` and therefore also resets the orientation.
  Arguably right for "read the file again", but it was not asked about.
- The gallery tile is 18 by 9 cells, chosen by eye for an 80x25 terminal. It
  could reasonably become a setting in `[Images]`.
- In the grid, the cursor colour wins over the picked colour, so a picked tile
  under the cursor only looks like the cursor. far2l marks the selection with a
  separate character; the grid could too.
- The slide show does not wait for a picture to finish decoding, so on a slow
  file system with a short interval frames will be skipped. A check on
  `iv.loading` in `slideStep` would fix it.
- `SlideShowDelay` is read and written but does not appear in the settings
  dialog.
- `vtui/framemanager_hidebars_test.go` calls `fm.renderPhase()` directly, which
  no test did before. If it turns out to be too heavy to call from a test,
  move the check one level down.
- `image_gallery_test.go` passes a nil context to `ImagePipe.LoadSync`, which
  the function handles but which no other test does.
- **A negative `z` cannot be honoured without changing vtui.** The attribute
  word in `vtui/colors.go` carries either an eight bit index or twenty four
  bits of RGB, and has no way of saying "whatever the terminal calls default".
  `Show` fills the viewer with an explicit dark background, so a picture the
  terminal is asked to keep under the glyphs ends up under an opaque fill and
  is never seen. Either the attribute needs a "default colour" state, or the
  graphics layer needs to tell the backend not to paint the cells a negative
  `z` placement covers.
- `ImageView.panMaxX` and `panMaxY` come from the last frame, so the arrow
  keys walk rather than pan until the picture has been drawn once. That is the
  right answer for a picture that fits, which is every picture before somebody
  zooms it, so the case never shows.
- `t=s` works wherever shared memory objects appear in the file system, which
  is Linux and the BSDs. On macOS and Windows `kittyShmPath` reports that the
  system has none, and the client gets `EBADF`. Doing better would need cgo
  and `shm_open`, for a medium almost nothing uses.
- Leaving the alternate screen drops its pictures but does not tell the store
  that they are gone, so the images themselves wait for the `kittyMaxImages`
  eviction. The same was already true of the erase path.
- `ImagePipeline.run` still calls the loader with `context.Background()`, so
  the context now threaded down to the external decoder is never cancelled in
  practice. The timeout works; the cancellation is waiting for the pipeline to
  learn how to drop a job.
- Only the still picture is taken from a converter that could give more: an
  animated `webp` arrives as its first frame. Animation belongs with step 12.
- `ExternalTimeout` and `DecoderPriority` are read and written but do not
  appear in the settings dialog, same as `SlideShowDelay`.