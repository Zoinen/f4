# f4 Asynchronous Virtual File System (VFS)

## Overview

The VFS in `f4` is designed to be fully non-blocking. This architecture ensures that the UI remains responsive even when performing operations on high-latency remote systems (SFTP, FTP) or slow storage devices.

## Core Design Principles

### 1. Context-Aware Operations
Every method in the `vfs.VFS` interface accepts a `context.Context`. This allows for:
*   **Instant Cancellation:** If a user navigates away from a directory that is still loading, the background operation is immediately aborted.
*   **Timeouts:** Prevention of UI hangs on stale network connections.

### 2. Streaming Directory Listing
`ReadDir` does not return a complete slice of items. Instead, it uses a callback pattern:
```go
ReadDir(ctx context.Context, path string, onChunk func([]VFSItem)) error
```
As chunks of files are read from the source (e.g., first 100 files from a directory of 10,000), they are immediately posted to the UI thread. The user can start interacting with visible files while the rest are still being fetched in the background.

### 3. The `ErrLoading` Pattern (Reactive Rendering)
For random access operations (used by Viewer and Editor), the VFS and its buffers use a "Try-and-Trigger" approach:
1.  The UI requests a range of bytes.
2.  If the data is not in the local cache, the buffer immediately returns `piecetable.ErrLoading` and triggers a background fetch for that specific chunk.
3.  The UI renders a `[ Loading... ]` placeholder and continues its loop.
4.  Once the data arrives, a `Redraw` is triggered, and the actual content replaces the placeholder.

### 4. Background Indexing
To support features like word wrapping and fast navigation in the Editor, `f4` performs background indexing of line breaks (`\n`). As bytes stream in, a background goroutine scans them and updates the `LineIndex` incrementally.

## Why this matters for FISH+
This architecture was specifically chosen to support the **FISH+** protocol (see [FISH+.md](FISH+.md)). By allowing operations to be partial, cancellable, and asynchronous, we can offload heavy computations (like searching or indexing) to the remote server while keeping the local `f4` instance lightweight and fast.
