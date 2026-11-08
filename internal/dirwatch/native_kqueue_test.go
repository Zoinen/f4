//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package dirwatch

import (
	"sync"
	"testing"
	"time"
)

func TestIdleKqueueWatchStopsPromptly(t *testing.T) {
	dir := t.TempDir()
	var elapsed time.Duration
	for range 5 {
		watch, err := Watch(dir, Options{}, func() {})
		if err != nil {
			t.Fatal(err)
		}
		if !watch.Native() {
			watch.Close()
			t.Fatal("kqueue watch unavailable")
		}
		// Let the reader enter its idle kernel wait before closing it.
		time.Sleep(15 * time.Millisecond)
		started := time.Now()
		watch.Close()
		elapsed += time.Since(started)
	}
	if average := elapsed / 5; average > 16*time.Millisecond {
		t.Fatalf("idle kqueue watch took %v on average to close; budget 16ms", average)
	}
}

func TestKqueueSourceConcurrentClose(t *testing.T) {
	for range 20 {
		source, err := newNativeSource(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		var closers sync.WaitGroup
		for range 4 {
			closers.Go(source.close)
		}
		closers.Wait()
	}
}
