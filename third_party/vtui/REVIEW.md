# Doubts and unfinished business in vtui

Decisions taken in a hurry, things noticed in passing, and things left alone
on purpose. One file, so a review can be one sitting.

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
