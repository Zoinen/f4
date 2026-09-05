# Issue #247 solution review

## Candidate 1: leave the existing 100 ms worker-side throttle unchanged

The copy/extraction worker already limits its own aggregate progress callback
to roughly ten updates per second, and the queue view coalesces refreshes. This
does not cover foreground/background progress dialogs: `DialogReporter` posts
every received state directly to the UI task queue. Native Windows validation
with a 3000-file archive still produced about 7,640 redraw requests, so this
candidate does not address the remaining path.

## Candidate 2: move archive extraction or progress formatting onto the UI thread

This would avoid cross-thread dialog updates, but it would make the expensive
archive and filesystem work block keyboard dispatch and rendering directly.
It reverses the purpose of `RunAsync` and is rejected because it worsens the
reported symptom and risks freezing the entire application.

## Candidate 3: latest-state UI scheduler for every progress dialog

Selected. `DialogReporter` retains only the newest scan/transfer state, posts
the first update immediately, and schedules at most one later update every
100 ms. Archive extraction, ordinary copy/delete operations, and advanced
progress tasks all share this reporter, so the fix covers the common path
without changing worker cancellation or progress accounting. The scheduler is
stopped before the dialog closes; a regression test verifies burst coalescing,
latest-state delivery, and no updates after stop.

## Three-pass review

1. Correctness: the worker keeps reporting every state; only rendering is
   coalesced. The latest progress state wins, and the final forced update is
   delivered before the completion callback closes the dialog when no earlier
   update is pending.
2. Concurrency: all scheduler state is protected by one mutex. The UI task
   never holds that mutex while mutating widgets or requesting a redraw, and
   stopping prevents timer callbacks from posting new work after close.
3. Scope/regression: the change is limited to dialog progress rendering. The
   queue reporter retains its existing coalescing behavior, cancellation and
   operation execution are untouched, and the interval matches the existing
   100 ms worker-side progress cadence.
