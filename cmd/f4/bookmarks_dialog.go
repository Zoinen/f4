package main

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Bookmarks dialog: ten fixed rows, one per slot, reachable from
// F9 → Commands → Bookmarks. Same layout and keys as far2l's
// BookmarksMenu (bookmarks/BookmarksMenu.cpp) — the port is of the UX,
// not of the code.

// bookmarksFrame wraps a vtui.VMenu so the dialog can draw a hotkey hint
// on its bottom border without extending vtui itself, exactly the way
// userMenuFrame does it for the F2 menu.
type bookmarksFrame struct {
	*vtui.VMenu
	bottomHint string
	onClose    func()
}

// IsDone doubles as the close hook. vtui has no OnClose on VMenu and Esc
// closes through the embedded menu's own SetExitCode, which a wrapper
// cannot intercept — but the frame manager drops a frame the moment
// IsDone reports true, so that is the point where "after the dialog goes
// away" work can be queued. Fires once.
func (b *bookmarksFrame) IsDone() bool {
	done := b.VMenu.IsDone()
	if done && b.onClose != nil {
		cb := b.onClose
		b.onClose = nil
		vtui.FrameManager.PostTask(cb)
	}
	return done
}

func (b *bookmarksFrame) Show(scr *vtui.ScreenBuf) {
	b.VMenu.Show(scr)
	if b.bottomHint == "" {
		return
	}
	x1, _, x2, y2 := b.GetPosition()
	vtui.NewPainter(scr).DrawTitle(x1, y2, x2, b.bottomHint, vtui.Palette[vtui.ColMenuTitle])
}

// bookmarksDialog owns the table as loaded from disk plus the menu that
// displays it. Every mutation writes straight back to file, so there is
// no "apply" step and nothing to undo on Esc.
type bookmarksDialog struct {
	pf   *PanelsFrame
	file string
	set  BookmarkSet
	menu *vtui.VMenu
}

// ShowBookmarksDialog is the entry point wired to CmBookmarks.
func ShowBookmarksDialog(pf *PanelsFrame) {
	ShowBookmarksDialogAt(pf, 0, nil)
}

// ShowBookmarksDialogAt opens the dialog with the cursor on a given slot
// and runs onClose (may be nil) once the dialog is gone. The drive menu
// uses both: far2l's F4 there opens this dialog on the slot under the
// cursor and returns to the menu afterwards.
func ShowBookmarksDialogAt(pf *PanelsFrame, slot int, onClose func()) {
	d, err := newBookmarksDialog(pf, BookmarksFilePath())
	if err != nil {
		vtui.ShowMessage(Msg("Bookmarks.Title"),
			fmt.Sprintf(Msg("Bookmarks.LoadError"), err),
			[]string{"&Ok"})
		if onClose != nil {
			onClose()
		}
		return
	}
	d.open(slot, onClose)
}

// newBookmarksDialog reads the table from path. The error is returned
// rather than displayed so the caller decides how to surface it — and so
// this half can be exercised without a live UI.
func newBookmarksDialog(pf *PanelsFrame, path string) (*bookmarksDialog, error) {
	set, err := LoadBookmarks(path)
	if err != nil {
		return nil, err
	}
	return &bookmarksDialog{pf: pf, file: path, set: set}, nil
}

// open builds the menu and pushes it as a modal frame, cursor on slot.
func (d *bookmarksDialog) open(slot int, onClose func()) {
	// Empty rows carry CmBookmarkEmptySlot, which is permanently disabled:
	// vtui then draws them dimmed and swallows Enter on them, which is
	// exactly the "empty slot is a no-op" behavior far2l has. No other
	// menu uses this command, so it never needs re-enabling.
	vtui.FrameManager.DisabledCommands.Disable(CmBookmarkEmptySlot)

	d.menu = vtui.NewVMenu(Msg("Bookmarks.Title"))
	d.render()
	d.menu.SetSelectPos(slot)

	w, h := d.size()
	x := (d.pf.lastW - w) / 2
	y := (d.pf.lastH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	d.menu.SetPosition(x, y, x+w-1, y+h-1)

	d.menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if !e.KeyDown {
			return false
		}
		shift := e.ControlKeyState&vtinput.ShiftPressed != 0
		ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0

		slot := d.slotAt(d.menu.SelectPos)

		// Ins stores the panel's directory, Del clears the slot, F4 edits
		// the path by hand. All three overwrite without asking, as far2l
		// does, and each one hits the disk immediately.
		if !ctrl && !shift && !alt {
			switch e.VirtualKeyCode {
			case vtinput.VK_INSERT:
				d.saveCurrentDir(slot)
				return true
			case vtinput.VK_DELETE:
				d.clearSlot(slot)
				return true
			case vtinput.VK_F4:
				d.editPath(slot)
				return true
			}
		}

		// Shift+Up / Shift+Down swap the slot with its neighbour and take
		// the cursor along, so it stays on the same bookmark.
		if shift && !ctrl && !alt {
			switch e.VirtualKeyCode {
			case vtinput.VK_UP:
				d.moveSlot(slot, -1)
				return true
			case vtinput.VK_DOWN:
				d.moveSlot(slot, +1)
				return true
			}
		}

		// Keys the top frame declines fall through to vtui's own global
		// handlers — F1 opens help, F9 activates the menu bar (which a
		// TypeMenu frame does not block), F12 opens the screen list.
		// None of that belongs on top of a modal dialog, so the whole
		// F-key range is swallowed, as the F2 user menu does. F10 is
		// left to vtui, which closes the menu with it.
		if !ctrl && !shift && !alt &&
			e.VirtualKeyCode >= vtinput.VK_F1 && e.VirtualKeyCode <= vtinput.VK_F12 &&
			e.VirtualKeyCode != vtinput.VK_F10 {
			return true
		}
		return false
	}

	// Enter (and a mouse click) on a populated row: vtui pops the menu on
	// its own once we return, so the navigation is posted for after the
	// frame stack has settled. Empty rows never get here — their command
	// is disabled, so vtui swallows the key first.
	d.menu.OnAction = func(uiIdx int) {
		slot := d.slotAt(uiIdx)
		if slot < 0 || d.set[slot].IsEmpty() {
			return
		}
		bookmark := d.set[slot]
		pf := d.pf
		vtui.FrameManager.PostTask(func() {
			if fsp := pf.getActivePanel(); fsp != nil {
				pf.navigateToBookmark(fsp, bookmark)
			}
		})
	}

	vtui.FrameManager.PushMenu(&bookmarksFrame{
		VMenu:      d.menu,
		bottomHint: Msg("Bookmarks.BottomHint"),
		onClose:    onClose,
	})
}

// render rebuilds all ten rows in place, keeping the cursor where it was.
// The frame may already be on screen: vtui picks the new items up on the
// next redraw.
func (d *bookmarksDialog) render() {
	pos := d.menu.SelectPos
	d.menu.Items = nil
	d.menu.ItemCount = 0
	for i := range d.set {
		item := vtui.MenuItem{Text: d.rowText(i), UserData: i}
		if d.set[i].IsEmpty() {
			item.Command = CmBookmarkEmptySlot
		}
		d.menu.AddItem(item)
	}
	d.menu.SetSelectPos(pos)
}

// rowText formats one row: the hotkey reminder far2l prints in its own
// dialog title, the slot digit, then the path (or the empty marker).
func (d *bookmarksDialog) rowText(slot int) string {
	path := d.set[slot].Path
	if path == "" {
		path = Msg("Bookmarks.EmptySlot")
	}
	return fmt.Sprintf("%s %d   %s", Msg("Bookmarks.RowPrefix"), slot, escapeAmpersand(path))
}

// size returns the menu box dimensions: wide enough for the longest row
// and for the bottom hint, tall enough for all ten slots, clamped to the
// console — same shape of arithmetic as menuSize in user_menu_ui.go.
func (d *bookmarksDialog) size() (int, int) {
	w := 60
	for i := range d.set {
		if rw := runewidth.StringWidth(d.rowText(i)) + 4; rw > w {
			w = rw
		}
	}
	if minForHint := runewidth.StringWidth(Msg("Bookmarks.BottomHint")) + 2; w < minForHint {
		w = minForHint
	}
	if d.pf.lastW > 0 && w > d.pf.lastW-4 {
		w = d.pf.lastW - 4
	}
	if w < 24 {
		w = 24
	}

	h := len(d.set) + 2
	maxH := d.pf.lastH - 6
	if maxH < 5 {
		maxH = 5
	}
	if h > maxH {
		h = maxH
	}
	return w, h
}

// saveCurrentDir records the active panel's directory in the slot.
func (d *bookmarksDialog) saveCurrentDir(slot int) {
	fsp := d.pf.getActivePanel()
	if fsp == nil || slot < 0 {
		return
	}
	d.set.setCurrentDir(slot, fsp.vfs.GetPath())
	d.persist()
}

// clearSlot empties the slot. No confirmation, matching far2l.
func (d *bookmarksDialog) clearSlot(slot int) {
	if slot < 0 {
		return
	}
	d.set.deleteAtSlot(slot)
	d.persist()
}

// editPath asks for a path and stores it in the slot. Empty input counts
// as a cancel rather than "clear the slot" — that is what Del is for.
func (d *bookmarksDialog) editPath(slot int) {
	if slot < 0 || slot >= len(d.set) {
		return
	}
	current := d.set[slot].Path
	vtui.InputBox(Msg("Bookmarks.EditTitle"), Msg("Bookmarks.EditPrompt"), current, func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		d.set.setCurrentDir(slot, text)
		d.persist()
	})
}

// moveSlot swaps the slot with the neighbour delta rows away and moves
// the cursor with it. A move off either end is a no-op.
func (d *bookmarksDialog) moveSlot(slot, delta int) {
	target := slot + delta
	if slot < 0 || target < 0 || target >= len(d.set) {
		return
	}
	d.set.swapSlots(slot, target)
	if d.persist() {
		d.menu.SetSelectPos(target)
	}
}

// persist writes the table back and re-renders. On a write failure the
// on-disk state wins: the in-memory copy is reloaded so the dialog never
// shows changes that were not saved.
func (d *bookmarksDialog) persist() bool {
	err := SaveBookmarks(d.file, d.set)
	if err != nil {
		vtui.ShowMessage(Msg("Bookmarks.Title"),
			fmt.Sprintf(Msg("Bookmarks.SaveError"), err),
			[]string{"&Ok"})
		if reloaded, lerr := LoadBookmarks(d.file); lerr == nil {
			d.set = reloaded
		}
	}
	d.render()
	vtui.FrameManager.Redraw()
	return err == nil
}

// deleteAtSlot clears the slot, leaving the rest of the table alone.
func (s *BookmarkSet) deleteAtSlot(i int) {
	if i < 0 || i >= len(s) {
		return
	}
	s[i] = Bookmark{}
}

// swapSlots exchanges two slots. Out-of-range indices are ignored so
// callers can pass "cursor ± 1" without bounds-checking first.
func (s *BookmarkSet) swapSlots(a, b int) {
	if a < 0 || a >= len(s) || b < 0 || b >= len(s) {
		return
	}
	s[a], s[b] = s[b], s[a]
}

// setCurrentDir stores path in the slot and clears the plugin fields:
// whoever edits a slot from the dialog is recording a filesystem
// directory, not a plugin location.
func (s *BookmarkSet) setCurrentDir(i int, path string) {
	if i < 0 || i >= len(s) {
		return
	}
	s[i] = Bookmark{Path: path}
}

// slotAt maps a menu row to its slot index, or -1 when the row is out of
// range. Rows and slots are 1:1 today; going through UserData keeps that
// an implementation detail.
func (d *bookmarksDialog) slotAt(uiPos int) int {
	if d.menu == nil || uiPos < 0 || uiPos >= len(d.menu.Items) {
		return -1
	}
	slot, ok := d.menu.Items[uiPos].UserData.(int)
	if !ok || slot < 0 || slot >= len(d.set) {
		return -1
	}
	return slot
}
