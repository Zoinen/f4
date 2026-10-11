package observer

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// growingFile is a vfs.ReadAtCloser over a file that a running extraction is
// still writing. ExtractItem cannot hand the bytes out any other way (API v6
// has no item stream), but it does not have to be finished before the first
// of them is used: a reader that needs bytes [off, off+n) waits only until
// the file has grown that far, so a nested archive can start reading the
// member while the module is still producing it, instead of after (f4#1678).
//
// The file is created by the module, so it is opened on the first read.
// finish reports the extraction's end; a failed extraction fails the reads
// that still wait for bytes it never wrote. Close removes the file, at once
// or, when the extraction is still running, as soon as it ends.
type growingFile struct {
	path string
	hint int64 // the item's size from the listing, until the real one is known

	mu       sync.Mutex
	f        *os.File
	done     bool
	err      error
	closed   bool
	finished chan struct{}
	pos      int64
}

// growingFilePoll is how often a reader that is waiting checks the file.
var growingFilePoll = 2 * time.Millisecond

func newGrowingFile(path string, hint int64) *growingFile {
	return &growingFile{path: path, hint: hint, finished: make(chan struct{})}
}

// finish records how the extraction ended and wakes the readers.
func (g *growingFile) finish(err error) {
	g.mu.Lock()
	if g.done {
		g.mu.Unlock()
		return
	}
	g.done, g.err = true, err
	closed := g.closed
	close(g.finished)
	g.mu.Unlock()
	if closed {
		_ = os.Remove(g.path)
	}
}

// Size is the item's size from the listing until the extraction has ended,
// and the size of the file it wrote after.
func (g *growingFile) Size() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.done && g.err == nil {
		if fi, err := os.Stat(g.path); err == nil {
			return fi.Size()
		}
	}
	return g.hint
}

func (g *growingFile) openLocked() error {
	if g.f != nil {
		return nil
	}
	f, err := os.Open(g.path) //nolint:gosec // G304: the path is this instance's own private temp directory plus a name the caller chose.
	if err != nil {
		return err
	}
	g.f = f
	return nil
}

// ReadAt waits until the file holds p's worth of bytes at off, or the
// extraction has ended, then reads. A read that reaches the end of a finished
// file returns what is there with io.EOF.
func (g *growingFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("observer: negative read offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()
			return 0, os.ErrClosed
		}
		done, extractErr := g.done, g.err
		var have int64 = -1
		if err := g.openLocked(); err == nil {
			if fi, err := g.f.Stat(); err == nil {
				have = fi.Size()
			}
		} else if done && !errors.Is(err, os.ErrNotExist) {
			g.mu.Unlock()
			return 0, err
		}
		if have >= off+int64(len(p)) || done {
			f := g.f
			g.mu.Unlock()
			if f == nil {
				if extractErr != nil {
					return 0, extractErr
				}
				return 0, io.EOF
			}
			n, err := f.ReadAt(p, off)
			if extractErr != nil && (err == io.EOF || err == io.ErrUnexpectedEOF) {
				// The extraction failed before it wrote this far.
				return n, extractErr
			}
			return n, err
		}
		g.mu.Unlock()

		select {
		case <-g.finished:
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(growingFilePoll):
		}
	}
}

func (g *growingFile) Read(ctx context.Context, p []byte) (int, error) {
	g.mu.Lock()
	off := g.pos
	g.mu.Unlock()
	n, err := g.ReadAt(ctx, p, off)
	g.mu.Lock()
	g.pos += int64(n)
	g.mu.Unlock()
	if n > 0 && err == io.EOF {
		err = nil
	}
	return n, err
}

// Close releases the file and removes it, now or when the extraction ends.
func (g *growingFile) Close() error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return nil
	}
	g.closed = true
	done := g.done
	f := g.f
	g.f = nil
	g.mu.Unlock()

	var err error
	if f != nil {
		err = f.Close()
	}
	if done {
		if rerr := os.Remove(g.path); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
	}
	return err
}
