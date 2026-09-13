# Initialize history viewport before exposing its rows

## Root Cause

The history ListView was visible with its default contentY while selection and
viewport positioning were deferred. A synchronous visibility regression observed
offset 0 followed by 2140.57. A rendered-frame-only test missed this state because
its event-loop timing allowed the callbacks to finish first.

## Solution

Keep history rows transparent until an explicit initial layout pass realizes row
geometry and applies the selection and native viewport. Use forceLayout and a
queued initialization, not a timer or arbitrary delay. Reinitialize replaced
models without changing normal keyboard or pointer scrolling behavior.

## Prevention

Check visible state immediately after menu publication as well as subsequent
frames and a full-scene echo. Retain 175% leaf alignment coverage. Further testing
can stress asynchronous filtering and unusually large imported histories.
