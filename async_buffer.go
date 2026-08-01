package main

import (
	"context"
	"io"
	"sync"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// AsyncBuffer provides non-blocking access to a file, returning ErrLoading
// and triggering background fetches when data is missing.
type AsyncBuffer struct {
	file      vfs.ReadAtCloser
	size      int
	ctx       context.Context
	cancelCtx context.CancelFunc

	mu        sync.Mutex
	loaded    map[int][]byte // Chunk index -> Data
	fetching  map[int]bool   // Chunk index -> is currently being fetched
	chunkSize int
}

func NewAsyncBuffer(ctx context.Context, f vfs.ReadAtCloser) *AsyncBuffer {
	bCtx, bCancel := context.WithCancel(ctx)
	return &AsyncBuffer{
		file:      f,
		size:      int(f.Size()),
		ctx:       bCtx,
		cancelCtx: bCancel,
		loaded:    make(map[int][]byte),
		fetching:  make(map[int]bool),
		chunkSize: 256 * 1024, // 256 KB chunks
	}
}

func (b *AsyncBuffer) Close() {
	b.cancelCtx()
}

func (b *AsyncBuffer) Size() int {
	return b.size
}

// Prewarm synchronously loads the first chunk so the first render
// never sees ErrLoading, avoiding a brief [Loading...] flash.
func (b *AsyncBuffer) prewarm() {
	if b.size == 0 {
		return
	}
	sz := b.chunkSize
	if sz > b.size {
		sz = b.size
	}
	data := make([]byte, sz)
	n, err := b.file.ReadAt(b.ctx, data, 0)
	if b.ctx.Err() != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil || err == io.EOF {
		b.loaded[0] = data[:n]
	}
}

func (b *AsyncBuffer) Read(offset, length int) ([]byte, error) {
	if offset < 0 || offset >= b.size || length <= 0 {
		return nil, nil
	}
	if offset+length > b.size {
		length = b.size - offset
	}

	startChunk := offset / b.chunkSize
	endChunk := (offset + length - 1) / b.chunkSize

	res := make([]byte, 0, length)
	missingData := false

	b.mu.Lock()
	for i := startChunk; i <= endChunk; i++ {
		if data, ok := b.loaded[i]; ok {
			// Chunk is loaded. Extract needed bytes.
			cStart := i * b.chunkSize

			takeStart := offset - cStart
			if takeStart < 0 {
				takeStart = 0
			}

			takeEnd := (offset + length) - cStart
			if takeEnd > len(data) {
				takeEnd = len(data)
			}

			if takeEnd > takeStart {
				res = append(res, data[takeStart:takeEnd]...)
			}
		} else {
			missingData = true
			if !b.fetching[i] {
				b.fetching[i] = true
				go b.fetchChunk(i)
			}
		}
	}
	b.mu.Unlock()

	if missingData {
		return nil, piecetable.ErrLoading
	}
	return res, nil
}

func (b *AsyncBuffer) fetchChunk(idx int) {
	off := int64(idx * b.chunkSize)
	sz := b.chunkSize
	if off+int64(sz) > int64(b.size) {
		sz = int(int64(b.size) - off)
	}

	buf := make([]byte, sz)
	n, err := b.file.ReadAt(b.ctx, buf, off)

	vtui.FrameManager.PostTask(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.fetching, idx)

		if b.ctx.Err() == nil && (err == nil || err == io.EOF) {
			b.loaded[idx] = buf[:n]
			vtui.FrameManager.Redraw()
		} else if err != nil && err != context.Canceled {
			// Report error but allow retry on next UI scroll
			vtui.DebugLog("AsyncBuffer: failed to fetch chunk %d: %v", idx, err)
		}
	})
}
