package app

import (
	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// viewerApplication is the application side of viewer.Application: the action
// registry, the hotkey manager and the frame commands the viewer itself does
// not own.
type viewerApplication struct{}

func (viewerApplication) MenuBarItems(area string) []vtui.MenuBarItem {
	return BuildMenuBarItems(area)
}

func (viewerApplication) KeyBarLabels(area string, fallbacks *vtui.KeySet) *vtui.KeySet {
	return keymap.KeyBarLabelsForArea(area, fallbacks)
}

func (viewerApplication) ActionForKey(area, key string) string {
	if keymap.GlobalHotkeysMgr == nil {
		return ""
	}
	return keymap.GlobalHotkeysMgr.GetAction(area, key)
}

func (viewerApplication) HandleCommand(v *viewer.ViewerView, cmd int, args any) bool {
	switch cmd {
	case appcmd.CmSwitchToEditor:
		ActionSwitchViewerToEditor(v)
		return true
	case appcmd.CmSearch:
		ActionViewerSearch(v)
		return true
	}
	return HandleWorkspaceForkCommand(cmd, args)
}

func (viewerApplication) LookupHotkey(e *vtinput.InputEvent) bool {
	return macroLookupHotkey(macro.MacroMgr, e)
}

var _ viewer.Application = viewerApplication{}

func init() { viewer.App = viewerApplication{} }
