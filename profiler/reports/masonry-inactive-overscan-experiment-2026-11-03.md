# Masonry inactive overscan experiment, 2026-11-03

The o185 diagnostic restricted the inactive shell panel's
incubation plan to visible rows. Both panels still received the
catalog. The candidate passed the first-viewport and foreground
checks at 160 Hz and DPR 1.75.

The instrumentation revealed a mismatch between shell activation
and the layout receiving the wheel. Construction starts fell
from 160 to 75 on the measured scrolling layout. The other
layout stayed at 73 starts. Thus the diagnostic throttled the
very layout it was intended to help. Coverage holds rose from
246 to 4,786. The run delivered more scheduled inputs, but that
coincided with the camera repeatedly waiting for coverage.

This is a negative result for using shell activation to select
an overscan target. It does not test reducing construction on
the non-scrolling panel. A follow-up should track wheel or
camera ownership directly and verify the owner of the reduced
work before interpreting cadence or coverage.

## Corrected panel isolation

The fixed-input harness selects side 0 explicitly. The o186
diagnostic limited side 1 only. Joining coverage to layout owners
confirmed that the scrolling side still started 160 roots while
the other side fell from 73 to 55. First-viewport and foreground
checks passed.

Compared with o184, the longest construction lifetime fell from
1,655 to 481 ms. Median elapsed stayed at 10.0 ms. Coverage holds
rose from 246 to 268. Display gaps above 9.375 ms rose from four
to six, with a maximum of 62.50 ms. These single runs do not
establish a benefit from suppressing the other panel's overscan.
The diagnostic remains isolated.
