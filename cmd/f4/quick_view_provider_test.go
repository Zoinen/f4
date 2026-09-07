package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type quickViewTestProvider struct {
	name     string
	priority int
	match    func(vfs.QuickViewRequest) bool
	preview  func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error)
}

func (provider *quickViewTestProvider) Name() string  { return provider.name }
func (provider *quickViewTestProvider) Priority() int { return provider.priority }
func (provider *quickViewTestProvider) CanPreview(request vfs.QuickViewRequest) bool {
	return provider.match == nil || provider.match(request)
}
func (provider *quickViewTestProvider) Preview(ctx context.Context, request vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
	return provider.preview(ctx, request)
}

func registerQuickViewTestProvider(t *testing.T, provider vfs.QuickViewProvider) {
	t.Helper()
	registration, err := vfs.RegisterQuickViewProvider(provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)
}

func newQuickViewProviderFixture(t *testing.T, namesAndData map[string][]byte) (*QuickViewPanel, *FileSystemPanel, *vtui.ScreenBuf) {
	t.Helper()
	dir := t.TempDir()
	entries := make([]*fileEntry, 0, len(namesAndData))
	for name, data := range namesAndData {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &fileEntry{VFSItem: vfs.VFSItem{Name: name, Size: int64(len(data))}})
	}
	filesystem := vfs.NewOSVFS(dir)
	panel := &FileSystemPanel{vfs: filesystem, entries: entries}
	panel.ScreenObject.SetPosition(0, 0, 39, 19)
	quickView := NewQuickViewPanel(panel)
	quickView.SetPosition(0, 0, 39, 19)
	t.Cleanup(quickView.Close)

	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	vtui.SetDefaultPalette()
	return quickView, panel, screen
}

func waitForQuickView(t *testing.T, ready func() bool) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("timed out waiting for Quick View provider")
		}
	}
}

func TestQuickViewProviderRunsAsynchronouslyAndRendersResult(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	provider := &quickViewTestProvider{
		name:     "test-quick-view-async",
		priority: 1000,
		match:    func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qvasync") },
		preview: func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			close(started)
			<-release
			return vfs.QuickViewResult{Label: "Special preview", Lines: []string{"first", "second"}}, nil
		},
	}
	registerQuickViewTestProvider(t, provider)
	quickView, _, screen := newQuickViewProviderFixture(t, map[string][]byte{"movie.qvasync": []byte("fallback")})

	quickView.Show(screen)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	if !quickView.cacheLoading {
		t.Fatal("Quick View did not expose its loading state")
	}
	close(release)
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	if quickView.cacheLabel != "Special preview" || len(quickView.cacheLines) != 2 || quickView.cacheLines[0] != "first" {
		t.Fatalf("provider result was not applied: label=%q lines=%v", quickView.cacheLabel, quickView.cacheLines)
	}
	quickView.Show(screen)
	if len(quickView.displayLines) != 2 {
		t.Fatalf("provider lines did not use ordinary renderer: %v", quickView.displayLines)
	}
}

func TestQuickViewProviderUnsupportedTriesNextAndFallsBack(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	provider := func(name string, priority int, result vfs.QuickViewResult, err error) *quickViewTestProvider {
		return &quickViewTestProvider{
			name: name, priority: priority,
			match: func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qvchain") },
			preview: func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
				mu.Lock()
				calls = append(calls, name)
				mu.Unlock()
				return result, err
			},
		}
	}
	registerQuickViewTestProvider(t, provider("test-qv-chain-low", 999, vfs.QuickViewResult{Lines: []string{"handled"}}, nil))
	registerQuickViewTestProvider(t, provider("test-qv-chain-high", 1000, vfs.QuickViewResult{}, vfs.ErrQuickViewUnsupported))
	quickView, _, screen := newQuickViewProviderFixture(t, map[string][]byte{"movie.qvchain": []byte("fallback")})

	quickView.Show(screen)
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	if strings.Join(gotCalls, ",") != "test-qv-chain-high,test-qv-chain-low" {
		t.Fatalf("provider calls = %v", gotCalls)
	}
	if len(quickView.cacheLines) != 1 || quickView.cacheLines[0] != "handled" {
		t.Fatalf("lower-priority result = %v", quickView.cacheLines)
	}

	decliner := &quickViewTestProvider{
		name:     "test-qv-fallback-decliner",
		priority: 1000,
		match:    func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qvfallback") },
		preview: func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			return vfs.QuickViewResult{}, vfs.ErrQuickViewUnsupported
		},
	}
	registerQuickViewTestProvider(t, decliner)
	fallbackView, _, fallbackScreen := newQuickViewProviderFixture(t, map[string][]byte{"notes.qvfallback": []byte("ordinary text\n")})
	fallbackView.Show(fallbackScreen)
	waitForQuickView(t, func() bool { return !fallbackView.cacheLoading })
	if len(fallbackView.cacheLines) != 1 || fallbackView.cacheLines[0] != "ordinary text" || fallbackView.cacheBinary {
		t.Fatalf("ordinary fallback was not restored: binary=%t lines=%v", fallbackView.cacheBinary, fallbackView.cacheLines)
	}
}

func TestQuickViewProviderErrorStopsFallback(t *testing.T) {
	parseErr := errors.New("broken container")
	var lowCalls atomic.Int32
	registerQuickViewTestProvider(t, &quickViewTestProvider{
		name: "test-qv-error-low", priority: 999,
		match: func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qverror") },
		preview: func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			lowCalls.Add(1)
			return vfs.QuickViewResult{Lines: []string{"wrong"}}, nil
		},
	})
	registerQuickViewTestProvider(t, &quickViewTestProvider{
		name: "test-qv-error-high", priority: 1000,
		match: func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qverror") },
		preview: func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			return vfs.QuickViewResult{}, parseErr
		},
	})
	quickView, _, screen := newQuickViewProviderFixture(t, map[string][]byte{"movie.qverror": []byte("generic fallback")})
	quickView.Show(screen)
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	if !errors.Is(quickView.cacheReadErr, parseErr) || !strings.Contains(quickView.cacheReadErr.Error(), "test-qv-error-high") {
		t.Fatalf("provider error = %v", quickView.cacheReadErr)
	}
	if lowCalls.Load() != 0 || len(quickView.cacheLines) != 0 {
		t.Fatalf("error incorrectly fell through: low calls=%d lines=%v", lowCalls.Load(), quickView.cacheLines)
	}
}

func TestQuickViewProviderSelectionChangeCancelsStaleResult(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	provider := &quickViewTestProvider{
		name:     "test-qv-cancel",
		priority: 1000,
		match:    func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qvcancel") },
		preview: func(ctx context.Context, request vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			if strings.HasPrefix(request.Item.Name, "a") {
				close(started)
				<-ctx.Done()
				close(cancelled)
				return vfs.QuickViewResult{Lines: []string{"stale"}}, nil
			}
			return vfs.QuickViewResult{Lines: []string{"fresh"}}, nil
		},
	}
	registerQuickViewTestProvider(t, provider)
	quickView, panel, screen := newQuickViewProviderFixture(t, map[string][]byte{
		"a.qvcancel": []byte("a"),
		"b.qvcancel": []byte("b"),
	})
	// Map iteration does not promise row order.
	for index, entry := range panel.entries {
		if entry.Name == "a.qvcancel" {
			panel.cursorIdx = index
		}
	}
	quickView.Show(screen)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first provider call did not start")
	}
	for index, entry := range panel.entries {
		if entry.Name == "b.qvcancel" {
			panel.cursorIdx = index
		}
	}
	quickView.Show(screen)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("selection change did not cancel old provider")
	}
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	if len(quickView.cacheLines) != 1 || quickView.cacheLines[0] != "fresh" {
		t.Fatalf("stale result replaced new selection: %v", quickView.cacheLines)
	}
}

func TestQuickViewProviderCloseCancelsPreview(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	registerQuickViewTestProvider(t, &quickViewTestProvider{
		name:     "test-qv-close-cancel",
		priority: 1000,
		match:    func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qvclose") },
		preview: func(ctx context.Context, _ vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return vfs.QuickViewResult{}, ctx.Err()
		},
	})
	quickView, _, screen := newQuickViewProviderFixture(t, map[string][]byte{"movie.qvclose": []byte("data")})
	quickView.Show(screen)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	quickView.Close()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("closing Quick View did not cancel provider context")
	}
}

func TestQuickViewProviderCacheKeyIncludesRevisionAndVFS(t *testing.T) {
	var calls atomic.Int32
	provider := &quickViewTestProvider{
		name:     "test-qv-cache-key",
		priority: 1000,
		match:    func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qvkey") },
		preview: func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			count := calls.Add(1)
			return vfs.QuickViewResult{Lines: []string{string(rune('0' + count))}}, nil
		},
	}
	registerQuickViewTestProvider(t, provider)
	quickView, panel, screen := newQuickViewProviderFixture(t, map[string][]byte{"same.qvkey": []byte("one")})
	panel.entries[0].Revision = "revision-one"
	quickView.Show(screen)
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	quickView.Show(screen)
	if calls.Load() != 1 {
		t.Fatalf("unchanged selection parsed %d times", calls.Load())
	}

	panel.entries[0].Revision = "revision-two"
	quickView.Show(screen)
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	if calls.Load() != 2 {
		t.Fatalf("revision change parsed %d times, want 2", calls.Load())
	}

	otherDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(otherDir, "same.qvkey"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	panel.vfs = vfs.NewOSVFS(otherDir)
	quickView.Show(screen)
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	if calls.Load() != 3 {
		t.Fatalf("VFS change parsed %d times, want 3", calls.Load())
	}
}

func TestQuickViewImagePreviewPrecedesRegisteredProviders(t *testing.T) {
	var calls atomic.Int32
	provider := &quickViewTestProvider{
		name:     "test-qv-image-precedence",
		priority: 1000,
		match:    func(request vfs.QuickViewRequest) bool { return strings.HasSuffix(request.Path, ".qoi") },
		preview: func(context.Context, vfs.QuickViewRequest) (vfs.QuickViewResult, error) {
			calls.Add(1)
			return vfs.QuickViewResult{Lines: []string{"metadata"}}, nil
		},
	}
	registerQuickViewTestProvider(t, provider)
	qoi := []byte{'q', 'o', 'i', 'f', 0, 0, 0, 1, 0, 0, 0, 1, 4, 0, 0xff, 0xff, 0, 0, 0xff}
	quickView, _, screen := newQuickViewProviderFixture(t, map[string][]byte{"image.qoi": qoi})
	quickView.Show(screen)
	waitForQuickView(t, func() bool { return quickView.imageSurf != nil })
	if !quickView.cacheImage || calls.Load() != 0 {
		t.Fatalf("image precedence: cacheImage=%t provider calls=%d", quickView.cacheImage, calls.Load())
	}
}
