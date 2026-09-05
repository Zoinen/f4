# Issue #248 solution review

## Problem

The local and remote PTY readers parsed every output chunk immediately and
called `vtui.FrameManager.Redraw()` after every chunk. `FrameManager` coalesces
the notification channel, but the UI loop consumes the notification before
the next render, so a sustained ConPTY stream can still force one full frame
per read. The parser and terminal history must remain lossless and immediate;
only redundant wake-ups should be removed.

## Candidate 1: redraw after every PTY read

This is the current implementation. It is the simplest and gives the lowest
possible display latency, but it couples the renderer rate to ConPTY chunking.
ConPTY can split one command into many reads, so heavy output repeatedly walks
all visible frames and flushes the renderer while the parser is still busy.
This directly reproduces the input lag and stutter from the issue.

**Pass 1:** rejected because the Windows stress run produced hundreds of
redraw requests during a few thousand lines and the UI became visibly slow.

## Candidate 2: parse PTY output on the UI goroutine

Posting every output chunk as a UI task would serialize parsing and rendering,
and could make the terminal state easier to reason about. It would also block
keyboard events behind large output bursts, turn parser work into a UI pause,
and allow the task queue to grow without bound.

**Pass 2:** rejected because it moves the expensive work onto the same path
that must process user input. It would worsen the reported lag and would not
help remote PTYs.

## Candidate 3: coalesce redraw requests for 16 ms

The PTY reader continues parsing synchronously in its existing goroutine, so
no output is discarded and terminal state remains available to subsequent
input. The first request in a burst calls `Redraw` immediately. Further
requests are suppressed for 16 ms, approximately one 60 Hz frame, after which
the next burst wakes the renderer again. The scheduler is per `PanelsFrame`,
and it is stopped during frame shutdown so a late PTY read cannot wake a closed
frame.

**Pass 3:** selected. It preserves immediate feedback for short commands,
caps redundant full-frame work during sustained output, applies equally to
local and remote PTYs, and does not alter ANSI parsing, scrollback, or shell
input ordering.

## Regression and validation

- `TestTerminalRedrawSchedulerCoalescesBurst` verifies one redraw for a burst,
  another redraw after the interval, and no redraw after shutdown.
- `go test ./... -count=1` passed.
- Native Windows x64 build passed.
- A real Windows ConPTY run processed 2,000 generated `cmd.exe` lines through
  `output-2000`, returned to the prompt, and reduced redraw requests to 47.
- `git diff --check` passed.

## Risk review

The change does not throttle parsing or PTY reads, so it cannot lose bytes or
change ANSI state. A frame may display several output chunks at once, which is
the intended behavior for a terminal UI. The initial redraw is immediate, and
the 16 ms window is below ordinary terminal input feedback tolerance. The
scheduler is stopped before PTY shutdown and all calls are mutex-protected,
covering concurrent reader and close paths.
