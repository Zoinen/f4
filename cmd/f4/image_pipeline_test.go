package main

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func imageTestSurface(w, h int) *vtui.ImageSurface {
	return vtui.NewImageSurfaceFromPix(w, h, w*4, make([]byte, w*h*4))
}

// newTestPipeline answers on the calling thread and decodes with a stub, so
// that a test needs neither a file system nor a running interface.
func newTestPipeline(load imageLoader) *ImagePipeline {
	p := NewImagePipeline()
	p.load = load
	p.dispatch = func(fn func()) { fn() }
	return p
}

func TestImagePipelineDecodesOnce(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	release := make(chan struct{})

	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		return imageTestSurface(4, 4), "stub", nil
	})

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		p.Load(nil, "a.png", func(res ImageResult) {
			if res.Err != nil {
				t.Errorf("decoding failed: %v", res.Err)
			}
			wg.Done()
		})
	}
	close(release)
	wg.Wait()

	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 1 {
		t.Errorf("two requests for one picture must share a decode, got %d", got)
	}

	// A picture already in hand is handed over on the spot.
	answered := false
	p.Load(nil, "a.png", func(res ImageResult) { answered = res.Surface.Valid() })
	if !answered {
		t.Error("a cached picture must be delivered before Load returns")
	}

	mu.Lock()
	got = calls
	mu.Unlock()
	if got != 1 {
		t.Errorf("a cached picture must not be decoded again, got %d decodes", got)
	}
}

func TestImagePipelineEvictsTheOldest(t *testing.T) {
	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		return imageTestSurface(100, 100), "stub", nil
	})
	// Room for two pictures of forty thousand bytes each.
	p.limit = 90000

	for _, name := range []string{"a", "b", "c"} {
		if res := p.LoadSync(context.Background(), nil, name); res.Err != nil {
			t.Fatalf("%s: %v", name, res.Err)
		}
	}

	count, bytes := p.CacheStats()
	if count != 2 || bytes != 80000 {
		t.Errorf("cache holds %d pictures, %d bytes", count, bytes)
	}
	if _, ok := p.Cached(nil, "a"); ok {
		t.Error("the least recently used picture must have been thrown out")
	}
	if _, ok := p.Cached(nil, "c"); !ok {
		t.Error("the newest picture must stay")
	}
}

func TestImagePipelineKeepsTheNewestPictureHoweverLarge(t *testing.T) {
	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		return imageTestSurface(100, 100), "stub", nil
	})
	p.limit = 1

	if res := p.LoadSync(context.Background(), nil, "a"); res.Err != nil {
		t.Fatalf("decoding failed: %v", res.Err)
	}
	if count, _ := p.CacheStats(); count != 1 {
		t.Errorf("the picture on screen must survive its own size, cache holds %d", count)
	}
}

func TestImagePipelinePrefetchFollowsTheView(t *testing.T) {
	started := make(chan string, 8)
	gate := make(chan struct{})

	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		started <- path
		<-gate
		return imageTestSurface(2, 2), "stub", nil
	})
	p.workers = 1

	p.Prefetch(nil, []string{"a", "b", "c"})
	if first := <-started; first != "a" {
		t.Fatalf("the nearest neighbour goes first, got %q", first)
	}

	// The view moved: b and c are not neighbours any more, d is.
	p.Prefetch(nil, []string{"a", "d"})
	close(gate)

	if second := <-started; second != "d" {
		t.Errorf("a neighbour left behind must give up its place, got %q", second)
	}
	select {
	case extra := <-started:
		t.Errorf("nothing else should have been decoded, got %q", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestImagePipelineDoesNotCacheFailures(t *testing.T) {
	var mu sync.Mutex
	calls := 0

	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			return nil, "", errors.New("broken file")
		}
		return imageTestSurface(4, 4), "stub", nil
	})

	if res := p.LoadSync(context.Background(), nil, "a"); res.Err == nil {
		t.Fatal("the failure must be reported")
	}
	if res := p.LoadSync(context.Background(), nil, "a"); res.Err != nil {
		t.Fatalf("the second attempt must be made: %v", res.Err)
	}

	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 2 {
		t.Errorf("a failure must not be remembered as a result, got %d decodes", got)
	}
}

func TestImagePipelineInvalidateAndClear(t *testing.T) {
	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		return imageTestSurface(4, 4), "stub", nil
	})

	p.LoadSync(context.Background(), nil, "a")
	p.LoadSync(context.Background(), nil, "b")

	p.Invalidate(nil, "a")
	if _, ok := p.Cached(nil, "a"); ok {
		t.Error("an invalidated picture must be gone")
	}
	if _, ok := p.Cached(nil, "b"); !ok {
		t.Error("only the named picture may be dropped")
	}

	p.Clear()
	if count, bytes := p.CacheStats(); count != 0 || bytes != 0 {
		t.Errorf("after Clear the cache holds %d pictures, %d bytes", count, bytes)
	}
}

func TestImagePipelineLoadSyncHonoursCancellation(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	p := newTestPipeline(func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error) {
		<-release
		return imageTestSurface(4, 4), "stub", nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if res := p.LoadSync(ctx, nil, "a"); res.Err == nil {
		t.Error("a cancelled request must not wait for the decoder")
	}
}

func TestImageNeighbourhood(t *testing.T) {
	paths := []string{"0", "1", "2", "3", "4"}

	if got := ImageNeighbourhood(paths, 2, 2); !reflect.DeepEqual(got, []string{"3", "1", "4", "0"}) {
		t.Errorf("neighbours of the middle: %v", got)
	}
	if got := ImageNeighbourhood(paths, 0, 1); !reflect.DeepEqual(got, []string{"1"}) {
		t.Errorf("neighbours of the first: %v", got)
	}
	if got := ImageNeighbourhood(paths, 4, 1); !reflect.DeepEqual(got, []string{"3"}) {
		t.Errorf("neighbours of the last: %v", got)
	}
	if got := ImageNeighbourhood(paths, -1, 2); got != nil {
		t.Errorf("a position outside the list has no neighbours: %v", got)
	}
	if got := ImageNeighbourhood(paths, 2, 0); got != nil {
		t.Errorf("a radius of zero has no neighbours: %v", got)
	}
}
