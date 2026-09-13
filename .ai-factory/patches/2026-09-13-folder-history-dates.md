# Folder visit timestamps and native date columns

## Root Cause

Folder history reserved a console timestamp column but its mutation path created
records without a timestamp. Native history consequently rendered leading spaces.
The regression logged a zero timestamp before the fix.

## Solution

Stamp each accepted folder visit, including revisits, preserving alias deduplication
and locks. Reuse the dated native history columns; legacy entries have a blank date
on the right until revisited. Clone the QML font before choosing the monospace family
so metadata styling cannot mutate the shared UI font.

## Prevention

Cover new visits, revisits, aliases, locks, persisted timestamps and legacy entries.
Retain shared dated-layout leaf alignment and rendering checks at DPR 1.75.
Additional coverage can exercise imported history and bookmark-only rows live.

## Tags

`#history` `#timestamps` `#qml` `#regression-first`
