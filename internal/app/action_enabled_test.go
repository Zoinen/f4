package app

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// shareCapableVFS wraps *vfs.OSVFS with a no-op vfs.ShareLinkProvider so
// TestShareLinkEnabled can exercise shareLinkEnabled's VFS-support branch
// without a real cloud backend. Its three methods are never actually called
// -- shareLinkEnabled only does a type assertion -- so their bodies are
// unreachable stand-ins.
type shareCapableVFS struct {
	*vfs.OSVFS
}

func (shareCapableVFS) ShareLinkInfo(context.Context, string) (vfs.ShareLinkInfo, error) {
	return vfs.ShareLinkInfo{}, nil
}

func (shareCapableVFS) CreateShareLink(context.Context, string, vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	return vfs.ShareLink{}, nil
}

func (shareCapableVFS) RevokeShareLink(context.Context, string) error { return nil }

var _ vfs.ShareLinkProvider = shareCapableVFS{}

// TestMenuHonoursEnabled checks the menu half of the mechanism (f4#1356): an
// action with Enabled()==false stays in the menu -- unlike Visible, which
// removes the item -- but its vtui.MenuItem comes back Disabled, which is
// what dims the row and blocks its own activation inside vtui.
func TestMenuHonoursEnabled(t *testing.T) {
	preserveActionRegistry(t)
	enabled := true
	action.RegisterAction(action.Action{
		Name:     "Test.Enabled.MenuItem",
		Area:     "Shell",
		Label:    "Sometimes Enabled",
		MenuPath: "Commands",
		Enabled:  func() bool { return enabled },
		Handler:  func() bool { return true },
	})

	findDisabled := func() (found, disabled bool) {
		for _, m := range BuildMenuBarItems("Shell") {
			for _, it := range m.SubItems {
				if plainMenuText(it.Text) == "Sometimes Enabled" {
					return true, it.Disabled
				}
			}
		}
		return false, false
	}

	found, disabled := findDisabled()
	if !found {
		t.Fatal("an enabled action's menu item is missing")
	}
	if disabled {
		t.Error("an enabled action's menu item must not be Disabled")
	}

	enabled = false
	found, disabled = findDisabled()
	if !found {
		t.Fatal("a disabled action's menu item must stay in the menu, only dimmed")
	}
	if !disabled {
		t.Error("a disabled action's menu item must come back Disabled")
	}
}

// TestRunActionHonoursEnabled checks the hotkey/activation half of the
// mechanism: RunAction refuses to call Handler at all once Enabled() is
// false, regardless of who called it (menu OnClick, a resolved hotkey, the
// key bar) -- this is what replaces the old silent no-op with a command that
// visibly can't be invoked.
func TestRunActionHonoursEnabled(t *testing.T) {
	preserveActionRegistry(t)
	enabled := true
	called := false
	action.RegisterAction(action.Action{
		Name:    "Test.Enabled.RunAction",
		Area:    "Shell",
		Label:   "Test Run",
		Enabled: func() bool { return enabled },
		Handler: func() bool { called = true; return true },
	})

	if !RunAction("Test.Enabled.RunAction") {
		t.Error("an enabled action should run")
	}
	if !called {
		t.Error("an enabled action's Handler should have run")
	}

	called = false
	enabled = false
	if RunAction("Test.Enabled.RunAction") {
		t.Error("a disabled action must not report success")
	}
	if called {
		t.Error("a disabled action's Handler must never run")
	}
}

// TestRunActionWithoutEnabledIsUnaffected makes sure the vast majority of
// actions, which set no Enabled at all, keep running exactly as before.
func TestRunActionWithoutEnabledIsUnaffected(t *testing.T) {
	preserveActionRegistry(t)
	called := false
	action.RegisterAction(action.Action{
		Name:    "Test.Enabled.Unset",
		Area:    "Shell",
		Label:   "No Enabled Hook",
		Handler: func() bool { called = true; return true },
	})

	if !RunAction("Test.Enabled.Unset") {
		t.Error("an action with no Enabled predicate should run")
	}
	if !called {
		t.Error("an action with no Enabled predicate should have its Handler run")
	}
}

// TestActivePanelHasSelectionTarget checks the condition function wired to
// File.Attributes (Ctrl+A), File.Copy (F5), File.Move (F6), File.Delete (F8)
// and File.DeletePermanent (Shift+Del): it must mirror
// FileSystemPanel.GetSelectedNames exactly, since that is the same rule
// actionFileAttributes, actionCopyMove and actionDeleteWithDisposition use to
// decide whether they have anything to act on (f4#1356's silent no-op).
func TestActivePanelHasSelectionTarget(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	fsp := pf.GetActivePanel()
	fsp.Vfs = vfs.NewOSVFS(t.TempDir())
	fsp.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file.txt"}},
	}

	fsp.SetCursorIndex(0)
	if activePanelHasSelectionTarget() {
		t.Error("cursor on \"..\" with nothing marked should have no target")
	}

	fsp.SetCursorIndex(1)
	if !activePanelHasSelectionTarget() {
		t.Error("cursor on a real entry should be a target")
	}

	fsp.SetCursorIndex(0)
	fsp.Entries[1].Selected = true
	if !activePanelHasSelectionTarget() {
		t.Error("an explicitly marked entry should count as a target even with the cursor back on \"..\"")
	}
}

// TestActivePanelHasSelectionTarget_NoPanelsFrame makes sure the predicate
// degrades to "no target" rather than panicking when asked outside any
// panels frame (e.g. a full-screen editor/viewer with no panel behind it in
// this workspace).
func TestActivePanelHasSelectionTarget_NoPanelsFrame(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	if activePanelHasSelectionTarget() {
		t.Error("with no panels frame at all, there is no target")
	}
}

// TestFileCopyMoveEnabled checks the Enabled predicate wired to File.Copy
// (F5) and File.Move (F6): it must mirror actionCopyMove's own refusal
// branches for a passive player-panel playlist (Player.MoveRefused /
// Player.LocalOnly) so the key and menu item dim instead of the hotkey
// opening one of those error dialogs (f4#1356, part 3).
func TestFileCopyMoveEnabled(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	fsp := pf.GetActivePanel()
	fsp.Vfs = vfs.NewOSVFS(t.TempDir())
	fsp.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file.txt"}},
	}
	fsp.SetCursorIndex(0)

	if fileCopyMoveEnabled(false)() || fileCopyMoveEnabled(true)() {
		t.Error("cursor on \"..\" with nothing marked should disable both Copy and Move")
	}

	fsp.SetCursorIndex(1)
	if !fileCopyMoveEnabled(false)() || !fileCopyMoveEnabled(true)() {
		t.Error("a real target with no player panel opposite should enable both Copy and Move")
	}

	// A player panel opposite: F6 (move) must stay disabled regardless of
	// the source, and F5 (copy) only for a non-local source.
	//
	// The player panel is built through the same panel.NewPlayerPanel
	// constructor actionCopyMove/ToggleAltPanel use in production, not a
	// bare &panel.PlayerPanel{} literal: PlayerPanel.Close (called on every
	// AltPanel by PanelsFrame.Close, including the deferred pf.Close below
	// and paneltest.SwapFrameManager's own cleanup) unconditionally closes
	// its unexported stop channel, which panics with "close of nil channel"
	// on a zero-value PlayerPanel that never went through the constructor
	// (found via PR #1613's Test (linux/amd64) job, which is where this
	// literal actually ran Close() for the first time).
	pf.AltPanels[1-pf.ActiveIdx] = panel.NewPlayerPanel(fsp)
	if fileCopyMoveEnabled(true)() {
		t.Error("Move must stay disabled with a player panel opposite (Player.MoveRefused)")
	}
	if !fileCopyMoveEnabled(false)() {
		t.Error("Copy from a local filesystem must stay enabled with a player panel opposite")
	}

	fsp.Vfs = vfs.NewNullVFS(0)
	if fileCopyMoveEnabled(false)() {
		t.Error("Copy from a non-local filesystem must disable with a player panel opposite (Player.LocalOnly)")
	}

	// The tree (Ctrl+T) focused in the active slot, with the player panel
	// from above still sitting in the opposite slot: F5/F6 must target the
	// tree's highlighted node and stay enabled, exactly as with no player
	// opposite at all. fileCopyMoveEnabled and actionCopyMove both check
	// treeCopyMoveTarget before ever looking at the opposite slot's
	// PlayerPanel, so the two f4#1356/f4#1602 "part 3" checks act on
	// mutually exclusive destinations and must not suppress one another.
	fsp.Vfs = vfs.NewOSVFS(t.TempDir())
	tp := panel.NewTreePanel(fsp)
	tp.SetFocus(true)
	pf.AltPanels[pf.ActiveIdx] = tp
	if !fileCopyMoveEnabled(false)() {
		t.Error("Copy should stay enabled with the tree focused even though a player panel occupies the opposite slot")
	}
	if !fileCopyMoveEnabled(true)() {
		t.Error("Move should stay enabled with the tree focused even though a player panel occupies the opposite slot (Player.MoveRefused must not apply to a tree destination)")
	}
}

// TestSymlinkEditEnabled checks the Enabled predicate wired to
// File.EditSymlink: it must mirror actionEditSymlink's own refusal branches
// (SymlinkEdit.OneFile / NotSymlink / Unsupported) so the menu item dims
// instead of the action opening one of those error dialogs (f4#1356, part 4).
func TestSymlinkEditEnabled(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	fsp := pf.GetActivePanel()
	fsp.Vfs = vfs.NewOSVFS(t.TempDir())
	fsp.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file.txt"}},
		{VFSItem: vfs.VFSItem{Name: "link", IsSymlink: true}},
	}

	fsp.SetCursorIndex(0)
	if symlinkEditEnabled() {
		t.Error("cursor on \"..\" with nothing marked should disable Edit Symlink")
	}

	fsp.SetCursorIndex(1)
	if symlinkEditEnabled() {
		t.Error("a regular file target must disable Edit Symlink (SymlinkEdit.NotSymlink)")
	}

	fsp.SetCursorIndex(2)
	if !symlinkEditEnabled() {
		t.Error("a symlink target on a SymlinkVFS-capable panel must enable Edit Symlink")
	}

	fsp.SetItemSelected(1, true)
	fsp.SetItemSelected(2, true)
	if symlinkEditEnabled() {
		t.Error("more than one marked item must disable Edit Symlink (SymlinkEdit.OneFile)")
	}

	fsp.SetItemSelected(1, false)
	fsp.Vfs = vfs.NewNullVFS(0)
	if symlinkEditEnabled() {
		t.Error("a VFS without SymlinkVFS support must disable Edit Symlink (SymlinkEdit.Unsupported)")
	}
}

// TestShareLinkEnabled checks the Enabled predicate wired to File.Share: it
// must mirror actionShareLink's own remaining refusal branch -- exactly one
// selected entry (Share.SelectOne) -- on top of the vfs.ShareLinkProvider
// support check File.Share's Visible predicate already applies separately,
// since Enabled is consulted independently of Visible (f4#1356, part 5).
func TestShareLinkEnabled(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	fsp := pf.GetActivePanel()
	fsp.Vfs = shareCapableVFS{vfs.NewOSVFS(t.TempDir())}
	fsp.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file.txt"}},
		{VFSItem: vfs.VFSItem{Name: "other.txt"}},
	}

	fsp.SetCursorIndex(0)
	if shareLinkEnabled() {
		t.Error("cursor on \"..\" with nothing marked should disable Share")
	}

	fsp.SetCursorIndex(1)
	if !shareLinkEnabled() {
		t.Error("a single target on a ShareLinkProvider-capable panel must enable Share")
	}

	fsp.SetItemSelected(1, true)
	fsp.SetItemSelected(2, true)
	if shareLinkEnabled() {
		t.Error("more than one marked item must disable Share (Share.SelectOne)")
	}

	fsp.SetItemSelected(2, false)
	fsp.Vfs = vfs.NewOSVFS(t.TempDir())
	if shareLinkEnabled() {
		t.Error("a VFS without ShareLinkProvider support must disable Share")
	}
}

// TestCursorEntryAndOneRegularFileEnabled covers the predicates behind F3, F4,
// Create Link, Rename and the Base64 commands (f4#1356, viklequick's list):
// the cursor on ".." dims them; a folder keeps F3/F4 (size, attributes) but
// not the Base64 commands, which want exactly one regular file.
func TestCursorEntryAndOneRegularFileEnabled(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	if cursorEntryEnabled() || oneRegularFileEnabled() {
		t.Error("with no panels frame at all, nothing is enabled")
	}

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	fsp := pf.GetActivePanel()
	fsp.Vfs = vfs.NewOSVFS(t.TempDir())
	fsp.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "dir", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "a.txt"}},
		{VFSItem: vfs.VFSItem{Name: "b.txt"}},
	}

	fsp.SetCursorIndex(0)
	if cursorEntryEnabled() || oneRegularFileEnabled() {
		t.Error("the cursor on \"..\" should dim F3/F4 and the Base64 commands")
	}
	fsp.SetCursorIndex(1)
	if !cursorEntryEnabled() {
		t.Error("F3/F4 on a folder are size and attributes, so they stay enabled")
	}
	if oneRegularFileEnabled() {
		t.Error("a folder is not a regular file for the Base64 commands")
	}
	fsp.SetCursorIndex(2)
	if !cursorEntryEnabled() || !oneRegularFileEnabled() {
		t.Error("a regular file enables all of them")
	}
	fsp.Entries[2].Selected = true
	fsp.Entries[3].Selected = true
	if oneRegularFileEnabled() {
		t.Error("two marked files are not \"exactly one file\"")
	}
	fsp.Entries = nil
	if cursorEntryEnabled() {
		t.Error("an empty list has no entry under the cursor")
	}
}
