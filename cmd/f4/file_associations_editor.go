package main

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// assocEditorState holds the mutable slice + backing file for the
// interactive associations editor. Modelled after userMenuState so the
// save/reload story looks the same to a maintainer switching between
// the two features.
type assocEditorState struct {
	pf         *PanelsFrame
	sourcePath string
	items      []FileAssoc
}

// ShowFileAssociations opens the associations editor (F9 → Commands →
// File associations). The list is loaded on entry; each edit persists
// immediately (matches far2l's behaviour of saving on OK per record).
func ShowFileAssociations(pf *PanelsFrame) {
	if pf == nil {
		return
	}
	path := AssociationsFilePath()
	list, err := LoadAssociations(path)
	if err != nil {
		vtui.ShowMessage(" File associations ",
			fmt.Sprintf("Failed to read associations:\n%v", err),
			[]string{"&Ok"})
		return
	}
	s := &assocEditorState{pf: pf, sourcePath: path, items: list}
	s.openList(0)
}

// openList (re)pushes the associations list menu, positioned near the
// screen centre. selected is the cursor row to restore after an edit.
func (s *assocEditorState) openList(selected int) {
	title := " " + Msg("FileAssoc.EditorTitle") + " "
	menu := vtui.NewVMenu(title)

	if len(s.items) == 0 {
		// Placeholder row so the empty menu has something visible. Its
		// UserData=-1 makes selectedIndex() return -1, and Enter on it
		// falls through to the "bootstrap first entry" Ins branch.
		menu.AddItem(vtui.MenuItem{
			Text:     " " + Msg("FileAssoc.EmptyHint") + " ",
			UserData: -1,
		})
	} else {
		descW, maskW := assocColumnWidths(s.items)
		for i, a := range s.items {
			menu.AddItem(vtui.MenuItem{
				Text:     formatAssocRow(a, descW, maskW),
				UserData: i,
			})
		}
	}

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
	if maxRowW < 40 {
		maxRowW = 40
	}
	h := len(menu.Items) + 2
	if h < 5 {
		h = 5
	}
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

	if selected >= 0 && selected < len(s.items) {
		menu.SetSelectPos(selected)
	}

	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if !e.KeyDown {
			return false
		}
		ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		shift := e.ControlKeyState&vtinput.ShiftPressed != 0

		// Ins — add a fresh association.
		if e.VirtualKeyCode == vtinput.VK_INSERT && !ctrl && !alt && !shift {
			insertAt := s.selectedIndex(menu)
			if insertAt < 0 {
				insertAt = len(s.items)
			}
			menu.Close()
			vtui.FrameManager.PostTask(func() {
				s.editAt(insertAt, true)
			})
			return true
		}
		// Del — delete under cursor with confirmation.
		if e.VirtualKeyCode == vtinput.VK_DELETE && !ctrl && !alt && !shift {
			idx := s.selectedIndex(menu)
			if idx < 0 || idx >= len(s.items) {
				return true
			}
			it := s.items[idx]
			dlg := vtui.ShowMessageOn(menu, " "+Msg("FileAssoc.DeleteTitle")+" ",
				fmt.Sprintf(Msg("FileAssoc.DeleteConfirm"), assocDisplayLabel(it)),
				[]string{"&Delete", "Cancel"})
			// Destructive — render on the WarnDialog palette (see #379).
			dlg.IsWarning = true
			dlg.OnResult = func(code int) {
				if code != 0 {
					return
				}
				s.items = append(s.items[:idx], s.items[idx+1:]...)
				if !s.save() {
					return
				}
				menu.Close()
				next := idx
				if next >= len(s.items) {
					next = len(s.items) - 1
				}
				vtui.FrameManager.PostTask(func() {
					s.openList(next)
				})
			}
			return true
		}
		// Ctrl+Up / Ctrl+Down — reorder within the list, no wrap.
		if e.VirtualKeyCode == vtinput.VK_UP && ctrl && !alt && !shift {
			s.reorder(menu, -1)
			return true
		}
		if e.VirtualKeyCode == vtinput.VK_DOWN && ctrl && !alt && !shift {
			s.reorder(menu, +1)
			return true
		}
		// F4 or Enter on an entry — open the edit dialog.
		if (e.VirtualKeyCode == vtinput.VK_F4 || e.VirtualKeyCode == vtinput.VK_RETURN) &&
			!ctrl && !alt && !shift {
			idx := s.selectedIndex(menu)
			if idx < 0 || idx >= len(s.items) {
				// Empty-list Enter falls through to Ins so users can
				// bootstrap without knowing about Ins.
				if len(s.items) == 0 && e.VirtualKeyCode == vtinput.VK_RETURN {
					menu.Close()
					vtui.FrameManager.PostTask(func() {
						s.editAt(0, true)
					})
					return true
				}
				return true
			}
			menu.Close()
			vtui.FrameManager.PostTask(func() {
				s.editAt(idx, false)
			})
			return true
		}
		return false
	}

	// vtui's default OnAction fires on Enter for enabled rows; keep our
	// OnKeyDown handler authoritative so the two paths agree.
	menu.OnAction = func(row int) {
		if row < 0 || row >= len(menu.Items) {
			return
		}
		idx, ok := menu.Items[row].UserData.(int)
		if !ok || idx < 0 || idx >= len(s.items) {
			return
		}
		menu.Close()
		vtui.FrameManager.PostTask(func() {
			s.editAt(idx, false)
		})
	}

	vtui.FrameManager.PushMenu(&userMenuFrame{
		VMenu:      menu,
		bottomHint: " Ins F4 Del Ctrl+Up/Down ",
	})
}

func (s *assocEditorState) selectedIndex(menu *vtui.VMenu) int {
	pos := menu.SelectPos
	if pos < 0 || pos >= len(menu.Items) {
		return -1
	}
	idx, ok := menu.Items[pos].UserData.(int)
	if !ok {
		return -1
	}
	return idx
}

func (s *assocEditorState) reorder(menu *vtui.VMenu, delta int) {
	idx := s.selectedIndex(menu)
	if idx < 0 || idx >= len(s.items) {
		return
	}
	target := idx + delta
	if target < 0 || target >= len(s.items) {
		return
	}
	s.items[idx], s.items[target] = s.items[target], s.items[idx]
	if !s.save() {
		// Roll back so the on-disk file and in-memory list agree.
		s.items[idx], s.items[target] = s.items[target], s.items[idx]
		return
	}
	menu.Close()
	vtui.FrameManager.PostTask(func() {
		s.openList(target)
	})
}

func (s *assocEditorState) save() bool {
	if err := SaveAssociations(s.sourcePath, s.items); err != nil {
		vtui.ShowMessage(" File associations ",
			fmt.Sprintf("Failed to save associations:\n%v", err),
			[]string{"&Ok"})
		return false
	}
	return true
}

// editAt opens the per-entry editor. When isCreate is true, idx marks
// the insertion position for a brand-new record.
func (s *assocEditorState) editAt(idx int, isCreate bool) {
	var work FileAssoc
	if !isCreate && idx >= 0 && idx < len(s.items) {
		work = s.items[idx]
	}

	title := " " + Msg("FileAssoc.EditTitle") + " "
	if isCreate {
		title = " " + Msg("FileAssoc.NewTitle") + " "
	}

	const width = 74
	// Rows: mask (1), description (1), 6 slot rows (checkbox+edit
	// stacked as one visual row each), buttons (1); plus top/bottom
	// padding (2) and label lines. We keep the layout compact by
	// putting each command slot on its own row.
	const height = 4 /*pad+borders*/ + 2 /*mask+desc*/ + 2 /*spacing*/ + assocKindCount + 2 /*buttons*/ + 2 /*bottom padding*/

	dlg := vtui.NewCenteredDialog(width, height, title)
	dlg.ShowClose = true

	editMask := vtui.NewEdit(0, 0, width-4, work.Mask)
	attachHistory(editMask, fileMasksHistoryID)
	editDesc := vtui.NewEdit(0, 0, width-4, work.Description)

	// One (checkbox, edit) per slot. Checkbox label is the far2l key
	// name so the association-in-UI matches the file-on-disk vocabulary.
	slotChecks := [assocKindCount]*vtui.Checkbox{}
	slotEdits := [assocKindCount]*vtui.Edit{}
	for k := 0; k < assocKindCount; k++ {
		labelText := assocSlotLabel(AssocKind(k))
		chk := vtui.NewCheckbox(0, 0, labelText, false)
		if work.Enabled[k] {
			chk.State = 1
		}
		chkX1, _, chkX2, _ := chk.GetPosition()
		chkWidth := chkX2 - chkX1 + 1
		ed := vtui.NewEdit(0, 0, width-4-chkWidth-2, work.Commands[k])
		slotChecks[k] = chk
		slotEdits[k] = ed
	}

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Save"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	// Register widgets. Order matters for Tab traversal, so we register
	// in the visual order (mask → desc → each slot pair → buttons).
	dlg.AddItem(editMask)
	dlg.AddItem(editDesc)
	for k := 0; k < assocKindCount; k++ {
		dlg.AddItem(slotChecks[k])
		dlg.AddItem(slotEdits[k])
	}
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	// Layout — labels on their own rows keep long English/Russian
	// captions from squeezing the edits.
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)

	maskLbl := vtui.NewLabel(0, 0, "&"+Msg("FileAssoc.MaskLabel")+":", editMask)
	descLbl := vtui.NewLabel(0, 0, "&"+Msg("FileAssoc.DescLabel")+":", editDesc)
	dlg.AddItem(maskLbl)
	dlg.AddItem(descLbl)

	vbox.Add(maskLbl, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editMask, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(descLbl, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(editDesc, vtui.Margins{}, vtui.AlignFill)

	for k := 0; k < assocKindCount; k++ {
		row := vtui.NewHBoxLayout(0, 0, width-4, 1)
		row.Add(slotChecks[k], vtui.Margins{Right: 1}, vtui.AlignLeft)
		row.Add(slotEdits[k], vtui.Margins{}, vtui.AlignFill)
		top := 0
		if k == 0 {
			top = 1
		}
		vbox.Add(row, vtui.Margins{Top: top}, vtui.AlignFill)
	}

	btnRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	btnRow.HorizontalAlign = vtui.AlignCenter
	btnRow.Spacing = 2
	btnRow.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	btnRow.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(btnRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		mask := strings.TrimSpace(editMask.GetText())
		if mask == "" {
			vtui.ShowMessageOn(dlg, " "+Msg("FileAssoc.ErrorTitle")+" ",
				Msg("FileAssoc.EmptyMask"),
				[]string{"&Ok"})
			return
		}
		newAssoc := FileAssoc{
			Mask:        mask,
			Description: strings.TrimSpace(editDesc.GetText()),
		}
		commitHistory(editMask, mask)
		for k := 0; k < assocKindCount; k++ {
			newAssoc.Commands[k] = slotEdits[k].GetText()
			newAssoc.Enabled[k] = slotChecks[k].State != 0
		}

		newItems := make([]FileAssoc, 0, len(s.items)+1)
		targetIdx := idx
		if isCreate {
			if targetIdx < 0 {
				targetIdx = 0
			}
			if targetIdx > len(s.items) {
				targetIdx = len(s.items)
			}
			newItems = append(newItems, s.items[:targetIdx]...)
			newItems = append(newItems, newAssoc)
			newItems = append(newItems, s.items[targetIdx:]...)
		} else {
			newItems = append(newItems, s.items...)
			if targetIdx >= 0 && targetIdx < len(newItems) {
				newItems[targetIdx] = newAssoc
			}
		}
		s.items = newItems
		if !s.save() {
			return
		}
		dlg.Close()
		vtui.FrameManager.PostTask(func() {
			s.openList(targetIdx)
		})
	}

	dlg.SetFocusedItem(editMask)
	vtui.FrameManager.Push(dlg)
}

// assocColumnWidths measures how wide the description and mask columns
// should be so the list rows align. Both are capped so a rogue label
// can't push the row width past the screen.
func assocColumnWidths(list []FileAssoc) (descW, maskW int) {
	for _, a := range list {
		if w := runewidth.StringWidth(assocDisplayLabel(a)); w > descW {
			descW = w
		}
		if w := runewidth.StringWidth(a.Mask); w > maskW {
			maskW = w
		}
	}
	if descW > 30 {
		descW = 30
	}
	if maskW > 30 {
		maskW = 30
	}
	return descW, maskW
}

func formatAssocRow(a FileAssoc, descW, maskW int) string {
	label := assocDisplayLabel(a)
	if pad := descW - runewidth.StringWidth(label); pad > 0 {
		label = label + strings.Repeat(" ", pad)
	}
	mask := a.Mask
	if pad := maskW - runewidth.StringWidth(mask); pad > 0 {
		mask = mask + strings.Repeat(" ", pad)
	}
	return fmt.Sprintf("%s  │  %s  │  %s", label, mask, assocEnabledSummary(a))
}

// assocEnabledSummary emits a compact "E V _" style row that shows at
// a glance which slots the association populates: E for Execute, X for
// AltExec, V for View, v for AltView, D for Edit (from "Editor"), d
// for AltEdit. Missing slots become "_".
func assocEnabledSummary(a FileAssoc) string {
	glyphs := [assocKindCount]string{"E", "X", "V", "v", "D", "d"}
	var b strings.Builder
	for k := 0; k < assocKindCount; k++ {
		if a.Enabled[k] && strings.TrimSpace(a.Commands[k]) != "" {
			b.WriteString(glyphs[k])
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// assocSlotLabel returns the checkbox label for the given slot. Uses
// the plain far2l key name so the UI vocabulary matches the file, and
// an ampersand shortcut for keyboard focus.
func assocSlotLabel(k AssocKind) string {
	switch k {
	case AssocExecute:
		return "&Execute  "
	case AssocAltExec:
		return "&AltExec  "
	case AssocView:
		return "&View     "
	case AssocAltView:
		return "A&ltView  "
	case AssocEdit:
		return "E&dit     "
	case AssocAltEdit:
		return "Al&tEdit  "
	}
	return "?"
}
