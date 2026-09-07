package main

import (
	"context"
	"io"
	"sync"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// ViewerBackend provides async random access to a file using small cache window.
type ViewerBackend struct {
	file       vfs.ReadAtCloser
	size       int64
	codepage   int
	dataOffset int64 // bytes skipped from the on-disk file (the UTF-8 BOM)

	path    string
	owner   vfs.VFS
	indexer vfs.LineIndexer

	// totalLines is what the far side last reported, and totalForSize is the
	// file size it reported it for. A log file grows while it is being read,
	// so the total is only reused while the size it was counted at still
	// holds; anything else would put the viewer at the wrong offset.
	totalLines   int64
	totalForSize int64

	mu                     sync.Mutex
	cacheOff               int64
	cacheData              []byte
	isFetching             bool
	readErr                error
	readErrOff             int64
	readErrLen             int
	fetchOff               int64
	fetchLen               int
	fetchCancel            context.CancelFunc
	fetchSerial            uint64
	readNotificationWanted bool
	ready                  chan struct{}
	closeOnce              sync.Once
	closeErr               error

	// A line start can be farther away than the cache window (a minified
	// document or a file containing one very long line is a common example).
	// Keep the backward scan cursor between non-blocking retries. Without this
	// state every retry starts at lineSeekTarget; once the cache has moved more
	// than one window backwards, that jumps it forward again and the two cache
	// positions oscillate forever.
	lineSeekTarget int64
	lineSeekCurr   int64
	lineSeekActive bool

	ctx       context.Context
	cancelCtx context.CancelFunc
}

func NewViewerBackend(ctx context.Context, v vfs.VFS, path string) (*ViewerBackend, error) {
	f, err := v.Open(ctx, path)
	if err != nil {
		return nil, err
	}

	bCtx, bCancel := context.WithCancel(context.Background())
	b := &ViewerBackend{
		file:         f,
		size:         f.Size(),
		path:         path,
		owner:        v,
		totalLines:   -1,
		totalForSize: -1,
		ctx:          bCtx,
		cancelCtx:    bCancel,
	}
	if indexer, ok := v.(vfs.LineIndexer); ok {
		b.indexer = indexer
	}
	return b, nil
}

func (b *ViewerBackend) Close() error {
	b.closeOnce.Do(func() {
		if b.cancelCtx != nil {
			b.cancelCtx()
		}
		b.mu.Lock()
		if b.fetchCancel != nil {
			b.fetchCancel()
		}
		b.cacheData = nil
		b.signalReadyLocked()
		b.mu.Unlock()
		b.closeErr = b.file.Close()
	})
	return b.closeErr
}

func (b *ViewerBackend) Size() int64 {
	// The write below was already locked, but the read that follows it was
	// not, so a viewer drawing on the UI goroutine raced a background task
	// refreshing the size -- jumpToEnd runs Size through RunAsync while Show
	// is calling it too. Both go through the mutex now.
	if b.file != nil {
		newSize := b.file.Size() - b.dataOffset
		if newSize < 0 {
			newSize = 0
		}
		b.mu.Lock()
		b.size = newSize
		size := b.size
		b.mu.Unlock()
		return size
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.size
}

func (b *ViewerBackend) ReadAt(offset int64, length int) ([]byte, error) {
	b.mu.Lock()
	if b.ctx != nil && b.ctx.Err() != nil {
		b.mu.Unlock()
		return nil, b.ctx.Err()
	}
	if offset < 0 || length <= 0 || offset >= b.size {
		b.mu.Unlock()
		return nil, io.EOF
	}
	if offset+int64(length) > b.size {
		length = int(b.size - offset)
	}
	// Check cache hit
	if b.cacheData != nil && offset >= b.cacheOff && offset+int64(length) <= b.cacheOff+int64(len(b.cacheData)) {
		start := offset - b.cacheOff
		data := b.cacheData[start : start+int64(length)]
		b.mu.Unlock()
		return data, nil
	}
	if b.readErr != nil {
		if offset >= b.readErrOff && offset+int64(length) <= b.readErrOff+int64(b.readErrLen) {
			err := b.readErr
			b.mu.Unlock()
			return nil, err
		}
		// A new source range is a fresh request, not a retry loop for the
		// failed row. Let navigation away from an unreadable region proceed.
		b.readErr = nil
	}

	// The read-access contract distinguishes a bounded native file read from a
	// network/extraction operation. Local misses need no goroutine/UI round trip.
	fetchOff := max(int64(0), offset-64*1024)
	fetchLen := int(min(int64(256*1024), b.size-fetchOff))
	// Large explicit requests are not retained in the viewport buffer.
	oversized := offset+int64(length) > fetchOff+int64(fetchLen)
	if oversized {
		fetchOff, fetchLen = offset, length
	}
	direct := documentReadAccess(b.file, b.owner) == vfs.ReadAccessDirectLocal
	if direct {
		b.mu.Unlock()
		data := make([]byte, fetchLen)
		n, err := readDocumentBytes(b.ctx, b.file, data, b.dataOffset+fetchOff)
		if err != nil && err != io.EOF {
			b.mu.Lock()
			b.readErr, b.readErrOff, b.readErrLen = err, fetchOff, fetchLen
			b.mu.Unlock()
			return nil, err
		}
		b.mu.Lock()
		if b.ctx.Err() != nil {
			b.mu.Unlock()
			return nil, b.ctx.Err()
		}
		if !oversized {
			b.cacheOff, b.cacheData = fetchOff, data[:n]
		}
		b.readErr = nil
		b.mu.Unlock()
		start := int(offset - fetchOff)
		if start >= n {
			return nil, io.EOF
		}
		end := min(start+length, n)
		return data[start:end], err
	}
	// Only the newest missing range is retained. A superseded read may finish,
	// but it cannot replace the visible source window or cause a stale redraw.
	b.readNotificationWanted = true
	if !b.isFetching || offset < b.fetchOff || offset+int64(length) > b.fetchOff+int64(b.fetchLen) {
		b.fetchOff, b.fetchLen = fetchOff, fetchLen
		b.fetchSerial++
		if b.fetchCancel != nil {
			b.fetchCancel()
		}
	}
	if !b.isFetching {
		b.isFetching = true
		var post func(func() bool)
		if frames := vtui.FrameManager; frames != nil {
			post = frames.PostTaskWithRedrawDecision
		}
		go b.fetchLatest(post)
	}
	b.mu.Unlock()
	return nil, piecetable.ErrLoading
}

func documentReadAccess(file vfs.ReadAtCloser, owner vfs.VFS) vfs.ReadAccessProfile {
	if source, ok := file.(vfs.ReadAccessProfiler); ok {
		return source.ReadAccessProfile()
	}
	if _, ok := file.(*vfs.MemoryReadAtCloser); ok {
		return vfs.ReadAccessDirectLocal
	}
	if owner != nil {
		return owner.GetCapabilities().ReadAccess
	}
	return vfs.ReadAccessUnknownExpensive
}

func (b *ViewerBackend) signalReadyLocked() {
	if b.ready != nil {
		close(b.ready)
	}
	b.ready = make(chan struct{})
}

// ReadContext is for background navigation. Completion wakes it immediately;
// unlike polling, readiness does not depend on the UI consuming a posted task.
func (b *ViewerBackend) ReadContext(ctx context.Context, offset int64, length int) ([]byte, error) {
	for {
		b.mu.Lock()
		if b.ready == nil {
			b.ready = make(chan struct{})
		}
		ready := b.ready
		b.mu.Unlock()
		data, err := b.ReadAt(offset, length)
		if err != piecetable.ErrLoading {
			return data, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.ctx.Done():
			return nil, b.ctx.Err()
		case <-ready:
		}
	}
}

func (b *ViewerBackend) fetchLatest(post func(func() bool)) {
	for {
		b.mu.Lock()
		off, length, serial := b.fetchOff, b.fetchLen, b.fetchSerial
		ctx, cancel := context.WithCancel(b.ctx)
		b.fetchCancel = cancel
		b.mu.Unlock()
		data := make([]byte, length)
		n, err := readDocumentBytes(ctx, b.file, data, b.dataOffset+off)
		b.mu.Lock()
		stale := off != b.fetchOff || length != b.fetchLen || ctx.Err() != nil
		cancel()
		b.fetchCancel = nil
		if b.ctx.Err() != nil {
			b.isFetching = false
			b.signalReadyLocked()
			b.mu.Unlock()
			return
		}
		if stale {
			b.mu.Unlock()
			continue
		}
		if err == nil || err == io.EOF {
			b.cacheOff, b.cacheData = off, data[:n]
			b.readErr = nil
		} else {
			b.readErr, b.readErrOff, b.readErrLen = err, off, length
		}
		b.isFetching = false
		b.signalReadyLocked()
		b.mu.Unlock()
		if post != nil {
			post(func() bool {
				b.mu.Lock()
				defer b.mu.Unlock()
				return b.ctx.Err() == nil && b.fetchSerial == serial && !b.isFetching && b.readNotificationWanted
			})
		}
		return
	}
}

// A ready visible window may omit an unfinished overscan row. Its optional
// read can finish quietly; a later missing visible read renews the notification.
func (b *ViewerBackend) suppressOptionalReadNotification() {
	b.mu.Lock()
	b.readNotificationWanted = false
	b.mu.Unlock()
}

// LastReadError distinguishes an incomplete source range from one whose read
// actually failed; semantic navigation must not acknowledge the latter.
func (b *ViewerBackend) LastReadError() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.readErr
}

// SearchFrom asks the file system for the first occurrence of pattern at or
// after off. searched is false when the file system cannot answer, and the
// caller then scans the file itself; when it is true, an offset of -1 means
// the file was searched and the pattern is not in it.
//
// The difference matters most where it costs most: searching a remote file
// by reading it means downloading it, and a host that can grep its own copy
// answers in one round trip no matter how large the file is.
func (b *ViewerBackend) SearchFrom(ctx context.Context, pattern string, off int64) (int64, bool) {
	if b.owner == nil || pattern == "" || !b.owner.GetCapabilities().HasSearch {
		return 0, false
	}
	matches, err := b.owner.Search(ctx, b.path, pattern)
	if err != nil || matches == nil {
		return 0, false
	}
	for at := range matches {
		logicalAt := at - b.dataOffset
		if logicalAt >= off {
			return logicalAt, true
		}
	}
	return -1, true
}

// SearchBefore asks a searchable file system for the last occurrence strictly
// before off. It mirrors SearchFrom for reverse repeat-search.
func (b *ViewerBackend) SearchBefore(ctx context.Context, pattern string, off int64) (int64, bool) {
	if b.owner == nil || pattern == "" || !b.owner.GetCapabilities().HasSearch {
		return 0, false
	}
	matches, err := b.owner.Search(ctx, b.path, pattern)
	if err != nil || matches == nil {
		return 0, false
	}
	last := int64(-1)
	for at := range matches {
		logicalAt := at - b.dataOffset
		if logicalAt < off && logicalAt > last {
			last = logicalAt
		}
	}
	return last, true
}

// LineStart reports the byte offset where the given one-based line begins.
// A file system that can index lines answers it without moving the file; one
// that cannot is scanned here, which for a remote file means downloading it,
// so the caller runs this in a background task and lets the user cancel.
//
// The scan reads through the file handle rather than through ReadAt on
// purpose: the cache is a window meant for what is on screen, and pushing a
// whole file through it would evict exactly what the viewer is drawing.
func (b *ViewerBackend) LineStart(ctx context.Context, line int64) (int64, bool) {
	if line <= 1 {
		return 0, true
	}
	if b.indexer != nil {
		idx, err := b.indexer.LineIndex(ctx, b.path, line, 1)
		if err == nil {
			if len(idx.Offsets) > 0 {
				start := idx.Offsets[0] - b.dataOffset
				if start < 0 {
					start = 0
				}
				return start, true
			}
			// The far side counted the file and the line is not in it.
			if idx.Total >= 0 && line > idx.Total {
				return 0, false
			}
		}
	}

	size := b.Size()
	buf := make([]byte, 64*1024)
	remaining := line - 1
	var off int64
	for off < size {
		if ctx.Err() != nil {
			return 0, false
		}
		n, err := b.file.ReadAt(ctx, buf, b.dataOffset+off)
		for i := 0; i < n; i++ {
			if buf[i] != '\n' {
				continue
			}
			remaining--
			if remaining == 0 {
				start := off + int64(i) + 1
				if start >= size {
					// The newline that ends the file starts no line.
					return 0, false
				}
				return start, true
			}
		}
		off += int64(n)
		if err != nil {
			break
		}
	}
	return 0, false
}

// LineStartFromEnd reports where the last n lines of the file begin, asking
// the file system to do the counting. It answers false whenever that is not
// possible — a local file, a file system without the feature, a remote host
// without the tool for it, or any error at all — and the caller then falls
// back to reading, which is what it did before this existed.
//
// Two round trips at worst: one for the total, one for the offset. The total
// is kept, so paging around a file that is not growing costs one.
func (b *ViewerBackend) LineStartFromEnd(ctx context.Context, n int64) (int64, bool) {
	if b.indexer == nil || n <= 0 {
		return 0, false
	}
	size := b.Size()

	b.mu.Lock()
	total := b.totalLines
	known := total >= 0 && b.totalForSize == size
	b.mu.Unlock()

	if !known {
		idx, err := b.indexer.LineIndex(ctx, b.path, 1, 0)
		if err != nil || idx.Total < 0 {
			return 0, false
		}
		total = idx.Total
		b.mu.Lock()
		b.totalLines = total
		b.totalForSize = size
		b.mu.Unlock()
	}
	if total <= 0 {
		return 0, false
	}

	first := total - n + 1
	if first < 1 {
		first = 1
	}
	idx, err := b.indexer.LineIndex(ctx, b.path, first, 1)
	if err != nil || len(idx.Offsets) == 0 {
		return 0, false
	}
	start := idx.Offsets[0] - b.dataOffset
	if start < 0 {
		start = 0
	}
	return start, true
}
func (b *ViewerBackend) FindLineStart(offset int64) int64 {
	resolved, ready := b.TryFindLineStart(offset)
	if !ready {
		return offset
	}
	return resolved
}

// TryFindLineStart is the non-blocking variant used by semantic GUI seeking.
// A false ready result means the requested cache window is being fetched; the
// caller must retain its last complete viewport and retry on the next redraw.
func (b *ViewerBackend) TryFindLineStart(offset int64) (resolved int64, ready bool) {
	resolved, ready, _ = b.tryFindLineStartBounded(offset, 0)
	return
}

// The native UI may yield a local seek after bounded work even when every
// read is immediately ready. Console callers retain the synchronous adapter.
func (b *ViewerBackend) tryFindLineStartBounded(offset int64, maxBytes int) (resolved int64, ready, yielded bool) {
	if offset <= 0 {
		b.mu.Lock()
		b.lineSeekActive = false
		b.lineSeekCurr = 0
		b.mu.Unlock()
		return 0, true, false
	}

	b.mu.Lock()
	if !b.lineSeekActive || b.lineSeekTarget != offset {
		b.lineSeekTarget = offset
		b.lineSeekCurr = offset
		b.lineSeekActive = true
	}
	curr := b.lineSeekCurr
	b.mu.Unlock()

	chunkSize := int64(4096)
	processed := 0
	for curr > 0 {
		start := curr - chunkSize
		if start < 0 {
			start = 0
		}

		data, err := b.ReadAt(start, int(curr-start))
		if err == piecetable.ErrLoading {
			b.mu.Lock()
			if b.lineSeekActive && b.lineSeekTarget == offset {
				b.lineSeekCurr = curr
			}
			b.mu.Unlock()
			return offset, false, false
		}
		if err != nil {
			b.finishLineStartSeek(offset)
			return offset, false, false
		}

		for i := len(data) - 1; i >= 0; i-- {
			if data[i] == '\n' {
				b.finishLineStartSeek(offset)
				return start + int64(i) + 1, true, false
			}
		}
		processed += len(data)
		curr = start
		if curr > 0 && maxBytes > 0 && processed >= maxBytes {
			b.mu.Lock()
			if b.lineSeekActive && b.lineSeekTarget == offset {
				b.lineSeekCurr = curr
			}
			b.mu.Unlock()
			return offset, false, true
		}
	}
	b.finishLineStartSeek(offset)
	return 0, true, false
}

func (b *ViewerBackend) finishLineStartSeek(offset int64) {
	b.mu.Lock()
	if b.lineSeekActive && b.lineSeekTarget == offset {
		b.lineSeekActive = false
		b.lineSeekCurr = 0
	}
	b.mu.Unlock()
}
