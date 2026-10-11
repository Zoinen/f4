package app

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// fakeTitledVFS is a minimal vfs.VFS that only exists to carry a title, for
// exercising the non-local branches of history_bridge.go: they never touch
// anything but GetTitle() and the dynamic type of the value, so the embedded
// vfs.VFS is left nil on purpose (calling anything else on it would panic,
// which is exactly what would flag a test reaching further than it should).
type fakeTitledVFS struct {
	vfs.VFS
	title string
}

func (f *fakeTitledVFS) GetTitle() string { return f.title }

// setupHistoryBridgeTestPanel wires up a panels frame with a real local
// active panel, on the larger screen buffer the history dialog's own
// resize() requires (see initHistoryTestScreen) — the same setup
// viewer_editor_history_test.go's tests build inline, factored out here
// since most of this file's tests need it too.
func setupHistoryBridgeTestPanel(t *testing.T) (*panel.PanelsFrame, *panel.FileSystemPanel, string) {
	t.Helper()
	initHistoryTestScreen(t)
	dir := t.TempDir()
	pf := panel.NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(120, 40)
	fsp := pf.Panels[0].(*panel.FileSystemPanel)
	pf.SwitchToVFS(fsp, vfs.NewOSVFS(dir))
	pf.ActiveIdx = 0
	paneltest.WaitForLoad(t, fsp)
	return pf, fsp, dir
}

func withStubHistoryProvider(t *testing.T) stubHistoryProvider {
	t.Helper()
	previous := vtui.GlobalHistoryProvider
	provider := stubHistoryProvider{}
	vtui.GlobalHistoryProvider = provider
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })
	return provider
}

func TestLoadViewerEditorHistory_NoProviderReturnsNil(t *testing.T) {
	previous := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = nil
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	if got := loadViewerEditorHistory(); got != nil {
		t.Fatalf("loadViewerEditorHistory() with no provider = %#v, want nil", got)
	}
}

// TestLoadViewerEditorHistory_SkipsBadRecordsAndFillsDefaults covers the
// per-record decoding loop directly: malformed JSON and a record with no
// path are dropped, and a record that never carried a Display (an older
// on-disk record, or one built by hand) falls back to its Path.
func TestLoadViewerEditorHistory_SkipsBadRecordsAndFillsDefaults(t *testing.T) {
	provider := withStubHistoryProvider(t)
	provider.SaveHistory(viewerEditorHistoryID, []string{
		`not json at all`,
		`{"path":""}`,
		`{"path":"/a/b.txt"}`,
	})

	entries := loadViewerEditorHistory()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want exactly the one valid record", entries)
	}
	if entries[0].Path != "/a/b.txt" || entries[0].Display != "/a/b.txt" {
		t.Fatalf("entry = %#v, want Display defaulted to Path", entries[0])
	}
	if entries[0].Mode != historyModeView {
		t.Fatalf("Mode = %q, want it defaulted to view", entries[0].Mode)
	}
}

func TestSaveViewerEditorHistory_NoProviderIsANoOp(t *testing.T) {
	previous := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = nil
	t.Cleanup(func() { vtui.GlobalHistoryProvider = previous })

	// Must not panic with no provider to write to.
	saveViewerEditorHistory([]viewerEditorHistoryEntry{{Path: "/x"}})
}

// TestRememberViewerEditorHistory_SkipsTransientTerminalLog covers the #408
// guard mentioned in the source: the terminal log is generated live and has
// no stable file to reopen later, so it is never remembered.
func TestRememberViewerEditorHistory_SkipsTransientTerminalLog(t *testing.T) {
	withStubHistoryProvider(t)

	RememberViewerEditorHistory(terminal.NewTerminalLogVFS(nil, nil), "term://log", historyModeView)

	if entries := loadViewerEditorHistory(); len(entries) != 0 {
		t.Fatalf("the terminal log was remembered: %#v", entries)
	}
}

// TestRememberViewerEditorHistory_TitledVFSPrefixesDisplay covers the
// non-local, TitleProvider branch: the file's VFS is not the local disk, but
// it can name itself (a network drive, a plugin mount), so the dialog row
// gets a "title:path" display the same way the panel border does.
func TestRememberViewerEditorHistory_TitledVFSPrefixesDisplay(t *testing.T) {
	withStubHistoryProvider(t)
	fs := &fakeTitledVFS{title: "remote"}

	RememberViewerEditorHistory(fs, "/some/path", historyModeView)

	entries := loadViewerEditorHistory()
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want 1", entries)
	}
	if entries[0].Local {
		t.Fatal("a titled, non-OSVFS entry was marked Local")
	}
	if entries[0].VFSTitle != "remote" || entries[0].Display != "remote:/some/path" {
		t.Fatalf("entry = %#v, want a title-prefixed display", entries[0])
	}

	// A path that already carries the title prefix (as the dialog itself
	// writes back, e.g. via revisits) does not get it doubled.
	RememberViewerEditorHistory(fs, "remote:/other", historyModeView)
	entries = loadViewerEditorHistory()
	if entries[0].Path != "remote:/other" || entries[0].Display != "remote:/other" {
		t.Fatalf("already-prefixed entry = %#v, want Display unchanged", entries[0])
	}
}

// TestLimitViewerEditorHistory pins the eviction policy: locked entries are
// never dropped even past the limit, and unlocked ones are trimmed off the
// tail in place, oldest-appearing-last order (limitViewerEditorHistory is
// always fed newest-first).
func TestLimitViewerEditorHistory(t *testing.T) {
	mk := func(n int, locked map[int]bool) []viewerEditorHistoryEntry {
		out := make([]viewerEditorHistoryEntry, n)
		for i := range out {
			out[i] = viewerEditorHistoryEntry{Path: fmt.Sprintf("/f%d", i), Lock: locked[i]}
		}
		return out
	}

	tests := []struct {
		name       string
		entries    []viewerEditorHistoryEntry
		limit      int
		wantKept   int
		wantLocked int
	}{
		{"limit<=0 keeps everything", mk(5, nil), 0, 5, 0},
		{"under the limit keeps everything", mk(3, nil), 10, 3, 0},
		{"over the limit trims the tail", mk(5, nil), 3, 3, 0},
		{"locked entries survive beyond the unlocked budget", mk(5, map[int]bool{0: true, 4: true}), 3, 3, 2},
		{"more locked than the limit keeps every locked entry", mk(5, map[int]bool{0: true, 1: true, 2: true, 3: true}), 2, 4, 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := limitViewerEditorHistory(tc.entries, tc.limit)
			if len(got) != tc.wantKept {
				t.Fatalf("len = %d, want %d: %#v", len(got), tc.wantKept, got)
			}
			locked := 0
			for _, e := range got {
				if e.Lock {
					locked++
				}
			}
			if locked != tc.wantLocked {
				t.Fatalf("locked kept = %d, want %d: %#v", locked, tc.wantLocked, got)
			}
			for _, e := range tc.entries {
				if !e.Lock {
					continue
				}
				found := false
				for _, k := range got {
					if k.Path == e.Path {
						found = true
					}
				}
				if !found {
					t.Fatalf("locked entry %q was dropped: %#v", e.Path, got)
				}
			}
		})
	}
}

// TestSameViewerEditorHistoryFile covers the dedup key directly: a mismatch
// on Local, VFSType or VFSTitle never matches regardless of Path, a local
// pair compares through panel.SameFolderHistoryPath (so "." segments do not
// defeat it), and a non-local pair compares Path verbatim.
func TestSameViewerEditorHistoryFile(t *testing.T) {
	base := viewerEditorHistoryEntry{Local: true, VFSType: "*vfs.OSVFS", Path: "/a/b.txt"}
	withField := func(e viewerEditorHistoryEntry, set func(*viewerEditorHistoryEntry)) viewerEditorHistoryEntry {
		set(&e)
		return e
	}

	tests := []struct {
		name string
		a, b viewerEditorHistoryEntry
		want bool
	}{
		{"identical local entries match", base, base, true},
		{"different Local flags never match", base, withField(base, func(e *viewerEditorHistoryEntry) { e.Local = false }), false},
		{"different VFSType never match", base, withField(base, func(e *viewerEditorHistoryEntry) { e.VFSType = "*other" }), false},
		{"different VFSTitle never match", base, withField(base, func(e *viewerEditorHistoryEntry) { e.VFSTitle = "x" }), false},
		{"local entries compare by normalized path", base, withField(base, func(e *viewerEditorHistoryEntry) { e.Path = "/a/./b.txt" }), true},
		{"local entries with a different path do not match", base, withField(base, func(e *viewerEditorHistoryEntry) { e.Path = "/a/c.txt" }), false},
		{
			"non-local entries compare paths exactly",
			viewerEditorHistoryEntry{Local: false, VFSType: "t", Path: "/x"},
			viewerEditorHistoryEntry{Local: false, VFSType: "t", Path: "/x"},
			true,
		},
		{
			"non-local entries with different paths do not match",
			viewerEditorHistoryEntry{Local: false, VFSType: "t", Path: "/x"},
			viewerEditorHistoryEntry{Local: false, VFSType: "t", Path: "/y"},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameViewerEditorHistoryFile(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameViewerEditorHistoryFile(%#v, %#v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestViewerEditorHistoryVFS covers the source-resolution helper behind
// openViewerEditorHistoryEntry and revealViewerEditorHistoryEntry: a local
// entry gets a fresh OSVFS at its directory (no panel lookup at all), a
// non-local entry is matched against whichever of the active/inactive panels
// carries the same dynamic VFS type and the same title, and no match at all
// resolves to nil.
func TestViewerEditorHistoryVFS(t *testing.T) {
	t.Run("local entry gets a fresh OSVFS at its directory", func(t *testing.T) {
		dir := t.TempDir()
		entry := viewerEditorHistoryEntry{Local: true, Path: filepath.Join(dir, "f.txt")}

		got := viewerEditorHistoryVFS(&panel.PanelsFrame{}, entry)
		osvfs, ok := got.(*vfs.OSVFS)
		if !ok {
			t.Fatalf("got %T, want *vfs.OSVFS", got)
		}
		if filepath.Clean(osvfs.GetPath()) != filepath.Clean(dir) {
			t.Fatalf("GetPath() = %q, want %q", osvfs.GetPath(), dir)
		}
	})

	t.Run("non-local entry matches the panel with the same type and title", func(t *testing.T) {
		fs := &fakeTitledVFS{title: "remote"}
		pf := &panel.PanelsFrame{ActiveIdx: 0}
		pf.Panels[0] = &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(".")}
		pf.Panels[1] = &panel.FileSystemPanel{Vfs: fs}
		entry := viewerEditorHistoryEntry{VFSType: fmt.Sprintf("%T", fs), VFSTitle: "remote", Path: "/x"}

		if got := viewerEditorHistoryVFS(pf, entry); got != vfs.VFS(fs) {
			t.Fatalf("got %#v, want the matching panel's VFS", got)
		}
	})

	t.Run("mismatched title on an otherwise matching type does not match", func(t *testing.T) {
		fs := &fakeTitledVFS{title: "remote"}
		pf := &panel.PanelsFrame{ActiveIdx: 0}
		pf.Panels[0] = &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(".")}
		pf.Panels[1] = &panel.FileSystemPanel{Vfs: fs}
		entry := viewerEditorHistoryEntry{VFSType: fmt.Sprintf("%T", fs), VFSTitle: "gone", Path: "/x"}

		if got := viewerEditorHistoryVFS(pf, entry); got != nil {
			t.Fatalf("got %#v, want nil", got)
		}
	})

	t.Run("no panel of a matching type returns nil", func(t *testing.T) {
		pf := &panel.PanelsFrame{ActiveIdx: 0}
		pf.Panels[0] = &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(".")}
		pf.Panels[1] = &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(".")}
		entry := viewerEditorHistoryEntry{VFSType: "*app.fakeTitledVFS", VFSTitle: "gone", Path: "/x"}

		if got := viewerEditorHistoryVFS(pf, entry); got != nil {
			t.Fatalf("got %#v, want nil", got)
		}
	})
}

// TestOpenViewerEditorHistoryEntry_NoSourceShowsMessage covers the guard: an
// entry whose VFS can no longer be resolved (its plugin unmounted, the panel
// moved on) tells the user instead of opening nothing silently.
func TestOpenViewerEditorHistoryEntry_NoSourceShowsMessage(t *testing.T) {
	pf, _, _ := setupHistoryBridgeTestPanel(t)
	entry := viewerEditorHistoryEntry{Local: false, VFSType: "*app.fakeTitledVFS", VFSTitle: "gone", Path: "/x"}

	if openViewerEditorHistoryEntry(pf, entry, historyModeView) {
		t.Fatal("opened an entry whose source VFS could not be resolved")
	}
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || dlg.GetTitle() != i18n.Msg("History.ViewEditTitle") {
		t.Fatalf("top frame = %#v, want the source-unavailable message", vtui.FrameManager.GetTopFrame())
	}
}

// TestOpenViewerEditorHistoryEntry_DispatchesByMode covers the routing that
// is this function's whole reason to exist: view mode reaches
// actionOpenViewer, edit mode reaches ActionOpenEditor. Both remember the
// history entry synchronously before doing anything async, so the mode that
// was actually recorded is the routing's own trustworthy witness.
func TestOpenViewerEditorHistoryEntry_DispatchesByMode(t *testing.T) {
	withStubHistoryProvider(t)
	pf, _, dir := setupHistoryBridgeTestPanel(t)
	path := writeHistoryFile(t, dir, "a.txt")

	if !openViewerEditorHistoryEntry(pf, viewerEditorHistoryEntry{Local: true, Path: path}, HistoryModeEdit) {
		t.Fatal("edit mode reported it did not open")
	}
	if entries := loadViewerEditorHistory(); len(entries) != 1 || entries[0].Mode != HistoryModeEdit {
		t.Fatalf("edit mode was not routed to ActionOpenEditor: %#v", entries)
	}
	closeOpenedHistoryFile(t, path, HistoryModeEdit)

	if !openViewerEditorHistoryEntry(pf, viewerEditorHistoryEntry{Local: true, Path: path}, historyModeView) {
		t.Fatal("view mode reported it did not open")
	}
	if entries := loadViewerEditorHistory(); len(entries) != 1 || entries[0].Mode != historyModeView {
		t.Fatalf("view mode was not routed to actionOpenViewer: %#v", entries)
	}
	closeOpenedHistoryFile(t, path, historyModeView)
}

// closeOpenedHistoryFile waits for the async open a history entry triggered
// to land as a real editor or viewer frame on path, then closes it. Leaving
// it open keeps the file handle (the editor's mapping, the viewer's backend)
// alive past the test, and on Windows t.TempDir's cleanup then fails to
// delete a file another handle still holds open.
func closeOpenedHistoryFile(t *testing.T, path string, mode viewerEditorHistoryMode) {
	t.Helper()
	v := vfs.NewOSVFS(filepath.Dir(path))
	if mode == HistoryModeEdit {
		var ev *editor.EditorView
		pumpUntil(t, "the editor to open "+path, func() bool {
			ev, _ = FindOpenedEditor(v, path)
			return ev != nil
		})
		ev.Close()
		return
	}
	var vv *viewer.ViewerView
	pumpUntil(t, "the viewer to open "+path, func() bool {
		vv, _ = findOpenedViewer(v, path)
		return vv != nil
	})
	vv.Close()
}

// TestRevealViewerEditorHistoryEntry_NonLocalShowsMessage covers the guard
// revealViewerEditorHistoryEntry opens with: only a local file can be shown
// in a panel, so a non-local entry (or no active panel) reports the same
// source-unavailable message openViewerEditorHistoryEntry uses, instead of
// silently doing nothing. The "already there" and "navigate" branches are
// already covered by TestIssue408RevealHistoryEntry* in
// issue408_reveal_test.go.
func TestRevealViewerEditorHistoryEntry_NonLocalShowsMessage(t *testing.T) {
	pf, _, _ := setupHistoryBridgeTestPanel(t)

	if revealViewerEditorHistoryEntry(pf, viewerEditorHistoryEntry{Local: false, Path: "remote:/x"}) {
		t.Fatal("revealed a non-local entry")
	}
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || dlg.GetTitle() != i18n.Msg("History.ViewEditTitle") {
		t.Fatalf("top frame = %#v, want the source-unavailable message", vtui.FrameManager.GetTopFrame())
	}
}

// TestActionViewerEditorHistory_EmptyHistoryShowsMessage covers the guard at
// the very top of the action: with nothing remembered yet, Alt+F11 explains
// that instead of opening an empty dialog.
func TestActionViewerEditorHistory_EmptyHistoryShowsMessage(t *testing.T) {
	withStubHistoryProvider(t)
	pf, _, _ := setupHistoryBridgeTestPanel(t)

	actionViewerEditorHistory(pf)

	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || dlg.GetTitle() != i18n.Msg("History.Title") {
		t.Fatalf("top frame = %#v, want the empty-history message", vtui.FrameManager.GetTopFrame())
	}
}

// openHistoryBridgeMenu remembers one file and opens the viewer/editor
// history dialog on it, returning the live menu and search for a test to
// drive keys through.
func openHistoryBridgeMenu(t *testing.T, pf *panel.PanelsFrame, path string) (*vtui.VMenu, *historySearch) {
	t.Helper()
	RememberViewerEditorHistory(vfs.NewOSVFS(filepath.Dir(path)), path, historyModeView)
	actionViewerEditorHistory(pf)
	menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
	if !ok {
		t.Fatalf("top frame = %T, want the viewer/editor history menu", vtui.FrameManager.GetTopFrame())
	}
	search := activeHistorySearch
	t.Cleanup(func() {
		if activeHistorySearch != nil {
			activeHistorySearch.cleanup()
		}
	})
	return menu, search
}

// TestActionViewerEditorHistory_CtrlF10RevealsAndCloses covers the
// search.onCtrlF10 closure wired in actionViewerEditorHistory: Ctrl+F10
// reveals the selected entry's folder in the panel and closes the dialog,
// the same #408 shortcut the command and folder histories already have.
func TestActionViewerEditorHistory_CtrlF10RevealsAndCloses(t *testing.T) {
	withStubHistoryProvider(t)
	pf, fsp, _ := setupHistoryBridgeTestPanel(t)
	other := t.TempDir()
	target := writeHistoryFile(t, other, "target.txt")

	menu, _ := openHistoryBridgeMenu(t, pf, target)
	menu.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_F10, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	pumpUntil(t, "the panel to reveal the history entry's folder", func() bool {
		return filepath.Clean(fsp.Vfs.GetPath()) == filepath.Clean(other)
	})
	if !menu.IsDone() {
		t.Fatal("Ctrl+F10 did not close the history dialog")
	}
}

// TestActionViewerEditorHistory_ReturnAndFunctionKeysOpenWithOverride covers
// openCurrent through the three keys that drive it: Return opens with the
// entry's own remembered mode, F3 forces view, F4 forces edit.
func TestActionViewerEditorHistory_ReturnAndFunctionKeysOpenWithOverride(t *testing.T) {
	tests := []struct {
		name     string
		key      uint16
		wantMode viewerEditorHistoryMode
	}{
		{"Return opens with the entry's own mode", vtinput.VK_RETURN, HistoryModeEdit},
		{"F3 forces view", vtinput.VK_F3, historyModeView},
		{"F4 forces edit", vtinput.VK_F4, HistoryModeEdit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withStubHistoryProvider(t)
			pf, _, dir := setupHistoryBridgeTestPanel(t)
			path := writeHistoryFile(t, dir, "a.txt")
			RememberViewerEditorHistory(vfs.NewOSVFS(dir), path, HistoryModeEdit)

			actionViewerEditorHistory(pf)
			menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
			if !ok {
				t.Fatalf("top frame = %T, want the history menu", vtui.FrameManager.GetTopFrame())
			}
			t.Cleanup(func() {
				if activeHistorySearch != nil {
					activeHistorySearch.cleanup()
				}
			})

			menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: tc.key})

			if !menu.IsDone() {
				t.Fatal("opening the entry did not close the dialog")
			}
			entries := loadViewerEditorHistory()
			if len(entries) != 1 || entries[0].Mode != tc.wantMode {
				t.Fatalf("history after open = %#v, want mode %q", entries, tc.wantMode)
			}
			// Let the async open this triggered land instead of leaving its
			// goroutine to write to a channel nobody drains, and close what it
			// opened so the temp dir can be removed on Windows.
			closeOpenedHistoryFile(t, path, tc.wantMode)
		})
	}
}

// TestActionViewerEditorHistory_EscAndPlainF10CleanUpWithoutClosing covers
// the branch that lets the dialog's own default Escape/F10 handling take
// over: our OnKeyDown only tears down the search overlay it installed
// (activeHistorySearch) and reports the key unhandled.
func TestActionViewerEditorHistory_EscAndPlainF10CleanUpWithoutClosing(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  uint16
	}{
		{"Escape", vtinput.VK_ESCAPE},
		{"plain F10", vtinput.VK_F10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withStubHistoryProvider(t)
			pf, _, dir := setupHistoryBridgeTestPanel(t)
			menu, _ := openHistoryBridgeMenu(t, pf, filepath.Join(dir, "a.txt"))

			// Ask our OnKeyDown directly: menu.ProcessKey would report the key
			// handled, because the VMenu's own default Escape/F10 handling
			// (closing the dialog) takes over exactly when ours declines it.
			handled := menu.OnKeyDown(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: tc.key})
			if handled {
				t.Fatalf("%s was reported as handled by the viewer/editor history action", tc.name)
			}
			if activeHistorySearch != nil {
				t.Fatal("the search overlay was not cleaned up")
			}
		})
	}
}

// setupTwoEntryHistoryMenu remembers two files (the second, "locked.txt",
// most recently, so it is index 0 and the dialog's default selection) and
// opens the viewer/editor history dialog on them.
func setupTwoEntryHistoryMenu(t *testing.T) (menu *vtui.VMenu, locked, other string) {
	t.Helper()
	withStubHistoryProvider(t)
	pf, _, dir := setupHistoryBridgeTestPanel(t)
	other = filepath.Join(dir, "other.txt")
	locked = filepath.Join(dir, "locked.txt")
	RememberViewerEditorHistory(vfs.NewOSVFS(dir), other, historyModeView)
	RememberViewerEditorHistory(vfs.NewOSVFS(dir), locked, historyModeView)

	actionViewerEditorHistory(pf)
	var ok bool
	menu, ok = vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
	if !ok {
		t.Fatalf("top frame = %T, want the history menu", vtui.FrameManager.GetTopFrame())
	}
	t.Cleanup(func() {
		if activeHistorySearch != nil {
			activeHistorySearch.cleanup()
		}
	})
	return menu, locked, other
}

// TestActionViewerEditorHistory_PlainDeleteCancelKeepsEverything covers the
// unmodified Delete key: it asks before wiping the whole history, unlike
// Shift+Delete (TestViewerEditorHistoryDialogDeletesEntry in
// viewer_editor_history_test.go), which drops just the selected entry — and
// Cancel leaves both entries untouched.
func TestActionViewerEditorHistory_PlainDeleteCancelKeepsEverything(t *testing.T) {
	menu, _, _ := setupTwoEntryHistoryMenu(t)

	menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE})
	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm.OnResult == nil {
		t.Fatalf("top frame = %#v, want the clear-all confirmation", vtui.FrameManager.GetTopFrame())
	}
	if confirm.GetTitle() != i18n.Msg("History.ViewEditTitle") {
		t.Fatalf("confirm title = %q", confirm.GetTitle())
	}

	confirm.OnResult(1) // Cancel
	if entries := loadViewerEditorHistory(); len(entries) != 2 {
		t.Fatalf("Cancel changed the history: %#v", entries)
	}
}

// TestActionViewerEditorHistory_PlainDeleteOkKeepsOnlyLockedEntries covers
// the same confirm dialog's Ok branch: everything unlocked is wiped, a
// locked entry (Insert, the same star the other histories use) survives.
func TestActionViewerEditorHistory_PlainDeleteOkKeepsOnlyLockedEntries(t *testing.T) {
	menu, locked, _ := setupTwoEntryHistoryMenu(t)

	// Lock the default selection, the most recently remembered entry
	// ("locked.txt", index 0 — see setupTwoEntryHistoryMenu).
	menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_INSERT})

	menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE})
	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want the clear-all confirmation", vtui.FrameManager.GetTopFrame())
	}
	confirm.OnResult(0) // Ok

	entries := loadViewerEditorHistory()
	if len(entries) != 1 || entries[0].Path != locked || !entries[0].Lock {
		t.Fatalf("Ok kept = %#v, want only the locked entry %q", entries, locked)
	}
}

// TestActionViewerEditorHistory_PlainDeleteClearsMenuWhenNothingIsLocked
// covers the branch confirmAndClearViewerEditorHistory takes when no entry
// was pinned: the dialog has nothing left to show, so it closes itself
// instead of leaving an empty list on screen.
func TestActionViewerEditorHistory_PlainDeleteClearsMenuWhenNothingIsLocked(t *testing.T) {
	withStubHistoryProvider(t)
	pf, _, dir := setupHistoryBridgeTestPanel(t)
	menu, _ := openHistoryBridgeMenu(t, pf, filepath.Join(dir, "a.txt"))

	menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE})
	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want the clear-all confirmation", vtui.FrameManager.GetTopFrame())
	}
	confirm.OnResult(0)

	if entries := loadViewerEditorHistory(); len(entries) != 0 {
		t.Fatalf("history not cleared: %#v", entries)
	}
	if !menu.IsDone() {
		t.Fatal("the dialog stayed open with nothing left to show")
	}
}

// TestActionViewerEditorHistory_CtrlCCopiesPathToClipboard covers the last
// unmodified-key branch: Ctrl+C (and Ctrl+Insert, its usual twin) copies the
// selected entry's path, the same as the command history's own shortcut
// (TestActionCommandHistory_CtrlIns_CopiesToClipboard).
func TestActionViewerEditorHistory_CtrlCCopiesPathToClipboard(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  uint16
	}{
		{"Ctrl+C", vtinput.VK_C},
		{"Ctrl+Insert", vtinput.VK_INSERT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withStubHistoryProvider(t)
			pf, _, dir := setupHistoryBridgeTestPanel(t)
			path := filepath.Join(dir, "a.txt")
			menu, _ := openHistoryBridgeMenu(t, pf, path)
			vtui.SetClipboard("")

			menu.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true,
				VirtualKeyCode: tc.key, ControlKeyState: vtinput.LeftCtrlPressed,
			})
			if got := waitForHistoryClipboard(t, path); got != path {
				t.Errorf("clipboard = %q, want %q", got, path)
			}
		})
	}
}

// TestActionViewerEditorHistory_UnhandledKeyIsNotConsumed is the fallback at
// the end of OnKeyDown: an ordinary character with no meaning to this dialog
// is left for the menu's own default handling, not swallowed.
func TestActionViewerEditorHistory_UnhandledKeyIsNotConsumed(t *testing.T) {
	withStubHistoryProvider(t)
	pf, _, dir := setupHistoryBridgeTestPanel(t)
	menu, _ := openHistoryBridgeMenu(t, pf, filepath.Join(dir, "a.txt"))

	if menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("an unrelated key was reported as handled")
	}
}
