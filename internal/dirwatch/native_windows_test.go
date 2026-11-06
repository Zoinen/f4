//go:build windows

package dirwatch

import (
	"testing"
	"time"
)

func TestIdleWindowsWatchStopsPromptly(t *testing.T) {
	dir := t.TempDir()
	var closeTime time.Duration
	for range 5 {
		watch, err := Watch(dir, Options{}, func() {})
		if err != nil {
			t.Fatal(err)
		}
		if !watch.Native() {
			watch.Close()
			t.Skip("native notifications are unavailable in the temp directory")
		}
		// Give the notification goroutine time to enter its idle wait.
		time.Sleep(15 * time.Millisecond)
		started := time.Now()
		watch.Close()
		closeTime += time.Since(started)
	}
	if average := closeTime / 5; average > 55*time.Millisecond {
		t.Fatalf("idle native watch took %v on average to close", average)
	}
}
