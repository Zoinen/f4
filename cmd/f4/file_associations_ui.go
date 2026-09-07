package main

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// tryFileAssociation is the intercept called at the top of the Enter /
// F3 / F4 dispatchers. It returns true when it fully handles the file
// (a single association matched and was run, or the picker was shown
// with matches to choose from); on false the caller falls back to its
// default behaviour (spawn/xdg-open/built-in viewer/editor).
//
// The active FileSystemPanel must be the one holding the file: token
// substitution snapshots active/passive panels via snapshotPanel.
func tryFileAssociation(pf *PanelsFrame, kind AssocKind) bool {
	if pf == nil {
		return false
	}
	fsp := pf.getActivePanel()
	if fsp == nil {
		return false
	}
	idx := fsp.GetCursorIndex()
	if idx < 0 || idx >= len(fsp.entries) {
		return false
	}
	entry := fsp.entries[idx]
	// Directories never get associations: their Enter/F3/F4 semantics
	// are directory-specific (cd, size calc, attributes).
	if entry.IsDir {
		return false
	}
	name := fsp.GetSelectedName()
	if name == "" || name == ".." {
		return false
	}

	list, err := LoadAssociations(AssociationsFilePath())
	if err != nil || len(list) == 0 {
		return false
	}
	matches := MatchingAssociations(list, name, kind)
	if len(matches) == 0 {
		return false
	}

	if len(matches) == 1 {
		runAssociationCommand(pf, matches[0], kind)
		return true
	}
	showAssociationPicker(pf, matches, kind)
	return true
}

// runAssociationCommand hands the command from a single matched slot
// to executeMenuCommands. That gives us token substitution (!.!, !\!,
// !?prompt?…!, everything user_menu_subst already supports), history,
// PTY wiring, OSC 133, panel hiding, and terminal muting for free.
func runAssociationCommand(pf *PanelsFrame, a FileAssoc, kind AssocKind) {
	cmd := strings.TrimSpace(a.Commands[kind])
	if cmd == "" {
		return
	}
	executeMenuCommands(pf, []string{cmd})
}

// showAssociationPicker pops a VMenu with one row per matched
// association ("<description>  |  <command>"). Enter runs the pick.
// The bottom hint mirrors far2l's picker (description on the left,
// command preview after a vertical bar).
func showAssociationPicker(pf *PanelsFrame, matches []FileAssoc, kind AssocKind) {
	title := " " + assocPickerTitle(kind) + " "
	menu := vtui.NewVMenu(title)

	// Column widths: pad descriptions to the widest so the "│" line
	// aligns vertically. Command preview follows with runewidth clipping.
	descW := 0
	for _, a := range matches {
		w := runewidth.StringWidth(assocDisplayLabel(a))
		if w > descW {
			descW = w
		}
	}
	if descW > 32 {
		descW = 32
	}

	for i, a := range matches {
		label := assocDisplayLabel(a)
		labelPad := label
		pad := descW - runewidth.StringWidth(label)
		if pad > 0 {
			labelPad = label + strings.Repeat(" ", pad)
		}
		preview := strings.TrimSpace(a.Commands[kind])
		text := fmt.Sprintf("%s  │  %s", labelPad, preview)
		menu.AddItem(vtui.MenuItem{Text: text, UserData: i})
	}

	// Size the menu around the widest row, capped to the screen.
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	maxRowW := runewidth.StringWidth(title) + 4
	for _, it := range menu.Items {
		if w := runewidth.StringWidth(it.Text) + 6; w > maxRowW {
			maxRowW = w
		}
	}
	if maxRowW > scrW-4 {
		maxRowW = scrW - 4
	}
	h := len(matches) + 2
	if maxH := scrH - 4; h > maxH && maxH > 5 {
		h = maxH
	}
	x := (scrW - maxRowW) / 2
	y := (scrH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	menu.SetPosition(x, y, x+maxRowW-1, y+h-1)

	menu.OnAction = func(row int) {
		menu.Close()
		if row < 0 || row >= len(menu.Items) {
			return
		}
		pickIdx, ok := menu.Items[row].UserData.(int)
		if !ok || pickIdx < 0 || pickIdx >= len(matches) {
			return
		}
		vtui.FrameManager.PostTask(func() {
			runAssociationCommand(pf, matches[pickIdx], kind)
		})
	}
	// Esc must close cleanly. VMenu default handles this, but we keep
	// the redraw explicit so no extra key is needed to repaint.
	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if e.KeyDown && e.VirtualKeyCode == vtinput.VK_ESCAPE {
			menu.Close()
			return true
		}
		return false
	}
	vtui.FrameManager.PushMenu(menu)
}

// assocDisplayLabel picks the best label for a picker / list row:
// Description first, then Mask. Empty entries fall back to a marker
// so the row is at least selectable.
func assocDisplayLabel(a FileAssoc) string {
	if s := strings.TrimSpace(a.Description); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.Mask); s != "" {
		return s
	}
	return "(unnamed)"
}

// assocPickerTitle localises the picker title per action kind. Kept
// simple: the user is picking "how to open / view / edit this file",
// and the title reflects that verb.
func assocPickerTitle(kind AssocKind) string {
	switch kind {
	case AssocView, AssocAltView:
		return Msg("FileAssoc.PickTitle.View")
	case AssocEdit, AssocAltEdit:
		return Msg("FileAssoc.PickTitle.Edit")
	default:
		return Msg("FileAssoc.PickTitle.Open")
	}
}
