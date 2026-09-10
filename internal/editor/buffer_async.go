package editor

import (
	"context"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"sync"
)

// AsyncBuffer retains the editor's original-source chunks. Missing reads use a
// single sequential loader, not a goroutine per chunk. The indexer can consume
// the same source directly with ReadContext without polling the UI task queue.
type AsyncBuffer struct {
	File       vfs.ReadAtCloser
	size       int
	fileOffset int64
	ctx        context.Context
	cancelCtx  context.CancelFunc

	mu           sync.Mutex
	loaded       map[int][]byte
	fetching     map[int]bool
	ChunkSize    int
	worker       bool
	readToken    chan struct{}
	errors       map[int]error
	redrawQueued bool
}

func NewAsyncBuffer(ctx context.Context, f vfs.ReadAtCloser) *AsyncBuffer {
	return NewAsyncBufferWithOffset(ctx, f, 0)
}

// NewAsyncBufferWithOffset presents the file from fileOffset onward. This is
// used for text files whose on-disk UTF-8 BOM is metadata rather than editor
// content; all buffer offsets remain logical offsets into the displayed text.
func NewAsyncBufferWithOffset(ctx context.Context, f vfs.ReadAtCloser, fileOffset int64) *AsyncBuffer {
	if fileOffset < 0 {
		fileOffset = 0
	}
	rawSize := int64(0)
	if f != nil {
		rawSize = f.Size()
	}
	size := rawSize - fileOffset
	if size < 0 {
		size = 0
	}
	bCtx, bCancel := context.WithCancel(ctx)
	return &AsyncBuffer{
		File:       f,
		size:       int(size),
		fileOffset: fileOffset,
		ctx:        bCtx,
		cancelCtx:  bCancel,
		loaded:     make(map[int][]byte),
		fetching:   make(map[int]bool),
		ChunkSize:  32 * 1024, // 32 KB chunks
	}
}

func (b *AsyncBuffer) Close() {
	if b.cancelCtx != nil {
		b.cancelCtx()
	}
}

func (b *AsyncBuffer) Size() int { return b.size }

// seedPrefix transfers the already-read encoding probe into the original
// source buffer. A partial first chunk remains readable without a second read.
func (b *AsyncBuffer) SeedPrefix(prefix []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for off := 0; off < len(prefix); off += b.ChunkSize {
		b.loaded[off/b.ChunkSize] = prefix[off:min(off+b.ChunkSize, len(prefix))]
	}
}

func (b *AsyncBuffer) Prewarm() {
	_, _ = b.ReadContext(b.ctx, 0, min(b.ChunkSize, b.size))
}

func (b *AsyncBuffer) Read(offset, length int) ([]byte, error) {
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	if offset < 0 || offset >= b.size || length <= 0 {
		return nil, nil
	}
	length = min(length, b.size-offset)
	b.mu.Lock()
	defer b.mu.Unlock()
	missing := false
	var failure error
	for i := offset / b.ChunkSize; i <= (offset+length-1)/b.ChunkSize; i++ {
		end := min(offset+length-i*b.ChunkSize, b.ChunkSize)
		if len(b.loaded[i]) >= end {
			continue
		}
		if err := b.errors[i]; err != nil {
			failure = err
			delete(b.errors, i) // A subsequent explicit request may retry.
			continue
		}
		missing = true
		b.fetching[i] = true
	}
	if missing && !b.worker {
		b.worker = true
		var post func(func() bool)
		if frames := vtui.FrameManager; frames != nil {
			post = frames.PostTaskWithRedrawDecision
		}
		go b.fetchPending(post)
	}
	if failure != nil {
		return nil, failure
	}
	if missing {
		return nil, piecetable.ErrLoading
	}
	return b.copyRangeLocked(offset, length), nil
}

func (b *AsyncBuffer) copyRangeLocked(offset, length int) []byte {
	if offset/b.ChunkSize == (offset+length-1)/b.ChunkSize {
		start := offset % b.ChunkSize
		return b.loaded[offset/b.ChunkSize][start : start+length]
	}
	result := make([]byte, 0, length)
	for at := offset; at < offset+length; {
		data := b.loaded[at/b.ChunkSize]
		start := at % b.ChunkSize
		n := min(len(data)-start, offset+length-at)
		result = append(result, data[start:start+n]...)
		at += n
	}
	return result
}

// ReadContext waits for real bytes, never a timer or a UI redraw. Reads are
// bounded by the existing chunk size and serialized with interactive misses.
func (b *AsyncBuffer) ReadContext(ctx context.Context, offset, length int) ([]byte, error) {
	if offset < 0 || offset >= b.size || length <= 0 {
		return nil, nil
	}
	length = min(length, b.size-offset)
	for i := offset / b.ChunkSize; i <= (offset+length-1)/b.ChunkSize; i++ {
		if err := b.loadChunk(ctx, i); err != nil {
			return nil, err
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.copyRangeLocked(offset, length), nil
}

func (b *AsyncBuffer) loadChunk(ctx context.Context, idx int) error {
	b.mu.Lock()
	if b.readToken == nil {
		b.readToken = make(chan struct{}, 1)
		b.readToken <- struct{}{}
	}
	token := b.readToken
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.ctx.Done():
		return b.ctx.Err()
	case <-token:
	}
	defer func() { token <- struct{}{} }()
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sz := min(b.ChunkSize, b.size-idx*b.ChunkSize)
	b.mu.Lock()
	prefix := b.loaded[idx]
	b.mu.Unlock()
	if len(prefix) >= sz {
		return nil
	}
	data := make([]byte, sz)
	copy(data, prefix)
	readCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(b.ctx, cancel)
	_, err := fileops.ReadDocumentBytes(readCtx, b.File, data[len(prefix):], b.fileOffset+int64(idx*b.ChunkSize+len(prefix)))
	stop()
	cancel()
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ctx.Err(); err != nil {
		return err
	}
	b.loaded[idx] = data
	delete(b.errors, idx)
	return nil
}

func (b *AsyncBuffer) fetchPending(post func(func() bool)) {
	for {
		b.mu.Lock()
		idx := -1
		for pending := range b.fetching {
			if idx < 0 || pending < idx {
				idx = pending
			}
		}
		if idx < 0 || b.ctx.Err() != nil {
			clear(b.fetching)
			b.worker = false
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
		err := b.loadChunk(b.ctx, idx)
		b.mu.Lock()
		delete(b.fetching, idx)
		if err != nil && b.ctx.Err() == nil {
			if b.errors == nil {
				b.errors = make(map[int]error)
			}
			b.errors[idx] = err
		}
		queueRedraw := b.ctx.Err() == nil && post != nil && !b.redrawQueued
		if queueRedraw {
			b.redrawQueued = true
		}
		b.mu.Unlock()
		if queueRedraw {
			post(func() bool {
				b.mu.Lock()
				b.redrawQueued = false
				b.mu.Unlock()
				return b.ctx.Err() == nil
			})
		}
	}
}

// readDocumentBytes respects short reads and errors without manufacturing zero
// bytes. Callers bound dst to their source window or prepared document size.
