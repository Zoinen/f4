package main

import (
	"context"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAsyncBuffer_LoadingCycle(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	content := []byte("This is a test file content for async buffer.")
	tmp := filepath.Join(t.TempDir(), "test.txt")
	v := vfs.NewOSVFS(t.TempDir())
	wc, err := v.Create(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wc.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := v.Open(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	// Create buffer with very small chunks (10 bytes) to trigger multi-chunk logic
	buf := NewAsyncBuffer(context.Background(), f)
	buf.chunkSize = 10
	defer buf.Close()

	// 1. Initial read should return ErrLoading
	data, err := buf.Read(0, 5)
	if err != piecetable.ErrLoading {
		t.Errorf("Expected ErrLoading, got %v", err)
	}
	if data != nil {
		t.Error("Data should be nil when loading")
	}

	// 2. Process tasks (wait for the fetch goroutine to post and for us to run the task)
	success := false
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
		}

		// Check if data is now available
		_, err = buf.Read(0, 5)
		if err == nil {
			success = true
			break
		}
		// Small sleep to avoid busy loop
		time.Sleep(5 * time.Millisecond)
	}

	if !success {
		t.Fatalf("Read failed after fetch: %v", err)
	}

	// 3. Verify data content
	data, err = buf.Read(0, 5)
	if err != nil {
		t.Errorf("Read failed after fetch: %v", err)
	}
	if string(data) != "This " {
		t.Errorf("Wrong data: expected 'This ', got %q", string(data))
	}
}

func TestAsyncBuffer_BoundaryRead(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Content: 0123456789ABCDEFGHIJ (20 bytes)
	content := []byte("0123456789ABCDEFGHIJ")
	tmp := filepath.Join(t.TempDir(), "boundary.txt")
	if err := os.WriteFile(tmp, content, 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(t.TempDir())
	f, err := v.Open(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	// Chunk size 10.
	buf := NewAsyncBuffer(context.Background(), f)
	buf.chunkSize = 10
	defer buf.Close()

	// 1. Read spanning across chunk 0 and chunk 1: "89AB"
	// Indices 8, 9 (Chunk 0) and 10, 11 (Chunk 1)
	deadline := time.Now().Add(1 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		var err error
		data, err = buf.Read(8, 4)
		if err == nil {
			break
		}
		if err != piecetable.ErrLoading {
			t.Fatalf("Unexpected error: %v", err)
		}
		// Drain pending tasks
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if data == nil {
		t.Fatal("Timed out waiting for data")
	}
	if string(data) != "89AB" {
		t.Errorf("Boundary read failed: expected '89AB', got %q", string(data))
	}
}

func TestAsyncBuffer_PartialChunkAtEOF(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	content := []byte("Short") // 5 bytes
	tmp := filepath.Join(t.TempDir(), "eof.txt")
	if err := os.WriteFile(tmp, content, 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(t.TempDir())
	f, err := v.Open(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	buf := NewAsyncBuffer(context.Background(), f)
	buf.chunkSize = 100 // Chunk is larger than file
	defer buf.Close()

	deadline := time.Now().Add(1 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		var err error
		data, err = buf.Read(0, 5)
		if err == nil {
			break
		}
		if err != piecetable.ErrLoading {
			t.Fatalf("Unexpected error: %v", err)
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if data == nil {
		t.Fatal("Timed out waiting for data")
	}
	if string(data) != "Short" {
		t.Errorf("EOF chunk failed: expected 'Short', got %q", string(data))
	}
}

func TestAsyncBuffer_WithOffsetHidesFilePrefix(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte("visible text")...)
	f := &vfs.MemoryReadAtCloser{Data: raw}
	buf := NewAsyncBufferWithOffset(context.Background(), f, vfs.UTF8BOMSize)
	defer buf.Close()

	if got, want := buf.Size(), len("visible text"); got != want {
		t.Fatalf("logical size = %d, want %d", got, want)
	}
	buf.prewarm()
	data, err := buf.Read(0, len("visible text"))
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	if string(data) != "visible text" {
		t.Fatalf("logical data = %q, want %q", string(data), "visible text")
	}
}

func TestAsyncBuffer_ConcurrentAccess(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Create a decent sized file
	content := make([]byte, 1024*1024) // 1MB
	for i := range content {
		content[i] = byte(i % 256)
	}

	tmp := filepath.Join(t.TempDir(), "concurrent.bin")
	if err := os.WriteFile(tmp, content, 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(t.TempDir())
	f, err := v.Open(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	buf := NewAsyncBuffer(context.Background(), f)
	buf.chunkSize = 64 * 1024 // 64KB chunks
	defer buf.Close()

	// Spin up a worker to constantly pump the UI task queue (simulating fm.Run)
	ctx, cancel := context.WithCancel(context.Background())
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for {
			select {
			case <-ctx.Done():
				return
			case task := <-vtui.FrameManager.TaskChan:
				task()
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-pumpDone
	})

	// Fire 50 concurrent reads across different overlapping chunks
	done := make(chan bool)
	for i := 0; i < 50; i++ {
		go func(offset int) {
			for retries := 0; retries < 100; retries++ {
				_, err := buf.Read(offset, 100)
				if err == nil {
					break
				}
				if err != piecetable.ErrLoading {
					t.Errorf("Unexpected error: %v", err)
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			done <- true
		}(i * 10000) // Stagger offsets
	}

	// Wait for all goroutines with polling
	deadline := time.Now().Add(3 * time.Second)
	completed := 0
	for completed < 50 && time.Now().Before(deadline) {
		select {
		case <-done:
			completed++
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	if completed < 50 {
		t.Fatal("Concurrency test timed out")
	}
}
func TestAsyncBuffer_CancellationMidFetch(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	v := vfs.NewOSVFS(t.TempDir())
	tmp := filepath.Join(t.TempDir(), "cancel.txt")
	if err := os.WriteFile(tmp, []byte("some content"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := v.Open(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	buf := NewAsyncBuffer(ctx, f)
	buf.chunkSize = 100
	defer buf.Close()

	// 1. Trigger fetch
	_, err = buf.Read(0, 5)
	if err != piecetable.ErrLoading {
		t.Fatal("Expected ErrLoading")
	}

	// 2. Cancel context while fetch is (presumably) in flight
	cancel()

	// 3. Pump tasks - the fetch result should be ignored because of b.ctx.Err()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// 4. Verification: data should NOT be in 'loaded' map
	buf.mu.Lock()
	if len(buf.loaded) > 0 {
		t.Error("Data was loaded into buffer after context cancellation")
	}
	buf.mu.Unlock()
}
func TestAsyncBuffer_RedundantFetchPrevention(t *testing.T) {
	// Tests that the 'fetching' map correctly prevents multiple goroutines
	// for the same chunk index.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	v := vfs.NewOSVFS(t.TempDir())
	tmp := filepath.Join(t.TempDir(), "redundant.txt")
	if err := os.WriteFile(tmp, make([]byte, 1000), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := v.Open(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	buf := NewAsyncBuffer(context.Background(), f)
	buf.chunkSize = 100
	defer buf.Close()

	// Trigger 10 simultaneous reads for the same offset. Read consults the
	// global frame manager, so these have to be joined before the test returns
	// or the next test's manager swap races them.
	var reads sync.WaitGroup
	reads.Add(10)
	defer reads.Wait()
	for i := 0; i < 10; i++ {
		go func() {
			defer reads.Done()
			_, _ = buf.Read(0, 5)
		}()
	}

	// Give goroutines time to start (poll instead of fixed sleep)
	deadline := time.Now().Add(100 * time.Millisecond)
	var fetchingLen int
	for time.Now().Before(deadline) {
		buf.mu.Lock()
		fetchingLen = len(buf.fetching)
		buf.mu.Unlock()
		if fetchingLen > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if fetchingLen > 1 {
		t.Errorf("Expected at most 1 in-flight fetch for chunk 0, got %d", fetchingLen)
	}
}

func TestAsyncBuffer_ContextRace(t *testing.T) {
	// Simulates the scenario where a context is cancelled exactly when data arrives.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	content := []byte("Race test content")
	v := vfs.NewOSVFS(t.TempDir())
	tmp := filepath.Join(t.TempDir(), "race.txt")
	if err := os.WriteFile(tmp, content, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := v.Open(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	iterations := 20 // Reduced from 100 to speed up the test
	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		buf := NewAsyncBuffer(ctx, f)
		buf.chunkSize = 5

		// Start fetch
		go func() {
			_, _ = buf.Read(0, 5)
		}()

		// Immediate cancel to hit the race window in fetchChunk
		cancel()

		// Pump tasks with a short deadline
		deadline := time.Now().Add(20 * time.Millisecond)
		for time.Now().Before(deadline) {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			default:
				time.Sleep(2 * time.Millisecond)
			}
		}
		buf.Close()
	}
	// If no panic or deadlock occurred, the mutex/PostTask logic is likely sound.
}

type mockErrorFile struct {
	vfs.ReadAtCloser
	errToReturn error
}

func (m *mockErrorFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	return 0, m.errToReturn
}

func TestAsyncBuffer_ErrorRecovery(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	f := &mockErrorFile{errToReturn: io.ErrUnexpectedEOF}
	// Manual Size() for mock
	buf := &AsyncBuffer{
		file:      f,
		size:      100,
		ctx:       context.Background(),
		loaded:    make(map[int][]byte),
		fetching:  make(map[int]bool),
		chunkSize: 10,
	}

	// 1. Trigger read that fails
	_, err := buf.Read(0, 5)
	if err != piecetable.ErrLoading {
		t.Fatal("Should report loading")
	}

	// 2. Process tasks to handle the failure
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// 3. Verify 'fetching' state was cleared so we can retry
	buf.mu.Lock()
	isFetching := buf.fetching[0]
	buf.mu.Unlock()

	if isFetching {
		t.Error("Fetching flag was not cleared after read error")
	}

	// 4. Fix the error in mock and retry
	f.errToReturn = nil
	_, err = buf.Read(0, 5)
	if err != piecetable.ErrLoading {
		t.Fatal("Should trigger fetch again after error recovery")
	}
}
