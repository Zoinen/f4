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

	mu         sync.Mutex
	cacheOff   int64
	cacheData  []byte
	isFetching bool
	readErr    error

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
	if b.cancelCtx != nil {
		b.cancelCtx()
	}
	b.mu.Lock()
	b.cacheData = nil
	b.mu.Unlock()
	return b.file.Close()
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
	defer b.mu.Unlock()

	if b.readErr != nil {
		return nil, b.readErr
	}

	// Check cache hit
	if b.cacheData != nil && offset >= b.cacheOff && offset < b.cacheOff+int64(len(b.cacheData)) {
		start := offset - b.cacheOff
		avail := int64(len(b.cacheData)) - start
		cacheReachesEOF := b.cacheOff+int64(len(b.cacheData)) >= b.size
		if int64(length) <= avail || cacheReachesEOF {
			if int64(length) > avail {
				length = int(avail)
			}
			return b.cacheData[start : start+int64(length)], nil
		}
		// A short read at a cache-window boundary is not EOF. Fetch a window
		// covering the request instead; otherwise wrapped-row discovery treats
		// every cache edge as a real line boundary and slowly drifts off course.
	}

	if offset >= b.size {
		return nil, io.EOF
	}

	// Cache miss -> Trigger fetch in background
	if !b.isFetching {
		b.isFetching = true

		fetchOff := offset - 64*1024
		if fetchOff < 0 {
			fetchOff = 0
		}
		fetchLen := 256 * 1024 // 256KB streaming window
		if fetchOff+int64(fetchLen) > b.size {
			fetchLen = int(b.size - fetchOff)
		}

		// Read here, on the goroutine that starts the fetch. The fetch
		// outlives this call and posts its result when it lands, so reading
		// the global from inside it races anything that reassigns
		// vtui.FrameManager while the read is still in flight -- in the tests,
		// the next test to swap one in.
		frames := vtui.FrameManager
		go func() {
			buf := make([]byte, fetchLen)
			n, err := b.file.ReadAt(b.ctx, buf, b.dataOffset+fetchOff)

			var cached []byte
			if n > 0 {
				if b.codepage != 0 && b.codepage != 65001 {
					decoded, decErr := vfs.DecodeBytes(buf[:n], b.codepage)
					if decErr == nil {
						cached = decoded
					} else {
						cached = buf[:n]
					}
				} else {
					cached = buf[:n]
				}
			}

			frames.PostTask(func() {
				b.mu.Lock()
				if b.ctx.Err() == nil {
					if err == nil || err == io.EOF {
						b.cacheOff = fetchOff
						b.cacheData = cached
					} else {
						b.readErr = err
					}
				}
				b.isFetching = false
				b.mu.Unlock()
				frames.Redraw()
			})
		}()
	}
	return nil, piecetable.ErrLoading
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
	if offset <= 0 {
		b.mu.Lock()
		b.lineSeekActive = false
		b.lineSeekCurr = 0
		b.mu.Unlock()
		return 0, true
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
			return offset, false
		}
		if err != nil {
			b.finishLineStartSeek(offset)
			return offset, true
		}

		for i := len(data) - 1; i >= 0; i-- {
			if data[i] == '\n' {
				b.finishLineStartSeek(offset)
				return start + int64(i) + 1, true
			}
		}
		curr = start
	}
	b.finishLineStartSeek(offset)
	return 0, true
}

func (b *ViewerBackend) finishLineStartSeek(offset int64) {
	b.mu.Lock()
	if b.lineSeekActive && b.lineSeekTarget == offset {
		b.lineSeekActive = false
		b.lineSeekCurr = 0
	}
	b.mu.Unlock()
}
