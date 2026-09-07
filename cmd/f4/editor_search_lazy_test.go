package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// pumpUITasks runs queued UI tasks until the returned stop func is called.
// Chunk fetches, indexer batches and redraws all post work to the UI thread,
// and a queue nobody drains eventually blocks the goroutines posting to it.
func pumpUITasks() (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			wg.Wait()
		})
	}
}

// lazyEditorBuffer writes a file spanning several AsyncBuffer chunks and wraps
// it the way showEditor does for UTF-8, but without prewarming: that is the
// state a freshly opened large file is in while the background indexer is
// still walking it.
func lazyEditorBuffer(t *testing.T, content string) (*piecetable.PieceTable, *AsyncBuffer) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	v := vfs.NewOSVFS(dir)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	buf := NewAsyncBuffer(context.Background(), f)
	t.Cleanup(buf.Close)
	return piecetable.NewWithBuffer(buf), buf
}

// stalledFile is a backing file whose reads never return, standing in for a
// remote file system that has gone quiet. It is what keeps a buffer parked on
// ErrLoading for as long as a test needs: a chunk fetch that can complete on
// its own would race whatever the test is trying to observe.
type stalledFile struct {
	size    int64
	release chan struct{}
}

func (s *stalledFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	select {
	case <-s.release:
		return 0, io.EOF
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *stalledFile) Read(ctx context.Context, p []byte) (int, error) {
	return s.ReadAt(ctx, p, 0)
}

func (s *stalledFile) Close() error { return nil }
func (s *stalledFile) Size() int64  { return s.size }

// stalledEditorBuffer wraps content in a buffer that reports the right size
// but never manages to load any of it.
func stalledEditorBuffer(t *testing.T, content string) (*piecetable.PieceTable, *AsyncBuffer) {
	t.Helper()

	f := &stalledFile{size: int64(len(content)), release: make(chan struct{})}
	t.Cleanup(func() { close(f.release) })

	buf := NewAsyncBuffer(context.Background(), f)
	t.Cleanup(buf.Close)
	return piecetable.NewWithBuffer(buf), buf
}

// bigSearchCorpus is several AsyncBuffer chunks of filler with one needle
// parked well past the first one, so a search that only sees resident data
// cannot find it.
func bigSearchCorpus() (content, needle string, needleOff int) {
	needle = "NEEDLE-IN-THE-TAIL"
	head := strings.Repeat("aaa bbbbb\n", 70000) // 700 KB, chunks 0..2
	tail := strings.Repeat("aaa bbbbb\n", 30000)
	return head + needle + "\n" + tail, needle, len(head)
}

// TestReadSearchSnapshot_WaitsForLazilyLoadedChunks is the regression test for
// searching a large file straight after opening it: pt.Bytes() gave up on the
// first unfetched chunk and the editor reported "Failed to read file buffer".
func TestReadSearchSnapshot_WaitsForLazilyLoadedChunks(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content, needle, needleOff := bigSearchCorpus()
	pt, _ := lazyEditorBuffer(t, content)

	// The bug as it was: the whole-buffer read fails outright, because only a
	// fraction of the file is resident.
	if _, err := pt.Bytes(); err != piecetable.ErrLoading {
		t.Fatalf("precondition: want a not-yet-loaded buffer (ErrLoading), got %v", err)
	}

	stop := pumpUITasks()
	defer stop()

	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &vtui.TaskContext{Context: c, Cancel: cancel}

	data, err := readSearchSnapshot(ctx, pt)
	if err != nil {
		t.Fatalf("readSearchSnapshot: %v", err)
	}
	if len(data) != len(content) {
		t.Fatalf("snapshot length = %d, want %d", len(data), len(content))
	}
	if !bytes.Equal(data, []byte(content)) {
		t.Fatal("snapshot does not match file content")
	}

	// The point of the fix: the search now reaches text that was never
	// resident when it started.
	if got := bytes.Index(data, []byte(needle)); got != needleOff {
		t.Fatalf("needle at %d, want %d", got, needleOff)
	}
}

// TestFindAllMatchSpans_OverLazyBuffer covers the reported path end to end:
// Find All over a freshly opened large file collects the occurrences that live
// beyond the prewarmed chunk instead of erroring out.
func TestFindAllMatchSpans_OverLazyBuffer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content, needle, needleOff := bigSearchCorpus()
	pt, _ := lazyEditorBuffer(t, content)

	stop := pumpUITasks()
	defer stop()

	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &vtui.TaskContext{Context: c, Cancel: cancel}

	data, err := readSearchSnapshot(ctx, pt)
	if err != nil {
		t.Fatalf("readSearchSnapshot: %v", err)
	}

	spans, err := findAllMatchSpans(c, data, needle, true, false, false)
	if err != nil {
		t.Fatalf("findAllMatchSpans: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(spans))
	}
	if spans[0].Off != needleOff || spans[0].Len != len(needle) {
		t.Fatalf("span = {%d,%d}, want {%d,%d}", spans[0].Off, spans[0].Len, needleOff, len(needle))
	}

	// findMatch drives the plain Search path over the same snapshot.
	off, mLen, err := findMatch(data, needle, true, false, false, false, false, 0)
	if err != nil {
		t.Fatalf("findMatch: %v", err)
	}
	if off != needleOff || mLen != len(needle) {
		t.Fatalf("findMatch = (%d,%d), want (%d,%d)", off, mLen, needleOff, len(needle))
	}
}

// TestReadSearchSnapshot_StopsOnCancel makes sure cancelling the progress
// dialog drops the assembler promptly instead of reading on to the end of a
// huge file.
func TestReadSearchSnapshot_StopsOnCancel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	content, _, _ := bigSearchCorpus()
	pt, _ := stalledEditorBuffer(t, content)

	// The backing read never returns, so the assembler is parked on
	// ErrLoading when the cancel arrives.
	c, cancel := context.WithCancel(context.Background())
	ctx := &vtui.TaskContext{Context: c, Cancel: cancel}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	if _, err := readSearchSnapshot(ctx, pt); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancel took %v, want prompt return", elapsed)
	}
}

// TestReadSearchSnapshot_FullyResidentBuffer keeps the in-memory case honest:
// an ordinary small file still comes back in one piece.
func TestReadSearchSnapshot_FullyResidentBuffer(t *testing.T) {
	pt := piecetable.New([]byte("hello\nworld\n"))

	data, err := readSearchSnapshot(nil, pt)
	if err != nil {
		t.Fatalf("readSearchSnapshot: %v", err)
	}
	if string(data) != "hello\nworld\n" {
		t.Fatalf("got %q", string(data))
	}

	// An edited buffer mixes Original and Add pieces; the assembler walks
	// logical offsets, so the splice has to come back in order.
	pt.Insert(5, []byte(" there"))
	data, err = readSearchSnapshot(nil, pt)
	if err != nil {
		t.Fatalf("readSearchSnapshot after insert: %v", err)
	}
	if string(data) != "hello there\nworld\n" {
		t.Fatalf("got %q", string(data))
	}
}

// TestReadSearchSnapshot_EmptyBuffer covers the new-file case.
func TestReadSearchSnapshot_EmptyBuffer(t *testing.T) {
	data, err := readSearchSnapshot(nil, piecetable.New(nil))
	if err != nil {
		t.Fatalf("readSearchSnapshot: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("got %d bytes, want 0", len(data))
	}
}
