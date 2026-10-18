# Resist panel edges immediately and release ownership before momentum

## Problem and Root Cause
The rubber-band curve initially responded almost 1:1, unlike the viewer edge
response (0.25). ScrollEnd scheduled a 60 ms delay; subsequent OS momentum
stopped the restore animation and stretched the edge again. Real Cocoa traces
also show a second Qt ScrollBegin introducing the inertial stream, so Begin
alone cannot establish that a new finger gesture has started.

## Solution
Use a 0.25 initial response in the bounded resistance curve and its inverse.
Start return synchronously on ScrollEnd. If a separate end is omitted, the
first momentum event at an already stretched edge also starts return without
applying that delta. Momentum reaching an edge starts return immediately.
Latch suppression across return completion and inertial Begin events, clearing
it only on a direct ScrollUpdate. Normal in-bounds momentum is preserved.

## Verification and Prevention
The resistance regression failed in all five modes before the fix. Tests also
require a running return immediately after release, unchanged position after
inertial Begin/Momentum, forward progress of the spring and no late re-stretch.
New finger input and momentum hitting the opposite edge are covered. Keep real
native-to-Qt phase traces: synthetic idealized Begin/Update/End alone would
miss Cocoa's momentum Begin. Further useful coverage is a repeated physical
trackpad release/regrab test during the spring animation.

Tags: #qt #qml #macos #overscroll #momentum #gesture-ownership #regression

## Follow-up: Regrab During Return
Suppressing all Begin events left the return running under a new finger contact.
Begin now pauses at the current visible pose. A direct Update, including a
zero-delta update, stops the spring without clamping before applying movement.
An inertial continuation instead resumes the same spring's progress, retaining
momentum suppression. The new regrab assertion failed in all five modes before
the fix: the pose moved during the 40 ms hold after Begin. Coverage also checks
immediate movement farther into overscroll and a second release/return.
