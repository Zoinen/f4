package panel

import (
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"testing"
	"time"
)

// ponytail: this file is a copy of internal/paneltest's helpers, kept because
// an in-package test cannot import a package that imports it. Ceiling: twenty
// lines that drift apart silently. Upgrade path is to make the panel's tests an
// external test package (package panel_test), which is what
// internal/paneltest/doc.go already describes — the tests that would then need
// the package's private members are the ones to weigh against it.

// waitForDirectoryLoads blocks until no directory-load worker is running
// anywhere in the process.
//
// The workers read vtui.FrameManager and config.App while they run, so a test
// that replaces either one has to know they are all finished first. Panels are
// created deep inside panel.PanelsFrame.ResizeConsole as well as directly, so the
// caller usually has no panel to wait on and this asks the question globally
// instead.
func waitForDirectoryLoads(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		DirectoryLoadWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for the directory-load workers to stop")
	}
}

// drainAsyncClipboard is terminal.WaitForAsyncClipboard in the shape a frame-manager
// drain takes. Clipboard writes run asynchronously because they may wait for
// far2l IPC, and SetClipboard reads vtui.FrameManager.
func drainAsyncClipboard(*testing.T) { terminal.WaitForAsyncClipboard() }

// swapFrameManager is testutil.SwapFrameManager carrying the two background
// workers this package leaves running. Both read the global frame manager, so
// both have to be joined before it is replaced — which is the whole reason the
// shared helper takes its drains from the caller.
func swapFrameManager(t *testing.T) func() {
	t.Helper()
	return testutil.SwapFrameManager(t, drainAsyncClipboard, waitForDirectoryLoads)
}

// stubHistoryProvider is a vtui history provider backed by a map, so a test can
// hand the panel a folder history without touching the user's files.
type stubHistoryProvider map[string][]string

func (s stubHistoryProvider) LoadHistory(name string) []string { return s[name] }
func (s stubHistoryProvider) SaveHistory(name string, h []string) {
	s[name] = append([]string(nil), h...)
}
