package panel

import (
	"strings"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// DriveToolActionName is the hot key action of the drive menu's row for the
// tool whose key (DriveToolChoice.Key) is key. F4 on the row in the menu and
// the "Hot key:" field of the Tools page in the Drive chooser window bind the
// same action (mitek1977-coder, f4#1148).
func DriveToolActionName(key string) string {
	switch key {
	case driveToolKeyOtherPanel:
		return keymap.DriveMenuActionName("other")
	case driveToolKeyTemporaryPanel:
		return keymap.DriveMenuActionName("temporary")
	case driveToolKeyRegistry:
		return keymap.DriveMenuActionName("platform.windows-registry")
	}
	return keymap.DriveMenuActionName("tool." + driveToolCleanName(key))
}

// driveToolCleanName is a plugin tool's name as its menu row shows it: no
// numbering in front, no ampersands.
func driveToolCleanName(name string) string {
	if idx := strings.Index(name, ". "); idx != -1 {
		name = name[idx+2:]
	}
	return strings.ReplaceAll(name, "&", "")
}

// DriveToolHotkeyText is the tool's hot key as the one-character field shows
// it: the letter or digit, or "" when it has none.
func DriveToolHotkeyText(key, label string) string {
	hm := keymap.GlobalHotkeysMgr
	if hm == nil {
		return ""
	}
	action := DriveToolActionName(key)
	return currentPluginHotkeyText(hm, action, strings.ReplaceAll(label, "&", ""), declaredHotkeyString(PluginActionDefaultShortcut(action)))
}

// SetDriveToolHotkey gives the tool the hot key text, a letter or a digit, and
// takes it from whichever row of the drive menu held it; "" takes the tool's
// hot key away. ok is false, and nothing changes, for any other text.
func SetDriveToolHotkey(key, label, text string) (changed, ok bool) {
	hm := keymap.GlobalHotkeysMgr
	if hm == nil {
		return false, true
	}
	action := DriveToolActionName(key)
	return applyPluginHotkeyChoice(hm, action, strings.ReplaceAll(label, "&", ""), declaredHotkeyString(PluginActionDefaultShortcut(action)), text)
}

// HotkeyEdit is a one-character hot key field that keys the way the field of
// the plugin hot key dialog does: a letter or a digit replaces what it holds,
// Delete and Backspace empty it. OnChange runs after each change.
type HotkeyEdit struct {
	*pluginHotkeyEdit
	OnChange func(text string)
}

func NewHotkeyEdit(text string) *HotkeyEdit {
	return &HotkeyEdit{pluginHotkeyEdit: &pluginHotkeyEdit{vtui.NewEdit(0, 0, 1, text)}}
}

func (e *HotkeyEdit) ProcessKey(ev *vtinput.InputEvent) bool {
	before := e.GetText()
	handled := e.pluginHotkeyEdit.ProcessKey(ev)
	if after := e.GetText(); after != before && e.OnChange != nil {
		e.OnChange(after)
	}
	return handled
}
