# Issue 397 solution review

The reporter still reproduced the crash from a Windows shortcut with a
non-default console buffer/window layout. The attached logs show
`ReadEvent error: No process is on the other end of the pipe`, followed by an
80x25 fallback. In the current `vtinput v0.1.4`, that read error closes the
event channel immediately; the f4 event loop then loses its input source while
the console host is still applying the resize.

## Three-pass comparison

### Pass 1: ignore the error or change f4's resize handling

This does not address the ownership of the event channel. `vtinput` still
terminates its reader goroutine on the transient pipe error, so f4 can still
leave its main loop. Rejected.

### Pass 2: add a retry wrapper inside f4

The failing read and event-channel lifecycle are private to `vtinput`; f4
cannot reliably retry it without duplicating console-reader logic. This would
also leave other `vtinput` consumers exposed to the same resize race. Rejected.

### Pass 3: recover in vtinput and pin f4 to the reviewed revision

The fix belongs in `vtinput`: classify only
`ERROR_PIPE_NOT_CONNECTED` as transient on Windows, retry with a bounded delay,
reset the retry counter after a successful read, and stop promptly when the
reader closes. The Windows regression tests cover the error classification and
event-channel recovery. Until the upstream PR is merged, f4 pins the upstream
module path to the immutable `Zoinen/vtinput` commit containing that fix.
This is the selected solution.

## Safety review

The retry is limited to the resize-specific Windows error; broken pipes and
operation-aborted errors remain terminal. The retry count is bounded, the
reader's close signal interrupts the delay, and successful reads clear the
counter. The change therefore preserves shutdown behavior while preventing a
single transient resize disconnect from ending f4's event stream.

## Validation plan

1. Run the related `vtinput` Windows tests from the PR revision.
2. Run f4's full test suite and vet on Windows.
3. Build a native Windows binary and repeat launches with the reported fixed
   buffer/window layout and resize-related settings, checking that the process
   stays alive and the event stream continues.
