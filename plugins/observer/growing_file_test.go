package observer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A reader that asks for bytes the extraction has not written yet waits for
// them and gets them, without waiting for the whole file (f4#1678).
func TestGrowingFileReadsWhileTheFileIsWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item-1")
	want := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KiB
	g := newGrowingFile(path, int64(len(want)))
	t.Cleanup(func() { _ = g.Close() })

	release := make(chan struct{})
	go func() {
		f, err := os.Create(path) //nolint:gosec // G304: test path.
		if err != nil {
			g.finish(err)
			return
		}
		half := len(want) / 2
		_, _ = f.Write(want[:half])
		<-release // the second half is held back until the test has read the first
		_, _ = f.Write(want[half:])
		_ = f.Close()
		g.finish(nil)
	}()

	ctx := context.Background()
	first := make([]byte, 1000)
	if n, err := g.ReadAt(ctx, first, 100); n != 1000 || err != nil || !bytes.Equal(first, want[100:1100]) {
		t.Fatalf("read of the written half = %d, %v", n, err)
	}
	if g.Size() != int64(len(want)) {
		t.Fatalf("Size before the end = %d, want the listing's %d", g.Size(), len(want))
	}

	// A read into the held-back half blocks until it is written.
	got := make(chan []byte, 1)
	go func() {
		p := make([]byte, 500)
		n, _ := g.ReadAt(ctx, p, int64(len(want))-500)
		got <- p[:n]
	}()
	select {
	case <-got:
		t.Fatal("a read past the written part returned before the bytes existed")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case p := <-got:
		if !bytes.Equal(p, want[len(want)-500:]) {
			t.Fatal("waited read returned wrong bytes")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting read never finished")
	}

	// Sequential Read walks the file and ends with EOF.
	all, err := io.ReadAll(ctxReader{g, ctx})
	if err != nil || !bytes.Equal(all, want) {
		t.Fatalf("sequential read = %d bytes, %v", len(all), err)
	}
}

type ctxReader struct {
	g   *growingFile
	ctx context.Context
}

func (r ctxReader) Read(p []byte) (int, error) { return r.g.Read(r.ctx, p) }

func TestGrowingFileFailedExtractionFailsWaitingReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item-2")
	g := newGrowingFile(path, 1000)
	t.Cleanup(func() { _ = g.Close() })
	boom := errors.New("module failed")
	go func() {
		_ = os.WriteFile(path, []byte("only ten b"), 0o600)
		g.finish(boom)
		g.finish(nil) // a second finish changes nothing
	}()

	p := make([]byte, 100)
	if _, err := g.ReadAt(context.Background(), p, 0); !errors.Is(err, boom) {
		t.Fatalf("read past what the failed extraction wrote = %v, want its error", err)
	}
	// What was written is still readable.
	q := make([]byte, 5)
	if n, err := g.ReadAt(context.Background(), q, 0); n != 5 || err != nil || string(q) != "only " {
		t.Fatalf("read of written bytes = %d, %v", n, err)
	}
	if g.Size() != 1000 {
		t.Fatalf("Size after a failure = %d, want the listing's 1000", g.Size())
	}
}

func TestGrowingFileFinishedWithoutAFileAndCancel(t *testing.T) {
	dir := t.TempDir()
	g := newGrowingFile(filepath.Join(dir, "never"), 10)
	g.finish(nil)
	if n, err := g.ReadAt(context.Background(), make([]byte, 4), 0); n != 0 || err != io.EOF {
		t.Fatalf("read of an item that wrote nothing = %d, %v", n, err)
	}
	if n, err := g.ReadAt(context.Background(), nil, 0); n != 0 || err != nil {
		t.Fatalf("empty read = %d, %v", n, err)
	}
	if _, err := g.ReadAt(context.Background(), make([]byte, 1), -1); err == nil {
		t.Fatal("negative offset accepted")
	}

	waiting := newGrowingFile(filepath.Join(dir, "later"), 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := waiting.ReadAt(ctx, make([]byte, 4), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v", err)
	}
	_ = waiting.Close()
	if _, err := waiting.ReadAt(context.Background(), make([]byte, 4), 0); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("read after Close = %v, want ErrClosed", err)
	}
}

// Close during a running extraction removes the file when the extraction
// ends; Close after it removes it at once.
func TestGrowingFileCloseRemovesTheFile(t *testing.T) {
	dir := t.TempDir()

	running := filepath.Join(dir, "running")
	g := newGrowingFile(running, 4)
	if err := os.WriteFile(running, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ReadAt(context.Background(), make([]byte, 2), 0); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(running); err != nil {
		t.Fatalf("the file of a running extraction was removed on Close: %v", err)
	}
	g.finish(nil)
	if _, err := os.Stat(running); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file survived the end of the extraction: %v", err)
	}

	finished := filepath.Join(dir, "finished")
	h := newGrowingFile(finished, 4)
	if err := os.WriteFile(finished, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.finish(nil)
	if h.Size() != 4 {
		t.Fatalf("Size after the end = %d, want the file's 4", h.Size())
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(finished); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Close after the end left the file: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}
