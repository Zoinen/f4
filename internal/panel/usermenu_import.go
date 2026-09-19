package panel

import (
	"fmt"
	"reflect"

	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// MergeUserMenus merges an ordered menu without replacing existing commands.
// Separators belong to positions, not a set: identical lines between different
// groups must survive, including when repairing an earlier incomplete import.
func MergeUserMenus(existing, incoming []UserMenuItem) ([]UserMenuItem, int) {
	result := append([]UserMenuItem(nil), existing...)
	cursor := len(result)
	// Anchor an existing import by its first ordinary entry. New imports go
	// after the existing menu; leading separators stay with the imported group.
	for _, item := range incoming {
		if item.IsSeparator() {
			continue
		}
		for i, old := range result {
			if reflect.DeepEqual(old, item) {
				cursor = i
				break
			}
		}
		break
	}
	leading := 0
	for leading < len(incoming) && incoming[leading].IsSeparator() {
		leading++
	}
	for leading > 0 && cursor > 0 && reflect.DeepEqual(result[cursor-1], incoming[leading-1]) {
		cursor--
		leading--
	}
	added := 0
	for _, item := range incoming {
		match := -1
		if item.IsSeparator() {
			if cursor < len(result) && reflect.DeepEqual(result[cursor], item) {
				match = cursor
			}
		} else {
			for i := cursor; i < len(result); i++ {
				if reflect.DeepEqual(result[i], item) {
					match = i
					break
				}
			}
		}
		if match >= 0 {
			cursor = match + 1
			continue
		}
		result = append(result, UserMenuItem{})
		copy(result[cursor+1:], result[cursor:len(result)-1])
		result[cursor] = item
		cursor++
		added++
	}
	return result, added
}

// ShowFar3UserMenuImport shares one dialog between the command and Settings.
// A Settings callback stages the imported tree; nil saves the global menu.
func ShowFar3UserMenuImport(stage func([]UserMenuItem) error) *dialog.FileDialog {
	dlg := dialog.NewFileDialog(i18n.Msg("UserMenu.Far3.Title"), 12)
	edit := vtui.NewEdit(0, 0, 10, "")
	edit.SetId("far3-usermenu-source")
	label := vtui.NewLabel(0, 0, i18n.Msg("UserMenu.Far3.Path"), edit)
	label.SetId("far3-usermenu-label")
	noteKey := "UserMenu.Far3.Merge"
	if stage != nil {
		noteKey = "UserMenu.Far3.Draft"
	}
	note := vtui.NewLabel(0, 0, i18n.Msg(noteKey), nil)
	note.SetId("far3-usermenu-note")
	detailKey := "UserMenu.Far3.Keep"
	if stage != nil {
		detailKey = "UserMenu.Far3.Apply"
	}
	detail := vtui.NewLabel(0, 0, i18n.Msg(detailKey), nil)
	detail.SetId("far3-usermenu-detail")
	importButton := vtui.NewButton(0, 0, i18n.Msg("History.Far3.Import"))
	importButton.SetId("far3-usermenu-import")
	importButton.IsDefault = true
	cancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	cancel.SetId("far3-usermenu-cancel")
	for _, item := range []vtui.UIElement{label, edit, note, detail, importButton, cancel} {
		dlg.AddItem(item)
	}
	width, height := dlg.Size()
	layout := vtui.NewAutoLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	layout.PinTop(label, 0).PinLeft(label, 0).StackVertical(1, label, edit, note).
		FillWidth(edit, 0, 0).PinLeft(note, 0).PinLeft(detail, 0).StackVertical(0, note, detail).PinBottom(importButton, 0).PinBottom(cancel, 0).
		StackHorizontal(2, importButton, cancel).CenterHorizontalGroup(importButton, cancel)
	dlg.SetLayout(func() { layout.SetPosition(dlg.X1+2, dlg.Y1+2, dlg.X2-2, dlg.Y2-2) })
	cancel.OnClick = func() { dlg.Close() }
	importButton.OnClick = func() {
		path, err := dialog.Far3UserMenuPath(edit.GetText())
		var items []UserMenuItem
		if err == nil {
			items, err = LoadFarMenuFile(path)
		}
		if err == nil && len(items) == 0 {
			err = fmt.Errorf("%s", i18n.Msg("UserMenu.Far3.Empty"))
		}
		added := 0
		if err == nil {
			if stage != nil {
				err = stage(items)
			} else {
				var existing []UserMenuItem
				existing, err = LoadMainMenu(MainMenuFilePath())
				if err == nil {
					var merged []UserMenuItem
					merged, added = MergeUserMenus(existing, items)
					if added != 0 {
						err = SaveMainMenu(MainMenuFilePath(), merged)
					}
				}
			}
		}
		if err != nil {
			vtui.ShowMessage(i18n.Msg("UserMenu.Far3.Title"), err.Error(), []string{i18n.Msg("vtui.Ok")})
			return
		}
		dlg.Close()
		if stage == nil {
			vtui.ShowMessage(i18n.Msg("UserMenu.Far3.Title"), fmt.Sprintf(i18n.Msg("UserMenu.Far3.Result"), added), []string{i18n.Msg("vtui.Ok")})
		}
	}
	dlg.SetFocusedItem(edit)
	vtui.FrameManager.Push(dlg)
	return dlg
}
