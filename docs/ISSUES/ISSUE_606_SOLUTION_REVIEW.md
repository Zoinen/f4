# Issue #606 solution review

## Problem model

The FUSE bridge used one mutex around every VFS call. That preserved safety
for stateful backends, but it also forced unrelated directory listings,
metadata requests, and reads on a remote mount into a single queue. The SFTP
client used by `SFTPVFS` multiplexes independent requests safely over one SSH
connection, so this serialization was an avoidable latency multiplier for
searches and ordinary tools such as `rg`.

## Three candidate solutions

### A — Remove the bridge lock

This would maximize concurrency with the smallest diff, but it would also
expose every existing VFS implementation to concurrent calls. Stateful
sessions such as FISH+, MTP, and device backends could interleave protocol
messages or close a resource while another call is using it.

### B — Explicit backend opt-in with read/write locking (selected)

Keep serialization as the default contract. Add a small `vfs.ConcurrentVFS`
capability and let a backend opt in only after its client and handles are
known to support independent operations. The bridge uses shared locking for
read-side calls from opted-in backends, while mutations and shutdown remain
exclusive. SFTP is the first opt-in backend.

### C — Fixed worker pool per mount

Route all bridge calls through a bounded worker pool and configure the pool
size per backend. This gives tighter control over remote-server load, but it
adds queueing, cancellation, shutdown, and ordering machinery before the
backend safety contract is expressed. It is a useful later refinement if a
backend needs a limit other than the kernel's existing request window.

## Three-pass review of option B

1. **Correctness pass.** Default backends still take the old exclusive path.
   SFTP read/list/stat/open calls can overlap, while mkdir/remove/rename,
   writes, attribute changes, symlink creation, and bridge close remain
   exclusive. Directory cache and operation statistics retain their own locks.

2. **Adversarial pass.** Bridge close takes the exclusive lock and therefore
   waits for active shared calls. A file handle has a separate lifecycle lock
   so `ReadAt` cannot race `Close` after read-side parallelism is enabled.
   The opt-in interface is capability-based, so a backend cannot become
   concurrent accidentally through an unrelated `VFSCapabilities` flag.

3. **Regression pass.** The bridge test suite keeps the default fake backend
   serialized, and a new test proves that an explicitly concurrent backend
   actually overlaps independent reads. Focused race tests cover the bridge,
   spooling, random reads, and close paths. The implementation is limited to
   the FUSE bridge and SFTP capability declaration; no other backend changes
   are implied.

## Validation target

The local environment can exercise the bridge concurrency contract and native
ARM64 builds. A live SFTP/FUSE latency benchmark requires a reachable remote
host and FUSE mount privileges, so the final report must distinguish the
verified bridge behavior from that unavailable end-to-end measurement.
