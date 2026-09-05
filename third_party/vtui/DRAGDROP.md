# Drag and drop in vtui

`dragdrop.go` holds everything about drag and drop that is not tied to a
display server: the payload types, the two registration points, and the
`text/uri-list` codec every protocol needs. A backend adds the protocol; the
application adds the policy. Neither knows about the other.

## For the application

Install one drop target:

    vtui.SetDropTarget(myTarget)   // myTarget implements HandleDrag

`HandleDrag` is called for every step of a gesture over our window and
answers what a drop *would* do at `ev.X, ev.Y` (cell coordinates). That
answer is what gives the pointer its copy / move cursor, so it must be quick
and must not open dialogs. `DragDrop` is the phase where the work actually
starts; start it in the background and return.

To hand files to other applications:

    action, err := vtui.StartDrag(payload, vtui.DropCopy|vtui.DropMove)

`err` is `ErrDragUnsupported` on every backend without a protocol, which is
every terminal today. Check `vtui.DropSupported()` / `vtui.DragOutSupported()`
before offering the feature in the UI.

## For a backend

Implement `DragBackend` and register it once the window exists:

    vtui.SetDragBackend(host)

Then, for every step of an incoming gesture, build a `DragEvent` and call
`vtui.DeliverDragEvent(ev)`. Rules a backend has to follow:

- convert device pixels to cell coordinates before filling `X` and `Y`;
- decode dragged files into `Payload.Paths` (use `ParseURIList`), leave
  anything that is not a local file in `Payload.URIs`;
- send exactly one `DragEnter`, any number of `DragOver`, and then one
  `DragLeave` or one `DragDrop`;
- report the returned `DropAction` back to the source as the protocol
  requires.

`DeliverDragEvent` may be called from the backend's own goroutine: it hops to
the UI thread itself and gives up after `DragDeliverTimeout`, because a
display server will not wait for a UI stuck behind a modal dialog.

## Diagnosing a gesture that does nothing
Note that vtui redirects stderr to a file in its crash directory, so
anything gogpu itself reports goes there rather than to the terminal, and
piping the application's output will not catch it. The path is named in
debug.log at startup; the file is deleted on exit if it stayed empty.

With `VTUI_DEBUG=1` every decision on both paths writes a line to
debug.log, and which line is *missing* says where the gesture died:

- `DND: drag backend is now ...` and `GOGPU_DND: drag callbacks registered`
  appear once at startup. Without them nothing else can work, and the
  display server named there decides which of gogpu's own backends answers.
- `GOGPU_DND: OnDragDrop fired ...` is an incoming drop arriving from gogpu.
  Missing means gogpu never told us, so the question is its platform side,
  not ours.
- `GOGPU_DND: drop ... lands on cell X,Y of a CxR screen` is the pixel to
  cell conversion. A cell outside the screen is the HiDPI question in
  REVIEW.md, and it looks exactly like a drop that was ignored.
- `DND: no drop target is installed` means the payload arrived before, or
  without, the application registering a target.
- `GOGPU_DND: main loop update callback is running` appears on the first
  frame. Without it no drag out can ever be handed over; the tick counts in
  `drag out asked for ...` and in the give-up line say the same for one
  gesture.
- `GOGPU_DND: gogpu took the drag out ...` separates "gogpu refused the
  gesture" from "gogpu accepted it and nothing ever came back".
## Status

- core, uri-list codec: done (this file's package)
- X11 (XDND): both directions done, in x11_xdnd.go
- X11 limitation: only copy is announced when dragging out. A move would
  mean deleting the originals because a target said it took them, and no
  file is worth that much trust until the behaviour is tested widely.
- X11 limitation: XdndProxy is not followed when looking for a target, so a
  window that only accepts drops through a proxy is not seen
- X11 limitation: an INCR (incremental) selection transfer is refused
  rather than half read, so a drop of an enormous file list currently
  fails visibly instead of silently losing entries
- Wayland (wl_data_device): planned
- gogpu (its own Windows / macOS / X11 / Wayland backends behind one API):
  both directions done, in gogpu_dnd.go. The drag source half needs gogpu
  v0.50.0 or later
- gogpu limitation: only copy is announced for a drop. gogpu reports a
  finished drop and nothing before it, so neither the actions the source
  allows nor the modifiers held are known, and nothing travels back to the
  source either, which is the one thing a move would need
- gogpu limitation: nothing arrives before the drop, so a target cannot
  highlight anything while the pointer is still moving. The gesture is
  replayed as one enter immediately followed by the drop
  - gogpu, X11, unfixed as of 0.50.1: a drop from another application is
    lost more often than it arrives - measured at 4 callbacks in 10 attempts
    - and the position of the ones that do arrive is always 0,0. Both come
    from gogpu's connection layer rather than its XDND code: a synchronous
    round trip discards every event that arrives while it waits for its
    reply, and the incoming drop path makes one at exactly the wrong moment
    (gogpu/gogpu#431). The position is worked around by taking it from the
    pointer whenever a drop is reported at exactly the origin while the
    pointer is elsewhere; the lost drops cannot be worked around from here,
    because the window belongs to gogpu
  - gogpu limitation: only copy is offered when dragging out, as on X11 and
      for the same reason. gogpu's DragData carries no action either, so there
      is nothing else it could be told
  - gogpu, X11, fixed in gogpu 0.50.1: a drag out used to reach no target
    at all. Three defects stacked - XDND messages sent with a mask the
    window manager swallowed, a stale ButtonRelease ending the session
    before it began, and the wait for XdndFinished discarding the
    SelectionRequest that carries the data. Both directions work once
    go.mod carries the fix
  - gogpu limitation: a drag out begins on the first frame after it is asked
    for, since the platform side may only be touched from the main loop. The
    request wakes that loop itself, so the delay is a frame, not a wait
- win32 GUI backend (OLE): both directions done, in win32_dnd_windows.go
  (drag source, ole32!DoDragDrop) and win32_droptarget_windows.go (drop
  target, ole32!RegisterDragDrop). CF_HDROP is the format both directions
  speak; the source also offers CF_UNICODETEXT so a text field receives the
  names rather than nothing
- win32 limitation: the drop target is built only for amd64 and arm64.
  IDropTarget takes POINTL by value, and where an eight-byte struct sits in
  a call frame differs between the 64-bit ABIs and 32-bit stdcall. Nothing
  ships for 32-bit Windows, so those builds keep the WM_DROPFILES path and
  register no target
- win32: WS_EX_ACCEPTFILES and WM_DROPFILES are kept as a fallback for
  sources that never speak OLE, and for the case where RegisterDragDrop
  fails. That path learns of the gesture only after the drop, so it
  synthesises enter, over and drop back to back and cannot report an effect
  to the source
- Windows under Wine: a drag out reaches other Wine windows only. Wine
  bridges XDND into OLE for drops coming in, but has no bridge the other
  way, so a payload offered to a native X11 application finds no target and
  the pointer says no. That is upstream, not here
- macOS outside gogpu: planned
- terminals: no protocol exists; nothing is registered, so both directions
  are reported as unsupported rather than half working