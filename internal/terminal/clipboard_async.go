package terminal

import (
	"sync"
)

var (
	asyncClipboardMu sync.Mutex
	asyncClipboardWG sync.WaitGroup
)

// SetClipboardAsync keeps the UI responsive while making the lifetime of the
// clipboard worker observable to tests. SetF4Clipboard may read the global
// FrameManager while it negotiates with far2l, so tests must not replace that
// manager until these workers have finished.
func SetClipboardAsync(text string) {
	asyncClipboardMu.Lock()
	asyncClipboardWG.Add(1)
	asyncClipboardMu.Unlock()
	go func() {
		defer asyncClipboardWG.Done()
		SetF4Clipboard(text)
	}()
}

// WaitForAsyncClipboard blocks until all clipboard workers started so far
// have finished. The mutex prevents a new worker from being added while the
// wait is in progress.
func WaitForAsyncClipboard() {
	asyncClipboardMu.Lock()
	defer asyncClipboardMu.Unlock()
	asyncClipboardWG.Wait()
}
