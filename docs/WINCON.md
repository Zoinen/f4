# Pictures over a Windows console

`internal/wincon` puts a picture in a window over the console, for a console
that has no way of showing one itself.

## 1. conhost only, and on purpose

The overlay draws on a classic console window and nothing else, and it decides
that from two facts it asks for on the spot, not from how f4 was started.

**The window says what it is.** `GetConsoleWindow` is asked for its class name
(`wincon.ClassifyConsoleWindow`). `ConsoleWindowClass`, visible, is conhost:
draw. `PseudoConsoleWindow` is the 0x0 helper window on this side of a
pseudoconsole -- Windows Terminal, VS Code, anything on the far end of a pty --
and it is reported *visible* whether or not its tab is on screen, because
OpenConsole minimizes it rather than hiding it. Visibility alone therefore
said "draw here" and every frame went into a window with no client area,
which is the picture-never-appears report of #805 (handover F2, F3). The class
decides; an unfamiliar class is not trusted either.

**The terminal says what it can draw.** Before the overlay is considered at
all, f4 asks DA1 when no graphics protocol has been chosen and nothing forced
one (`probeGraphicsIfUnknown`). Windows Terminal answers with parameter `4`
on every launch path, including the default-terminal handoff where
`WT_SESSION` is absent, and takes the sixel path; conhost on 10.0.22000
answers `ESC[?1;0c` and falls through to the overlay (F13, F14). So the one
case the environment could not describe -- Terminal as the default
application, f4 started from a shortcut -- is settled by asking.

## 2. Not a child, not owned: a top-level layered window

The overlay is a `WS_POPUP` window with no parent and no owner, styled
`WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE`. It
used to be a child of the console window, and everything that was wrong with
it followed from that one relationship.

**Why not a child.** The console window belongs to conhost.exe. A parent/child
relationship across processes attaches the two threads' input queues, and so
does owner/owned, transitively (Raymond Chen, "Is it legal to have a
cross-process parent/child or owner/owned window relationship?", 2013-04-12;
and 2011-03-31). Measured on 10.0.22000 with `tools/f4imgprobe`: for as long
as a child window of the console existed, the console accepted no keys and no
mouse; destroy it and input returned (handover F22). That, not repainting, is
the "flickers and freezes" of #805 on that build -- the child's picture was
not even damaged by a console repaint there (F6 did not reproduce). A window
with no parent and no owner has nothing to couple, and the same field run
showed it visible, directly above the console, surviving a repaint whole and
freezing nothing (F23).

**What a child got for free, and how this window does it instead.**

- *Following the console.* A 33 ms timer on the pump thread reads the
  console's client origin and rectangle, whether it is alive, visible or
  minimized, and what sits directly above it in the z-order, and decides what
  to do (`trackStep` in `layered.go`, tested without a window): hide when the
  console is minimized or hidden, move when it moves, close when it dies, and
  restack to sit directly above the console -- or at the top of the ordinary
  band if the window above is topmost. The field run confirmed the tracker is
  a requirement, not a nicety: without one the window stays where it was when
  the console moves (Q9).
- *Painting.* The frame is pushed with `UpdateLayeredWindow`: position, size
  and premultiplied BGRA pixels in one call. There is no `WM_PAINT`, nothing to
  erase, no black rectangle before the first paint, and no region -- the gaps
  between thumbnails are alpha zero in the frame. `SetBounds` is kept for the
  caller and does nothing.
- *The mouse.* `WS_EX_TRANSPARENT` passes clicks through to the console, which
  is what keeps text selection working under a picture.

**Two invariants.** No caller waits on the pump thread: public methods write
into `overlay_state.go` and post one thread message. And the console window is
only ever *read* -- `GetClientRect`, `ClientToScreen`, `IsWindow`,
`IsWindowVisible`, `IsIconic`, `GetWindow`, `GetWindowLongPtrW` -- never
`SendMessage`, `SetWindowPos` or `ShowWindow` with its handle, because those
wait on conhost, and that wait is exactly where the child window used to hang.

## 3. The geometry is the easy part here

Unlike a terminal emulator, a console answers directly:

- `GetCurrentConsoleFontEx` gives the pixel size of a character cell. Nothing
  to infer, nothing to round, no escape sequence for somebody else to swallow.
- `GetClientRect` gives the text area, and the text starts at its corner —
  there is no menu bar inside a console's client area and no logical-pixel
  scaling to undo.

So the whole of `docs/TTYX.md` section 3, which is about working out where the
grid is, has no counterpart here. That part of the X side is guesswork forced
by terminals that do not answer; Windows answers.

## 4. What is tested and what is not

`geometry.go` is arithmetic and policy, has no system calls in it, and is
tested on every platform: cell rectangles, clipping, the union that decides
which window one frame goes into, and the composing into a device independent
bitmap — which is bottom-up with its channels the other way round, and both
mistakes look like a picture rather than an error. Composing and not copying:
a picture can arrive as overlapping pieces, which is what a stack of
transparent sixel layers from a program in the built-in terminal is, and a
copy leaves the top layer alone on the screen.

`overlay_state.go` is the same kind of file as `geometry.go`: what was asked
for, what is on the screen, and what the pump thread therefore has to do, with
no system calls in it. It is tested everywhere — coalescing, a change arriving
while the pump thread is busy, placing twice in the same spot, hiding, showing
again, clearing the region, refusing to record anything once closed, and the
move that waits for its pixels while the region and the repaint do not.

`overlay_windows.go` is the part that calls `user32` and `gdi32`. It compiles
for `windows/amd64` and `windows/arm64`, the shape of it is the same as the X
overlay, and the only report from a real console so far is issue #805.

When a picture does not appear, `VTUI_DEBUG=1` gives one `WINCON:` line per
frame that changed: the size and corner of the window and how many pictures
went into it. No lines means the frame never reached the overlay; lines with a
black rectangle on the screen means the request reached the pump thread and
something below it went wrong. `[Images] Overlay=0` turns the overlay off
altogether — the same setting serves both platforms, and `X11Overlay` is the
name it had when the X side was the only one — which is how to tell an overlay
fault from anything else.

Beside them is a summary line, at most one a second and only for a second in
which something happened:

    WINCON: 1.0s frames=57 new=2 scale=812ms/406ms window=3ms \
            pump=6 move=2 rgn=2 inval=4 paint=4 blank=1 gaveup=0

`frames` is how many times a frame reached the overlay and `new` how many of
them were not the frame before, so `frames` high with `new` low is a console
redrawing under a still picture and costs nothing, while the two rising
together is a picture being rescaled sixty times a second. `scale` is the
total and then the worst single scale of the period, on the thread that holds
the screen lock: a camera JPEG is tens of megapixels and the resampler is
plain Go, so a frozen f4 with a large number here is frozen *there* and
nowhere near a window call. `paint` and `blank` come from the pump thread —
`blank` counts the paints that found no frame buffer, which is what a black
rectangle looks like from inside. `gaveup` names the reason the last frame
was abandoned.

The pump thread counts rather than logs, deliberately: its input queue is
attached to conhost's, so a write to a file on that thread is one more way of
stopping the console it is drawing over. See `internal/wincon/stats.go`.
