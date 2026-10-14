# Preserve local host information across Ctrl+L reopening

## Root Cause
The cached host snapshot was owned by the disposable InfoPanel. Ctrl+L creates
a new panel after closing, so each opening started a new system_profiler task.

## Solution
Use an independently owned shared HostCache with nonblocking detached snapshots.
Serve the last successful value even while stale. Coalesce background refreshes
with a cancellable gate and keep all probe I/O outside the snapshot lock. Panel
closure cancels its request, not the successful cache. Omit technical Mac model
identifiers and duplicate architecture labels. Convert recognized built-in
display-type identifiers to descriptive names, preserving external display names.

## Verification
New cache regression initially failed (NewHostCache absent). After implementation,
test fresh reuse, snapshot isolation and reads during a deliberately blocked
refresh. A panel reopening regression checks first-pass data with no new task.
Parser checks require MacBook Pro and Built-in Liquid Retina XDR Display; live
collection confirms ARM64 without duplication. Further coverage: monitor hotplug
and simultaneous refresh cancellation in two information panels.

Tags: #sysinfo #cache #panel-lifetime #darwin #regression
