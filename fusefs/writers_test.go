package fusefs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/unxed/f4/vfs"
)

func newWriterTestBridge() *bridge {
	return &bridge{writers: make(map[string]*writeHandle)}
}

func TestWriterTableSharesOneHandlePerPath(t *testing.T) {
	b := newWriterTestBridge()

	first, created := b.acquireWriter("/a.txt")
	if !created {
		t.Fatal("the first writer of a file has to be told it is the first")
	}
	second, created := b.acquireWriter("/a.txt")
	if created {
		t.Fatal("a second writer of the same file must not be told to stage its own copy")
	}
	if first != second {
		t.Fatal("two writers of one file got two handles; the last close would silently win")
	}
	if got := b.openWriters(); got != 1 {
		t.Fatalf("openWriters = %d, want 1", got)
	}

	if last := b.releaseWriter(second); last {
		t.Fatal("committing while another writer is still open would truncate its work")
	}
	if b.writerFor("/a.txt") == nil {
		t.Fatal("the handle disappeared while a writer still held it")
	}
	if last := b.releaseWriter(first); !last {
		t.Fatal("the last release has to report itself, or the file is never committed")
	}
	if b.writerFor("/a.txt") != nil || b.openWriters() != 0 {
		t.Fatal("the handle outlived its last writer")
	}
}

func TestWriterTableKeepsPathsApart(t *testing.T) {
	b := newWriterTestBridge()
	a, _ := b.acquireWriter("/a.txt")
	c, _ := b.acquireWriter("/b.txt")
	if a == c {
		t.Fatal("two different files share one handle")
	}
	if b.openWriters() != 2 {
		t.Fatalf("openWriters = %d, want 2", b.openWriters())
	}
	b.releaseWriter(a)
	if b.writerFor("/b.txt") == nil {
		t.Fatal("closing one file took the other one down with it")
	}
}

func TestWriterTableUnderConcurrentOpens(t *testing.T) {
	b := newWriterTestBridge()
	const n = 64
	var wg sync.WaitGroup
	handles := make([]*writeHandle, n)
	creations := make([]bool, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			handles[i], creations[i] = b.acquireWriter("/busy.bin")
		}(i)
	}
	wg.Wait()

	created := 0
	for i := range creations {
		if creations[i] {
			created++
		}
		if handles[i] != handles[0] {
			t.Fatal("concurrent opens produced more than one handle for one file")
		}
	}
	if created != 1 {
		t.Fatalf("%d openers were told to stage a copy, want exactly 1", created)
	}

	for i := 0; i < n; i++ {
		last := b.releaseWriter(handles[i])
		if last != (i == n-1) {
			t.Fatalf("release %d reported last=%v", i, last)
		}
	}
	if b.openWriters() != 0 {
		t.Fatal("the table did not empty")
	}
}

type lifecycleVFS struct {
	*fakeVFS
	mu        sync.Mutex
	data      []byte
	handles   []*lifecycleWriter
	closeErrs []error
}

func newLifecycleVFS(closeErrs ...error) *lifecycleVFS {
	return &lifecycleVFS{fakeVFS: newFakeVFS(true), closeErrs: closeErrs}
}

func (v *lifecycleVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasWrite: true}
}

func (v *lifecycleVFS) OpenWriteAt(ctx context.Context, path string) (vfs.WriterAtCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	w := &lifecycleWriter{owner: v}
	if len(v.closeErrs) > len(v.handles) {
		w.closeErr = v.closeErrs[len(v.handles)]
	}
	v.handles = append(v.handles, w)
	return w, nil
}

type lifecycleWriter struct {
	owner    *lifecycleVFS
	closed   bool
	closes   int
	closeErr error
}

func (w *lifecycleWriter) WriteAt(p []byte, off int64) (int, error) {
	w.owner.mu.Lock()
	defer w.owner.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	end := int(off) + len(p)
	if end > len(w.owner.data) {
		w.owner.data = append(w.owner.data, make([]byte, end-len(w.owner.data))...)
	}
	return copy(w.owner.data[int(off):], p), nil
}

func (w *lifecycleWriter) Truncate(size int64) error {
	w.owner.mu.Lock()
	defer w.owner.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	if size < int64(len(w.owner.data)) {
		w.owner.data = w.owner.data[:size]
	} else {
		w.owner.data = append(w.owner.data, make([]byte, int(size)-len(w.owner.data))...)
	}
	return nil
}

func (w *lifecycleWriter) Close() error {
	w.owner.mu.Lock()
	defer w.owner.mu.Unlock()
	w.closes++
	w.closed = true
	return w.closeErr
}

type stagedLifecycleVFS struct {
	*fakeVFS
	mu      sync.Mutex
	commits []string
}

func newStagedLifecycleVFS() *stagedLifecycleVFS {
	return &stagedLifecycleVFS{fakeVFS: newFakeVFS(true)}
}

func (v *stagedLifecycleVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasWrite: true}
}

func (v *stagedLifecycleVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &stagedLifecycleWriter{owner: v}, nil
}

type stagedLifecycleWriter struct {
	bytes.Buffer
	owner *stagedLifecycleVFS
}

func (w *stagedLifecycleWriter) Close() error {
	w.owner.mu.Lock()
	defer w.owner.mu.Unlock()
	w.owner.commits = append(w.owner.commits, w.String())
	return nil
}

func TestWriteHandleMultipleFlushAndWriteAfterFlush(t *testing.T) {
	ctx := context.Background()
	v := newLifecycleVFS()
	b := newBridge(v, "/root", Options{})
	t.Cleanup(b.close)

	wh, _, err := b.acquireWriteHandle(ctx, "/root/out.txt")
	if err != nil {
		t.Fatalf("acquire writer: %v", err)
	}
	if _, err := wh.writeAt(ctx, b, []byte("a"), 0); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := b.flushWriter(ctx, wh); err != nil {
		t.Fatalf("first Flush: %v", err)
	}
	if err := b.flushWriter(ctx, wh); err != nil {
		t.Fatalf("second Flush: %v", err)
	}
	if len(v.handles) != 1 {
		t.Fatalf("two Flush calls used %d handles, want one", len(v.handles))
	}
	if v.handles[0].closes != 1 {
		t.Fatalf("two Flush calls closed the handle %d times, want once", v.handles[0].closes)
	}

	if _, err := wh.writeAt(ctx, b, []byte("b"), 1); err != nil {
		t.Fatalf("write after Flush: %v", err)
	}
	if len(v.handles) != 2 {
		t.Fatalf("write after Flush opened %d handles, want 2", len(v.handles))
	}
	if err := b.flushWriter(ctx, wh); err != nil {
		t.Fatalf("Flush after another write: %v", err)
	}
	if err := b.finishWriter(ctx, wh); err != nil {
		t.Fatalf("Release after Flush: %v", err)
	}
	if got := string(v.data); got != "ab" {
		t.Fatalf("backend data = %q, want %q", got, "ab")
	}
	if v.handles[1].closes != 1 {
		t.Fatalf("reopened handle closed %d times, want once", v.handles[1].closes)
	}
}

func TestWriteHandleFlushErrorReturnedOnce(t *testing.T) {
	ctx := context.Background()
	v := newLifecycleVFS(os.ErrPermission)
	b := newBridge(v, "/root", Options{})
	t.Cleanup(b.close)

	wh, _, err := b.acquireWriteHandle(ctx, "/root/out.txt")
	if err != nil {
		t.Fatalf("acquire writer: %v", err)
	}
	if err := b.flushWriter(ctx, wh); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("first Flush = %v, want os.ErrPermission", err)
	}
	if err := b.flushWriter(ctx, wh); err != nil {
		t.Fatalf("second Flush = %v, want success", err)
	}
	if err := b.finishWriter(ctx, wh); err != nil {
		t.Fatalf("Release after failed Flush = %v", err)
	}
	if v.handles[0].closes != 1 {
		t.Fatalf("failed close was retried %d times, want once", v.handles[0].closes)
	}
}

func TestWriteHandleReleaseWithoutFlushClosesWriter(t *testing.T) {
	ctx := context.Background()
	v := newLifecycleVFS()
	b := newBridge(v, "/root", Options{})
	t.Cleanup(b.close)

	wh, _, err := b.acquireWriteHandle(ctx, "/root/out.txt")
	if err != nil {
		t.Fatalf("acquire writer: %v", err)
	}
	if _, err := wh.writeAt(ctx, b, []byte("saved"), 0); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := b.finishWriter(ctx, wh); err != nil {
		t.Fatalf("Release without Flush: %v", err)
	}
	if len(v.handles) != 1 {
		t.Fatalf("Release used %d handles, want one", len(v.handles))
	}
	if v.handles[0].closes != 1 {
		t.Fatalf("Release closed the handle %d times, want once", v.handles[0].closes)
	}
}

func TestStagedWriteAfterFlushIsCommittedAgain(t *testing.T) {
	ctx := context.Background()
	v := newStagedLifecycleVFS()
	b := newBridge(v, "/root", Options{})
	t.Cleanup(b.close)

	wh, _, err := b.acquireWriteHandle(ctx, "/root/out.txt")
	if err != nil {
		t.Fatalf("acquire writer: %v", err)
	}
	if _, err := wh.writeAt(ctx, b, []byte("a"), 0); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := b.flushWriter(ctx, wh); err != nil {
		t.Fatalf("first Flush: %v", err)
	}
	if err := b.flushWriter(ctx, wh); err != nil {
		t.Fatalf("second Flush: %v", err)
	}
	if _, err := wh.writeAt(ctx, b, []byte("b"), 1); err != nil {
		t.Fatalf("write after Flush: %v", err)
	}
	if err := b.flushWriter(ctx, wh); err != nil {
		t.Fatalf("Flush after another write: %v", err)
	}
	if err := b.finishWriter(ctx, wh); err != nil {
		t.Fatalf("Release after Flush: %v", err)
	}
	if len(v.commits) != 2 || v.commits[0] != "a" || v.commits[1] != "ab" {
		t.Fatalf("commits = %q, want [a ab]", v.commits)
	}
}

func TestStagedReleaseWithoutFlushCommitsAndCloses(t *testing.T) {
	ctx := context.Background()
	v := newStagedLifecycleVFS()
	b := newBridge(v, "/root", Options{})
	t.Cleanup(b.close)

	wh, _, err := b.acquireWriteHandle(ctx, "/root/out.txt")
	if err != nil {
		t.Fatalf("acquire writer: %v", err)
	}
	if _, err := wh.writeAt(ctx, b, []byte("saved"), 0); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := b.finishWriter(ctx, wh); err != nil {
		t.Fatalf("Release without Flush: %v", err)
	}
	if len(v.commits) != 1 || v.commits[0] != "saved" {
		t.Fatalf("commits = %q, want [saved]", v.commits)
	}
	if _, err := wh.staged.Size(); err == nil {
		t.Fatal("staging file remained usable after Release")
	}
}
