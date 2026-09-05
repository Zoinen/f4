package main

import (
	"sync"

	"github.com/unxed/vtui"
)

var (
	asyncClipboardMu sync.Mutex
	asyncClipboardWG sync.WaitGroup
)

// setClipboardAsync keeps the UI responsive while making the lifetime of the
// clipboard worker observable to tests. vtui.SetClipboard may read the global
// FrameManager while it negotiates with far2l, so tests must not replace that
// manager until these workers have finished.
func setClipboardAsync(text string) {
	asyncClipboardMu.Lock()
	asyncClipboardWG.Add(1)
	asyncClipboardMu.Unlock()
	go func() {
		defer asyncClipboardWG.Done()
		vtui.SetClipboard(text)
	}()
}

// waitForAsyncClipboard blocks until all clipboard workers started so far
// have finished. The mutex prevents a new worker from being added while the
// wait is in progress.
func waitForAsyncClipboard() {
	asyncClipboardMu.Lock()
	defer asyncClipboardMu.Unlock()
	asyncClipboardWG.Wait()
}
