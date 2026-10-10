//go:build !windows

package panel

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// A file only root can read is not read on the UI goroutine, which is where
// Show runs: the sudo password prompt for it is a dialog that goroutine has
// to show, so an elevated read from there would wait for itself. Show has to
// return with the preview still loading, and the worker's answer arrives
// through the task queue like any other preview's.
func TestQuickViewHandsARefusedReadToTheWorker(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root is refused nothing, so there is no refused read to hand over")
	}
	quickView, pnl, screen := newQuickViewProviderFixture(t, map[string][]byte{"locked.txt": []byte("secret\n")})
	locked := filepath.Join(pnl.Vfs.GetPath(), "locked.txt")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })

	quickView.Show(screen)
	if !quickView.cacheLoading {
		t.Fatalf("Show answered the refused read itself: err=%v lines=%v", quickView.cacheReadErr, quickView.cacheLines)
	}
	waitForQuickView(t, func() bool { return !quickView.cacheLoading })
	// No sudo in a test, so the worker is refused as well, and says so.
	if !errors.Is(quickView.cacheReadErr, fs.ErrPermission) {
		t.Fatalf("worker result: err=%v lines=%v, want the refusal", quickView.cacheReadErr, quickView.cacheLines)
	}
}
