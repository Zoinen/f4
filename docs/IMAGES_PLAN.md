# Image and video support in f4 — plan and handover

This file is the entry point for the work on issue #186 (viewing pictures in
f4). It is written so that the work can be continued with nothing but the
repository at hand. Read it first, then, as needed:

- `TERMINAL.md` — the built-in terminal. Section 8 covers the graphics it
  accepts from child processes, the queries it answers, and the list of known
  deviations and defects worth reading before filing one.
- `TTYX.md` — the X connection behind the terminal: how the terminal window is
  identified, and the overlay window that shows a picture where no image
  protocol exists at all.
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

**8a. Sixel in the built-in terminal.** Issue #610. `sixel_decode.go` turns
the body of a `DCS ... q` sequence into a surface and `sixel_terminal.go` puts
it on the grid through the placement list the kitty receiver already uses, so
scrolling, resizing, clipping and erasing come for free. The parser grew the
two states a device control string needs, which it never had: `ESC P` used to
be swallowed and the body of the sequence spilled onto the screen as text.
Alongside it: `DECSDM` (private mode 80), `XTSMGRAPHICS`, and a primary device
attributes answer that contains the 4 without which no client will even try to
send a picture.

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

**B2. Windows console: black rectangle and a frozen interface.** Issue #805,
**open**. Two faults have been found and fixed and neither of them was the
whole of it: the reporter came back on the build that carries both and said
nothing had changed. The console overlay had never been run on Windows, and
the first report found the threading. `Place`, `Hide` and `SetBounds` made their window calls on
the calling thread; on a window owned by another thread those calls wait for
that thread, and the overlay's pump thread shares an input queue with conhost.
`RenderExternal` runs with the screen locked, so f4 waited on conhost while
conhost was what f4 needed to draw and to read keys. The window operations are
now written into `wincon/overlay_state.go` and applied on the pump thread after
one `PostThreadMessageW`; see `WINCON.md` section 2 for the invariant and the rule
that keeps it.

The black rectangle was the second half and did not go with the first. A frame
places the window, reshapes it, and only then hands over the pixels, so the
pump thread could show a window with nothing in it — and `WM_ERASEBKGND` is
refused, so an unpainted window keeps whatever it last held, which the first
time is black. It stayed that way for as long as scaling the photograph took.
`overlayState.take` now holds the move back until the frame buffer has been
replaced, so the window is shown, or resized, in the same wake-up that paints
it; a resize needed the same rule because `paint` blits at the frame buffer's
size and leaves the rest of a larger window alone.

What the thread says now, and what has to be told apart before anything else
is written, is that the three reports do not describe one fault:

- Windows 10, conhost: black and wedged, unchanged by either fix.
- Windows 11, console: black, but `Esc` comes straight back, and quick view is
  black too. f4-gui shows the same files, so nothing is wrong with the decode.
  The default terminal there is Windows Terminal, where the overlay is
  deliberately never installed — so this is very likely the sixel path and not
  this entry at all.
- Some pictures show, and after a few of them the machine runs hot and `Esc`
  is not answered until the process is killed.

The instrumentation for that is in place: `WINCON:` summary lines, one a
second, described in `WINCON.md` section 4. The three readings that separate
these are `scale` (time on the thread holding the screen lock), `frames`
against `new` (a rescale storm), and `blank` (paints that found no frame
buffer). Nothing further should be guessed until one of those logs arrives.

A later paired `f4probe 5` run on Windows 11 21H2 / build 22000 closed the
host-identification measurement, but not the application case above. Classic
conhost returned a visible `ConsoleWindowClass` with a 960x480 client and DA1
without sixel. Windows Terminal returned a *visible* 0x0
`PseudoConsoleWindow` owned by its hosting window and DA1 with sixel parameter
4. This confirms that visibility alone is the wrong overlay gate and DA1 is
the right capability fallback. The WT run had `WT_SESSION`, so direct-launch
handoff without that variable still needs the `VTUI_DEBUG=1` f4 log described
above; inbox conhost on 24H2/25H2 also remains unmeasured.

**6b. Kitty polish, what is left.** Unicode placeholders (`U=1` and the
character `U+10EEEE`), and a negative `z`, which needs a change in vtui first:
see the entry in section 8.

**7. iTerm2 and sixel output in vtui.** Add `GraphicsITerm2` (OSC 1337, base64
PNG) and `GraphicsSixel` (DCS, up to 256 palette colours, dithering) to
`graphics.go`; detect them from `TERM_PROGRAM=iTerm.app` and from a `CSI c`
answer containing 4. Test the shape of the sequences, not the pixels.

**8. Accepting iTerm2 in the built-in terminal.** OSC 1337 in `handleOSC`,
feeding the same placement layer. The sixel half of this step is done; see
entry 8a in section 4.

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

**11. Done.** far2l's own image channel is answered in `far2l_image.go`.
This turned out not to be an optional extra: far2l's TTY backend asks over the
far2l channel *instead of* probing kitty the moment the far2l extension is
answered, so f4's kitty receiver was never reached and far2l's image viewer
said the backend had no graphics. Caps, set and delete are implemented;
transformation is refused and therefore never asked for, because the
capabilities do not claim it.

**12b. A window over the Windows console.** Done for conhost, where `cmd.exe`
lives and no image protocol exists; see `WINCON.md`. Windows Terminal is
deliberately not covered, because it renders sixel and that is the better
answer. It has now been run on Windows once, by the reporter of issue #805,
and both faults that run found are fixed; nothing since has come back from a
real console.

**12a. Done.** The overlay is installed as vtui's external graphics renderer,
so quick view, the thumbnail grid, the file viewer and the pictures a program
prints into the built-in terminal all reach it without any of them being routed
anywhere. That last one is issue #273: a program that prints sixel or kitty
into f4's own terminal was decoded correctly and then had nowhere to be drawn.

**12. Video.** Started, as mpv drawing into f4's own overlay window; see
`VIDEO.md`. That covers X11 and only X11, and mpv never hands f4 a frame, so
what is left is not merely a second decoder: a frame source everything can
consume, and a ladder of renderers below the overlay — terminal graphics, ANSI
half-blocks, a poster frame — so that `F3` on a video has a useful answer on
every cell of the build matrix. Written up there rather than here, including
why the GPU and wasm routes were looked at and set aside.

**13. Text over a sixel picture.** On the hardware and in Windows Terminal the
screen is one bitmap, so writing a character clears the cell it lands in and
punches a hole in whatever image was there. Here a placement is drawn over the
cells, so the text goes under the picture instead. It shows immediately: the
cursor rule leaves a prompt on the last row of an image, and that prompt is
then invisible. Either the graphics layer learns a z index that puts a picture
under the glyphs — see the entry in section 8, which is the same blocker as
kitty's negative `z` — or the receiver has to keep per row slices and erase
them as cells are written, which is what `ImageSlice` does in Windows Terminal.

**14. Full colour over sixel, both halves.** Done. The sender is vtui's, and
it has two forms: a register redefined between bands, which every decoder that
resolves registers as it reads them honours, and a stack of transparent
single-palette images at one cell, for Windows Terminal, which does not.

f4's receiver takes both. The redefinition was always immediate — see
`TestSixelRegisterRedefinitionIsImmediate` — and the stack needed the
placements to compose rather than replace. Two things carry it: the decoder
already leaves an unpainted pixel at zero alpha under `P2=1`, and overlapping
placements are kept in arrival order by `kittyAddPlacement`, which only
replaces on a matching kitty image and placement id and a sixel has neither.
What had to change was the composing: both overlay frame buffers copied the
source bytes, so a layer erased the one under it and only the last one
survived. They compose now, source over destination.

This matters more than it looks: f4 running inside f4 inside Windows Terminal
is a stack of layers arriving at f4's own built-in terminal, so the two halves
of the protocol are asymmetric until the receiver takes what the sender emits.
Note that the encoder is the vtui repository, not f4.

Konsole's sixel decoder is not compatible with that full-colour form: it keeps
indexed pixels and mutates the shared palette when a later band redefines a
register, recolouring bands that were already decoded. f4 therefore selects
vtui's existing kitty transport for Konsole 22.04 and later, where the terminal
supports the raw RGB/RGBA subset used by the image viewer. The same transport
is preferred when the environment identifies another Kitty-capable terminal
(kitty, Ghostty, WezTerm, Contour, wayst, Rio, or Warp). Terminals without a
known Kitty path keep the full-colour sixel path.

Windows Terminal has the same boundary and no kitty path to fall back to, and
**that case is no longer f4's**. f4 used to set `VTUI_SIXEL_PALETTE=adaptive`
there — a single median-cut palette, which is banding on any photograph, and
chosen by writing an environment variable into its own process to configure a
library it links. vtui now sends several transparent DCS layers at one cell
instead, each with a palette of its own, and picks that itself from
`WT_SESSION` beside the check that already picks the raster cell geometry for
the same terminal. See `../vtui/GRAPHICS.md`. The division that came out of
it is worth keeping: **vtui knows terminals, f4 knows the user.**

`graphics_compat.go` is still on the wrong side of that line — the table of
Konsole, Ghostty, Contour, wayst, Rio and Warp is knowledge about terminals
living in an application — and moving it into vtui's protocol detection, with
only the override left here, is queued rather than done.

**15. Nothing.** The device attributes answer used to be swallowed when a
chunk ended on its final byte, which kept every client from ever asking for a
picture. Fixed; see the trap in section 7.

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
- **The sixel cursor rule is Windows Terminal's, not xterm's.** After a
  picture the text cursor is left at the sixel active position: the column the
  picture started in, and the row the sixel cursor reached. A dump that ends in
  a graphics new line therefore leaves the next line of text below the picture
  and one that ends in data leaves it on top of the picture. That second case
  looks like a defect and is reported as one about once a year, but it is the
  only rule under which a program can print an image on the bottom row at all:
  any rule that moves the cursor past the picture scrolls a line away to make
  room for a cursor the client never asked to move. xterm and mlterm both use
  their own algorithms, and they do not agree with each other either. A client
  that wants the text below the image sends a line feed.
- **256 colour registers are reported, and that is what full colour needs.**
  A decoder that resolves a register at the moment it is used lets an encoder
  redefine one between bands and paint an unlimited number of colours through
  256 registers. That is the full-colour form used by f4's own receiver and by
  terminals that preserve those changes as they arrive. Windows Terminal's
  indexed raster buffer does not make that promise, and the answer there is
  now layering rather than a smaller palette: several transparent images at
  one cell, 255 colours each. Reporting a larger number through
  `XTSMGRAPHICS` would be true of our decoder — the palette grows on demand —
  but it would invite an encoder to quantise to a palette of that size for no
  gain.
- **The receiver uses the real cell size, not the VT340's 10x20.** Windows
  Terminal rasterises sixel into a fixed virtual cell to emulate the hardware.
  We cannot: `CSI 16 t` already tells the child what our cell really is, and a
  client that asks and then finds its picture drawn to a different scale has
  been lied to. It also has to match what vtui's encoder does on Linux, or f4
  inside f4 would draw itself at the wrong size.
- **Raster attributes truncate the picture.** A client that declares `Ph` and
  `Pv` gets exactly that raster, and data past it is dropped. It bounds the
  allocation before the data is read, and it stops a last band padded with
  empty sixels from making the image taller than the client advertised. Data
  with no raster attributes still grows freely, which is what level 1 sixel is.
- **A sixel placement carries no image id.** Sixel has no server side store,
  so there is nothing to address. The placements are flagged instead, and the
  id and number forms of the kitty delete command skip them: every one of them
  would otherwise answer to the zero that `a=d,d=I` defaults to.
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

**Never hold a byte of child output back on a guess.** This one cost a day of
not understanding why no client would send a picture.
`AnsiParser.exciseWindowsSync` hides the background `cd` command that keeps the
panel and the shell in step, and it used to survive a chunk boundary by
withholding any tail of the incoming data that could be the start of `cd /d "`.
A lone `c` qualifies. So a child that had just sent `CSI c` and was waiting for
its device attributes never got them: the byte would have been released by the
next chunk, and a child waiting for an answer sends no next chunk. It timed
out and concluded the terminal has no sixel. Every sixel client opens with that
query, so the whole feature was dead while looking implemented.

The fix is to print those bytes and only remember them, in `seenWindowsSync`,
so the next chunk can still match across the split; the erase-line the excision
already emits then wipes the fragment off the screen together with the rest of
the command. `TestWindowsSyncExcisedAtEveryChunkBoundary` walks every split
point of the command and `TestAnsiQueryAtChunkEndIsAnswered` walks the queries.
The rule to carry forward: the parser may defer a byte once it has seen the
whole seven byte marker, never on a partial one, and the same goes for any
heuristic added later.

**The primary device attributes answer changed, and it is not only about
sixel.** It was `CSI ? 1 ; 2 c`, a VT100 with an advanced video option; it is
now `CSI ? 62 ; 4 c`, a VT220 with sixel. The `4` had to be added because
nothing sends a picture without it, and the level had to rise with it because
sixel does not exist below VT220. But the level is what programs use to decide
which sequences they may send at all, so the blast radius is wider than the
graphics path, and anything that starts behaving differently in the built-in
terminal should be checked against this before anything else. The string in
`console_passthrough_test.go` is a payload for a muted-PTY test and not an
assertion about the answer, so nothing in the suite guards the old value.

**A fresh terminal has its cursor on the bottom row.** Rule 2 in
`TERMINAL.md`: a terminal inside a file manager initialises at `(0, height-1)`
so that output sticks to the bottom. A test that prints something and then
looks at row zero finds spaces, and a test that prints a picture without
setting the cursor first places it on the last row and scrolls the screen. Both
mistakes were made while writing `sixel_terminal_test.go`.

**A repeat introducer paints a run that a raster attribute may cut short.**
`!30~` inside a raster declared ten wide advances the active position by
thirty and paints ten. The position is what the end of the sequence reports,
so the two have to be tracked separately; conflating them makes the cursor
land in the wrong column on any picture whose data runs past its declared
width.

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
Sixel, added with issue #610:

- **The cursor rule is Windows Terminal's, and it was asked for by name.** It
  leaves the text cursor on top of the picture when the dump does not end in a
  graphics new line. The alternative rules — xterm's, mlterm's, the one the
  VT330 manual describes — all move the cursor past the picture and so make it
  impossible to print an image on the bottom row without scrolling a line away.
  The choice interacts badly with the placement model, though: see item 13 in
  section 5, because in Windows Terminal the text that lands on the image
  punches through it and here it does not.
- **256 colour registers is a report, not a limit.** The decoder's palette
  grows on demand up to 65535 registers and the number reported through
  `XTSMGRAPHICS` is a separate constant, `sixelColorRegisters`. It is set to
  what Windows Terminal reports on the reasoning in section 6; if some encoder
  turns out to need a larger number to take its full colour path, changing the
  constant is the whole of the change.
- **The HLS conversion is derived, not verified against hardware.** DEC's wheel
  is the familiar one turned by 240 degrees — hue 0 blue, 120 red, 240 green —
  and the code follows from that. The tests check the primaries. Nobody has
  compared a gradient against a VT340, and almost nothing emits HLS anyway:
  libsixel and vtui both write RGB.
- **The pixel aspect ratio from P1 is honoured.** Data with no raster
  attributes and no `P1` is drawn at two pixel rows per sixel row, which is
  what the table says and what Windows Terminal does — the arithmetic in the
  examples on microsoft/terminal#18134 only works out if it does. Every modern
  encoder overrides it with `"1;1`, so the path is rarely walked, and a file
  that expects square pixels without saying so will look twice as tall. That
  is also true in xterm.
- **A register that was never defined falls back to the default palette
  modulo sixteen.** The protocol leaves it undefined. Repeating the VT340 map
  is what xterm does and at least keeps such an image visible instead of black
  on black, but it is a guess.
- **An image taller or wider than the screen is cropped, not scrolled
  through.** The hardware scrolls as the sixel cursor advances and you end up
  looking at the bottom of the picture; here the top is kept and the rest is
  dropped. Doing it the other way would mean scrolling a row of history away
  per band, which for a picture near the pixel budget is a lot of rows for
  nothing.
- **Erasing a line does not erase the picture over it.** `EraseDisplay(2)`
  drops the placements of the screen, because that path already existed for
  kitty, but `EraseLine` and the rectangular erases do not touch them. On the
  hardware every one of those clears pixels too. It is the same missing
  machinery as item 13.
- **The decoder walks the body twice and holds the whole body in memory.**
  `AnsiParser` buffers up to 64 MB of device control string before the decoder
  sees any of it, where a streaming decoder would need neither the buffer nor
  the second pass. It has not been measured; the pass itself is cheap next to
  painting, and the buffer is bounded.
