package viewer

import (
	bytes "bytes"
	context "context"
	errors "errors"
	piecetable "github.com/unxed/f4/internal/piecetable"
	vfs "github.com/unxed/f4/vfs"
	vtui "github.com/unxed/vtui"
	testing "testing"
	time "time"
)

func TestViewerDirectLocalRangeReadyInCallingTurn(t *testing.T) {
	file := &documentRangeReader{data: bytes.Repeat([]byte("abcdef"), 200000), profile: vfs.ReadAccessDirectLocal}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{File: file, size: file.Size(), ctx: ctx, cancelCtx: cancel}
	defer backend.Close()
	got, err := backend.ReadAt(500000, 120)
	if err != nil || !bytes.Equal(got, file.data[500000:500120]) {
		t.Fatalf("direct miss not ready: %v", err)
	}
	if backend.isFetching || len(file.reads) != 1 || file.reads[0].length > 256*1024 {
		t.Fatalf("unbounded/deferred local read: %+v", file.reads)
	}
}

func TestViewerLineSeekDoesNotAcknowledgeFailedDirectRead(t *testing.T) {
	failure := errors.New("failed local source range")
	file := &failingViewerFile{err: failure}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{File: file, size: file.Size(), owner: vfs.NewOSVFS(t.TempDir()), ctx: ctx, cancelCtx: cancel}
	defer backend.Close()
	if _, ready := backend.TryFindLineStart(12); ready {
		t.Fatal("failed line seek was acknowledged as ready")
	}
	if !errors.Is(backend.LastReadError(), failure) {
		t.Fatalf("source error=%v", backend.LastReadError())
	}
}

func TestViewerRangeSupersessionOnlyCommitsNewestRequest(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	file := &documentRangeReader{data: bytes.Repeat([]byte("abcdef"), 600000), started: make(chan struct{}), release: make(chan struct{}), ignoreCancel: true}
	ctx, cancel := context.WithCancel(context.Background())
	backend := &ViewerBackend{File: file, size: file.Size(), ctx: ctx, cancelCtx: cancel}
	defer backend.Close()
	if _, err := backend.ReadAt(0, 32); err != piecetable.ErrLoading {
		t.Fatal(err)
	}
	select {
	case <-file.started:
	case <-time.After(time.Second):
		t.Fatal("first range not started")
	}
	_, _ = backend.ReadAt(1000000, 32)
	_, _ = backend.ReadAt(2000000, 32)
	close(file.release)
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	got, err := backend.ReadContext(readCtx, 2000000, 32)
	if err != nil || !bytes.Equal(got, file.data[2000000:2000032]) {
		t.Fatalf("newest range not ready: %v", err)
	}
	file.mu.Lock()
	defer file.mu.Unlock()
	if file.maxActive != 1 || len(file.reads) != 2 || file.reads[1].offset != 2000000-64*1024 {
		t.Fatalf("obsolete work queued or concurrent: active=%d reads=%+v", file.maxActive, file.reads)
	}
	if backend.cacheOff != 2000000-64*1024 {
		t.Fatalf("stale window committed at %d", backend.cacheOff)
	}
}
