# Doubts and unfinished business in vtui

Decisions taken in a hurry, things noticed in passing, and things left alone
on purpose. One file, so a review can be one sitting.
## The cluster registry never forgets

Every distinct multi rune grapheme cluster that reaches the screen is kept in
a process wide table for the life of the program, because a cell is an integer
and something has to own the string it stands for. Nothing frees entries and
nothing counts references. In a dialog heavy program this is a few dozen
strings; in a viewer paging through Devanagari or Arabic prose it could be
tens of thousands, each a handful of bytes. The ceiling is real but far away,
and when it matters the answer is probably a generation counter reset between
screens rather than reference counting.

## FillCharInfo now copies its input

`FillCharInfo` and `FillCharInfoWithSelection` take a byte slice and hand it to
the cluster iterator as a string, which allocates. They used to decode the
bytes in place, which is why they take bytes at all. Giving the iterator a byte
slice API would remove the copy; it was not worth doing before anyone measures
it.

## Format characters are assumed invisible

Width counts anything in Unicode category Cf as zero columns. That is right for
the joiners and the variation selectors, which is what the text actually
contains, but glibc gives U+00AD SOFT HYPHEN a column and we do not. Nobody has
complained, and the alternative is a table of exceptions.

## The emoji presentation width is a guess with a switch

A character followed by U+FE0F is two columns here, because that is what
modern emulators do, but a wcwidth terminal keeps the base width and the two
answers cannot both be right. `EmojiPresentationWide` exists so an application
can pick; nothing detects the terminal and sets it. Probing for this is
possible - draw and ask for the cursor position - and belongs with whatever
else ends up probing the terminal.
## The Highlighter interface does not say what its attributes index

`Highlight` returns one attribute slice per line and never states whether the
index is a byte, a rune or a cell. Before text was laid out by cluster the
three were close enough to get away with; they are not now, and a consumer
that guesses cells will drift by a few positions at the first emoji and stay
drifted. An application built on vtui reports exactly that symptom with
colorer. Nothing was changed here, because choosing the contract is the fix
and it deserves its own patch: see stage 3 of `UNICODE_PLAN.md`.

## Graphical backends draw the base character and drop the marks

x11, wayland and gogpu call `CellBaseRune` and render one glyph per cell, so a
composed character comes out bare there while the terminal renders it
correctly. Column counts agree either way, so nothing shifts; only the accent
is missing. This was the deliberate price of landing the cluster layer without
touching three font caches in the same patch.

## Cell width now comes from the buffer, not from the character

x11 and wayland used to ask go-runewidth how wide the character in a cell was,
which could disagree with the number of cells the layout engine actually gave
it. Both now read the span back out of the buffer with CellSpanAt, so the
renderer cannot disagree with the layout, and the cursor follows. gogpu still
derives the span inline in its glyph loop, where the same one line call would
do; it was left alone because it is already correct and stage 4 will be in that
code anyway. If a fourth backend ever appears, CellSpanAt is the thing it
should use.

## Nothing tests that a backend actually draws a wide cursor

CellSpanAt is covered, and it is where the logic lives, but the three call
sites are three hand edits inside render loops that need a window to run. A
regression there would be silent until somebody looks at a screen. The honest
fix is a headless backend that renders into an image buffer and lets a test
assert pixels; the x11 and wayland renderers already draw into an
image.RGBA and are two thirds of the way there. Nobody has needed it enough
yet.

## Shaping is nobody's job here

Arabic joining forms and Indic reordering are left to the font stack. vtui
counts columns and hands over text. This is fine for a terminal, where the
emulator does the same, and it is a visible limitation in the graphical
backends, where vtui is the emulator. No plan to address it.

## A gogpu drop is told nothing about the source

`OnDragDrop` hands over the paths and the position and nothing else: not the
actions the source allows, not the modifiers held at the drop, and there is
no channel back to the source to report what happened. So the backend
announces copy, always, and Shift or Ctrl over a gogpu window do nothing.
The modifiers we do pass on are the ones our own key handling last saw,
which during a drag from another application is usually nothing at all. If
gogpu ever grows a drag-over callback, this is the first thing to revisit.

## The drop position is trusted to be in the same pixels as the pointer

gogpu documents the drop position as physical pixels; its pointer callbacks
do not say. The host divides both by the same cell size, so on a HiDPI
window a drop would land in the wrong cell exactly when a click does.
Correcting it is a change to the coordinate handling of the whole host,
not to drag and drop, so it was deliberately left alone.

## Every drop costs two waits for the UI thread

The enter and the drop are delivered separately, and each gives the UI
thread up to `DragDeliverTimeout`. A target that answers at once, which is
the only one we have, never notices. A target that blocks turns half a
second of silence into a drop that did nothing.

## A drag out starts on the next frame, and only if one comes

The request is picked up by `OnUpdate`, which the main loop runs once per
iteration, and the loop is woken by `RequestRedraw`. That wakeup is an event
queued on the platform's own connection everywhere gogpu runs, so it cannot
be lost between the loop's last check and its next wait - which is exactly
the kind of claim that deserves a test rather than a paragraph. The handover
itself is covered now, and so is the promise that one gesture is started
once. The wakeup is not, and cannot be without a real window; the timeout is
what stands behind it meanwhile.

## Nothing suppresses our own mouse events during a drag out

The X11 backend drops pointer events while it is the drag source. Here the
platform grabs the pointer itself, so nothing should reach us at all; what
we do instead is clear the pressed button when the gesture ends, so a
release that never arrived cannot leave the host believing the button is
still down. Whether every platform really swallows that release is untested.

## The two shared drag errors moved out of the X11 file

`ErrDragBusy` and `ErrDragNoData` now live in dragdrop.go. They were never
about X11, and gogpu_dnd.go builds on platforms where x11_xdnd.go does not,
so they had to move for it to compile at all. Nothing else changed with them.
## Drag and drop under gogpu leaned on a gogpu release

Both halves were gogpu's and both were fixed upstream in 0.50.1
(gogpu/gogpu#431): a drop position lost to a discarded XdndPosition, and a
drag out that reached no target because of an event mask the window
manager swallowed, a stale ButtonRelease, and a discarded SelectionRequest.
go.mod now carries the fix, and the workarounds and the instrumentation
that was chasing them are gone.

What remains from that hunt, on purpose: the per-decision logging on both
paths, which is what made the second half of the investigation quick, and
the line naming the display server at startup. Both cost nothing unless
VTUI_DEBUG is set. The refactor that untied the source half of x11_xdnd.go
from X11Host also stays; it was the first step of a plan that is no longer
needed, but it is harmless and would be where to start if this returns.

The thing that was seen and never measured has now been measured, and it
is worse than it looked: a drag from a file manager into the window
arrives 4 times in 10. The cause is in gogpu's connection layer, where a
synchronous round trip reads the socket itself and discards every event
that is not the reply it wants - and the incoming drop path makes one on
every drag, before any position has been recorded. That also explains the
position, which is 0,0 on every drop that does arrive, because the handler
that would record it never runs.

None of it is ours to work around. The position guard in `dropPixels` is
the exception and stays until the fix ships. Work on the fix itself is
happening upstream in gogpu/gogpu#431.
## Font Fallback Hardcoded Paths and Startup I/O

To resolve broken Unicode/CJK/Emoji rendering in X11, Wayland, and gogpu,
we introduced a sequential search through standard system paths for fallback
fonts. While this operates as a self-sufficient solution, it comes with a
few trade-offs:
1. **Startup I/O Overhead:** Sequential `os.ReadFile` or `os.Stat` calls on a long list
   of candidate paths can introduce minor startup latency, especially on slow disks.
2. **Hardcoded Paths:** System font paths are highly distribution-dependent. If a
   user installs fonts in a non-standard directory, the fallbacks will not be found.
*Future improvement:* Introduce a configurable font path list in the application
settings or delegate font discovery to system tools like `fontconfig` on Unix-like
systems.



## Panics on the gogpu render thread used to be unreportable

gogpu invokes the draw and update callbacks on its own render thread and
re-panics the recovered value on the caller
(`internal/thread.(*Thread).CallVoid`). The value crosses the boundary, the
stack does not, so a crash report of a fault in our renderer pointed at gogpu's
threading code instead. `LogAndRepanic` now writes the real stack to the debug
log before the value is handed over. Any future callback invoked by a foreign
thread should be wrapped the same way.

`App.OnClose` was being used as if it were a window close request. It is the
teardown notification of `App.shutdown()`, so the exit confirmation dialog was
being opened while the application was already going down, panic unwinding
included. Whether a genuine, vetoable window close should raise the
confirmation dialog is still open: gogpu has `Window.SetOnClose`, which does
return a bool, but the primary window is not reachable through the public App
API from here.
## The highlighter contract is rune indices, not byte offsets

`UNICODE_PLAN.md` recommended indexing `Highlighter.Highlight` attributes by
byte offset, on the grounds that a byte oriented grammar produces that
naturally and that it survives any later change to clustering. Runes were
chosen instead, because every producer and every consumer that exists already
speaks runes: colorer reports code point offsets, the chroma based highlighter
appends one attribute per rune of each token, and the editor drawing the
result counts runes as it decodes. Byte offsets would have meant changing all
of them at once, to fix a bug that was about code points versus UTF-16 units.

If a highlighter turns up that genuinely works in bytes, the honest move is a
second mapper next to the first, not a redefinition of the contract underneath
code that already relies on it.

## Touchpad scroll deltas below one notch are dropped in gogpu

`gogpu_host.go` converts the fractional `OnScroll` delta into whole wheel
events with `int(math.Abs(dy))` and returns early when that truncates to
zero. A precision touchpad produces a stream of small deltas, so smooth
scrolling is effectively dead there; ebiten clamps any nonzero delta up to
one event instead, which is jumpy in the other direction. The fix is an
accumulator per host that carries the remainder into the next event. It was
not done when the system-lines multiplication moved out of the hosts because
nobody had a precision touchpad complaint on file.

## Clipboard tests leak through the global clipboard

`TestClipboard_RoundTripsThroughInternalBuffer` fails intermittently in a
full `go test .` run with a value set by another test ("secret text"),
because the internal clipboard buffer is package global and tests run in
source order share it. It passes in isolation. Not a wheel/scroll regression;
observed on Windows while touching unrelated code.

## Ebitengine Monitor Initialization and GUI Lag on Windows

1. **Monitor Scale Factor:** `ebiten.Monitor()` can return `nil` before `ebiten.RunGame` is called under some Windows graphics configurations (especially virtual machines, RDP, or multi-monitor setups), leading to a nil pointer dereference. We added a safety guard fallback of `1.0` scale.
2. **GUI Lag:** Users report visual lag/sluggishness in both `gogpu` and `ebiten` backends on Windows. This could be due to CPU rasterization overhead, GPU driver sync issues, or high polling rates. Needs further performance profiling of the draw loops on various target machines.
## Declarative Bindings Architecture Proposals

Proposals for future architecture evolution (signals integration, virtual-tree diffing, zero-copy shm canvas, runtime introspection) are documented in `ARCH_PROPOSALS.md`.
## Classic Win32 Console API Renderer (`--tty=winapi`)

We added a dedicated Win32 Console API backend using `WriteConsoleOutputW`, `SetConsoleCursorPosition`, and `SetConsoleCursorInfo` without requiring VT/ANSI escape sequences.
1. **Wine Detection & Probing:** When running in `wineconsole` (where a Win32 console buffer is present), it defaults to `winapi`; when running from a raw Unix terminal via Wine, it uses `ansi`.
2. **Console Dimensions Fallback:** To avoid 0x0 buffer allocations in virtualized or non-standard TTY environments (e.g. Wine without a mapped conhost), `GetTerminalSize` falls back to `$COLUMNS`/`$LINES` or `80x25`.
3. **Color Quantization:** 24-bit RGB and 256-indexed palettes are dynamically quantized to the classic 16-color IRGB DOS/Win32 attribute space (`FOREGROUND_*` / `BACKGROUND_*`).

## Classic Win32 GUI / GDI Renderer (`--gui=win32`)

We implemented a native pure-Go Win32 graphical windowing backend using standard user32/gdi32 calls (`CreateWindowExW`, `SetDIBitsToDevice`, `BitBlt`, `WM_PAINT`, `WM_DROPFILES`) without requiring CGO or Direct3D:
1. **Wine Compatibility:** Works smoothly in Wine desktop environments out of the box and serves as the default GUI backend under Wine.
2. **Double-Buffering:** Rasterizes terminal text cells into an RGBA/BGRA DIB section, updating the window client area with zero flicker.
3. **Shell Drag & Drop:** Integrates with `WM_DROPFILES` via `shell32.dll` to support dropping files into the UI directly from file managers.
