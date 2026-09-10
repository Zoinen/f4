package editor

import (
	"context"
	"github.com/unxed/f4/vfs"
	"io"
	"sync"
	"sync/atomic"
)

type documentRangeReader struct {
	data         []byte
	profile      vfs.ReadAccessProfile
	shortRead    int
	started      chan struct{}
	release      chan struct{}
	ignoreCancel bool
	mu           sync.Mutex
	reads        []trackedReadRange
	active       int
	maxActive    int
	closes       atomic.Int32
}

func (f *documentRangeReader) Size() int64 { return int64(len(f.data)) }

func (f *documentRangeReader) Close() error { f.closes.Add(1); return nil }

func (f *documentRangeReader) ReadAccessProfile() vfs.ReadAccessProfile { return f.profile }

func (f *documentRangeReader) Read(ctx context.Context, dst []byte) (int, error) {
	return f.ReadAt(ctx, dst, 0)
}

func (f *documentRangeReader) ReadAt(ctx context.Context, dst []byte, off int64) (int, error) {
	f.mu.Lock()
	first := len(f.reads) == 0
	f.reads = append(f.reads, trackedReadRange{offset: off, length: len(dst)})
	f.active++
	f.maxActive = max(f.maxActive, f.active)
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
	if first && f.started != nil {
		close(f.started)
		if f.ignoreCancel {
			<-f.release
		} else {
			select {
			case <-f.release:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
	}
	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	if f.shortRead > 0 {
		dst = dst[:min(len(dst), f.shortRead)]
	}
	n := copy(dst, f.data[off:])
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}

type trackedReadRange struct {
	offset int64
	length int
}
